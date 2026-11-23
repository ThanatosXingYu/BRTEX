package tool

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

// deadProxy is a proxy URL nothing listens on: a fetch through it fails with
// "proxyconnect ... connection refused", which triggers the direct retry.
const deadProxy = "http://127.0.0.1:1"

// dialRecorder counts dials per address. With fail set, every dial returns it.
type dialRecorder struct {
	mu    sync.Mutex
	dials map[string]int
	fail  error
}

func (d *dialRecorder) dial(ctx context.Context, network, addr string) (net.Conn, error) {
	d.mu.Lock()
	if d.dials == nil {
		d.dials = map[string]int{}
	}
	d.dials[addr]++
	d.mu.Unlock()
	if d.fail != nil {
		return nil, d.fail
	}
	return (&net.Dialer{}).DialContext(ctx, network, addr)
}

func (d *dialRecorder) snapshot() map[string]int {
	d.mu.Lock()
	defer d.mu.Unlock()
	out := map[string]int{}
	for k, v := range d.dials {
		out[k] = v
	}
	return out
}

func sameCounts(got, want map[string]int) bool {
	if len(got) != len(want) {
		return false
	}
	for k, v := range want {
		if got[k] != v {
			return false
		}
	}
	return true
}

// counter is a concurrency-safe hit counter.
type counter struct {
	mu sync.Mutex
	n  int
}

func (c *counter) inc()     { c.mu.Lock(); c.n++; c.mu.Unlock() }
func (c *counter) get() int { c.mu.Lock(); defer c.mu.Unlock(); return c.n }

// srvAddr returns the "host:port" a client dials for srv.
func srvAddr(srv *httptest.Server) string { return srv.Listener.Addr().String() }

// crossChain serves A/a -> B/b -> C/c (200 HTML) on three distinct servers, so
// every hop needs its own connection (one dial per address per fetch).
func crossChain(t *testing.T) (a, b, c *httptest.Server, hits *[3]counter) {
	t.Helper()
	hits = &[3]counter{}
	c = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits[2].inc()
		w.Header().Set("content-type", "text/html")
		w.Write([]byte(`<html><body><h1>final page</h1></body></html>`))
	}))
	b = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits[1].inc()
		http.Redirect(w, r, c.URL+"/c", http.StatusFound)
	}))
	a = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits[0].inc()
		http.Redirect(w, r, b.URL+"/b", http.StatusFound)
	}))
	t.Cleanup(a.Close)
	t.Cleanup(b.Close)
	t.Cleanup(c.Close)
	return a, b, c, hits
}

// forwardProxy is a minimal plain-HTTP forward proxy. It answers every request
// with "Connection: close", so a client opens a new proxy connection per hop.
func forwardProxy(t *testing.T) (*httptest.Server, *counter) {
	t.Helper()
	n := &counter{}
	upstream := &http.Transport{DisableKeepAlives: true}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n.inc()
		out := r.Clone(r.Context())
		out.RequestURI = ""
		resp, err := upstream.RoundTrip(out)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		defer resp.Body.Close()
		for k, vs := range resp.Header {
			for _, v := range vs {
				w.Header().Add(k, v)
			}
		}
		w.Header().Set("Connection", "close")
		w.WriteHeader(resp.StatusCode)
		io.Copy(w, resp.Body)
	}))
	t.Cleanup(srv.Close)
	t.Cleanup(upstream.CloseIdleConnections)
	return srv, n
}

// Nil DialContext and false DisableDirectRetry leave the clients exactly as
// before: default client without a proxy/TLS option, custom transport with the
// default dialer otherwise, and the direct retry present whenever a proxy is set.
func TestWebFetchDialerNilUnchanged(t *testing.T) {
	if c := newFetchClient(WebFetchConfig{}); c.Transport != nil {
		t.Fatalf("zero config must keep the default transport, got %T", c.Transport)
	}
	for _, cfg := range []WebFetchConfig{{Proxy: deadProxy}, {InsecureTLS: true}} {
		tr, ok := newFetchClient(cfg).Transport.(*http.Transport)
		if !ok || tr.DialContext != nil {
			t.Fatalf("%+v: want *http.Transport with nil DialContext, got %T", cfg, newFetchClient(cfg).Transport)
		}
	}
	if newDirectClient(WebFetchConfig{}) != nil {
		t.Fatal("no proxy: there must be no direct retry client")
	}
	d := newDirectClient(WebFetchConfig{Proxy: deadProxy})
	if d == nil || d.Transport != nil || d.CheckRedirect != nil {
		t.Fatalf("proxy set: direct retry client must exist with default transport and policy, got %+v", d)
	}

	// Behaviour: a dead proxy still falls back to the origin through the direct path.
	var hits counter
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.inc()
		w.Header().Set("content-type", "text/html")
		w.Write([]byte(`<html><body><h1>Direct OK</h1></body></html>`))
	}))
	defer srv.Close()
	res := fetchURL(t, context.Background(), WebFetchConfig{Proxy: deadProxy}, srv.URL)
	if res.IsError || !strings.Contains(res.Flatten(), "Direct OK") || hits.get() != 1 {
		t.Fatalf("direct retry with nil options: hits=%d out=%q", hits.get(), res.Flatten())
	}
}

