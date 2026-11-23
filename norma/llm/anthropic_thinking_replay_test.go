package llm

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// Issue #181: with thinking display omitted, a non-streaming response carries
// {"type":"thinking","thinking":"","signature":...}. Replayed after a tool_use
// round trip, the block must keep its "thinking" key; without it the API (or a
// strict gateway) rejects the request with "missing field `thinking`".
func TestAnthropicReplayEmptyThinkingBeforeToolUse(t *testing.T) {
	var bodies [][]byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		bodies = append(bodies, raw)
		w.Header().Set("content-type", "application/json")
		w.Write([]byte(`{"type":"message","content":[{"type":"thinking","thinking":"","signature":"SIG_EMPTY"},{"type":"tool_use","id":"t1","name":"Bash","input":{"command":"ls"}}],"stop_reason":"tool_use"}`))
	}))
	defer srv.Close()

	p, _ := NewProvider(Config{Format: FormatAnthropic, BaseURL: srv.URL, APIKey: "k", Model: "m", APIVersion: "2023-06-01", ThinkingType: "enabled"})
	ctx := context.Background()
	history := []Message{UserText("go")}
	asst, _, _, err := p.Complete(ctx, CompletionRequest{Messages: history})
	if err != nil {
		t.Fatalf("first Complete: %v", err)
	}
	history = append(history, asst, Message{Role: RoleUser, Content: []ContentBlock{ToolResultText("t1", "ok", false)}})
	if _, _, _, err := p.Complete(ctx, CompletionRequest{Messages: history}); err != nil {
		t.Fatalf("second Complete: %v", err)
	}
	if len(bodies) != 2 {
		t.Fatalf("requests=%d, want 2", len(bodies))
	}

	var req struct {
		Messages []struct {
			Role    string           `json:"role"`
			Content []map[string]any `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(bodies[1], &req); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, bodies[1])
	}
	thinking := 0
	for _, m := range req.Messages {
		for _, b := range m.Content {
			if b["type"] != "thinking" {
				continue
			}
			thinking++
			th, ok := b["thinking"]
			if !ok {
				t.Fatalf("incomplete thinking block replayed (no \"thinking\" field): %v\n%s", b, bodies[1])
			}
			if th != "" || b["signature"] != "SIG_EMPTY" {
				t.Fatalf("thinking block altered on replay: %v", b)
			}
		}
	}
	if thinking != 1 {
		t.Fatalf("thinking blocks=%d, want 1\n%s", thinking, bodies[1])
	}
	if c := req.Messages[1].Content; len(c) != 2 || c[0]["type"] != "thinking" || c[1]["type"] != "tool_use" {
		t.Fatalf("assistant turn=%v, want [thinking, tool_use]", c)
	}
}
