package tool

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// redirectChain serves /a -> /b -> /c (200 HTML) and counts hits per path.
func redirectChain(t *testing.T) (*httptest.Server, func(string) int) {
	t.Helper()
	var mu sync.Mutex
	hits := map[string]int{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		hits[r.URL.Path]++
		mu.Unlock()
		switch r.URL.Path {
		case "/a":
			http.Redirect(w, r, "/b", http.StatusFound)
		case "/b":
			http.Redirect(w, r, "/c", http.StatusFound)
		case "/c":
			w.Header().Set("content-type", "text/html")
			w.Write([]byte(`<html><body><h1>final page</h1></body></html>`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv, func(p string) int { mu.Lock(); defer mu.Unlock(); return hits[p] }
}

type hopRecorder struct {
	mu   sync.Mutex
	urls []string
	vias []int
	ctxs []any
	fail error // returned from every call when non-nil
}

type ctxKey struct{}

func (h *hopRecorder) check(req *http.Request, via []*http.Request) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.urls = append(h.urls, req.URL.String())
	h.vias = append(h.vias, len(via))
	h.ctxs = append(h.ctxs, req.Context().Value(ctxKey{}))
	return h.fail
}

func fetchURL(t *testing.T, ctx context.Context, cfg WebFetchConfig, u string) Result {
	t.Helper()
	raw, _ := json.Marshal(map[string]any{"url": u})
	res, err := NewWebFetch(cfg).Call(ctx, raw, &ToolContext{})
	if err != nil {
		t.Fatalf("webfetch: %v", err)
	}
	return res
}

// (a) nil CheckRedirect keeps net/http's default policy: the chain is followed to
// the final page, and the default 10-redirect limit still applies.
func TestWebFetchCheckRedirectNilKeepsDefault(t *testing.T) {
	if c := newFetchClient(WebFetchConfig{}); c.CheckRedirect != nil {
		t.Fatal("nil config must leave http.Client.CheckRedirect nil")
	}
	if c := newFetchClient(WebFetchConfig{Proxy: "http://127.0.0.1:1"}); c.CheckRedirect != nil {
		t.Fatal("nil config must leave http.Client.CheckRedirect nil (custom transport)")
	}

	srv, hits := redirectChain(t)
	res := fetchURL(t, context.Background(), WebFetchConfig{}, srv.URL+"/a")
	out := res.Flatten()
	if res.IsError || !strings.Contains(out, "final page") || !strings.HasPrefix(out, "[200 OK] "+srv.URL+"/a\n") {
		t.Fatalf("redirect chain not followed: %q", out)
	}
	if hits("/a") != 1 || hits("/b") != 1 || hits("/c") != 1 {
		t.Fatalf("hits a=%d b=%d c=%d, want 1 each", hits("/a"), hits("/b"), hits("/c"))
	}

	loop := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, r.URL.Path+"x", http.StatusFound)
	}))
	defer loop.Close()
	res = fetchURL(t, context.Background(), WebFetchConfig{}, loop.URL+"/l")
	if !res.IsError || !strings.Contains(res.Flatten(), "stopped after 10 redirects") {
		t.Fatalf("default redirect limit not applied: %q", res.Flatten())
	}
}

// (b) The callback sees every hop with the target URL, the growing via chain and
// the original request context.
func TestWebFetchCheckRedirectEachHop(t *testing.T) {
	srv, _ := redirectChain(t)
	h := &hopRecorder{}
	ctx := context.WithValue(context.Background(), ctxKey{}, "marker")
	res := fetchURL(t, ctx, WebFetchConfig{CheckRedirect: h.check}, srv.URL+"/a")
	if res.IsError || !strings.Contains(res.Flatten(), "final page") {
		t.Fatalf("fetch failed: %q", res.Flatten())
	}
	want := []string{srv.URL + "/b", srv.URL + "/c"}
	if strings.Join(h.urls, ",") != strings.Join(want, ",") {
		t.Fatalf("hops = %v, want %v", h.urls, want)
	}
	for i := range h.urls {
		if h.vias[i] != i+1 {
			t.Fatalf("hop %d: len(via) = %d, want %d", i, h.vias[i], i+1)
		}
		if h.ctxs[i] != "marker" {
			t.Fatalf("hop %d: request context not kept (value %v)", i, h.ctxs[i])
		}
	}
}

// (c) A callback error stops the redirect and the fetch fails with that error.
func TestWebFetchCheckRedirectErrorStops(t *testing.T) {
	srv, hits := redirectChain(t)
	errHop := errors.New("hop blocked by gate")
	h := &hopRecorder{fail: errHop}
	res := fetchURL(t, context.Background(), WebFetchConfig{CheckRedirect: h.check}, srv.URL+"/a")
	out := res.Flatten()
	if !res.IsError || !strings.Contains(out, "Error fetching URL") || !strings.Contains(out, errHop.Error()) {
		t.Fatalf("fetch must fail with the callback error: %q", out)
	}
	if len(h.urls) != 1 || hits("/b") != 0 || hits("/c") != 0 {
		t.Fatalf("redirect not stopped: hops=%v b=%d c=%d", h.urls, hits("/b"), hits("/c"))
	}
	// The error is the callback's own (wrapped by net/http in a *url.Error).
	resp, err := newFetchClient(WebFetchConfig{CheckRedirect: h.check}).Get(srv.URL + "/a")
	if resp != nil {
		resp.Body.Close()
	}
	if !errors.Is(err, errHop) {
		t.Fatalf("client error = %v, want wrapping %v", err, errHop)
	}
}

// (d) The direct (no-proxy) retry client carries the same callback: a dead proxy
// forces the retry, and every hop there is still checked (and can be blocked).
func TestWebFetchCheckRedirectDirectRetry(t *testing.T) {
	srv, hits := redirectChain(t)
	h := &hopRecorder{}
	cfg := WebFetchConfig{Proxy: "http://127.0.0.1:1", CheckRedirect: h.check}
	res := fetchURL(t, context.Background(), cfg, srv.URL+"/a")
	if res.IsError || !strings.Contains(res.Flatten(), "final page") {
		t.Fatalf("direct retry should have fetched the page: %q", res.Flatten())
	}
	want := []string{srv.URL + "/b", srv.URL + "/c"}
	if strings.Join(h.urls, ",") != strings.Join(want, ",") {
		t.Fatalf("direct-retry hops = %v, want %v", h.urls, want)
	}

	errHop := errors.New("hop blocked on retry")
	hb := &hopRecorder{fail: errHop}
	before := hits("/b")
	res = fetchURL(t, context.Background(), WebFetchConfig{Proxy: "http://127.0.0.1:1", CheckRedirect: hb.check}, srv.URL+"/a")
	if !res.IsError || !strings.Contains(res.Flatten(), errHop.Error()) || hits("/b") != before {
		t.Fatalf("direct retry must be stopped by the callback: %q", res.Flatten())
	}
}