// CheckRedirect sees every hop on the primary client (through a live proxy) and
// on the direct retry client (dead proxy).
func TestWebFetchCheckRedirectBothClients(t *testing.T) {
	a, b, c, hits := crossChain(t)
	want := b.URL + "/b," + c.URL + "/c"

	proxy, proxied := forwardProxy(t)
	h := &hopRecorder{}
	res := fetchURL(t, context.Background(), WebFetchConfig{Proxy: proxy.URL, CheckRedirect: h.check}, a.URL+"/a")
	if res.IsError || !strings.Contains(res.Flatten(), "final page") {
		t.Fatalf("proxied fetch failed: %q", res.Flatten())
	}
	if got := strings.Join(h.urls, ","); got != want {
		t.Fatalf("primary-client hops = %s, want %s", got, want)
	}
	if proxied.get() != 3 {
		t.Fatalf("proxy saw %d requests, want 3 (no direct retry on the primary path)", proxied.get())
	}

	hd := &hopRecorder{}
	res = fetchURL(t, context.Background(), WebFetchConfig{Proxy: deadProxy, CheckRedirect: hd.check}, a.URL+"/a")
	if res.IsError || !strings.Contains(res.Flatten(), "final page") {
		t.Fatalf("direct retry failed: %q", res.Flatten())
	}
	if got := strings.Join(hd.urls, ","); got != want {
		t.Fatalf("direct-client hops = %s, want %s", got, want)
	}
	if hits[0].get() != 2 || hits[1].get() != 2 || hits[2].get() != 2 {
		t.Fatalf("origin hits a=%d b=%d c=%d, want 2 each", hits[0].get(), hits[1].get(), hits[2].get())
	}
}

// DialContext opens every connection of every hop, on the primary client (with
// and without a proxy) and on the direct retry client.
func TestWebFetchDialerEachHopBothClients(t *testing.T) {
	a, b, c, _ := crossChain(t)
	A, B, C := srvAddr(a), srvAddr(b), srvAddr(c)

	// Primary client, no proxy: one dial per origin hop.
	d := &dialRecorder{}
	res := fetchURL(t, context.Background(), WebFetchConfig{DialContext: d.dial}, a.URL+"/a")
	if res.IsError || !strings.Contains(res.Flatten(), "final page") {
		t.Fatalf("fetch failed: %q", res.Flatten())
	}
	if got, want := d.snapshot(), map[string]int{A: 1, B: 1, C: 1}; !sameCounts(got, want) {
		t.Fatalf("no-proxy dials = %v, want %v", got, want)
	}

	// Primary client through a live proxy: every hop dials the proxy (which closes
	// each connection), never the origins.
	proxy, proxied := forwardProxy(t)
	d = &dialRecorder{}
	res = fetchURL(t, context.Background(), WebFetchConfig{Proxy: proxy.URL, DialContext: d.dial}, a.URL+"/a")
	if res.IsError || !strings.Contains(res.Flatten(), "final page") {
		t.Fatalf("proxied fetch failed: %q", res.Flatten())
	}
	if got, want := d.snapshot(), map[string]int{srvAddr(proxy): 3}; !sameCounts(got, want) {
		t.Fatalf("proxied dials = %v, want %v", got, want)
	}
	if proxied.get() != 3 {
		t.Fatalf("proxy saw %d requests, want 3", proxied.get())
	}

	// Dead proxy: the primary client dials it once (refused), then the direct
	// retry client dials each origin hop through the same dialer.
	d = &dialRecorder{}
	res = fetchURL(t, context.Background(), WebFetchConfig{Proxy: deadProxy, DialContext: d.dial}, a.URL+"/a")
	if res.IsError || !strings.Contains(res.Flatten(), "final page") {
		t.Fatalf("direct retry failed: %q", res.Flatten())
	}
	if got, want := d.snapshot(), map[string]int{"127.0.0.1:1": 1, A: 1, B: 1, C: 1}; !sameCounts(got, want) {
		t.Fatalf("direct-retry dials = %v, want %v", got, want)
	}
}

