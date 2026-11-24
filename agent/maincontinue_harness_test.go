package agent

// End-to-end check that the continuation hook is really wired into the harness
// loop for the main agent: a turn that only writes text must be re-asked instead of
// ending the whole run.

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

// textOnlyThenAct plays: paragraph only, then a tool call, then a final answer.
type textOnlyThenAct struct {
	calls int
}

func (m *textOnlyThenAct) call(_ context.Context, _ llm.CompletionRequest) iter.Seq2[llm.StreamEvent, error] {
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
		case 0: // the failure shape: text only, natural stop
			seq(
				llm.StreamEvent{Type: llm.SETextDelta, Text: "已确认目标可达，下面开始扫描。"},
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

func TestMainContinueHookWiredIntoHarness(t *testing.T) {
	model := &textOnlyThenAct{}
	hooks := newMainContinueHooks(nil, actool.NewTodoStore(), "对 10.0.0.5 做端口扫描")
	term := runQuery(t, model.call, hooks, "对 10.0.0.5 做端口扫描")

	if term.Reason != harness.ReasonCompleted {
		t.Fatalf("reason=%v", term.Reason)
	}
	if model.calls < 3 {
		t.Fatalf("model called %d time(s): the text-only turn was not re-asked", model.calls)
	}
	if term.Text != "扫描完成。" {
		t.Fatalf("terminal text=%q", term.Text)
	}
	// The nudge must be present in the history the model saw, and the tool round must
	// have produced a result.
	var nudged, toolResult bool
	for _, m := range term.Messages {
		if strings.Contains(m.Text(), "还没有通过任何工具落地") {
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

// A question must not be re-asked.
func TestMainContinueHookIgnoresQuestionsInHarness(t *testing.T) {
	model := &textOnlyThenAct{}
	hooks := newMainContinueHooks(nil, actool.NewTodoStore(), "现在进度怎么样？")
	term := runQuery(t, model.call, hooks, "现在进度怎么样？")
	if model.calls != 1 {
		t.Fatalf("model called %d times, a question must end after one answer", model.calls)
	}
	if term.Reason != harness.ReasonCompleted {
		t.Fatalf("reason=%v", term.Reason)
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