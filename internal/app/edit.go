package app

// edit.go — zoom-to-edit view with a canvas-drawn ornate frame and a
// bubbles/textarea spliced inside the body area. Backdrop shows the
// rest of the board dimmed.
//
// Convention: the first non-empty line of the textarea is the title; the
// rest is the body. The frame's title row reflects the live first line.

import (
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type Editor struct {
	Ta     textarea.Model
	NoteID string
}

func NewEditor(n *Note, w, h int) Editor {
	rect := TargetRect(w, h)
	ta := textarea.New()
	ta.Prompt = ""
	ta.ShowLineNumbers = false
	ta.CharLimit = 0
	ta.SetWidth(editBodyWidth(rect))
	ta.SetHeight(editBodyHeight(rect))
	ta.SetValue(n.Title + "\n\n" + n.Body)
	ta.Focus()
	ta.CursorEnd()
	return Editor{Ta: ta, NoteID: n.ID}
}

func editBodyWidth(rect Rect) int {
	w := rect.W - 4
	if w < 10 {
		w = 10
	}
	return w
}

func editBodyHeight(rect Rect) int {
	h := rect.H - 6
	if h < 4 {
		h = 4
	}
	return h
}

func (e *Editor) Update(msg tea.Msg) tea.Cmd {
	var cmd tea.Cmd
	e.Ta, cmd = e.Ta.Update(msg)
	return cmd
}

func (e *Editor) Resize(w, h int) {
	rect := TargetRect(w, h)
	e.Ta.SetWidth(editBodyWidth(rect))
	e.Ta.SetHeight(editBodyHeight(rect))
}

// Split returns the live (title, body) from the textarea content. First
// non-empty line is the title; the rest (skipping one separator blank)
// is the body.
func (e *Editor) Split() (string, string) {
	val := e.Ta.Value()
	lines := strings.SplitN(val, "\n", 2)
	if len(lines) == 0 {
		return "", ""
	}
	title := strings.TrimSpace(lines[0])
	if len(lines) == 1 {
		return title, ""
	}
	rest := lines[1]
	if strings.HasPrefix(rest, "\n") {
		rest = rest[1:]
	}
	return title, rest
}

// View composes: blurred backdrop → ornate canvas frame at target rect →
// textarea spliced inside the frame → footer.
func (e Editor) View(w, h int, n *Note, stars []Star, textMode TextStyleMode, board *Board) string {
	if n == nil {
		return ""
	}
	rect := TargetRect(w, h)

	// 1. Backdrop — board dimmed so the card pops. The focused note is
	// OMITTED so the card appears on a cleaner background (no echo of
	// itself behind the frame). Strings touching the focused note are
	// re-anchored to the card's pin position so they stay attached
	// where the transition left them, instead of snapping back.
	//
	// Order matters: behind-strings → other notes → in-front-strings,
	// then dim the whole thing. This keeps `InFront=true` strings on top
	// of notes even after the blur is applied (without it, they'd sit
	// underneath because everything was being drawn before the notes).
	c := NewCanvas(w, h-1)
	drawCork(c, stars)
	editPin := &PinOverride{
		NoteID: n.ID,
		X:      rect.X + rect.W/2,
		Y:      rect.Y,
	}
	// Behind-strings: respect InFront flag; strings touching the focus
	// are deferred to the in-front pass via attachToTopID.
	drawStringsBehind(c, board, -1, n.ID, editPin)
	for _, bn := range board.Notes {
		if bn.ID == n.ID {
			continue
		}
		drawShadow(c, bn, board.Zoom)
		drawNote(c, bn, false, textMode, board.Zoom)
	}
	// In-front-strings (incl. those touching focus): drawn AFTER notes
	// so they sit on top of cards even when dimmed.
	drawStringsInFront(c, board, nil, -1, n.ID, editPin)
	c.Dim(0.38)

	// 2. Ornate frame — border in the note's own tint (not red).
	tint := GetTint(n.Tint)
	borderCol := tint.Paper
	liveTitle, _ := e.Split()
	frame := NewCanvas(rect.W, rect.H)
	drawNoteFrame(frame, Rect{X: 0, Y: 0, W: rect.W, H: rect.H},
		n, true, textMode, liveTitle, &borderCol)
	bg := c.Serialize()
	bg = SpliceOverlay(bg, frame.Serialize(), rect.X, rect.Y)

	// 3. Textarea inside the frame body area.
	bodyY := rect.Y + 3
	bodyX := rect.X + 2
	bg = SpliceOverlay(bg, e.Ta.View(), bodyX, bodyY)

	// 4. Footer.
	footer := lipgloss.NewStyle().
		Foreground(lipgloss.Color(Footer.Hex())).
		Width(w).Align(lipgloss.Center).
		Render("esc: place back  •  ctrl+s: save  •  alt+a: font menu")

	return bg + "\n" + footer
}

// --- shared helpers ---------------------------------------------------

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
