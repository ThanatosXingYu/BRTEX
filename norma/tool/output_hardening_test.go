package tool

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// A tool that bypasses Capture and returns a raw 100 KB output containing a
// marker string must still come out bounded: the marker alone is not trust.
func TestCaptureOnceSpoofedMarkerIsStillCapped(t *testing.T) {
	const max = 1000
	raw := "<persisted-output>" + strings.Repeat("x", 100*1024)
	tc := &ToolContext{MaxOutputChars: max, OutputDir: t.TempDir()}
	got := CaptureOnce(tc, raw)
	if len(got) > max+captureNoticeSlack {
		t.Fatalf("raw output with a forged marker came back unbounded: %d bytes", len(got))
	}
}

// Spilled output can contain secrets, so the file must be owner-only and the
// spill directory must not be traversable by other users.
func TestSpillFileAndDirAreOwnerOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits do not apply on Windows")
	}
	dir := filepath.Join(t.TempDir(), "spill")
	path, err := spillOutput(dir, strings.Repeat("s", 5000))
	if err != nil {
		t.Fatal(err)
	}
	fi, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if perm := fi.Mode().Perm(); perm != 0o600 {
		t.Fatalf("spill file mode %o, want 600", perm)
	}
	di, err := os.Stat(dir)
	if err != nil {
		t.Fatal(err)
	}
	if perm := di.Mode().Perm(); perm != 0o700 {
		t.Fatalf("spill dir mode %o, want 700", perm)
	}
}
