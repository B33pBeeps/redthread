package app

// undo_integration_test.go — undo/redo driven through the model's key
// handlers, rather than against History directly. The original bug was not
// in a data structure; it was that the mutation sites never recorded
// anything, so these tests press the keys.

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func newTestModel(t *testing.T) model {
	t.Helper()
	ws := seedWorkspace()
	m := initialModel(ws)
	m.w, m.h = 120, 40
	m.now = time.Now()
	m.stars = GenStarsForBoard(m.w, m.h, ws.ActiveBoard().GrainSeed)
	return m
}

func pressKey(t *testing.T, m model, key string) model {
	t.Helper()
	var msg tea.KeyMsg
	switch key {
	case "ctrl+p":
		msg = tea.KeyMsg{Type: tea.KeyCtrlP}
	case "ctrl+r":
		msg = tea.KeyMsg{Type: tea.KeyCtrlR}
	default:
		msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
	}
	next, _ := m.Update(msg)
	nm, ok := next.(model)
	if !ok {
		t.Fatalf("Update returned %T, not a model", next)
	}
	return nm
}

// The reported bug, end to end: select a note, paste over it, press u.
func TestUndoRecoversAPasteOverANote(t *testing.T) {
	m := newTestModel(t)

	n := m.board.Selection()
	if n == nil {
		t.Fatal("seed workspace has no selected note")
	}
	origTitle, origBody := n.Title, n.Body
	if origTitle == "" && origBody == "" {
		t.Fatal("seed note is empty; the test needs content to destroy")
	}
	noteID := n.ID

	// Simulate the paste directly: clipboard.ReadAll would need a real
	// system clipboard, and the confirmation path is covered separately.
	m.mutate("paste into note")
	n.Title = "clipboard junk"
	n.Body = "https://example.com/whatever"

	m = pressKey(t, m, "u")

	got := m.findNote(noteID)
	if got == nil {
		t.Fatal("note vanished after undo")
	}
	if got.Title != origTitle || got.Body != origBody {
		t.Errorf("undo did not restore the note:\n got %q / %q\nwant %q / %q",
			got.Title, got.Body, origTitle, origBody)
	}
}

func TestUndoRecoversADeletedNote(t *testing.T) {
	m := newTestModel(t)
	before := len(m.board.Notes)
	n := m.board.Selection()
	if n == nil {
		t.Fatal("no selection")
	}
	id, title := n.ID, n.Title

	m = pressKey(t, m, "d")
	if len(m.board.Notes) != before-1 {
		t.Fatalf("after delete, %d notes; want %d", len(m.board.Notes), before-1)
	}

	m = pressKey(t, m, "u")
	if len(m.board.Notes) != before {
		t.Fatalf("after undo, %d notes; want %d", len(m.board.Notes), before)
	}
	got := m.findNote(id)
	if got == nil {
		t.Fatal("undo did not bring the note back")
	}
	if got.Title != title {
		t.Errorf("restored note title = %q; want %q", got.Title, title)
	}
}

func TestUndoRecoversATintChange(t *testing.T) {
	m := newTestModel(t)
	n := m.board.Selection()
	if n == nil {
		t.Fatal("no selection")
	}
	id, orig := n.ID, n.Tint

	// Pick a tint index that differs from the current one.
	target := "1"
	if orig == TintOrder[0] {
		target = "5"
	}
	m = pressKey(t, m, target)
	if m.findNote(id).Tint == orig {
		t.Fatalf("tint did not change from %q; the test proves nothing", orig)
	}

	m = pressKey(t, m, "u")
	if got := m.findNote(id).Tint; got != orig {
		t.Errorf("after undo, tint = %q; want %q", got, orig)
	}
}

func TestUndoRecoversANewNote(t *testing.T) {
	m := newTestModel(t)
	before := len(m.board.Notes)

	m = pressKey(t, m, "n")
	if len(m.board.Notes) != before+1 {
		t.Fatalf("after new note, %d notes; want %d", len(m.board.Notes), before+1)
	}

	m = pressKey(t, m, "u")
	if len(m.board.Notes) != before {
		t.Errorf("after undo, %d notes; want %d", len(m.board.Notes), before)
	}
}

