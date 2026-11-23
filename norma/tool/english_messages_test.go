package tool

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"
)

// cjk matches CJK ideographs, CJK punctuation and full-width forms.
var cjk = regexp.MustCompile(`[\x{3400}-\x{9FFF}\x{3000}-\x{303F}\x{FF00}-\x{FFEF}]`)

// The interactive shell tools describe themselves to the model in English.
func TestShellToolDescriptionsAreEnglish(t *testing.T) {
	open := NewShellOpen()
	if !strings.HasPrefix(open.Description(), "Open a persistent PTY interactive session") ||
		!strings.Contains(open.Description(), "**Use Bash for one-shot, non-interactive commands.**") ||
		!strings.Contains(open.Description(), `'msf6? >\s*$'`) {
		t.Fatalf("shell_open description: %q", open.Description())
	}
	for _, tl := range []CoreTool{open, NewShellSend(), NewShellRead(), NewShellClose(), NewShellList()} {
		schema, err := json.Marshal(tl.InputSchema())
		if err != nil {
			t.Fatal(err)
		}
		if s := tl.Description() + string(schema); cjk.MatchString(s) {
			t.Errorf("%s: CJK text in description or schema: %s", tl.Name(), cjk.FindString(s))
		}
	}
}

// Background task completion notices are English.
func TestTaskCompletionMessagesAreEnglish(t *testing.T) {
	m := &Manager{}
	for _, tc := range []struct {
		task *Task
		want string
	}{
		{&Task{Command: "make", Kind: KindBash, Status: TaskCompleted, ExitCode: 0},
			`Task "make" completed (exit 0). Use TaskOutput or Read to read the output file.`},
		{&Task{Command: "make", Kind: KindBash, Status: TaskFailed, ExitCode: 2},
			`Task "make" failed (exit 2). Use TaskOutput or Read to read the output file.`},
		{&Task{Command: "yes", Kind: KindBash, Status: TaskFailed, oversize: true},
			`Task "yes" was terminated because its output exceeded the limit (` + string(KindBash) + `).`},
	} {
		if got := m.notifFor(tc.task).Summary; got != tc.want {
			t.Errorf("summary %q, want %q", got, tc.want)
		}
	}
}
