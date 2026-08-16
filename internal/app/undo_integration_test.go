package app

// undo_integration_test.go — undo/redo driven through the model's key
// handlers, rather than against History directly. The original bug was not
// in a data structure; it was that the mutation sites never recorded
// anything, so these tests press the keys.

import (
	"strings"
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

// stubClipboard makes readClipboard return fixed text for one test.
func stubClipboard(t *testing.T, text string) {
	t.Helper()
	prev := readClipboard
	readClipboard = func() (string, error) { return text, nil }
	t.Cleanup(func() { readClipboard = prev })
}

// Pasting over a note with content must warn first. This is the guard that
// would have prevented the original loss outright.
func TestPasteOverANonEmptyNoteAsksFirst(t *testing.T) {
	stubClipboard(t, "clipboard junk\n\nsome url")

	m := newTestModel(t)
	n := m.board.Selection()
	if n == nil || (n.Title == "" && n.Body == "") {
		t.Fatal("need a non-empty selected note")
	}
	origTitle, origBody := n.Title, n.Body

	// The first ctrl+p must arm the confirmation and change nothing.
	m = pressKey(t, m, "ctrl+p")
	got := m.findNote(n.ID)
	if got.Title != origTitle || got.Body != origBody {
		t.Fatalf("first ctrl+p replaced the note (%q / %q); it must warn first",
			got.Title, got.Body)
	}
	if m.pasteArmedUntil.IsZero() {
		t.Error("first ctrl+p did not arm the confirmation window")
	}
	if !strings.Contains(m.toast, "again") {
		t.Errorf("toast = %q; want a confirmation prompt", m.toast)
	}
}

// ...and the second press within the window goes through.
func TestSecondPasteReplacesAndIsUndoable(t *testing.T) {
	stubClipboard(t, "clipboard junk\n\nsome url")

	m := newTestModel(t)
	n := m.board.Selection()
	if n == nil || (n.Title == "" && n.Body == "") {
		t.Fatal("need a non-empty selected note")
	}
	id, origTitle, origBody := n.ID, n.Title, n.Body

	m = pressKey(t, m, "ctrl+p") // arms
	m = pressKey(t, m, "ctrl+p") // commits

	got := m.findNote(id)
	if got.Title != "clipboard junk" {
		t.Fatalf("second ctrl+p did not paste: title = %q", got.Title)
	}

	m = pressKey(t, m, "u")
	got = m.findNote(id)
	if got.Title != origTitle || got.Body != origBody {
		t.Errorf("undo after paste gave %q / %q; want %q / %q",
			got.Title, got.Body, origTitle, origBody)
	}
}

// An empty note has nothing to lose, so pasting into it should not nag.
func TestPasteIntoAnEmptyNoteDoesNotAsk(t *testing.T) {
	stubClipboard(t, "fresh content")

	m := newTestModel(t)
	m = pressKey(t, m, "n") // new notes start empty
	n := m.board.Selection()
	if n == nil {
		t.Fatal("no selection after creating a note")
	}
	if n.Title != "" || n.Body != "" {
		t.Skipf("new note is not empty (%q / %q)", n.Title, n.Body)
	}

	m = pressKey(t, m, "ctrl+p")
	if got := m.findNote(n.ID); got.Title != "fresh content" {
		t.Errorf("paste into an empty note gave %q; want it to apply immediately", got.Title)
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

// Reordering boards with `{` / `}` must be undoable. The snapshot has to
// happen before MoveActive runs, since it rearranges the slice in place.
func TestUndoRestoresBoardOrder(t *testing.T) {
	m := newTestModel(t)
	// Two more boards so a reorder is observable.
	m = pressKey(t, m, "B")
	m = pressKey(t, m, "esc")
	m = pressKey(t, m, "B")
	m = pressKey(t, m, "esc")
	if len(m.workspace.Boards) < 3 {
		t.Fatalf("need 3 boards, got %d", len(m.workspace.Boards))
	}

	order := func(m model) []string {
		var out []string
		for _, b := range m.workspace.Boards {
			out = append(out, b.Name)
		}
		return out
	}
	before := order(m)

	m = pressKey(t, m, "{")
	after := order(m)
	if equalStrings(before, after) {
		t.Fatalf("`{` did not reorder the boards: %v", after)
	}

	m = pressKey(t, m, "u")
	if got := order(m); !equalStrings(got, before) {
		t.Errorf("after undo, board order = %v; want %v", got, before)
	}
}

// A no-op reorder (a single board, or a wrap onto itself) must not
// consume an undo slot — otherwise `{` on a one-board workspace silently
// eats the entry holding a real change.
func TestNoOpBoardMoveDoesNotPushHistory(t *testing.T) {
	m := newTestModel(t)
	if len(m.workspace.Boards) != 1 {
		t.Skipf("seed has %d boards; this test wants 1", len(m.workspace.Boards))
	}
	before := m.history.Len()
	m = pressKey(t, m, "{")
	m = pressKey(t, m, "}")
	if got := m.history.Len(); got != before {
		t.Errorf("no-op board moves pushed %d history entries; want 0", got-before)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// Zoom, font, highlight, and the background are view state: they do not
// push undo entries, and undoing a *content* change must not rewind them.
// The snapshot is a whole-struct copy, so these fields ride along inside
// it — applyRestoredWorkspace has to carry the live values across.
func TestUndoLeavesViewStateAlone(t *testing.T) {
	m := newTestModel(t)

	m = pressKey(t, m, "n") // content change, snapshotted at the current view

	m = pressKey(t, m, "-") // zoom out
	m = pressKey(t, m, "c") // cycle the highlight color
	zoom, highlight := m.board.Zoom, m.board.HighlightColor
	if zoom == 0 {
		t.Fatal("zoom did not change")
	}

	m = pressKey(t, m, "u") // undo the note, not the view

	if got := m.board.Zoom; got != zoom {
		t.Errorf("undo changed zoom from %d to %d", zoom, got)
	}
	if got := m.board.HighlightColor; got != highlight {
		t.Errorf("undo changed the highlight from %d to %d", highlight, got)
	}
}

// View state is carried by GrainSeed rather than name, since a rename is
// itself undoable and would break a name-keyed match.
func TestUndoRenamePreservesViewState(t *testing.T) {
	m := newTestModel(t)
	m = pressKey(t, m, "-")
	zoom := m.board.Zoom
	if zoom == 0 {
		t.Fatal("zoom did not change")
	}
	original := m.board.Name

	m = pressKey(t, m, "R")
	for i := 0; i < 32; i++ { // clear the seeded buffer
		m = pressKey(t, m, "backspace")
	}
	for _, r := range "renamed" {
		m = pressKey(t, m, string(r))
	}
	m = pressKey(t, m, "enter")
	if m.board.Name != "renamed" {
		t.Fatalf("rename did not take: %q", m.board.Name)
	}

	m = pressKey(t, m, "u")
	if m.board.Name != original {
		t.Errorf("undo gave name %q; want %q", m.board.Name, original)
	}
	if got := m.board.Zoom; got != zoom {
		t.Errorf("after undoing a rename, zoom = %d; want %d", got, zoom)
	}
}

// Changing the background must not consume an undo slot either.
func TestViewStateChangesDoNotPushHistory(t *testing.T) {
	m := newTestModel(t)
	before := m.history.Len()

	m = pressKey(t, m, "-") // zoom
	m = pressKey(t, m, "=")
	m = pressKey(t, m, "0")
	m = pressKey(t, m, "c") // highlight

	if got := m.history.Len(); got != before {
		t.Errorf("view-state changes pushed %d undo entries; want 0", got-before)
	}
}
