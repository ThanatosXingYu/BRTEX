package noa

import (
	"regexp"
	"strings"
	"testing"
)

// Archive titles and labels are English; content stays verbatim.
func TestArchiveLabelsAreEnglish(t *testing.T) {
	child := CompressionBlock{BlockID: "b1", Tier: 1, StartRef: "m00001", EndRef: "m00004",
		Summary: "child summary", ArchiveRel: "tier1/b1.md", ArchivePath: "/archives/tier1/b1.md"}
	body := string(RenderArchive(ArchiveRenderInput{
		BlockID: "b2", Tier: 2, StartRef: "m00001", EndRef: "m00009",
		Summary: "parent summary",
		Entries: []ArchiveEntry{{Block: &child}},
	}))
	for _, want := range []string{
		"# Archive b2 · Tier 2 · m00001–m00009\n",
		"**Summary** (the version currently in effect in the context):\n\n> parent summary\n",
		"> Child archive: [`tier1/b1.md`](../tier1/b1.md)\n",
		"> Absolute path: `/archives/tier1/b1.md`\n",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("archive lacks %q:\n%s", want, body)
		}
	}
	if s := regexp.MustCompile(`[\x{3400}-\x{9FFF}\x{3000}-\x{303F}\x{FF00}-\x{FFEF}]`).FindString(body); s != "" {
		t.Errorf("CJK text %q in the rendered archive:\n%s", s, body)
	}
}
