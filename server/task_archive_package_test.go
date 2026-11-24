package server

import (
	"archive/tar"
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"syscall"
	"testing"
	"time"

	"github.com/Autumn-27/artex/agent"
	pgdb "github.com/Autumn-27/artex/db"
	"github.com/klauspost/compress/zstd"
)

func TestTaskArchivePackageFilesRoundTrip(t *testing.T) {
	dataDir := t.TempDir()
	taskID := "42"
	explorationID := int64(73)
	// The task directory packs whole: the main agent's run directory and a worker's.
	taskFile := filepath.Join(dataDir, "tasks", taskID, "main", "uploads", "evidence.txt")
	workerFile := filepath.Join(dataDir, "tasks", taskID, "i7", "notes.txt")
	// The task's command-output spills are packed with it, and those of another task stay.
	spillFile := filepath.Join(agent.TaskSpillDir(dataDir, taskID), "output-1-1.txt")
	unrelatedSpill := filepath.Join(agent.TaskSpillDir(dataDir, "43"), "output-1-2.txt")
	transcriptFile := filepath.Join(dataDir, "transcripts", "exp73-worker-1.jsonl")
	unrelatedTranscript := filepath.Join(dataDir, "transcripts", "exp74-worker-1.jsonl")
	for path, body := range map[string]string{
		taskFile:            "task evidence",
		workerFile:          "worker notes",
		spillFile:           "spilled output",
		unrelatedSpill:      "leave me hot",
		transcriptFile:      "transcript",
		unrelatedTranscript: "leave me hot",
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	stage, err := stageTaskArchiveFiles(dataDir, 1, taskID, explorationID)
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{taskFile, workerFile, spillFile, filepath.Dir(spillFile)} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("task file %s still hot after staging: %v", path, err)
		}
	}
	if _, err := os.Stat(unrelatedSpill); err != nil {
		t.Fatalf("the spill of another task was staged: %v", err)
	}
	if _, err := os.Stat(transcriptFile); !os.IsNotExist(err) {
		t.Fatalf("task transcript still hot after staging: %v", err)
	}
	if _, err := os.Stat(unrelatedTranscript); err != nil {
		t.Fatalf("unrelated transcript was staged: %v", err)
	}

	archivePath := taskArchivePath(dataDir, 1, taskID)
	streamPath := filepath.Join(stage.payload, filepath.FromSlash(pgdb.TaskArchiveLLMRecordsPath))
	if err := os.MkdirAll(filepath.Dir(streamPath), archiveDirMode); err != nil {
		t.Fatal(err)
	}
	streamBody := []byte("{\"id\":1,\"raw_request\":\"large\"}\n")
	if err := os.WriteFile(streamPath, streamBody, archiveFileMode); err != nil {
		t.Fatal(err)
	}
	snapshot := &pgdb.TaskArchiveSnapshot{
		FormatVersion: pgdb.TaskArchiveFormatVersion, TaskID: 42, ExplorationID: explorationID,
		StreamedTables: map[string]string{"llm_records": pgdb.TaskArchiveLLMRecordsPath},
	}
	original, compressed, checksum, err := writeTaskArchivePackage(archivePath, stage.payload, snapshot)
	if err != nil {
		t.Fatal(err)
	}
	if original == 0 || compressed == 0 || checksum == "" {
		t.Fatalf("invalid package metrics original=%d compressed=%d checksum=%q", original, compressed, checksum)
	}
	if err := stage.commit(); err != nil {
		t.Fatal(err)
	}
	extracted := filepath.Join(dataDir, "restore")
	if err := extractTaskArchivePackage(archivePath, checksum, extracted); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(filepath.Join(extracted, filepath.FromSlash(pgdb.TaskArchiveLLMRecordsPath))); err != nil || !bytes.Equal(got, streamBody) {
		t.Fatalf("streamed LLM archive payload=%q err=%v", got, err)
	}
	// The package holds the run directories under files/tasks/<id>/.
	for _, rel := range []string{filepath.Join("main", "uploads", "evidence.txt"), filepath.Join("i7", "notes.txt")} {
		if _, err := os.Stat(filepath.Join(extracted, "files", "tasks", taskID, rel)); err != nil {
			t.Fatalf("package entry files/tasks/%s/%s: %v", taskID, rel, err)
		}
	}
	installed, err := installTaskArchiveFiles(dataDir, extracted, taskID, 1, ExecPrivilege{})
	if err != nil {
		t.Fatal(err)
	}
	if err := installed.commit(); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]string{taskFile: "task evidence", workerFile: "worker notes", spillFile: "spilled output", transcriptFile: "transcript"} {
		got, err := os.ReadFile(path)
		if err != nil || string(got) != want {
			t.Fatalf("restored %s = %q, %v; want %q", path, got, err, want)
		}
	}
	if err := extractTaskArchivePackage(archivePath, "deadbeef", filepath.Join(dataDir, "bad-checksum")); err == nil {
		t.Fatal("checksum mismatch was accepted")
	}
}

