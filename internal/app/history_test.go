package app

// history_test.go — the undo/redo stack.
//
// The bug that motivated this: undo was a single snapshot slot written
// only by the delete path, so `ctrl+p` (paste over a note) replaced the
// title and body with no way back, and `u` reported "nothing to undo".
// Every mutation must now be undoable.

import (
	"testing"
	"time"
)

func testBoard() *Board {
	return &Board{
		Name:      "main",
		GrainSeed: 42,
		Notes: []*Note{
			{ID: "a", Title: "welcome", Body: "drag with the mouse", X: 2, Y: 1, Tint: "yellow"},
			{ID: "b", Title: "call mom", Body: "after 5pm", X: 20, Y: 8, Tint: "blue"},
		},
		Strings:  []*StringConn{{A: StringEnd{NoteID: "a"}, B: StringEnd{NoteID: "b"}}},
		Selected: "a",
	}
}

func testWorkspace() *Workspace {
	second := &Board{
		Name:      "ideas",
		GrainSeed: 7,
		Notes:     []*Note{{ID: "c", Title: "moonshots", Body: "solve navier stokes"}},
	}
	return &Workspace{
		Boards:     []*Board{testBoard(), second},
		Background: Background{Cork: true},
	}
}

func TestCloneWorkspaceIsDeep(t *testing.T) {
	orig := testWorkspace()
	clone := cloneWorkspace(orig)

	clone.Boards[0].Notes[0].Title = "clobbered"
	clone.Boards[1].Name = "renamed"
	clone.ActiveIdx = 1
	clone.Background.Cork = false

	if orig.Boards[0].Notes[0].Title != "welcome" {
		t.Errorf("mutating the clone changed the original note: %q", orig.Boards[0].Notes[0].Title)
	}
	if orig.Boards[1].Name != "ideas" {
		t.Errorf("mutating the clone changed the original board name: %q", orig.Boards[1].Name)
	}
	if orig.ActiveIdx != 0 {
		t.Errorf("mutating the clone changed the original ActiveIdx: %d", orig.ActiveIdx)
	}
	if !orig.Background.Cork {
		t.Error("mutating the clone changed the original background")
	}

	clone.Boards = append(clone.Boards, &Board{Name: "extra"})
	if len(orig.Boards) != 2 {
		t.Errorf("appending to the clone changed the original board count: %d", len(orig.Boards))
	}
}

// Deleting a whole board destroys far more than deleting a note, so it
// has to be undoable too — which means snapshotting the workspace, not
// just the active board.
func TestUndoRestoresADeletedBoard(t *testing.T) {
	h := NewHistory(10)
	ws := testWorkspace()

	h.Push(ws, "delete board")
	ws.Boards = ws.Boards[:1] // simulate DeleteBoard of the second

	restored, label, ok := h.Undo(ws)
	if !ok {
		t.Fatal("Undo reported nothing to undo after a board delete")
	}
	if label != "delete board" {
		t.Errorf("label = %q; want %q", label, "delete board")
	}
	if len(restored.Boards) != 2 {
		t.Fatalf("restored workspace has %d boards; want 2", len(restored.Boards))
	}
	if restored.Boards[1].Name != "ideas" {
		t.Errorf("restored second board = %q; want %q", restored.Boards[1].Name, "ideas")
	}
	if got := restored.Boards[1].Notes[0].Title; got != "moonshots" {
		t.Errorf("restored board lost its notes: %q", got)
	}
}

func TestCloneBoardIsDeep(t *testing.T) {
	orig := testBoard()
	clone := cloneBoard(orig)

	clone.Notes[0].Title = "clobbered"
	clone.Notes[0].Body = "clobbered"
	clone.Strings[0].Tight = true
	clone.Name = "other"

	if orig.Notes[0].Title != "welcome" {
		t.Errorf("mutating the clone changed the original note title: %q", orig.Notes[0].Title)
	}
	if orig.Notes[0].Body != "drag with the mouse" {
		t.Errorf("mutating the clone changed the original note body: %q", orig.Notes[0].Body)
	}
	if orig.Strings[0].Tight {
		t.Error("mutating the clone changed the original string")
	}
	if orig.Name != "main" {
		t.Errorf("mutating the clone changed the original board name: %q", orig.Name)
	}

	// Appending to the clone must not disturb the original's slice.
	clone.Notes = append(clone.Notes, &Note{ID: "c"})
	if len(orig.Notes) != 2 {
		t.Errorf("appending to the clone changed the original note count: %d", len(orig.Notes))
	}
}

