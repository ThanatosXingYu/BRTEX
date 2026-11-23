package subagent

import (
	"context"
	"encoding/json"
	"iter"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Autumn-27/norma/llm"
	"github.com/Autumn-27/norma/permission"
	"github.com/Autumn-27/norma/tool"
)

// fakeProvider always answers with a fixed line of text and ends the turn.
type fakeProvider struct{ reply string }

func (f fakeProvider) Stream(_ context.Context, _ llm.CompletionRequest) iter.Seq2[llm.StreamEvent, error] {
	return func(yield func(llm.StreamEvent, error) bool) {
		yield(llm.StreamEvent{Type: llm.SETextDelta, Text: f.reply}, nil)
		yield(llm.StreamEvent{Type: llm.SEMessageDelta, StopReason: "end_turn"}, nil)
		yield(llm.StreamEvent{Type: llm.SEMessageStop}, nil)
	}
}

func (f fakeProvider) Complete(ctx context.Context, req llm.CompletionRequest) (llm.Message, string, llm.Usage, error) {
	acc := llm.NewAccumulator()
	for ev, err := range f.Stream(ctx, req) {
		if err != nil {
			return llm.Message{}, "", llm.Usage{}, err
		}
		acc.Add(ev)
	}
	return acc.Message(), acc.StopReason, acc.Usage, nil
}

func TestSubagentDispatch(t *testing.T) {
	agentTool := NewTool(Config{
		Provider:    fakeProvider{reply: "subagent answer"},
		ParentTools: tool.NewRegistry(tool.DefaultTools()...),
		Agents: map[string]Definition{
			"researcher": {Description: "researches", Prompt: []string{"You research."}, PermissionMode: permission.ModeBypass},
		},
	})
	res, err := agentTool.Call(context.Background(), []byte(`{"agent_type":"researcher","prompt":"find x"}`), &tool.ToolContext{})
	if err != nil {
		t.Fatalf("call: %v", err)
	}
	if !strings.Contains(res.Flatten(), "subagent answer") {
		t.Fatalf("subagent result: %q", res.Flatten())
	}
}

func TestUnknownAgentType(t *testing.T) {
	agentTool := NewTool(Config{Provider: fakeProvider{}, Agents: map[string]Definition{}})
	res, _ := agentTool.Call(context.Background(), []byte(`{"agent_type":"nope","prompt":"x"}`), &tool.ToolContext{})
	if !res.IsError {
		t.Fatal("expected error for unknown agent_type")
	}
}

func TestRecursionDepthGuard(t *testing.T) {
	agentTool := NewTool(Config{
		Provider: fakeProvider{reply: "x"},
		MaxDepth: 2,
		Agents:   map[string]Definition{"a": {Prompt: []string{"p"}, PermissionMode: permission.ModeBypass}},
	})
	// Simulate being already at max depth.
	ctx := context.WithValue(context.Background(), depthKey, 2)
	res, _ := agentTool.Call(ctx, []byte(`{"agent_type":"a","prompt":"x"}`), &tool.ToolContext{})
	if !res.IsError || !strings.Contains(res.Flatten(), "recursion depth") {
		t.Fatalf("expected depth guard, got %q", res.Flatten())
	}
}

// toolThenTextProvider is fakeProvider with one tool round first: call 1 asks
// for the "Big" tool, call 2 ends with text. It keeps the requests it received,
// so a test can read the tool_result the subagent's model was shown.
type toolThenTextProvider struct {
	reqs []llm.CompletionRequest
}

func (p *toolThenTextProvider) Stream(_ context.Context, req llm.CompletionRequest) iter.Seq2[llm.StreamEvent, error] {
	p.reqs = append(p.reqs, req)
	evs := []llm.StreamEvent{
		{Type: llm.SEToolUseStart, ToolID: "c1", ToolName: "Big"},
		{Type: llm.SEToolInputJSON, Text: `{}`},
		{Type: llm.SEMessageDelta, StopReason: "tool_use"},
		{Type: llm.SEMessageStop},
	}
	if len(p.reqs) > 1 {
		evs = []llm.StreamEvent{
			{Type: llm.SETextDelta, Text: "done"},
			{Type: llm.SEMessageDelta, StopReason: "end_turn"},
			{Type: llm.SEMessageStop},
		}
	}
	return func(yield func(llm.StreamEvent, error) bool) {
		for _, ev := range evs {
			if !yield(ev, nil) {
				return
			}
		}
	}
}

