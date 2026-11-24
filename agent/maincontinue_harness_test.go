package agent

// End-to-end check that the continuation hook is really wired into the harness
// loop for the main agent: a turn that announces a next step but never calls a
// tool must be re-asked instead of ending the whole run — while a normal final
// answer must end the run untouched.

import (
	"context"
	"iter"
	"strings"
	"testing"

	"github.com/Autumn-27/norma/harness"
	"github.com/Autumn-27/norma/llm"
	"github.com/Autumn-27/norma/permission"
	actool "github.com/Autumn-27/norma/tool"
)

// announceThenAct plays: announcing text only, then a tool call, then a final answer.
type announceThenAct struct {
	calls int
}

func (m *announceThenAct) call(_ context.Context, _ llm.CompletionRequest) iter.Seq2[llm.StreamEvent, error] {
	i := m.calls
	m.calls++
	return func(yield func(llm.StreamEvent, error) bool) {
		seq := func(evs ...llm.StreamEvent) bool {
			for _, ev := range evs {
				if !yield(ev, nil) {
					return false
				}
			}
			return true
		}
		switch i {
		case 0: // the failure shape: announced step, text only, natural stop
			seq(
				llm.StreamEvent{Type: llm.SETextDelta, Text: "已确认目标可达，先扫描端口："},
				llm.StreamEvent{Type: llm.SEMessageDelta, StopReason: "end_turn"},
				llm.StreamEvent{Type: llm.SEMessageStop},
			)
		case 1: // after the nudge: act
			seq(
				llm.StreamEvent{Type: llm.SEToolUseStart, ToolID: "c1", ToolName: "LS"},
				llm.StreamEvent{Type: llm.SEToolInputJSON, Text: `{}`},
				llm.StreamEvent{Type: llm.SEMessageDelta, StopReason: "tool_use"},
				llm.StreamEvent{Type: llm.SEMessageStop},
			)
		default:
			seq(
				llm.StreamEvent{Type: llm.SETextDelta, Text: "扫描完成。"},
				llm.StreamEvent{Type: llm.SEMessageDelta, StopReason: "end_turn"},
				llm.StreamEvent{Type: llm.SEMessageStop},
			)
		}
	}
}

// answerOnly answers in one turn and never announces more work.
type answerOnly struct {
	calls int
	text  string
}

func (m *answerOnly) call(_ context.Context, _ llm.CompletionRequest) iter.Seq2[llm.StreamEvent, error] {
	m.calls++
	return func(yield func(llm.StreamEvent, error) bool) {
		yield(llm.StreamEvent{Type: llm.SETextDelta, Text: m.text}, nil)
		yield(llm.StreamEvent{Type: llm.SEMessageDelta, StopReason: "end_turn"}, nil)
		yield(llm.StreamEvent{Type: llm.SEMessageStop}, nil)
	}
}

func TestMainContinueHookWiredIntoHarness(t *testing.T) {
	model := &announceThenAct{}
	hooks := newMainContinueHooks(nil, actool.NewTodoStore())
	term := runQuery(t, model.call, hooks, "对 10.0.0.5 做端口扫描")

	if term.Reason != harness.ReasonCompleted {
		t.Fatalf("reason=%v", term.Reason)
	}
	if model.calls < 3 {
		t.Fatalf("model called %d time(s): the announcing text-only turn was not re-asked", model.calls)
	}
	if term.Text != "扫描完成。" {
		t.Fatalf("terminal text=%q", term.Text)
	}
	// The nudge must be present in the history the model saw, and the tool round must
	// have produced a result.
	var nudged, toolResult bool
	for _, m := range term.Messages {
		if strings.Contains(m.Text(), "[自动续跑]") {
			nudged = true
		}
		for _, b := range m.Content {
			if b.Type == llm.BlockToolResult {
				toolResult = true
			}
		}
	}
	if !nudged {
		t.Fatalf("continuation nudge missing from history: %+v", term.Messages)
	}
	if !toolResult {
		t.Fatal("tool round did not run")
	}
}

// A normal final answer must end the run on the first turn, untouched.
func TestMainContinueHookLeavesNormalAnswerAlone(t *testing.T) {
	for _, text := range []string{
		"扫描完成，端口 22 与 80 开放。",
		"目前共拿到 629 条订单，其中 577 条带真实卡密。",
		"还需要我继续扫描其余端口吗？",
		"建议先做权限梳理，再动上传点；由你决定。",
	} {
		model := &answerOnly{text: text}
		hooks := newMainContinueHooks(nil, actool.NewTodoStore())
		term := runQuery(t, model.call, hooks, "汇报一下当前进展")
		if model.calls != 1 {
			t.Fatalf("closing %q: model called %d times, a normal answer must not be re-asked", text, model.calls)
		}
		if term.Reason != harness.ReasonCompleted {
			t.Fatalf("closing %q: reason=%v", text, term.Reason)
		}
		if term.Text != text {
			t.Fatalf("closing %q: terminal text=%q", text, term.Text)
		}
	}
}

func runQuery(t *testing.T, call func(context.Context, llm.CompletionRequest) iter.Seq2[llm.StreamEvent, error], hooks harness.HookRunner, message string) *harness.Terminal {
	t.Helper()
	var term *harness.Terminal
	in := harness.QueryInput{
		Messages:       []llm.Message{llm.UserText(message)},
		Tools:          actool.NewRegistry(actool.NewLS()),
		PermissionMode: permission.ModeBypass,
		WorkingDir:     t.TempDir(),
		Hooks:          hooks,
	}
	for ev, err := range harness.Query(context.Background(), in, harness.QueryDeps{CallModel: call}) {
		if err != nil {
			t.Fatalf("query error: %v", err)
		}
		if ev.Kind == harness.KindResult {
			term = ev.Terminal
		}
	}
	if term == nil {
		t.Fatal("no terminal event")
	}
	return term
}
