package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const redactedBlockJSON = `{"type":"redacted_thinking","data":"EmwKAhgBEgy3va3pzix/LafPsn4aDFIT2Xlxh0L5L8rLVyIwxtE3rAFBa8cr3qpPkNRj2YfWXGmKDxH4mPnZ5sQ7vB5URj+="}`

// A redacted_thinking block from a non-streaming response must come back on
// replay byte-for-byte when thinking is enabled, and be stripped when it is not.
func TestAnthropicRedactedThinkingRoundTrip(t *testing.T) {
	var bodies [][]byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		bodies = append(bodies, raw)
		w.Header().Set("content-type", "application/json")
		w.Write([]byte(`{"type":"message","content":[` + redactedBlockJSON + `,{"type":"tool_use","id":"t1","name":"Bash","input":{}}],"stop_reason":"tool_use"}`))
	}))
	defer srv.Close()

	p, _ := NewProvider(Config{Format: FormatAnthropic, BaseURL: srv.URL, APIKey: "k", Model: "m", APIVersion: "2023-06-01", ThinkingType: "enabled"})
	ctx := context.Background()
	history := []Message{UserText("go")}
	asst, _, _, err := p.Complete(ctx, CompletionRequest{Messages: history})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if len(asst.Content) != 2 || asst.Content[0].Type != BlockRedactedThinking || asst.Content[0].Data == "" {
		t.Fatalf("redacted_thinking not parsed: %+v", asst.Content)
	}
	history = append(history, asst, Message{Role: RoleUser, Content: []ContentBlock{ToolResultText("t1", "ok", false)}})

	// Thinking enabled: replayed verbatim.
	if _, _, _, err := p.Complete(ctx, CompletionRequest{Messages: history}); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	var req struct {
		Messages []struct {
			Content []json.RawMessage `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(bodies[1], &req); err != nil {
		t.Fatalf("unmarshal: %v\n%s", err, bodies[1])
	}
	var want bytes.Buffer
	_ = json.Compact(&want, []byte(redactedBlockJSON))
	if got := req.Messages[1].Content[0]; !bytes.Equal(got, want.Bytes()) {
		t.Fatalf("redacted_thinking not replayed verbatim:\n got %s\nwant %s", got, want.Bytes())
	}

	// Thinking disabled for this request: stripped, tool_use kept.
	if _, _, _, err := p.Complete(ctx, CompletionRequest{Messages: history, Thinking: "disabled"}); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	if s := string(bodies[2]); strings.Contains(s, "redacted_thinking") || !strings.Contains(s, `"tool_use"`) {
		t.Fatalf("redacted_thinking must be stripped (and tool_use kept) when thinking is disabled:\n%s", s)
	}
}

// Streaming: a signed thinking block with no thinking deltas (display omitted)
// and a redacted_thinking block must both survive accumulation and replay
// exactly as Anthropic sent them.
func TestAnthropicStreamEmptyThinkingAndRedactedReplay(t *testing.T) {
	stub := sse(
		`data: {"type":"message_start","message":{"usage":{"input_tokens":5,"output_tokens":1}}}`, ``,
		`data: {"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":"","signature":""}}`, ``,
		`data: {"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"SIG_STREAM"}}`, ``,
		`data: {"type":"content_block_stop","index":0}`, ``,
		`data: {"type":"content_block_start","index":1,"content_block":{"type":"redacted_thinking","data":"REDACTED_DATA"}}`, ``,
		`data: {"type":"content_block_stop","index":1}`, ``,
		`data: {"type":"content_block_start","index":2,"content_block":{"type":"tool_use","id":"t1","name":"Bash","input":{}}}`, ``,
		`data: {"type":"content_block_delta","index":2,"delta":{"type":"input_json_delta","partial_json":"{\"command\":\"ls\"}"}}`, ``,
		`data: {"type":"content_block_stop","index":2}`, ``,
		`data: {"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":7}}`, ``,
		`data: {"type":"message_stop"}`, ``,
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(stub))
	}))
	defer srv.Close()
	p, _ := NewProvider(Config{Format: FormatAnthropic, BaseURL: srv.URL, APIKey: "k", Model: "m", ThinkingType: "enabled"})
	asst := collect(t, p, CompletionRequest{Messages: []Message{UserText("go")}})

	c := asst.Content
	if len(c) != 3 ||
		c[0].Type != BlockThinking || c[0].Thinking != "" || c[0].Signature != "SIG_STREAM" ||
		c[1].Type != BlockRedactedThinking || c[1].Data != "REDACTED_DATA" ||
		c[2].Type != BlockToolUse || string(c[2].Input) != `{"command":"ls"}` {
		t.Fatalf("accumulated=%+v, want [thinking(sig), redacted_thinking, tool_use]", c)
	}

	history := []Message{UserText("go"), asst, {Role: RoleUser, Content: []ContentBlock{ToolResultText("t1", "ok", false)}}}
	raw, _ := captureReq(t, Config{Format: FormatAnthropic, ThinkingType: "enabled"}, CompletionRequest{Messages: history}, sse(`data: {"type":"message_stop"}`, ``))
	for _, want := range []string{
		`{"type":"thinking","thinking":"","signature":"SIG_STREAM"}`,
		`{"type":"redacted_thinking","data":"REDACTED_DATA"}`,
	} {
		if !strings.Contains(raw, want) {
			t.Fatalf("replayed request missing %s\n%s", want, raw)
		}
	}
}
