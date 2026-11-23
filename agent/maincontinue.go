package agent

import (
	"context"
	"log"

	"github.com/Autumn-27/norma/harness"
	"github.com/Autumn-27/norma/llm"
	actool "github.com/Autumn-27/norma/tool"
)

// mainContinueLimit bounds how many "keep going" nudges one human turn may take.
// A main-agent run is one-shot: a text-only stop ends it, and the human would have
// to type "继续" to make progress. The nudge covers the case where the agent still
// has unfinished plan items but reported instead of acting; the cap keeps a model
// that keeps answering without closing its plan from looping.
const mainContinueLimit = 3

// mainContinueNudge is injected when the main agent stops with open todos.
const mainContinueNudge = "你上一轮只输出了文字，没有继续调用工具，但 TodoWrite 里仍有未完成的计划项。" +
	"请直接执行下一步：调用工具继续推进；确实全部完成时，先把这些项标记为 completed 再回复结论。" +
	"如果是用户在提问、只需要回答无需操作，请忽略本条并直接回答。"

// mainContinueHooks wraps the task hooks for the main agent and adds one
// continuation layer: a stop with no tool call while the run's todo list still has
// pending/in_progress items gets one blocking nudge. Counters reset on every real
// tool round, so a healthy tool→text→tool sequence is never affected.
type mainContinueHooks struct {
	inner   harness.HookRunner
	todos   *actool.TodoStore
	nudges  int
	toolRan bool
}

func newMainContinueHooks(inner harness.HookRunner, todos *actool.TodoStore) *mainContinueHooks {
	return &mainContinueHooks{inner: inner, todos: todos}
}

func (h *mainContinueHooks) PreToolUse(ctx context.Context, name string, input []byte) (bool, string, []byte) {
	if h.inner != nil {
		return h.inner.PreToolUse(ctx, name, input)
	}
	return false, "", nil
}

func (h *mainContinueHooks) PostToolUse(ctx context.Context, name string, input, result []byte, isErr bool) {
	if h.inner != nil {
		h.inner.PostToolUse(ctx, name, input, result, isErr)
	}
}

func (h *mainContinueHooks) Stop(ctx context.Context, messages []llm.Message) (bool, []string, string) {
	prevent, blocking, msg := false, []string(nil), ""
	if h.inner != nil {
		prevent, blocking, msg = h.inner.Stop(ctx, messages)
	}
	if prevent || len(blocking) > 0 || h.sawToolUse(messages) {
		// A blocking message already queued, a hard stop, or this round ran tools:
		// nothing to add, and the tool round refreshes the nudge budget.
		h.reset()
		return prevent, blocking, msg
	}
	if !h.hasOpenTodos() {
		// No plan open → this was a normal answer (a question, a report with no
		// pending work, or a refusal). Do not interrupt it.
		return prevent, blocking, msg
	}
	if stop := LastStopReason(ctx); IsRefusalStop(stop) {
		return prevent, blocking, msg
	}
	if h.nudges >= mainContinueLimit {
		log.Printf("[mainagent] open todos remain but the continuation cap %d was reached, letting the turn finish", mainContinueLimit)
		return prevent, blocking, msg
	}
	h.nudges++
	log.Printf("[mainagent] text-only stop with %d open todo(s), injecting continuation (%d/%d)", h.openTodos(), h.nudges, mainContinueLimit)
	return false, []string{mainContinueNudge}, ""
}

func (h *mainContinueHooks) reset() { h.nudges, h.toolRan = 0, false }

// sawToolUse reports whether the newest assistant turn called a tool.
func (h *mainContinueHooks) sawToolUse(messages []llm.Message) bool {
	for i := len(messages) - 1; i >= 0; i-- {
		m := messages[i]
		if m.Role != llm.RoleAssistant {
			continue
		}
		return len(m.ToolUses()) > 0
	}
	return false
}

func (h *mainContinueHooks) openTodos() int {
	n := 0
	for _, t := range h.todos.List() {
		if t.Status != actool.TodoCompleted {
			n++
		}
	}
	return n
}

func (h *mainContinueHooks) hasOpenTodos() bool { return h.openTodos() > 0 }