func TestCloneBoardPreservesEverything(t *testing.T) {
	orig := testBoard()
	orig.TextMode = TextBold
	orig.Zoom = -1
	orig.HighlightColor = 3
	clone := cloneBoard(orig)

	if clone.Name != orig.Name || clone.GrainSeed != orig.GrainSeed ||
		clone.TextMode != orig.TextMode || clone.Zoom != orig.Zoom ||
		clone.HighlightColor != orig.HighlightColor || clone.Selected != orig.Selected {
		t.Errorf("clone lost board fields:\n got %+v\nwant %+v", clone, orig)
	}
	if len(clone.Notes) != len(orig.Notes) || len(clone.Strings) != len(orig.Strings) {
		t.Fatalf("clone has %d notes / %d strings; want %d / %d",
			len(clone.Notes), len(clone.Strings), len(orig.Notes), len(orig.Strings))
	}
	for i := range orig.Notes {
		if *clone.Notes[i] != *orig.Notes[i] {
			t.Errorf("note %d differs:\n got %+v\nwant %+v", i, *clone.Notes[i], *orig.Notes[i])
		}
	}
}

// The core regression: a paste that replaces a note's text must be undoable.
func TestUndoRestoresAPasteOverANote(t *testing.T) {
	h := NewHistory(10)
	ws := testWorkspace()
	b := ws.Boards[0]

	h.Push(ws, "paste into note")
	b.Notes[0].Title = "clipboard junk"
	b.Notes[0].Body = "some url"

	restoredWs, label, ok := h.Undo(ws)
	if !ok {
		t.Fatal("Undo reported nothing to undo after a paste")
	}
	if label != "paste into note" {
		t.Errorf("undo label = %q; want %q", label, "paste into note")
	}
	restored := restoredWs.Boards[0]
	if restored.Notes[0].Title != "welcome" || restored.Notes[0].Body != "drag with the mouse" {
		t.Errorf("undo did not restore the note: got %q / %q",
			restored.Notes[0].Title, restored.Notes[0].Body)
	}
}

func TestUndoRedoRoundTrip(t *testing.T) {
	h := NewHistory(10)
	ws := testWorkspace()

	h.Push(ws, "edit")
	ws.Boards[0].Notes[0].Title = "edited"

	undone, _, ok := h.Undo(ws)
	if !ok {
		t.Fatal("Undo failed")
	}
	if undone.Boards[0].Notes[0].Title != "welcome" {
		t.Fatalf("after undo, title = %q; want the original", undone.Boards[0].Notes[0].Title)
	}

	redone, label, ok := h.Redo(undone)
	if !ok {
		t.Fatal("Redo reported nothing to redo")
	}
	if label != "edit" {
		t.Errorf("redo label = %q; want %q", label, "edit")
	}
	if redone.Boards[0].Notes[0].Title != "edited" {
		t.Errorf("after redo, title = %q; want the edited value", redone.Boards[0].Notes[0].Title)
	}
}

func TestUndoWalksBackMultipleSteps(t *testing.T) {
	h := NewHistory(10)
	ws := testWorkspace()

	for _, title := range []string{"one", "two", "three"} {
		h.Push(ws, "edit "+title)
		ws.Boards[0].Notes[0].Title = title
	}

	want := []string{"two", "one", "welcome"}
	for i, w := range want {
		var ok bool
		ws, _, ok = h.Undo(ws)
		if !ok {
			t.Fatalf("undo %d reported nothing to undo", i+1)
		}
		if got := ws.Boards[0].Notes[0].Title; got != w {
			t.Errorf("after undo %d, title = %q; want %q", i+1, got, w)
		}
	}
	if _, _, ok := h.Undo(ws); ok {
		t.Error("a fourth undo succeeded; the stack should be empty")
	}
}

func TestEmptyHistoryUndoRedoAreNoOps(t *testing.T) {
	h := NewHistory(10)
	ws := testWorkspace()

	if _, _, ok := h.Undo(ws); ok {
		t.Error("Undo on an empty history reported success")
	}
	if _, _, ok := h.Redo(ws); ok {
		t.Error("Redo on an empty history reported success")
	}
}

