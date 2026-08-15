package app

// backup_test.go — on-disk backup rotation.
//
// Undo protects a running session. Backups protect everything undo cannot:
// a crash, a bad paste noticed tomorrow, a file corrupted by something
// outside the app. Before this, a 400ms debounce overwrote the single
// notes.json and the previous content was simply gone.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readBackupDir(t *testing.T) []string {
	t.Helper()
	dir, err := BackupDir()
	if err != nil {
		t.Fatalf("BackupDir: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatalf("ReadDir(%s): %v", dir, err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

func TestSaveCreatesABackupOfThePreviousContent(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	ws := seedWorkspace()
	ws.Boards[0].Notes[0].Title = "original"
	if err := SaveWorkspace(ws); err != nil {
		t.Fatalf("first save: %v", err)
	}
	// The first save has no prior content to back up.
	if got := readBackupDir(t); len(got) != 0 {
		t.Errorf("first save produced backups %v; want none", got)
	}

	ws.Boards[0].Notes[0].Title = "overwritten"
	if err := SaveWorkspace(ws); err != nil {
		t.Fatalf("second save: %v", err)
	}

	names := readBackupDir(t)
	if len(names) == 0 {
		t.Fatal("second save produced no backup of the previous content")
	}

	// The backup must hold the ORIGINAL text, not the new one.
	dir, _ := BackupDir()
	data, err := os.ReadFile(filepath.Join(dir, names[0]))
	if err != nil {
		t.Fatalf("reading backup: %v", err)
	}
	if !strings.Contains(string(data), "original") {
		t.Errorf("backup does not contain the pre-save content:\n%s", data)
	}
	if strings.Contains(string(data), "overwritten") {
		t.Error("backup contains the post-save content; it was taken too late")
	}
}

// A backup is only useful if it loads.
func TestABackupIsAValidWorkspaceFile(t *testing.T) {
	home := t.TempDir()
	t.Setenv("XDG_DATA_HOME", home)

	ws := seedWorkspace()
	ws.Boards[0].Notes[0].Title = "the note I want back"
	if err := SaveWorkspace(ws); err != nil {
		t.Fatal(err)
	}
	ws.Boards[0].Notes[0].Title = "clobbered"
	if err := SaveWorkspace(ws); err != nil {
		t.Fatal(err)
	}

	names := readBackupDir(t)
	if len(names) == 0 {
		t.Fatal("no backup to restore")
	}
	dir, _ := BackupDir()
	data, err := os.ReadFile(filepath.Join(dir, names[0]))
	if err != nil {
		t.Fatal(err)
	}

	// Restore it the way a user would: copy it over notes.json.
	path, _ := DataPath()
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	restored, err := LoadWorkspace()
	if err != nil {
		t.Fatalf("restored backup does not load: %v", err)
	}
	if restored == nil || len(restored.Boards) == 0 {
		t.Fatal("restored backup produced an empty workspace")
	}
	if got := restored.Boards[0].Notes[0].Title; got != "the note I want back" {
		t.Errorf("restored title = %q; want the pre-clobber value", got)
	}
}

func TestBackupRotationIsBounded(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	ws := seedWorkspace()
	for i := 0; i < backupKeep*3; i++ {
		ws.Boards[0].Notes[0].Body = strings.Repeat("x", i+1) // ensure content changes
		if err := SaveWorkspace(ws); err != nil {
			t.Fatalf("save %d: %v", i, err)
		}
	}

	names := readBackupDir(t)
	if len(names) > backupKeep {
		t.Errorf("backup dir holds %d files; want at most %d", len(names), backupKeep)
	}
	if len(names) == 0 {
		t.Error("repeated saves produced no backups at all")
	}
}

// Saving identical content repeatedly (the debounce fires on idle ticks,
// board switches, and so on) must not churn the ring and evict real
// history behind unchanged copies.
func TestUnchangedSaveDoesNotRotate(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	ws := seedWorkspace()
	if err := SaveWorkspace(ws); err != nil {
		t.Fatal(err)
	}
	ws.Boards[0].Notes[0].Title = "a real change"
	if err := SaveWorkspace(ws); err != nil {
		t.Fatal(err)
	}
	afterRealChange := len(readBackupDir(t))

	for i := 0; i < 5; i++ {
		if err := SaveWorkspace(ws); err != nil {
			t.Fatal(err)
		}
	}
	if got := len(readBackupDir(t)); got != afterRealChange {
		t.Errorf("five no-op saves changed the backup count from %d to %d",
			afterRealChange, got)
	}
}

// Backups live beside the save file but must never be mistaken for it.
func TestBackupsDoNotDisturbTheMainSaveFile(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	ws := seedWorkspace()
	ws.Boards[0].Notes[0].Title = "current"
	if err := SaveWorkspace(ws); err != nil {
		t.Fatal(err)
	}
	ws.Boards[0].Notes[0].Title = "newer"
	if err := SaveWorkspace(ws); err != nil {
		t.Fatal(err)
	}

	got, err := LoadWorkspace()
	if err != nil {
		t.Fatalf("LoadWorkspace after backups: %v", err)
	}
	if got.Boards[0].Notes[0].Title != "newer" {
		t.Errorf("main file holds %q; want the most recent save",
			got.Boards[0].Notes[0].Title)
	}

	path, _ := DataPath()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var f diskFile
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatalf("main save file is not valid JSON: %v", err)
	}
	if f.SchemaVersion != schemaVersion {
		t.Errorf("schemaVersion = %d; want %d", f.SchemaVersion, schemaVersion)
	}
}
