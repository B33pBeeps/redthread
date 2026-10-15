package app

import (
	"encoding/json"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestNoteSizeDefaultsAndClamp(t *testing.T) {
	n := &Note{}
	if w, h := n.WorldSize(); w != baseNoteW || h != baseNoteH {
		t.Fatalf("default size = %dx%d", w, h)
	}
	if n.Dims(0) != DimsFor(0) {
		t.Errorf("unsized note dims differ from DimsFor")
	}
	n.SetWorldSize(1, 1)
	if n.W != minNoteW || n.H != minNoteH {
		t.Errorf("min clamp: %dx%d", n.W, n.H)
	}
	n.SetWorldSize(9999, 9999)
	if n.W != maxNoteW || n.H != maxNoteH {
		t.Errorf("max clamp: %dx%d", n.W, n.H)
	}
}

func TestNoteSizeJSON(t *testing.T) {
	b, _ := json.Marshal(&Note{ID: "a"})
	var raw map[string]any
	_ = json.Unmarshal(b, &raw)
	if _, ok := raw["w"]; ok {
		t.Errorf("default note should omit w/h: %s", b)
	}
	n := &Note{ID: "a"}
	n.SetWorldSize(50, 20)
	b, _ = json.Marshal(n)
	var back Note
	if err := json.Unmarshal(b, &back); err != nil || back.W != 50 || back.H != 20 {
		t.Errorf("round trip: %+v err=%v", back, err)
	}
}

func TestResizeHandleHit(t *testing.T) {
	n := &Note{X: 10, Y: 5}
	r := n.ScreenRect(0)
	if !n.OnResizeHandle(r.X+r.W-1, r.Y+r.H-1, 0) {
		t.Errorf("corner cell should hit")
	}
	if n.OnResizeHandle(r.X, r.Y, 0) || n.OnResizeHandle(r.X+r.W, r.Y+r.H, 0) {
		t.Errorf("non-corner cells should not hit")
	}
}

func TestMouseDragResizes(t *testing.T) {
	m := initialModel(seedWorkspace())
	m.w, m.h = 120, 40
	n := m.board.Notes[0]
	r := n.ScreenRect(m.board.Zoom)
	px, py := r.X+r.W-1, r.Y+r.H-1
	send := func(a tea.MouseAction, x, y int) {
		mm, _ := m.handleMouse(tea.MouseMsg{X: x, Y: y, Button: tea.MouseButtonLeft, Action: a})
		m = mm.(model)
	}
	x0, y0 := n.X, n.Y
	send(tea.MouseActionPress, px, py)
	send(tea.MouseActionMotion, px+10, py+4)
	send(tea.MouseActionRelease, px+10, py+4)
	w, h := n.WorldSize()
	if w != baseNoteW+10 || h != baseNoteH+4 {
		t.Errorf("size after drag = %dx%d", w, h)
	}
	if n.X != x0 || n.Y != y0 {
		t.Errorf("resize moved the note")
	}
	if m.resizeID != "" || m.grabID != "" {
		t.Errorf("drag state not cleared")
	}
}

func TestViewRendersResizedNotes(t *testing.T) {
	for _, z := range []int{ZoomMin, 0, ZoomMax} {
		for _, sz := range [][2]int{{1, 1}, {9999, 9999}} {
			ws := seedWorkspace()
			m := initialModel(ws)
			m.w, m.h = 80, 24
			m.board.Zoom = z
			m.stars = GenStarsForBoard(m.w, m.h, ws.ActiveBoard().GrainSeed)
			m.board.Notes[0].SetWorldSize(sz[0], sz[1])
			if m.View() == "" {
				t.Errorf("empty view zoom=%d size=%v", z, sz)
			}
		}
	}
}
