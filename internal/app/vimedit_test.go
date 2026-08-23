package app

// vimedit_test.go — the wiring between VimState and the note card. The
// state machine has its own tests; these cover the parts that only exist
// once it is connected: key routing, buffer hand-off between the two
// editors, and the opt-in.

import (
	"os"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

func newVimModel(t *testing.T) model {
	t.Helper()
	ws := seedWorkspace()
	ws.Vim = true
	m := initialModel(ws)
	m.w, m.h = 120, 40
	m.now = time.Now()
	m.stars = GenStarsForBoard(m.w, m.h, ws.ActiveBoard().GrainSeed)

	n := m.board.Selection()
	if n == nil {
		t.Fatal("seed workspace has no selection")
	}
	m.editor = NewVimEditor(n, m.w, m.h)
	m.mode = ModeEdit
	return m
}

func vimKey(t *testing.T, m model, key string) model {
	t.Helper()
	var msg tea.KeyMsg
	switch key {
	case "esc":
		msg = tea.KeyMsg{Type: tea.KeyEsc}
	case "enter":
		msg = tea.KeyMsg{Type: tea.KeyEnter}
	case "backspace":
		msg = tea.KeyMsg{Type: tea.KeyBackspace}
	case "ctrl+s":
		msg = tea.KeyMsg{Type: tea.KeyCtrlS}
	default:
		msg = tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
	}
	next, _ := m.Update(msg)
	nm, ok := next.(model)
	if !ok {
		t.Fatalf("Update returned %T", next)
	}
	return nm
}

func TestVimEditorStartsInNormalMode(t *testing.T) {
	m := newVimModel(t)
	if m.editor.Vim == nil {
		t.Fatal("vim editor has no state")
	}
	if m.editor.Vim.Mode != VimNormal {
		t.Errorf("mode = %v; want normal", m.editor.Vim.Mode)
	}
}

// The point of normal mode: letters navigate, they do not type.
func TestNormalModeKeysDoNotInsertText(t *testing.T) {
	m := newVimModel(t)
	before := m.editor.Vim.Text()

	for _, k := range []string{"j", "k", "l", "h", "w", "b", "e", "0", "$"} {
		m = vimKey(t, m, k)
	}
	if got := m.editor.Vim.Text(); got != before {
		t.Errorf("navigation keys changed the buffer:\n got %q\nwant %q", got, before)
	}
}

func TestInsertModeTypesThrough(t *testing.T) {
	m := newVimModel(t)
	m = vimKey(t, m, "i")
	if m.editor.Vim.Mode != VimInsert {
		t.Fatalf("i did not enter insert mode (mode %v)", m.editor.Vim.Mode)
	}
	for _, r := range "abc" {
		m = vimKey(t, m, string(r))
	}
	if !strings.Contains(m.editor.Vim.Text(), "abc") {
		t.Errorf("typed text did not reach the buffer: %q", m.editor.Vim.Text())
	}
}

// Round trip: type in insert, leave, navigate, type again. This is where
// the two editors have to agree about the buffer.
func TestInsertThenNormalRoundTrip(t *testing.T) {
	m := newVimModel(t)
	m = vimKey(t, m, "i")
	for _, r := range "XY" {
		m = vimKey(t, m, string(r))
	}
	m = vimKey(t, m, "esc")
	if m.editor.Vim.Mode != VimNormal {
		t.Fatalf("esc did not return to normal (mode %v)", m.editor.Vim.Mode)
	}
	afterInsert := m.editor.Vim.Text()
	if !strings.Contains(afterInsert, "XY") {
		t.Fatalf("text typed in insert was lost: %q", afterInsert)
	}

	// A normal-mode key must not add text now.
	m = vimKey(t, m, "j")
	if got := m.editor.Vim.Text(); got != afterInsert {
		t.Errorf("j after esc changed the buffer:\n got %q\nwant %q", got, afterInsert)
	}
}

// esc in normal mode leaves the card, matching esc without vim enabled.
func TestEscapeInNormalModeLeavesTheCard(t *testing.T) {
	m := newVimModel(t)
	m = vimKey(t, m, "esc")
	if m.mode == ModeEdit && m.transition == nil {
		t.Error("esc in normal mode neither left edit mode nor started a transition")
	}
}

// ...but esc in insert mode only returns to normal.
func TestEscapeInInsertModeStaysInTheCard(t *testing.T) {
	m := newVimModel(t)
	m = vimKey(t, m, "i")
	m = vimKey(t, m, "esc")
	if m.editor.Vim.Mode != VimNormal {
		t.Errorf("mode = %v; want normal", m.editor.Vim.Mode)
	}
	if m.transition != nil {
		t.Error("esc from insert mode started a leave transition; it should stay in the card")
	}
}

// App controls keep working regardless of the mode.
func TestControlKeysBypassVim(t *testing.T) {
	for _, key := range []string{"ctrl+s", "ctrl+y", "ctrl+p", "ctrl+e", "ctrl+c"} {
		if !isEditorControlKey(key) {
			t.Errorf("%s should bypass the vim state machine", key)
		}
	}
	for _, key := range []string{"i", "a", "d", "x", "esc", "j"} {
		if isEditorControlKey(key) {
			t.Errorf("%s should be handled by vim, not bypassed", key)
		}
	}
}

// Split must read the vim buffer, or edits made in normal mode never
// reach the note on the way out.
func TestSplitReadsTheVimBuffer(t *testing.T) {
	m := newVimModel(t)
	m.editor.Vim.SetText("new title\n\nnew body")
	title, body := m.editor.Split()
	if title != "new title" {
		t.Errorf("title = %q; want %q", title, "new title")
	}
	if body != "new body" {
		t.Errorf("body = %q; want %q", body, "new body")
	}
}

// Deleting in normal mode has to survive the trip back to the note.
func TestNormalModeEditReachesTheNote(t *testing.T) {
	m := newVimModel(t)
	id := m.editor.NoteID
	m.editor.Vim.SetText("title line\n\nbody line")
	m.editor.Vim.Row, m.editor.Vim.Col = 2, 0

	m = vimKey(t, m, "d")
	m = vimKey(t, m, "d") // delete "body line"

	title, body := m.editor.Split()
	if strings.Contains(body, "body line") {
		t.Errorf("dd did not remove the line: body = %q", body)
	}
	if title != "title line" {
		t.Errorf("title = %q; want it intact", title)
	}
	if n := m.findNote(id); n == nil {
		t.Fatal("note vanished")
	}
}

func TestVisualModeFromTheCard(t *testing.T) {
	m := newVimModel(t)
	m.editor.Vim.SetText("hello world")
	m.editor.Vim.Row, m.editor.Vim.Col = 0, 0

	m = vimKey(t, m, "v")
	if m.editor.Vim.Mode != VimVisual {
		t.Fatalf("v did not enter visual mode (mode %v)", m.editor.Vim.Mode)
	}
	for i := 0; i < 5; i++ {
		m = vimKey(t, m, "l")
	}
	m = vimKey(t, m, "d")
	if got, want := m.editor.Vim.Text(), "world"; got != want {
		t.Errorf("visual delete from the card gave %q; want %q", got, want)
	}
}

// Without the opt-in, the editor must behave exactly as before.
func TestVimIsOffByDefault(t *testing.T) {
	ws := seedWorkspace()
	if ws.Vim {
		t.Error("a fresh workspace has vim enabled; it must be opt-in")
	}
	m := initialModel(ws)
	m.w, m.h = 120, 40
	n := m.board.Selection()
	e := NewEditor(n, m.w, m.h)
	if e.Vim != nil {
		t.Error("NewEditor created vim state; only NewVimEditor should")
	}
}

func TestVimPreferencePersists(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	ws := seedWorkspace()
	ws.Vim = true
	if err := SaveWorkspace(ws); err != nil {
		t.Fatal(err)
	}
	got, err := LoadWorkspace()
	if err != nil {
		t.Fatal(err)
	}
	if !got.Vim {
		t.Error("the vim preference did not survive a save/load round trip")
	}
}

// Off is the default, so it should not be written to the file.
func TestVimOffIsOmittedFromTheFile(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	ws := seedWorkspace()
	ws.Vim = false
	if err := SaveWorkspace(ws); err != nil {
		t.Fatal(err)
	}
	path, _ := DataPath()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	data := string(raw)
	if strings.Contains(data, `"vim"`) {
		t.Errorf("a disabled vim preference was written to the file:\n%s", data)
	}
}

func TestEnvTruthy(t *testing.T) {
	for _, yes := range []string{"1", "true", "TRUE", "yes", "on", " 1 "} {
		if !envTruthy(yes) {
			t.Errorf("envTruthy(%q) = false; want true", yes)
		}
	}
	for _, no := range []string{"", "0", "false", "no", "off", "maybe"} {
		if envTruthy(no) {
			t.Errorf("envTruthy(%q) = true; want false", no)
		}
	}
}

// The card must render in every mode without panicking, at a few sizes.
func TestVimCardRendersInAllModes(t *testing.T) {
	for _, mode := range []VimMode{VimNormal, VimInsert, VimVisual} {
		for _, size := range [][2]int{{80, 24}, {120, 40}, {200, 60}} {
			ws := seedWorkspace()
			ws.Vim = true
			m := initialModel(ws)
			m.w, m.h = size[0], size[1]
			m.stars = GenStarsForBoard(m.w, m.h, ws.ActiveBoard().GrainSeed)
			n := m.board.Selection()
			m.editor = NewVimEditor(n, m.w, m.h)
			m.editor.Vim.Mode = mode
			if mode == VimVisual {
				m.editor.Vim.visRow, m.editor.Vim.visCol = 0, 0
				m.editor.Vim.Row, m.editor.Vim.Col = 0, 3
			}
			m.mode = ModeEdit
			if out := m.View(); out == "" {
				t.Errorf("empty view in %v at %dx%d", mode, size[0], size[1])
			}
		}
	}
}

// An empty note is the case most likely to index out of bounds.
func TestVimCardRendersAnEmptyNote(t *testing.T) {
	ws := seedWorkspace()
	ws.Vim = true
	m := initialModel(ws)
	m.w, m.h = 100, 30
	m.stars = GenStarsForBoard(m.w, m.h, ws.ActiveBoard().GrainSeed)
	n := m.board.Selection()
	n.Title, n.Body = "", ""
	m.editor = NewVimEditor(n, m.w, m.h)
	m.mode = ModeEdit
	if out := m.View(); out == "" {
		t.Error("empty view for an empty note")
	}
}
