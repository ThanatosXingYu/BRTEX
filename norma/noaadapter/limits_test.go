package noaadapter

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/Autumn-27/norma/llm"
	"github.com/Autumn-27/norma/noa"
)

func opaqueHistory() []llm.Message {
	return []llm.Message{
		userText("think about it"),
		{Role: llm.RoleAssistant, Content: []llm.ContentBlock{
			{Type: llm.BlockThinking, Thinking: "signed reasoning", Signature: "sig-1"},
			{Type: llm.BlockRedactedThinking, Data: "EqQBCkYIBxgCKkCopaque/encrypted+payload=="},
			llm.TextBlock("answer"),
		}},
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// A redacted thinking block survives Project/Reassemble byte for byte, and the
// session view replays it unchanged.
func TestRedactedThinkingRoundTripsByteForByte(t *testing.T) {
	msgs := opaqueHistory()
	before := mustJSON(t, msgs)

	cores, sc := Project(msgs)
	var redacted int
	for _, c := range cores {
		if c.ContentType == noa.CTReasoning && c.Text == "EqQBCkYIBxgCKkCopaque/encrypted+payload==" {
			redacted++
		}
	}
	if redacted != 1 {
		t.Fatalf("redacted thinking projected %d times, want 1: %+v", redacted, cores)
	}
	if got := mustJSON(t, Reassemble(cores, msgs, sc, ReassembleOptions{})); got != before {
		t.Fatalf("round trip changed the history:\n got %s\nwant %s", got, before)
	}

	// The session view tags user text, so compare the assistant message only.
	view := simSession(t, 40000).View(msgs)
	if len(view) != 2 || mustJSON(t, view[1]) != mustJSON(t, msgs[1]) {
		t.Fatalf("view changed the opaque blocks:\n got %s\nwant %s", mustJSON(t, view), before)
	}
	if mustJSON(t, msgs) != before {
		t.Fatal("the original history was modified")
	}
}

// A signed thinking block keeps its text and signature even when the view
// rewrites its core; an unsigned one still takes the rewrite.
func TestSignedThinkingIsNotRewritten(t *testing.T) {
	signed := llm.ContentBlock{Type: llm.BlockThinking, Thinking: "signed text", Signature: "signature"}
	unsigned := llm.ContentBlock{Type: llm.BlockThinking, Thinking: "plain text"}
	msgs := []llm.Message{{Role: llm.RoleAssistant, Content: []llm.ContentBlock{signed, unsigned, llm.TextBlock("answer")}}}

	cores, sc := Project(msgs)
	for i := range cores {
		if cores[i].ContentType == noa.CTReasoning {
			cores[i].Text = "rewritten"
		}
	}
	got := Reassemble(cores, msgs, sc, ReassembleOptions{})
	if len(got) != 1 || len(got[0].Content) != 3 {
		t.Fatalf("reassembled %+v", got)
	}
	if !reflect.DeepEqual(got[0].Content[0], signed) {
		t.Fatalf("signed block changed: %+v", got[0].Content[0])
	}
	if got[0].Content[1].Thinking != "rewritten" || got[0].Content[1].Signature != "" {
		t.Fatalf("unsigned block did not take the rewrite: %+v", got[0].Content[1])
	}
}

// Only large, non-Compress tool results are trimmed, largest first, keeping
// both ends around the marker.
func TestBoundToolCopiesTrimsLargestNonCompressResults(t *testing.T) {
	big := strings.Repeat("é", 3000) // multi-byte runes
	medium := strings.Repeat("m", 1000)
	small := strings.Repeat("s", 400)
	view := []noa.CoreMessage{
		{ID: "a", ContentType: noa.CTToolCall, ToolName: "Bash", Text: `{"command":"ls"}`},
		{ID: "b", ContentType: noa.CTToolResult, ToolName: "Bash", Text: medium},
		{ID: "c", ContentType: noa.CTToolResult, ToolName: "Read", Text: big},
		{ID: "d", ContentType: noa.CTToolResult, ToolName: noa.CompressToolName, Text: big},
		{ID: "e", ContentType: noa.CTToolResult, ToolName: "Bash", Text: small},
	}
	orig := append([]noa.CoreMessage(nil), view...)
	size := func(v []noa.CoreMessage) int {
		n := 0
		for _, m := range v {
			n += utf8.RuneCountInString(m.Text)
		}
		return n
	}
	// Trimming the largest eligible result is enough to reach this target.
	out, trimmed := boundToolCopies(view, size(view)-2000, size)
	if trimmed != 1 {
		t.Fatalf("trimmed %d copies, want 1", trimmed)
	}
	r := []rune(big)
	want := string(r[:128]) + boundMarker + string(r[len(r)-128:])
	if out[2].Text != want {
		t.Fatalf("largest result not trimmed to head+marker+tail: %q", out[2].Text)
	}
	if out[1].Text != medium || out[3].Text != big || out[4].Text != small || out[0].Text != orig[0].Text {
		t.Fatal("a copy other than the largest eligible result changed")
	}
	if !reflect.DeepEqual(view, orig) {
		t.Fatal("the input view was modified")
	}
	// An unreachable target trims every eligible result and nothing else.
	out, trimmed = boundToolCopies(view, 0, size)
	if trimmed != 2 || out[3].Text != big || out[4].Text != small || out[0].Text != orig[0].Text {
		t.Fatalf("trimmed=%d, compress=%v small=%v", trimmed, out[3].Text == big, out[4].Text == small)
	}
}

// A long session whose model never compresses stays under 70% of the window,
// with the originals, call arguments and tool pairing untouched.
func TestUncooperativeLongSessionStaysUnderToolBudget(t *testing.T) {
	const window = 40000
	sess := simSession(t, window)
	a := newSimAgent(t, sess, 1000000) // never compresses
	for range 60 {
		a.work(4000)
		assertValidRequest(t, a.observe(), a.turn)
	}
	if a.compressions != 0 {
		t.Fatal("the fixture compressed something")
	}
	before := mustJSON(t, a.history)
	view := sess.View(a.history)
	if final := estimateMessages(view); final > window*7/10 {
		t.Fatalf("final request %d tokens, over 70%% of the %d window", final, window)
	}
	if sess.lastTruncatedCount == 0 {
		t.Fatal("no tool-result copy was trimmed")
	}
	if mustJSON(t, a.history) != before {
		t.Fatal("the host history was modified")
	}
	for _, m := range a.history {
		for _, b := range m.Content {
			for _, inner := range b.Content {
				if strings.Contains(inner.Text, boundMarker) {
					t.Fatal("a trimmed copy reached the host history")
				}
			}
		}
	}
	assertValidRequest(t, view, a.turn)
	// Call arguments go out exactly as the model wrote them.
	inputs := map[string]string{}
	for _, m := range a.history {
		for _, b := range m.Content {
			if b.Type == llm.BlockToolUse {
				inputs[b.ID] = string(b.Input)
			}
		}
	}
	marked := 0
	for _, m := range view {
		for _, b := range m.Content {
			if b.Type == llm.BlockToolUse && string(b.Input) != inputs[b.ID] {
				t.Fatalf("tool_use %s arguments changed", b.ID)
			}
			for _, inner := range b.Content {
				if strings.Contains(inner.Text, boundMarker) {
					marked++
				}
			}
		}
	}
	if marked == 0 {
		t.Fatal("the final request carries no trimmed copy")
	}
	t.Logf("final=%d of %d (70%% = %d), lastTruncatedCount=%d, trimmed copies in request=%d",
		estimateMessages(view), window, window*7/10, sess.lastTruncatedCount, marked)
}
