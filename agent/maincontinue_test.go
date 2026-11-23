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

func toolUse(id string) llm.ContentBlock {
	return llm.ContentBlock{Type: llm.BlockToolUse, ID: id, Name: "LS", Input: []byte(`{}`)}
}

func TestMainContinueNudgesOnlyWithOpenTodos(t *testing.T) {
	todos := actool.NewTodoStore()
	todos.Tool().Call(context.Background(), []byte(`{"todos":[{"content":"scan","status":"in_progress"},{"content":"report","status":"pending"}]}`), nil)
	h := newMainContinueHooks(nil, todos, "扫描 10.0.0.5")

	prevent, blocking, _ := h.Stop(context.Background(), []llm.Message{llm.UserText("扫描 10.0.0.5"), assistant("done so far")})
	if prevent || len(blocking) != 1 {
		t.Fatalf("want one continuation nudge, got prevent=%v blocking=%d", prevent, len(blocking))
	}
	if h.nudges != 1 {
		t.Fatalf("nudges=%d want 1", h.nudges)
	}
	for i := 0; i < mainContinueLimit-1; i++ {
		if _, b, _ := h.Stop(context.Background(), []llm.Message{llm.UserText("扫描 10.0.0.5"), assistant("still talking")}); len(b) != 1 {
			t.Fatalf("iteration %d: want a nudge", i)
		}
	}
	if _, b, _ := h.Stop(context.Background(), []llm.Message{llm.UserText("扫描 10.0.0.5"), assistant("and more")}); len(b) != 0 {
		t.Fatal("cap must stop further nudges")
	}
}

// The failure this fix targets: an action request that never called a tool and
// wrote no plan, then stopped after one paragraph.
func TestMainContinueNudgesActionRequestWithoutPlan(t *testing.T) {
	h := newMainContinueHooks(nil, actool.NewTodoStore(), "对 10.0.0.5 做端口扫描")
	if _, b, _ := h.Stop(context.Background(), []llm.Message{llm.UserText("对 10.0.0.5 做端口扫描"), assistant("已确认目标可达，下面开始扫描。")}); len(b) != 1 {
		t.Fatal("an action request that never called a tool must be nudged once")
	}
	if h.nudges != 1 {
		t.Fatalf("nudges=%d want 1", h.nudges)
	}
	// Once the turn has run any tool, a later normal answer is left alone.
	h.nudges = 0
	msgs := []llm.Message{
		llm.UserText("对 10.0.0.5 做端口扫描"),
		assistant("scanning", toolUse("c1")),
		{Role: llm.RoleUser, Content: []llm.ContentBlock{llm.ToolResultText("c1", "22/tcp open", false)}},
		assistant("端口 22 开放，扫描完成。"),
	}
	if _, b, _ := h.Stop(context.Background(), msgs); len(b) != 0 {
		t.Fatal("a turn that already ran tools must not be nudged again")
	}
}

func TestMainContinueNoPlanCapAndQuestions(t *testing.T) {
	// Cap: the no-plan branch is bounded lower than the plan branch.
	h := newMainContinueHooks(nil, actool.NewTodoStore(), "扫描 10.0.0.5")
	for i := 0; i < mainContinueNoPlanLimit; i++ {
		if _, b, _ := h.Stop(context.Background(), []llm.Message{llm.UserText("扫描 10.0.0.5"), assistant("汇报")}); len(b) != 1 {
			t.Fatalf("iteration %d: want a nudge", i)
		}
	}
	if _, b, _ := h.Stop(context.Background(), []llm.Message{llm.UserText("扫描 10.0.0.5"), assistant("again")}); len(b) != 0 {
		t.Fatal("no-plan cap must stop further nudges")
	}

	// Pure questions are never nudged.
	for _, q := range []string{"现在进度怎么样？", "为什么没有找到漏洞", "有哪些资产", "what is the current status", "how many findings", "你好", "谢谢"} {
		hq := newMainContinueHooks(nil, actool.NewTodoStore(), q)
		if _, b, _ := hq.Stop(context.Background(), []llm.Message{llm.UserText(q), assistant("这是回答。")}); len(b) != 0 {
			t.Fatalf("question %q must not be nudged", q)
		}
	}
}

func TestMainContinueSkipsToolRoundsAndClosedPlans(t *testing.T) {
	todos := actool.NewTodoStore()
	todos.Tool().Call(context.Background(), []byte(`{"todos":[{"content":"scan","status":"completed"}]}`), nil)
	h := newMainContinueHooks(nil, todos, "扫描 10.0.0.5")
	if _, b, _ := h.Stop(context.Background(), []llm.Message{llm.UserText("扫描 10.0.0.5"), assistant("all done")}); len(b) != 0 {
		t.Fatal("a closed plan must never be nudged")
	}

	h = newMainContinueHooks(nil, actool.NewTodoStore(), "扫描 10.0.0.5")
	h.nudges = 2
	msgs := []llm.Message{
		llm.UserText("扫描 10.0.0.5"),
		assistant("looking", toolUse("c1")),
	}
	if _, b, _ := h.Stop(context.Background(), msgs); len(b) != 0 {
		t.Fatal("a tool round must not be nudged")
	}
	if h.nudges != 0 {
		t.Fatalf("tool round must reset the counter, nudges=%d", h.nudges)
	}
}

func TestMainContinueRespectsInnerHook(t *testing.T) {
	todos := actool.NewTodoStore()
	todos.Tool().Call(context.Background(), []byte(`{"todos":[{"content":"scan","status":"in_progress"}]}`), nil)
	h := newMainContinueHooks(stubStop{blocking: []string{"inner nudge"}}, todos, "扫描")
	_, b, _ := h.Stop(context.Background(), []llm.Message{llm.UserText("扫描"), assistant("x")})
	if len(b) != 1 || b[0] != "inner nudge" {
		t.Fatalf("inner hook wins, got %v", b)
	}

	h = newMainContinueHooks(stubStop{prevent: true}, todos, "扫描")
	if prevent, b, _ := h.Stop(context.Background(), []llm.Message{llm.UserText("扫描"), assistant("x")}); !prevent || len(b) != 0 {
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