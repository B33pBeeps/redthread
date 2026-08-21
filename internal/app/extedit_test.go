package app

// extedit_test.go — opening a note in $EDITOR.
//
// The round-trip is the whole feature: whatever the user's editor leaves
// in the temp file has to come back as the same title/body split the
// built-in editor uses, and a bad edit must never destroy the note.

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func TestResolveEditorPrefersVISUAL(t *testing.T) {
	t.Setenv("VISUAL", "myvisual")
	t.Setenv("EDITOR", "myeditor")
	if got, _ := resolveEditor(); got != "myvisual" {
		t.Errorf("resolveEditor() = %q; want VISUAL to win", got)
	}
}

func TestResolveEditorFallsBackToEDITOR(t *testing.T) {
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "myeditor")
	if got, _ := resolveEditor(); got != "myeditor" {
		t.Errorf("resolveEditor() = %q; want %q", got, "myeditor")
	}
}

// With neither set we must say so rather than guessing at vi and dumping
// the user into a program they may not know how to exit.
func TestResolveEditorReportsWhenUnset(t *testing.T) {
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "")
	if _, ok := resolveEditor(); ok {
		t.Error("resolveEditor() reported success with neither VISUAL nor EDITOR set")
	}
}

// $EDITOR is a command line, not a bare path: "code -w" and "emacsclient
// -nw" are both normal values.
func TestResolveEditorSplitsArguments(t *testing.T) {
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "code -w --new-window")
	name, args := editorCommand("/tmp/note.md")
	if name != "code" {
		t.Errorf("command name = %q; want %q", name, "code")
	}
	want := []string{"-w", "--new-window", "/tmp/note.md"}
	if len(args) != len(want) {
		t.Fatalf("args = %v; want %v", args, want)
	}
	for i := range want {
		if args[i] != want[i] {
			t.Fatalf("args = %v; want %v", args, want)
		}
	}
}

func TestComposeAndParseRoundTrip(t *testing.T) {
	for _, tc := range []struct{ title, body string }{
		{"welcome", "drag with the mouse"},
		{"just a title", ""},
		{"", "body only"},
		{"", ""},
		{"multi", "line one\nline two\n\nline four"},
		{"trailing", "body with trailing newline\n"},
		{"unicode ✻", "emoji 🎉 and — dashes"},
	} {
		gotTitle, gotBody := parseNoteFile(composeNoteFile(tc.title, tc.body))
		if gotTitle != tc.title {
			t.Errorf("title round-trip: got %q, want %q (body %q)", gotTitle, tc.title, tc.body)
		}
		if strings.TrimRight(gotBody, "\n") != strings.TrimRight(tc.body, "\n") {
			t.Errorf("body round-trip: got %q, want %q (title %q)", gotBody, tc.body, tc.title)
		}
	}
}

// The external file must split the same way the built-in editor does, or
// the same text means different things in the two editors.
func TestParseMatchesTheBuiltInEditorConvention(t *testing.T) {
	for _, raw := range []string{
		"title\n\nbody",
		"title\nbody",
		"title",
		"",
		"  spaced title  \n\nbody",
		"\n\nbody with no title",
	} {
		wantTitle, wantBody := splitClipboardText(raw)
		gotTitle, gotBody := parseNoteFile(raw)
		if gotTitle != wantTitle || gotBody != wantBody {
			t.Errorf("parseNoteFile(%q) = (%q, %q); built-in split gives (%q, %q)",
				raw, gotTitle, gotBody, wantTitle, wantBody)
		}
	}
}

func TestWriteNoteTempFile(t *testing.T) {
	n := &Note{ID: "abc123", Title: "welcome", Body: "drag with the mouse"}
	path, cleanup, err := writeNoteTempFile(n)
	if err != nil {
		t.Fatalf("writeNoteTempFile: %v", err)
	}
	defer cleanup()

	if filepath.Ext(path) != ".md" {
		t.Errorf("temp file %q does not end in .md; editors key syntax highlighting off it", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	title, body := parseNoteFile(string(data))
	if title != n.Title || strings.TrimRight(body, "\n") != n.Body {
		t.Errorf("temp file holds (%q, %q); want (%q, %q)", title, body, n.Title, n.Body)
	}

	cleanup()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("cleanup did not remove %s", path)
	}
}

// Cleanup must tolerate being called twice — the deferred call and the
// explicit one after a successful read can both fire.
func TestTempFileCleanupIsIdempotent(t *testing.T) {
	n := &Note{ID: "x", Title: "t", Body: "b"}
	path, cleanup, err := writeNoteTempFile(n)
	if err != nil {
		t.Fatal(err)
	}
	cleanup()
	cleanup() // must not panic
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("file still present after cleanup")
	}
}

