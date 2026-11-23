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
	// mainContinueNoPlanLimit bounds nudges for an action request that never wrote a
	// plan and has not called a single tool. Kept lower: without a plan the model may
	// legitimately have finished.
	mainContinueNoPlanLimit = 2
)

const mainContinueNudge = "你上一轮只输出了文字，没有继续调用工具，但 TodoWrite 里仍有未完成的计划项。" +
	"请直接执行下一步：调用工具继续推进；确实全部完成时，先把这些项标记为 completed 再回复结论。" +
	"如果是用户在提问、只需要回答无需操作，请忽略本条并直接回答。"

// mainContinueNoPlanNudge covers the shape this fix was written for: the operator
// asked for work, the turn called no tool at all and just reported, and the one-shot
// run then ended (progress depended on the human typing "继续").
const mainContinueNoPlanNudge = "这条指令还没有通过任何工具落地。如果任务确实需要操作，请现在调用第一步工具继续推进，" +
	"不要只回复一段文字就停下；如果该指令只需要口头回答、或工作已经全部完成，请直接说明并忽略本条。"

// mainContinueHooks adds a bounded continuation layer on top of the task hooks.
// A main-agent run is one-shot: a turn that ends without tool_use ends everything.
//
// Two shapes are nudged: a plan with pending/in_progress items, and an action
// request whose turn called no tool at all and wrote no plan. A turn that already
// ran tools and then answered, a question from the operator, and a provider refusal
// are never interrupted. The harness calls Stop at most once per text-only stretch
// (stopHookActive), and the counter here also resets on every tool round.
type mainContinueHooks struct {
	inner   harness.HookRunner
	todos   *actool.TodoStore
	message string
	nudges  int
}

func newMainContinueHooks(inner harness.HookRunner, todos *actool.TodoStore, message string) *mainContinueHooks {
	return &mainContinueHooks{inner: inner, todos: todos, message: message}
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
	if prevent || len(blocking) > 0 {
		return prevent, blocking, msg
	}
	if !lastAssistantTurnIsTextOnly(messages) {
		// This round acted, or produced nothing at all: nothing to nudge. A tool round
		// also refreshes the budget for the next stretch of narration.
		h.nudges = 0
		return prevent, blocking, msg
	}
	if stop := LastStopReason(ctx); IsRefusalStop(stop) {
		return prevent, blocking, msg
	}

	open := h.openTodos()
	switch {
	case open > 0:
		if h.nudges >= mainContinueLimit {
			log.Printf("[mainagent] %d open todo(s) remain but the continuation cap %d was reached, letting the turn finish", open, mainContinueLimit)
			return prevent, blocking, msg
		}
		h.nudges++
		log.Printf("[mainagent] text-only stop with %d open todo(s), injecting continuation (%d/%d)", open, h.nudges, mainContinueLimit)
		return false, []string{mainContinueNudge}, ""
	case len(h.todos.List()) == 0 && toolsInCurrentTurn(messages) == 0 && isActionRequest(h.message):
		if h.nudges >= mainContinueNoPlanLimit {
			log.Printf("[mainagent] action request still has no tool call after %d continuation(s), letting the turn finish", mainContinueNoPlanLimit)
			return prevent, blocking, msg
		}
		h.nudges++
		log.Printf("[mainagent] action request stopped with no tool call, injecting continuation (%d/%d)", h.nudges, mainContinueNoPlanLimit)
		return false, []string{mainContinueNoPlanNudge}, ""
	}
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

// lastAssistantTurnIsTextOnly reports whether the newest assistant message carries
// text and calls no tool — the shape a one-shot run ends on.
func lastAssistantTurnIsTextOnly(messages []llm.Message) bool {
	for i := len(messages) - 1; i >= 0; i-- {
		m := messages[i]
		if m.Role != llm.RoleAssistant {
			continue
		}
		return len(m.ToolUses()) == 0 && strings.TrimSpace(m.Text()) != ""
	}
	return false
}

// toolsInCurrentTurn counts tool calls after the operator's message that started
// this turn; turns reloaded from the transcript sit before it and do not count.
func toolsInCurrentTurn(messages []llm.Message) int {
	start := 0
	for i, m := range messages {
		if m.Role == llm.RoleUser && strings.TrimSpace(m.Text()) != "" {
			start = i
		}
	}
	n := 0
	for _, m := range messages[start:] {
		if m.Role == llm.RoleAssistant {
			n += len(m.ToolUses())
		}
	}
	return n
}

// isActionRequest reports whether the operator asked for work rather than for an
// answer. Questions, greetings and acknowledgements are "just answer me".
func isActionRequest(message string) bool {
	s := strings.TrimSpace(message)
	if s == "" {
		return false
	}
	if strings.HasSuffix(s, "?") || strings.HasSuffix(s, "？") {
		return false
	}
	lower := strings.ToLower(s)
	for _, q := range []string{"吗", "呢", "什么", "怎么", "为什么", "为何", "是否", "有没有", "多少", "哪些", "哪个", "谁", "何时", "几点", "进度", "状态", "情况如何"} {
		if strings.Contains(lower, q) {
			return false
		}
	}
	for _, q := range []string{"what", "why", "how", "when", "which", "who", "where"} {
		if strings.HasPrefix(lower, q+" ") || lower == q {
			return false
		}
	}
	if runes := []rune(s); len(runes) <= 8 {
		for _, g := range []string{"你好", "您好", "谢谢", "多谢", "好的", "收到", "ok", "okay", "thanks", "thank you", "hi", "hello"} {
			if strings.EqualFold(s, g) {
				return false
			}
		}
	}
	return true
}