// A new mutation after undoing invalidates the redo branch — standard
// editor behavior, and the alternative (keeping it) silently resurrects
// work the user already walked away from.
func TestNewMutationClearsRedo(t *testing.T) {
	h := NewHistory(10)
	ws := testWorkspace()

	h.Push(ws, "first")
	ws.Boards[0].Notes[0].Title = "first"
	ws, _, _ = h.Undo(ws)

	if !h.CanRedo() {
		t.Fatal("redo should be available immediately after an undo")
	}

	h.Push(ws, "second")
	ws.Boards[0].Notes[0].Title = "second"

	if h.CanRedo() {
		t.Error("redo survived a new mutation; it must be discarded")
	}
}

// The ring is bounded so a long session cannot grow without limit.
// Labels vary so coalescing does not collapse the run into one entry —
// that behavior has its own test.
func TestHistoryRingIsBounded(t *testing.T) {
	const limit = 5
	h := NewHistory(limit)
	ws := testWorkspace()

	for i := 0; i < limit*3; i++ {
		h.Push(ws, "edit "+string(rune('a'+i)))
		ws.Boards[0].Notes[0].X = i
	}
	if got := h.Len(); got > limit {
		t.Errorf("history holds %d entries; want at most %d", got, limit)
	}

	// It should still undo `limit` times, reaching the oldest retained state.
	n := 0
	for {
		var ok bool
		ws, _, ok = h.Undo(ws)
		if !ok {
			break
		}
		n++
		if n > limit+2 {
			t.Fatal("Undo did not terminate")
		}
	}
	if n != limit {
		t.Errorf("undid %d times; want %d (the ring capacity)", n, limit)
	}
}

// Holding an arrow key fires dozens of nudges. Without coalescing they
// would flood the ring and evict everything that matters.
func TestRapidSameLabelPushesCoalesce(t *testing.T) {
	h := NewHistory(50)
	ws := testWorkspace()
	base := time.Now()

	h.pushAt(ws, "move note", base)
	for i := 1; i <= 20; i++ {
		ws.Boards[0].Notes[0].X = i
		h.pushAt(ws, "move note", base.Add(time.Duration(i)*20*time.Millisecond))
	}
	if got := h.Len(); got != 1 {
		t.Errorf("20 rapid same-label pushes produced %d entries; want 1", got)
	}

	// Undoing returns to the state before the whole run.
	restored, _, ok := h.Undo(ws)
	if !ok {
		t.Fatal("Undo failed after coalesced pushes")
	}
	if got := restored.Boards[0].Notes[0].X; got != 2 {
		t.Errorf("coalesced undo restored X=%d; want the pre-run value 2", got)
	}
}

func TestPushesSeparatedByTimeDoNotCoalesce(t *testing.T) {
	h := NewHistory(50)
	ws := testWorkspace()
	base := time.Now()

	h.pushAt(ws, "move note", base)
	ws.Boards[0].Notes[0].X = 5
	h.pushAt(ws, "move note", base.Add(3*time.Second))

	if got := h.Len(); got != 2 {
		t.Errorf("pushes 3s apart produced %d entries; want 2", got)
	}
}

func TestDifferentLabelsDoNotCoalesce(t *testing.T) {
	h := NewHistory(50)
	ws := testWorkspace()
	base := time.Now()

	h.pushAt(ws, "move note", base)
	h.pushAt(ws, "tint note", base.Add(10*time.Millisecond))

	if got := h.Len(); got != 2 {
		t.Errorf("two different labels produced %d entries; want 2", got)
	}
}

// Undo must hand back an independent board — otherwise the caller mutating
// the restored value would corrupt the entry still sitting in the stack.
func TestUndoReturnsAnIndependentBoard(t *testing.T) {
	h := NewHistory(10)
	ws := testWorkspace()

	h.Push(ws, "edit")
	ws.Boards[0].Notes[0].Title = "edited"

	restored, _, _ := h.Undo(ws)
	restored.Boards[0].Notes[0].Title = "mutated after restore"

	again, _, ok := h.Redo(restored)
	if !ok {
		t.Fatal("Redo failed")
	}
	if got := again.Boards[0].Notes[0].Title; got != "edited" {
		t.Errorf("redo returned %q; the stacked entry was corrupted by a caller mutation", got)
	}
}