// --- integration ------------------------------------------------------

func newExtEditTestModel(t *testing.T) model {
	t.Helper()
	ws := seedWorkspace()
	m := initialModel(ws)
	m.w, m.h = 120, 40
	m.now = time.Now()
	m.stars = GenStarsForBoard(m.w, m.h, ws.ActiveBoard().GrainSeed)
	return m
}

func pressExtKey(t *testing.T, m model, key string) model {
	t.Helper()
	next, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
	nm, ok := next.(model)
	if !ok {
		t.Fatalf("Update returned %T, not a model", next)
	}
	return nm
}

// applyExternalEdit is where the note actually changes, so these drive it
// with real files on disk rather than stubbing the filesystem.

func editResult(t *testing.T, contents string, editorErr error) (*Note, string, bool) {
	t.Helper()
	n := &Note{ID: "n1", Title: "welcome", Body: "drag with the mouse"}
	path := filepath.Join(t.TempDir(), "note.md")
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatal(err)
	}
	toast, changed := applyExternalEdit(n, externalEditDoneMsg{
		noteID: n.ID, path: path, cleanup: func() {}, err: editorErr,
	})
	return n, toast, changed
}

func TestExternalEditAppliesChanges(t *testing.T) {
	n, _, changed := editResult(t, "new title\n\nnew body\n", nil)
	if !changed {
		t.Fatal("edit reported no change")
	}
	if n.Title != "new title" || n.Body != "new body" {
		t.Errorf("note = (%q, %q); want (%q, %q)", n.Title, n.Body, "new title", "new body")
	}
}

// Saving without editing must not mark the note dirty — otherwise every
// look at a note bumps its timestamp and burns an undo slot.
func TestExternalEditUnchangedFileIsANoOp(t *testing.T) {
	n, toast, changed := editResult(t, "welcome\n\ndrag with the mouse\n", nil)
	if changed {
		t.Errorf("an unchanged file reported a change (toast %q)", toast)
	}
	if n.Title != "welcome" || n.Body != "drag with the mouse" {
		t.Errorf("note was modified: (%q, %q)", n.Title, n.Body)
	}
}

// `:cq` in vim, or any non-zero exit, means "discard".
func TestExternalEditKeepsNoteWhenEditorFails(t *testing.T) {
	n, toast, changed := editResult(t, "clobbered\n\nclobbered\n", os.ErrPermission)
	if changed {
		t.Error("a failed editor still changed the note")
	}
	if n.Title != "welcome" {
		t.Errorf("note title = %q; want it untouched", n.Title)
	}
	if !strings.Contains(toast, "unchanged") {
		t.Errorf("toast = %q; want it to say the note is unchanged", toast)
	}
}

// Emptying the file is almost always an accident (a stray :q! on the wrong
// buffer, or an editor that truncates on open). Refuse rather than blank
// the note.
func TestExternalEditRefusesAnEmptyFile(t *testing.T) {
	for _, contents := range []string{"", "\n", "   \n\n  \n"} {
		n, toast, changed := editResult(t, contents, nil)
		if changed {
			t.Errorf("empty file %q blanked the note", contents)
		}
		if n.Title != "welcome" {
			t.Errorf("note title = %q after empty file %q; want it untouched", n.Title, contents)
		}
		if !strings.Contains(toast, "empty") {
			t.Errorf("toast = %q; want it to mention the empty file", toast)
		}
	}
}

// A missing temp file (deleted by the editor, or a crash between write and
// read) must not blank the note either.
func TestExternalEditSurvivesAMissingFile(t *testing.T) {
	n := &Note{ID: "n1", Title: "welcome", Body: "keep me"}
	toast, changed := applyExternalEdit(n, externalEditDoneMsg{
		noteID: n.ID,
		path:   filepath.Join(t.TempDir(), "does-not-exist.md"),
		err:    nil,
	})
	if changed {
		t.Error("a missing file reported a change")
	}
	if n.Body != "keep me" {
		t.Errorf("note body = %q; want it untouched", n.Body)
	}
	if !strings.Contains(toast, "unchanged") {
		t.Errorf("toast = %q; want it to say the note is unchanged", toast)
	}
}

