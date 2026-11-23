package agent

import (
	"context"
	"testing"

	"github.com/Autumn-27/norma/llm"
	actool "github.com/Autumn-27/norma/tool"
)

func assistant(text string, tools ...llm.ContentBlock) llm.Message {
	blocks := []llm.ContentBlock{llm.TextBlock(text)}
	blocks = append(blocks, tools...)
	return llm.Message{Role: llm.RoleAssistant, Content: blocks}
}

func TestMainContinueNudgesOnlyWithOpenTodos(t *testing.T) {
	todos := actool.NewTodoStore()
	todos.Tool().Call(context.Background(), []byte(`{"todos":[{"content":"scan","status":"in_progress"},{"content":"report","status":"pending"}]}`), nil)
	h := newMainContinueHooks(nil, todos)

	prevent, blocking, _ := h.Stop(context.Background(), []llm.Message{llm.UserText("go"), assistant("done so far")})
	if prevent || len(blocking) != 1 {
		t.Fatalf("want one continuation nudge, got prevent=%v blocking=%d", prevent, len(blocking))
	}
	if h.nudges != 1 {
		t.Fatalf("nudges=%d want 1", h.nudges)
	}

	// A second text-only stop (harness clears stopHookActive only after another
	// text round) still gets a nudge until the cap.
	for i := 0; i < mainContinueLimit-1; i++ {
		if _, b, _ := h.Stop(context.Background(), []llm.Message{llm.UserText("go"), assistant("still talking")}); len(b) != 1 {
			t.Fatalf("iteration %d: want a nudge", i)
		}
	}
	if _, b, _ := h.Stop(context.Background(), []llm.Message{llm.UserText("go"), assistant("and more")}); len(b) != 0 {
		t.Fatal("cap must stop further nudges")
	}
}

func TestMainContinueSkipsToolRoundsAndClosedPlans(t *testing.T) {
	todos := actool.NewTodoStore()
	todos.Tool().Call(context.Background(), []byte(`{"todos":[{"content":"scan","status":"completed"}]}`), nil)
	h := newMainContinueHooks(nil, todos)
	if _, b, _ := h.Stop(context.Background(), []llm.Message{llm.UserText("go"), assistant("all done")}); len(b) != 0 {
		t.Fatal("a closed plan must never be nudged")
	}

	// Empty plan: a plain answer/refusal passes through untouched.
	h = newMainContinueHooks(nil, actool.NewTodoStore())
	if _, b, _ := h.Stop(context.Background(), []llm.Message{llm.UserText("q?"), assistant("answer")}); len(b) != 0 {
		t.Fatal("no todo list → no nudge")
	}

	// Latest assistant turn called a tool → not a premature stop.
	todos = actool.NewTodoStore()
	todos.Tool().Call(context.Background(), []byte(`{"todos":[{"content":"scan","status":"in_progress"}]}`), nil)
	h = newMainContinueHooks(nil, todos)
	tu := llm.ContentBlock{Type: llm.BlockToolUse, ID: "c1", Name: "LS", Input: []byte(`{}`)}
	if _, b, _ := h.Stop(context.Background(), []llm.Message{llm.UserText("go"), assistant("looking", tu)}); len(b) != 0 {
		t.Fatal("a tool round must not be nudged")
	}
	if h.nudges != 0 {
		t.Fatalf("tool round must reset the counter, nudges=%d", h.nudges)
	}
}

func TestMainContinueRespectsInnerHook(t *testing.T) {
	todos := actool.NewTodoStore()
	todos.Tool().Call(context.Background(), []byte(`{"todos":[{"content":"scan","status":"in_progress"}]}`), nil)
	h := newMainContinueHooks(stubStop{blocking: []string{"inner nudge"}}, todos)
	_, b, _ := h.Stop(context.Background(), []llm.Message{llm.UserText("go"), assistant("x")})
	if len(b) != 1 || b[0] != "inner nudge" {
		t.Fatalf("inner hook wins, got %v", b)
	}

	h = newMainContinueHooks(stubStop{prevent: true}, todos)
	if prevent, b, _ := h.Stop(context.Background(), []llm.Message{llm.UserText("go"), assistant("x")}); !prevent || len(b) != 0 {
		t.Fatal("inner hard-stop must be respected")
	}
}

type stubStop struct {
	prevent  bool
	blocking []string
}

func (s stubStop) PreToolUse(context.Context, string, []byte) (bool, string, []byte) { return false, "", nil }
func (s stubStop) PostToolUse(context.Context, string, []byte, []byte, bool)        {}
func (s stubStop) Stop(context.Context, []llm.Message) (bool, []string, string) {
	return s.prevent, s.blocking, ""
}