// DisableDirectRetry: a proxy-caused failure is final; nothing reaches the
// origin through the direct path. The control run (false) shows the same setup
// does reach the origin when the retry is on.
func TestWebFetchDisableDirectRetry(t *testing.T) {
	var hits counter
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.inc()
		w.Header().Set("content-type", "text/html")
		w.Write([]byte(`<html><body><h1>origin</h1></body></html>`))
	}))
	defer srv.Close()

	cfg := WebFetchConfig{Proxy: deadProxy, DisableDirectRetry: true}
	if newDirectClient(cfg) != nil {
		t.Fatal("DisableDirectRetry: there must be no direct retry client")
	}
	res := fetchURL(t, context.Background(), cfg, srv.URL)
	if !res.IsError || !strings.Contains(res.Flatten(), "Error fetching URL") || !strings.Contains(res.Flatten(), "proxyconnect") {
		t.Fatalf("fetch must fail on the proxy error: %q", res.Flatten())
	}
	if hits.get() != 0 {
		t.Fatalf("origin hit %d times with DisableDirectRetry", hits.get())
	}

	// Same with a dialer: only the proxy is dialed, never the origin.
	d := &dialRecorder{}
	cfg.DialContext = d.dial
	res = fetchURL(t, context.Background(), cfg, srv.URL)
	if !res.IsError || hits.get() != 0 {
		t.Fatalf("fetch must fail without reaching the origin: hits=%d out=%q", hits.get(), res.Flatten())
	}
	if got, want := d.snapshot(), map[string]int{"127.0.0.1:1": 1}; !sameCounts(got, want) {
		t.Fatalf("dials = %v, want %v", got, want)
	}

	// Control: retry enabled, the origin is reached through the direct path.
	d = &dialRecorder{}
	res = fetchURL(t, context.Background(), WebFetchConfig{Proxy: deadProxy, DialContext: d.dial}, srv.URL)
	if res.IsError || !strings.Contains(res.Flatten(), "origin") || hits.get() != 1 {
		t.Fatalf("control: direct retry must reach the origin: hits=%d out=%q", hits.get(), res.Flatten())
	}
	if got, want := d.snapshot(), map[string]int{"127.0.0.1:1": 1, srvAddr(srv): 1}; !sameCounts(got, want) {
		t.Fatalf("control dials = %v, want %v", got, want)
	}
}

// A dialer error fails the fetch with that error, on the primary client and on
// the direct retry client, and the origin is never reached.
func TestWebFetchDialerErrorFails(t *testing.T) {
	var hits counter
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.inc()
		w.Write([]byte("origin"))
	}))
	defer srv.Close()
	errDial := errors.New("dial blocked by gate")

	for _, proxy := range []string{"", deadProxy} {
		d := &dialRecorder{fail: errDial}
		cfg := WebFetchConfig{Proxy: proxy, DialContext: d.dial}
		res := fetchURL(t, context.Background(), cfg, srv.URL)
		out := res.Flatten()
		if !res.IsError || !strings.Contains(out, "Error fetching URL") || !strings.Contains(out, errDial.Error()) {
			t.Fatalf("proxy=%q: fetch must fail with the dial error: %q", proxy, out)
		}

		clients := map[string]*http.Client{"primary": newFetchClient(cfg)}
		if proxy != "" {
			clients["direct"] = newDirectClient(cfg)
		}
		for name, c := range clients {
			resp, err := c.Get(srv.URL)
			if resp != nil {
				resp.Body.Close()
			}
			if !errors.Is(err, errDial) {
				t.Fatalf("proxy=%q %s client: error = %v, want wrapping %v", proxy, name, err, errDial)
			}
		}
	}
	if hits.get() != 0 {
		t.Fatalf("origin hit %d times through a failing dialer", hits.get())
	}
}

// With only a dialer set, the transport is a clone of http.DefaultTransport:
// environment proxy, timeouts and HTTP/2 are kept, the dialer is installed and
// keep-alive is disabled.
func TestWebFetchDialerKeepsDefaultTransportSettings(t *testing.T) {
	d := &dialRecorder{}
	c := newFetchClient(WebFetchConfig{DialContext: d.dial})
	tr, ok := c.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("want *http.Transport, got %T", c.Transport)
	}
	if tr.Proxy == nil {
		t.Error("Proxy is nil, want ProxyFromEnvironment from http.DefaultTransport")
	}
	if !tr.ForceAttemptHTTP2 {
		t.Error("ForceAttemptHTTP2 is false, want true")
	}
	if tr.TLSHandshakeTimeout != 10*time.Second {
		t.Errorf("TLSHandshakeTimeout = %v, want 10s", tr.TLSHandshakeTimeout)
	}
	if !tr.DisableKeepAlives {
		t.Error("DisableKeepAlives is false, want true")
	}
	if tr.DialContext == nil {
		t.Error("DialContext is nil, want the configured dialer")
	}
}

// Keep-alive is off whenever a dialer is set: two sequential fetches with the
// same client to the same server dial twice, on the DefaultTransport clone and
// on the proxy / TLS transport.
func TestWebFetchDialerDisablesKeepAlive(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte("ok"))
	}))
	defer srv.Close()

	for _, insecure := range []bool{false, true} {
		d := &dialRecorder{}
		c := newFetchClient(WebFetchConfig{DialContext: d.dial, InsecureTLS: insecure})
		for i := 0; i < 2; i++ {
			resp, err := c.Get(srv.URL)
			if err != nil {
				t.Fatalf("insecure=%v fetch %d: %v", insecure, i+1, err)
			}
			io.Copy(io.Discard, resp.Body)
			resp.Body.Close()
		}
		if got, want := d.snapshot(), map[string]int{srvAddr(srv): 2}; !sameCounts(got, want) {
			t.Fatalf("insecure=%v: dials = %v, want %v (one per fetch, no reuse)", insecure, got, want)
		}
	}
}
