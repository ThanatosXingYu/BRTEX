package agent

import (
	"context"
	"log"
	"strings"

	"github.com/Autumn-27/norma/harness"
	"github.com/Autumn-27/norma/llm"
	actool "github.com/Autumn-27/norma/tool"
)

const (
	// mainContinueLimit bounds nudges for a turn whose plan (TodoWrite) still has
	// items open.
	mainContinueLimit = 3
	// mainContinueAnnounceLimit bounds the "announced but never acted" nudges per
	// run. Every one of them is followed by a real model call, so this only caps
	// how often the agent may re-announce instead of reporting.
	mainContinueAnnounceLimit = 6
)

// mainContinueNudge is injected when the run's own plan still has open items.
const mainContinueNudge = "[自动续跑] 你上一条回复只输出了文字，没有继续调用工具，但 TodoWrite 里仍有未完成的计划项。" +
	"请直接调用工具执行下一步；确实全部完成时，先把这些项标记为 completed 再回复结论。" +
	"如果是用户在提问、只需要回答无需操作，请忽略本条并直接回答。"

// mainContinueAnnounceNudge covers the shape this fix was written for: the
// closing text announced the next step ("先看内容。", "Let me pull the facts.",
// "…并确认节拍：") but the turn ended before it was performed, so the operator
// had to type "继续" to make progress.
const mainContinueAnnounceNudge = "[自动续跑] 你上一条回复的最后一句已经宣布了下一步动作（例如“先看内容”“Let me pull the facts.”），" +
	"但这一轮没有调用任何工具，任务停在了半路。请立即调用工具执行这一步并继续推进，直到做完为止。" +
	"如果这一步其实已经完成（只是没写出来），请不要调用工具，直接给出最终结论。"

// mainContinueHooks adds a bounded continuation layer on top of the task hooks.
// A main-agent run is one-shot: a turn that ends without tool_use ends everything.
//
// Two shapes are nudged: a plan with pending/in_progress items, and a closing
// text that announces a step the model never performed. A reply that answers,
// summarizes, asks the operator a question, gives advice or waits for a worker
// is never interrupted, so a normal final answer stays exactly as the model
// wrote it.
type mainContinueHooks struct {
	inner     harness.HookRunner
	todos     *actool.TodoStore
	nudges    int // nudges since the last tool round
	announced int // "announced but not acted" nudges in this run
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

// AllowRepeatedStopBlocking tells the harness this hook may be asked again
// after every text-only stop. The harness otherwise fires the Stop hook at most
// once between two tool rounds; this hook carries its own budget instead.
func (h *mainContinueHooks) AllowRepeatedStopBlocking() bool { return true }

func (h *mainContinueHooks) Stop(ctx context.Context, messages []llm.Message) (bool, []string, string) {
	prevent, blocking, msg := false, []string(nil), ""
	if h.inner != nil {
		prevent, blocking, msg = h.inner.Stop(ctx, messages)
	}
	if prevent || len(blocking) > 0 {
		return prevent, blocking, msg
	}
	text := lastAssistantText(messages)
	if text == "" {
		// This round acted, or produced nothing at all: nothing to nudge. A tool round
		// also refreshes the budget for the next stretch of narration.
		h.nudges = 0
		return prevent, blocking, msg
	}
	if stop := LastStopReason(ctx); IsRefusalStop(stop) {
		return prevent, blocking, msg
	}

	if open := h.openTodos(); open > 0 {
		if h.nudges >= mainContinueLimit {
			log.Printf("[mainagent] %d open todo(s) remain but the continuation cap %d was reached, letting the turn finish", open, mainContinueLimit)
			return prevent, blocking, msg
		}
		h.nudges++
		h.announced++
		log.Printf("[mainagent] text-only stop with %d open todo(s), injecting continuation (%d/%d)", open, h.nudges, mainContinueLimit)
		return false, []string{mainContinueNudge}, ""
	}

	if strength, ok := pendingAction(text); ok {
		if h.announced >= mainContinueAnnounceLimit {
			log.Printf("[mainagent] the closing text still announces work, but the continuation cap %d was reached, letting the turn finish", mainContinueAnnounceLimit)
			return prevent, blocking, msg
		}
		h.nudges++
		h.announced++
		log.Printf("[mainagent] closing text announces an unfinished step (%v), injecting continuation (%d/%d)", strength, h.announced, mainContinueAnnounceLimit)
		return false, []string{mainContinueAnnounceNudge}, ""
	}

	h.nudges = 0
	return prevent, blocking, msg
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

// lastAssistantText returns the text of the newest assistant message when that
// message called no tool — the shape a one-shot run ends on. It returns "" for a
// tool round (the run acted) and for an empty reply.
func lastAssistantText(messages []llm.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		m := messages[i]
		if m.Role != llm.RoleAssistant {
			continue
		}
		if len(m.ToolUses()) != 0 {
			return ""
		}
		return strings.TrimSpace(m.Text())
	}
	return ""
}