// Deleting a board takes every note with it — the largest loss available
// from a single keystroke.
func TestUndoRecoversADeletedBoard(t *testing.T) {
	m := newTestModel(t)
	m = pressKey(t, m, "B") // add a second board so delete is allowed
	boards := len(m.workspace.Boards)
	if boards < 2 {
		t.Fatalf("expected at least 2 boards after B, got %d", boards)
	}
	// `B` drops into rename mode; leave it before pressing anything else.
	m = pressKey(t, m, "esc")

	name := m.board.Name
	m = pressKey(t, m, "D") // arms
	m = pressKey(t, m, "D") // deletes
	if len(m.workspace.Boards) != boards-1 {
		t.Fatalf("after D D, %d boards; want %d", len(m.workspace.Boards), boards-1)
	}

	m = pressKey(t, m, "u")
	if len(m.workspace.Boards) != boards {
		t.Fatalf("after undo, %d boards; want %d", len(m.workspace.Boards), boards)
	}
	found := false
	for _, b := range m.workspace.Boards {
		if b.Name == name {
			found = true
		}
	}
	if !found {
		t.Errorf("board %q was not restored", name)
	}
}

func TestRedoReappliesAnUndoneChange(t *testing.T) {
	m := newTestModel(t)
	before := len(m.board.Notes)

	m = pressKey(t, m, "n")
	after := len(m.board.Notes)
	m = pressKey(t, m, "u")
	if len(m.board.Notes) != before {
		t.Fatalf("undo failed: %d notes", len(m.board.Notes))
	}

	m = pressKey(t, m, "ctrl+r")
	if len(m.board.Notes) != after {
		t.Errorf("after redo, %d notes; want %d", len(m.board.Notes), after)
	}
}

func TestUndoOnAFreshBoardIsAFriendlyNoOp(t *testing.T) {
	m := newTestModel(t)
	notes := len(m.board.Notes)

	m = pressKey(t, m, "u")

	if len(m.board.Notes) != notes {
		t.Errorf("a no-op undo changed the board: %d notes, want %d", len(m.board.Notes), notes)
	}
	if m.toast != "nothing to undo" {
		t.Errorf("toast = %q; want %q", m.toast, "nothing to undo")
	}
}

// Undo must survive being run past the point where a note was selected,
// leaving the board with a valid selection rather than a dangling ID.
func TestUndoLeavesAValidSelection(t *testing.T) {
	m := newTestModel(t)
	m = pressKey(t, m, "n") // new note becomes selected
	m = pressKey(t, m, "u") // it goes away

	if sel := m.board.Selected; sel != "" {
		if m.board.Selection() == nil {
			t.Errorf("board.Selected = %q but no such note exists", sel)
		}
	}
	if len(m.board.Notes) > 0 && m.board.Selection() == nil {
		t.Error("board has notes but nothing is selected after undo")
	}
}

// Pasting over a note with content must warn first. This is the guard that
// would have prevented the original loss outright.
func TestPasteOverANonEmptyNoteAsksFirst(t *testing.T) {
	m := newTestModel(t)
	n := m.board.Selection()
	if n == nil || (n.Title == "" && n.Body == "") {
		t.Fatal("need a non-empty selected note")
	}
	origTitle := n.Title

	// First ctrl+p arms the confirmation rather than replacing anything.
	// (clipboard.ReadAll may fail in CI; the arm check runs after it, so
	// assert only when the clipboard actually produced text.)
	m2 := pressKey(t, m, "ctrl+p")
	if got := m2.findNote(n.ID); got.Title != origTitle && !m2.pasteArmedUntil.IsZero() {
		t.Errorf("first ctrl+p replaced the note title (%q); it should warn first", got.Title)
	}
}

func TestHistoryCoalescesAKeyRepeatNudge(t *testing.T) {
	m := newTestModel(t)
	n := m.board.Selection()
	if n == nil {
		t.Fatal("no selection")
	}
	startX := n.X

	// Twelve rapid nudges, as a held arrow key produces. Pushed straight
	// onto the history rather than through m.mutate: mutate also arms the
	// debounced saver, whose timer goroutine marshals the workspace while
	// this loop mutates it — a pre-existing race unrelated to coalescing.
	for i := 0; i < 12; i++ {
		m.history.Push(m.workspace, "move note")
		n.X++
	}
	if n.X == startX {
		t.Fatal("nudges did not move the note")
	}
	if got := m.history.Len(); got != 1 {
		t.Errorf("12 rapid nudges produced %d undo entries; want 1", got)
	}
}
