package agentcore

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Autumn-27/norma/tool"
)

// The WebFetch* options reach the WebFetch tool built by NewSession.
func TestWebFetchOptionsPassedThrough(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		if r.URL.Path == "/a" {
			http.Redirect(w, r, "/b", http.StatusFound)
			return
		}
		w.Write([]byte("final page"))
	}))
	defer srv.Close()
	raw, _ := json.Marshal(map[string]any{"url": srv.URL + "/a"})

	call := func(opts Options) tool.Result {
		t.Helper()
		opts.EnableWebFetch = true
		opts.DisableBackgroundTasks = true
		s := NewSession(opts)
		defer s.Close()
		wf, ok := s.registry.Get("WebFetch")
		if !ok {
			t.Fatal("WebFetch tool not registered")
		}
		res, err := wf.Call(context.Background(), raw, &tool.ToolContext{})
		if err != nil {
			t.Fatalf("webfetch: %v", err)
		}
		return res
	}

	var dials, hops atomic.Int32
	res := call(Options{
		WebFetchDialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			dials.Add(1)
			return (&net.Dialer{}).DialContext(ctx, network, addr)
		},
		WebFetchCheckRedirect: func(*http.Request, []*http.Request) error {
			hops.Add(1)
			return nil
		},
	})
	if res.IsError || !strings.Contains(res.Flatten(), "final page") {
		t.Fatalf("fetch failed: %q", res.Flatten())
	}
	if dials.Load() == 0 || hops.Load() != 1 {
		t.Fatalf("dials=%d hops=%d, want dials>0 and hops=1", dials.Load(), hops.Load())
	}

	before := hits.Load()
	res = call(Options{WebFetchProxy: "http://127.0.0.1:1", WebFetchDisableDirectRetry: true})
	if !res.IsError || hits.Load() != before {
		t.Fatalf("DisableDirectRetry not applied: hits %d->%d out=%q", before, hits.Load(), res.Flatten())
	}
}
