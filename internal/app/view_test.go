package app

// view_test.go — rendering details that are easy to get wrong and hard to
// see in a unit of the drawing code alone: the open card over a solid
// fill, and the board-switch crossfade.

import (
	"strings"
	"testing"
	"time"
)

// With a solid fill, the open card must paint the fill behind its own
// blank cells. The backdrop is dimmed, so the exact fill color on the
// card's title row can only come from the frame itself.
func TestEditCardUsesTheSolidFill(t *testing.T) {
	ws := seedWorkspace()
	ws.Background = Background{Cork: true, Color: "#1c2233"}
	m := initialModel(ws)
	t.Cleanup(m.saver.Cancel)
	m.w, m.h = 100, 30
	m.stars = GenStarsForBoard(m.w, m.h, ws.ActiveBoard().GrainSeed)
	n := m.board.Selection()
	if n == nil {
		t.Fatal("no selection")
	}
	m.editor = NewEditor(n, m.w, m.h)
	m.mode = ModeEdit

	lines := strings.Split(m.View(), "\n")
	rect := TargetRect(m.w, m.h)
	titleRow := lines[rect.Y+1]
	want := RGB{0x1c, 0x22, 0x33}.BgSGR()
	if !strings.Contains(titleRow, want) {
		t.Errorf("the card's title row never emits the fill %q; the terminal shows through the frame", want)
	}
}

// The outgoing board in a crossfade is drawn with its own highlight
// color, not the incoming board's, and the global is restored afterwards.
func TestCrossfadeDrawsOldBoardWithItsOwnHighlight(t *testing.T) {
	t.Cleanup(func() { ApplyTheme(false) })
	ws := seedWorkspace()
	ws.Boards[0].HighlightColor = 0
	ws.AddBoard("second")
	ws.Boards[1].HighlightColor = 3
	m := initialModel(ws)
	t.Cleanup(m.saver.Cancel)
	m.w, m.h = 100, 30
	m.now = time.Now()
	m.workspace.ActiveIdx = 0
	m.refreshActive()

	m.startBoardAnim(+1)
	m.workspace.CycleActive(1)
	m.refreshActive() // SelBorder now points at the incoming board's choice 3
	m.boardAnim = 0.1 // first half: the outgoing board is on screen

	out := m.View()
	local := easeInOutCubic(m.boardAnim * 2)
	oldDimmed := SelBorderChoices[0].Color.Dimmed(1.0 - 0.85*local)
	if !strings.Contains(out, oldDimmed.SGR()) {
		t.Errorf("outgoing board's selected note is not drawn in its own highlight %s", oldDimmed.Hex())
	}
	if SelBorder != SelBorderChoices[3].Color {
		t.Errorf("SelBorder = %s after the draw; want the active board's %s",
			SelBorder.Hex(), SelBorderChoices[3].Color.Hex())
	}
}