func TestTaskArchivePackageSkipsSymlink(t *testing.T) {
	dataDir := t.TempDir()
	payload := filepath.Join(dataDir, "payload")
	if err := os.MkdirAll(payload, archiveDirMode); err != nil {
		t.Fatal(err)
	}
	regular := filepath.Join(payload, "keep.txt")
	if err := os.WriteFile(regular, []byte("keep me"), archiveFileMode); err != nil {
		t.Fatal(err)
	}
	// A symlink appearing in the working directory should be skipped instead of failing the whole archive.
	if err := os.Symlink(regular, filepath.Join(payload, "link.txt")); err != nil {
		t.Skipf("symlink unsupported on this platform: %v", err)
	}

	archivePath := filepath.Join(dataDir, "archive.tar.zst")
	snapshot := &pgdb.TaskArchiveSnapshot{FormatVersion: pgdb.TaskArchiveFormatVersion, TaskID: 42}
	_, _, checksum, err := writeTaskArchivePackage(archivePath, payload, snapshot)
	if err != nil {
		t.Fatalf("archive should skip symlink, not fail: %v", err)
	}
	extracted := filepath.Join(dataDir, "restore")
	if err := extractTaskArchivePackage(archivePath, checksum, extracted); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(filepath.Join(extracted, "keep.txt")); err != nil || string(got) != "keep me" {
		t.Fatalf("regular file not archived: got=%q err=%v", got, err)
	}
	if _, err := os.Lstat(filepath.Join(extracted, "link.txt")); !os.IsNotExist(err) {
		t.Fatalf("symlink should have been skipped, but link.txt exists: %v", err)
	}
}

func TestTaskArchiveSkipsFIFO(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("named pipes cannot be created with mkfifo semantics on windows")
	}
	dataDir := t.TempDir()
	taskDir := filepath.Join(dataDir, "tasks", "42")
	if err := os.MkdirAll(filepath.Join(taskDir, "i7", "fakeproc"), 0o755); err != nil {
		t.Fatal(err)
	}
	regular := filepath.Join(taskDir, "i7", "keep.txt")
	if err := os.WriteFile(regular, []byte("keep me"), 0o600); err != nil {
		t.Fatal(err)
	}
	// A fake /proc tree leaves a FIFO behind. Opening it with no writer blocks
	// forever and would wedge the archive worker.
	fifo := filepath.Join(taskDir, "i7", "fakeproc", "cmdline")
	if err := syscall.Mkfifo(fifo, 0o600); err != nil {
		t.Fatalf("mkfifo: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		stage, err := stageTaskArchiveFiles(dataDir, 7, "42", 1)
		if err != nil {
			done <- err
			return
		}
		archivePath := filepath.Join(dataDir, "archive.tar.zst")
		snapshot := &pgdb.TaskArchiveSnapshot{FormatVersion: pgdb.TaskArchiveFormatVersion, TaskID: 42}
		_, _, checksum, err := writeTaskArchivePackage(archivePath, stage.payload, snapshot)
		if err != nil {
			done <- err
			return
		}
		extracted := filepath.Join(dataDir, "restore")
		if err := extractTaskArchivePackage(archivePath, checksum, extracted); err != nil {
			done <- err
			return
		}
		if got, err := os.ReadFile(filepath.Join(extracted, "files", "tasks", "42", "i7", "keep.txt")); err != nil || string(got) != "keep me" {
			done <- errors.New("regular file not archived")
			return
		}
		done <- nil
	}()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(60 * time.Second):
		t.Fatal("archiving a task with a FIFO hung instead of skipping it")
	}
}