func (p *toolThenTextProvider) Complete(ctx context.Context, req llm.CompletionRequest) (llm.Message, string, llm.Usage, error) {
	acc := llm.NewAccumulator()
	for ev, err := range p.Stream(ctx, req) {
		if err != nil {
			return llm.Message{}, "", llm.Usage{}, err
		}
		acc.Add(ev)
	}
	return acc.Message(), acc.StopReason, acc.Usage, nil
}

// subagentToolResult runs one subagent whose "Big" tool returns big and
// returns the tool_result text its model received on the next turn.
func subagentToolResult(t *testing.T, cfg Config, big string) string {
	t.Helper()
	bigTool := tool.Build(tool.Spec{
		Name:   "Big",
		Schema: map[string]any{"type": "object"},
		Permissions: func(context.Context, json.RawMessage, permission.Context) permission.Decision {
			return permission.Allowed()
		},
		Run: func(context.Context, json.RawMessage, *tool.ToolContext) (tool.Result, error) {
			return tool.Text(big), nil
		},
	})
	p := &toolThenTextProvider{}
	cfg.Provider = p
	cfg.ParentTools = tool.NewRegistry(bigTool)
	cfg.Agents = map[string]Definition{"a": {Prompt: []string{"p"}, PermissionMode: permission.ModeBypass}}
	out, err := Run(context.Background(), cfg, "a", "go", t.TempDir())
	if err != nil || out != "done" {
		t.Fatalf("Run = %q, %v", out, err)
	}
	if len(p.reqs) != 2 {
		t.Fatalf("model calls = %d, want 2", len(p.reqs))
	}
	msgs := p.reqs[1].Messages
	last := msgs[len(msgs)-1]
	if len(last.Content) != 1 || last.Content[0].Type != llm.BlockToolResult {
		t.Fatalf("last message is not a tool_result: %+v", last)
	}
	return last.Content[0].Content[0].Text
}

func TestSubagentOutputContextPropagated(t *testing.T) {
	big := strings.Repeat("x", 5000)

	t.Run("on: framed and spilled", func(t *testing.T) {
		dir := t.TempDir()
		got := subagentToolResult(t, Config{ToolOutputDir: dir, MaxToolOutputChars: 1000, UntrustedOutputBoundary: true}, big)
		if !strings.HasPrefix(got, "<<<UNTRUSTED_OUTPUT ") || !strings.Contains(got, "\n<<<END_UNTRUSTED_OUTPUT ") {
			t.Fatalf("tool_result not framed: %q", got)
		}
		if !strings.Contains(got, "<persisted-output>") || len(got) >= len(big) {
			t.Fatalf("tool_result not spilled (len=%d): %q", len(got), got)
		}
		files, err := os.ReadDir(dir)
		if err != nil || len(files) != 1 {
			t.Fatalf("spill dir: %v, %d files, want 1", err, len(files))
		}
		full, err := os.ReadFile(filepath.Join(dir, files[0].Name()))
		if err != nil || string(full) != big {
			t.Fatalf("spill file does not hold the full output (len=%d, err=%v)", len(full), err)
		}
	})

	t.Run("off: unchanged", func(t *testing.T) {
		got := subagentToolResult(t, Config{}, strings.Repeat("x", 40000))
		if want := tool.CaptureOnce(&tool.ToolContext{}, strings.Repeat("x", 40000)); got != want {
			t.Fatalf("option off changed the tool_result (len %d, want %d)", len(got), len(want))
		}
		if strings.Contains(got, "UNTRUSTED_OUTPUT") {
			t.Fatalf("option off framed the output")
		}
	})
}