// The temp file must be removed however the edit turns out.
func TestExternalEditCleansUpTheTempFile(t *testing.T) {
	n := &Note{ID: "n1", Title: "welcome", Body: "body"}
	path, cleanup, err := writeNoteTempFile(n)
	if err != nil {
		t.Fatal(err)
	}
	applyExternalEdit(n, externalEditDoneMsg{noteID: n.ID, path: path, cleanup: cleanup, err: nil})
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("temp file %s survived the edit", path)
	}
}

func TestOpenInEditorReportsMissingEDITOR(t *testing.T) {
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "")
	cmd, why := openInEditor(&Note{ID: "n1", Title: "t"})
	if cmd != nil {
		t.Error("openInEditor returned a command with no $EDITOR set")
	}
	if !strings.Contains(why, "EDITOR") {
		t.Errorf("reason = %q; want it to name $EDITOR", why)
	}
}

func TestOpenInEditorNeedsASelection(t *testing.T) {
	t.Setenv("EDITOR", "true")
	cmd, why := openInEditor(nil)
	if cmd != nil {
		t.Error("openInEditor returned a command for a nil note")
	}
	if why == "" {
		t.Error("openInEditor gave no reason for refusing")
	}
}

// Pressing `e` with no $EDITOR must say so rather than doing nothing.
func TestPressingEWithoutEDITORToasts(t *testing.T) {
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", "")
	m := newExtEditTestModel(t)
	m = pressExtKey(t, m, "e")
	if !strings.Contains(m.toast, "EDITOR") {
		t.Errorf("toast = %q; want it to mention $EDITOR", m.toast)
	}
}

// A real subprocess, end to end: this is the only test that proves the
// command we hand to tea.ExecProcess actually runs the user's editor
// against our temp file and that we read back what it wrote.
func TestRealEditorProcessRoundTrip(t *testing.T) {
	script := filepath.Join(t.TempDir(), "fake-editor.sh")
	body := "#!/bin/sh\nprintf 'edited by the editor\\n\\nnew body line\\n' > \"$1\"\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", script)

	n := &Note{ID: "n1", Title: "welcome", Body: "drag with the mouse"}

	// Mirror what openInEditor builds, then run it directly — ExecProcess
	// itself needs a live Program, but the command it wraps is ours.
	path, cleanup, err := writeNoteTempFile(n)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	name, args := editorCommand(path)
	if name != script {
		t.Fatalf("command = %q; want the script at %q", name, script)
	}
	if out, err := exec.Command(name, args...).CombinedOutput(); err != nil {
		t.Fatalf("fake editor failed: %v\n%s", err, out)
	}

	toast, changed := applyExternalEdit(n, externalEditDoneMsg{
		noteID: n.ID, path: path, cleanup: func() {}, err: nil,
	})
	if !changed {
		t.Fatalf("edit reported no change (toast %q)", toast)
	}
	if n.Title != "edited by the editor" {
		t.Errorf("title = %q; want %q", n.Title, "edited by the editor")
	}
	if n.Body != "new body line" {
		t.Errorf("body = %q; want %q", n.Body, "new body line")
	}
}

// The same path with an editor that exits non-zero without writing.
func TestRealEditorFailureLeavesTheNote(t *testing.T) {
	script := filepath.Join(t.TempDir(), "failing-editor.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("VISUAL", "")
	t.Setenv("EDITOR", script)

	n := &Note{ID: "n1", Title: "welcome", Body: "keep me"}
	path, cleanup, err := writeNoteTempFile(n)
	if err != nil {
		t.Fatal(err)
	}
	defer cleanup()

	name, args := editorCommand(path)
	runErr := exec.Command(name, args...).Run()
	if runErr == nil {
		t.Fatal("the failing editor exited 0")
	}

	_, changed := applyExternalEdit(n, externalEditDoneMsg{
		noteID: n.ID, path: path, cleanup: func() {}, err: runErr,
	})
	if changed {
		t.Error("a failed editor changed the note")
	}
	if n.Body != "keep me" {
		t.Errorf("body = %q; want it untouched", n.Body)
	}
}