func TestTaskArchiveStageCopyFallbackOnRenameFailure(t *testing.T) {
	dataDir := t.TempDir()
	taskFile := filepath.Join(dataDir, "tasks", "42", "uploads", "evidence.txt")
	if err := os.MkdirAll(filepath.Dir(taskFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(taskFile, []byte("task evidence"), 0o644); err != nil {
		t.Fatal(err)
	}
	original := stageRename
	t.Cleanup(func() { stageRename = original })
	stageRename = func(_, _ string) error { return errors.New("injected rename failure") }

	stage, err := stageTaskArchiveFiles(dataDir, 1, "42", 73)
	if err != nil {
		t.Fatalf("staging should fall back to copy when rename fails: %v", err)
	}
	staged := filepath.Join(stage.payload, "files", "tasks", "42", "uploads", "evidence.txt")
	if got, err := os.ReadFile(staged); err != nil || string(got) != "task evidence" {
		t.Fatalf("staged copy=%q err=%v", got, err)
	}
	raw, err := os.ReadFile(filepath.Join(stage.root, "journal.json"))
	if err != nil {
		t.Fatal(err)
	}
	var journal archiveStageJournal
	if err := json.Unmarshal(raw, &journal); err != nil {
		t.Fatal(err)
	}
	copied := false
	for _, m := range journal.Moves {
		if m.Copied {
			copied = true
		}
	}
	if !copied {
		t.Fatalf("journal did not record the copy fallback: %+v", journal.Moves)
	}
	// Rollback restores the live tree from the authoritative copy.
	recovered := &taskArchiveFileStage{root: stage.root, payload: stage.payload, journal: journal}
	if err := recovered.rollback(); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(taskFile); err != nil || string(got) != "task evidence" {
		t.Fatalf("rollback restore=%q err=%v", got, err)
	}
}

// A copied move whose source removal failed (still present in the live tree)
// must roll back copy-authoritatively: the source may have been partially
// removed after the copy, so it is replaced by the payload copy.
func TestTaskArchiveStageRollbackCopiedMoveReplacesSource(t *testing.T) {
	dataDir := t.TempDir()
	stage := &taskArchiveFileStage{
		root:    filepath.Join(taskArchiveRoot(dataDir), ".staging", "7"),
		payload: filepath.Join(taskArchiveRoot(dataDir), ".staging", "7", "payload"),
		journal: archiveStageJournal{
			ArchiveID: 7,
			TaskID:    "42",
			Moves: []archiveFileMove{{
				Source:   filepath.Join(dataDir, "tasks", "42"),
				Relative: filepath.Join("files", "tasks", "42"),
				Copied:   true,
			}},
		},
	}
	stagedFile := filepath.Join(stage.payload, "files", "tasks", "42", "evidence.txt")
	if err := os.MkdirAll(filepath.Dir(stagedFile), archiveDirMode); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stagedFile, []byte("copy"), archiveFileMode); err != nil {
		t.Fatal(err)
	}
	liveFile := filepath.Join(dataDir, "tasks", "42", "evidence.txt")
	if err := os.MkdirAll(filepath.Dir(liveFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(liveFile, []byte("live"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := stage.rollback(); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(liveFile); err != nil || string(got) != "copy" {
		t.Fatalf("rollback should restore the authoritative copy, got=%q err=%v", got, err)
	}
	if _, err := os.Stat(stagedFile); !os.IsNotExist(err) {
		t.Fatalf("staged copy remains after rollback: %v", err)
	}
	if _, err := os.Stat(stage.root); !os.IsNotExist(err) {
		t.Fatalf("staging root remains after rollback: %v", err)
	}
}

// Resuming a staged copy leaves the live source alone and discards stale
// .copying leftovers from an interrupted fallback.
func TestTaskArchiveStageResumeSkipsCopiedMoveAndCleansPartial(t *testing.T) {
	dataDir := t.TempDir()
	stage := &taskArchiveFileStage{
		root:    filepath.Join(taskArchiveRoot(dataDir), ".staging", "7"),
		payload: filepath.Join(taskArchiveRoot(dataDir), ".staging", "7", "payload"),
		journal: archiveStageJournal{
			ArchiveID: 7,
			TaskID:    "42",
			Moves: []archiveFileMove{{
				Source:   filepath.Join(dataDir, "tasks", "42"),
				Relative: filepath.Join("files", "tasks", "42"),
				Copied:   true,
			}},
		},
	}
	stagedFile := filepath.Join(stage.payload, "files", "tasks", "42", "evidence.txt")
	if err := os.MkdirAll(filepath.Dir(stagedFile), archiveDirMode); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stagedFile, []byte("copy"), archiveFileMode); err != nil {
		t.Fatal(err)
	}
	liveFile := filepath.Join(dataDir, "tasks", "42", "evidence.txt")
	if err := os.MkdirAll(filepath.Dir(liveFile), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(liveFile, []byte("live"), 0o600); err != nil {
		t.Fatal(err)
	}
	partial := filepath.Join(stage.payload, "files", "tasks", "42") + ".copying"
	if err := os.MkdirAll(partial, archiveDirMode); err != nil {
		t.Fatal(err)
	}
	if err := stage.applyMoves(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(partial); !os.IsNotExist(err) {
		t.Fatalf("partial copy dir not cleaned on resume: %v", err)
	}
	if got, err := os.ReadFile(liveFile); err != nil || string(got) != "live" {
		t.Fatalf("resume touched the live source: got=%q err=%v", got, err)
	}
	if got, err := os.ReadFile(stagedFile); err != nil || string(got) != "copy" {
		t.Fatalf("staged copy=%q err=%v", got, err)
	}
}

func TestTaskArchiveStageJournalRollsBackInterruptedMoves(t *testing.T) {
	dataDir := t.TempDir()
	taskFile := filepath.Join(dataDir, "tasks", "42", "main", "evidence.txt")
	transcriptFile := filepath.Join(dataDir, "transcripts", "exp73-worker.jsonl")
	for _, path := range []string{taskFile, transcriptFile} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(path), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	stage, err := stageTaskArchiveFiles(dataDir, 9, "42", 73)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(stage.root, "journal.json"))
	if err != nil {
		t.Fatal(err)
	}
	var journal archiveStageJournal
	if err := json.Unmarshal(raw, &journal); err != nil {
		t.Fatal(err)
	}
	if len(journal.Moves) != 2 {
		t.Fatalf("journal moves=%d, want 2", len(journal.Moves))
	}
	recovered := &taskArchiveFileStage{root: stage.root, payload: stage.payload, journal: journal}
	if err := recovered.rollback(); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{taskFile, transcriptFile} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("interrupted archive did not restore %s: %v", path, err)
		}
	}
}

func TestTaskArchiveRestoreJournalRollsBackInterruptedInstall(t *testing.T) {
	dataDir := t.TempDir()
	extracted := filepath.Join(dataDir, "archives", "tasks", ".restore", "11-test")
	source := filepath.Join(extracted, "files", "tasks", "42", "main", "evidence.txt")
	if err := os.MkdirAll(filepath.Dir(source), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("evidence"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := installTaskArchiveFiles(dataDir, extracted, "42", 11, ExecPrivilege{})
	if err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(dataDir, "tasks", "42", "main", "evidence.txt")
	if _, err := os.Stat(destination); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(extracted, "restore-journal.json"))
	if err != nil {
		t.Fatal(err)
	}
	var journal archiveRestoreJournal
	if err := json.Unmarshal(raw, &journal); err != nil {
		t.Fatal(err)
	}
	if journal.ArchiveID != 11 || len(journal.Moves) != 1 {
		t.Fatalf("unexpected restore journal: %+v", journal)
	}
	recovered := &taskArchiveRestoreFiles{extracted: extracted, moves: journal.Moves}
	if err := recovered.rollback(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(destination); !os.IsNotExist(err) {
		t.Fatalf("hot destination remains after restore rollback: %v", err)
	}
	if got, err := os.ReadFile(source); err != nil || string(got) != "evidence" {
		t.Fatalf("restore source=%q err=%v", got, err)
	}
}

func TestTaskArchiveDeletePackageCanResumeFromStagedPath(t *testing.T) {
	dataDir := t.TempDir()
	archivePath := taskArchivePath(dataDir, 21, "42")
	if err := os.MkdirAll(filepath.Dir(archivePath), archiveDirMode); err != nil {
		t.Fatal(err)
	}
	staged := archivePath + ".deleting-21"
	if err := os.WriteFile(staged, []byte("archive"), archiveFileMode); err != nil {
		t.Fatal(err)
	}
	got, moved, err := stageTaskArchivePackageDelete(archivePath, 21)
	if err != nil || !moved || got != staged {
		t.Fatalf("resume staged package path=%q moved=%v err=%v", got, moved, err)
	}
}

func TestTaskArchivePackageRejectsTraversal(t *testing.T) {
	var compressed bytes.Buffer
	encoder, err := zstd.NewWriter(&compressed)
	if err != nil {
		t.Fatal(err)
	}
	tw := tar.NewWriter(encoder)
	content := []byte("escape")
	if err := tw.WriteHeader(&tar.Header{Name: "../escape.txt", Mode: 0o600, Size: int64(len(content))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := encoder.Close(); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "malicious.tar.zst")
	if err := os.WriteFile(path, compressed.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := extractTaskArchivePackage(path, "", root); err == nil {
		t.Fatal("path traversal archive was accepted")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(root), "escape.txt")); !os.IsNotExist(err) {
		t.Fatalf("traversal wrote outside destination: %v", err)
	}
}
