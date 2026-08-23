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

	// Vim is non-nil when modal editing is on. It owns the buffer in
	// normal and visual mode; the textarea takes over for insert.
	Vim *VimState
}

func NewEditor(n *Note, w, h int) Editor {
	rect := TargetRect(w, h)
	ta := textarea.New()
	ta.Prompt = ""
	ta.ShowLineNumbers = false
	ta.CharLimit = 0
	ta.SetWidth(editBodyWidth(rect))
	ta.SetHeight(editBodyHeight(rect))
	ta.SetValue(composeEditorValue(n.Title, n.Body))
	ta.Focus()
	ta.CursorEnd()
	return Editor{Ta: ta, NoteID: n.ID}
}

// NewVimEditor is NewEditor with modal editing enabled, starting in
// normal mode as vim does.
func NewVimEditor(n *Note, w, h int) Editor {
	e := NewEditor(n, w, h)
	e.Vim = NewVimState(composeEditorValue(n.Title, n.Body))
	return e
}

// SyncToTextarea pushes the vim buffer into the textarea, for entering
// insert mode. The textarea's own cursor cannot be placed across wrapped
// lines, so this positions it as closely as the widget allows: the right
// row when lines are short, and the right column within that row.
func (e *Editor) SyncToTextarea() {
	if e.Vim == nil {
		return
	}
	e.Ta.SetValue(e.Vim.Text())
	for i := 0; i < 1000 && e.Ta.Line() > 0; i++ {
		e.Ta.CursorUp()
	}
	for i := 0; i < e.Vim.Row; i++ {
		e.Ta.CursorDown()
	}
	e.Ta.SetCursor(e.Vim.Col)
}

// SyncFromTextarea pulls edits made in insert mode back into the vim
// buffer, so normal mode sees what was typed.
func (e *Editor) SyncFromTextarea() {
	if e.Vim == nil {
		return
	}
	e.Vim.SetText(e.Ta.Value())
	e.Vim.Row = e.Ta.Line()
	e.Vim.Col = e.Ta.LineInfo().ColumnOffset + e.Ta.LineInfo().StartColumn
	e.Vim.clamp()
}

// composeEditorValue builds the textarea seed text. Empty notes seed as
// an empty string so the cursor lands on row 1, not row 3.
func composeEditorValue(title, body string) string {
	if title == "" && body == "" {
		return ""
	}
	if body == "" {
		return title
	}
	return title + "\n\n" + body
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
	if e.Vim != nil && e.Vim.Mode != VimInsert {
		val = e.Vim.Text()
	}
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
func (e Editor) View(w, h int, n *Note, stars []Star, textMode TextStyleMode, board *Board, bgColor *RGB) string {
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
	if bgColor != nil {
		c.DefaultBg = bgColor
	}
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

	// 3. Textarea inside the frame body area. The textarea stores plain
	// ASCII internally; we apply the board's font as a display-only pass
	// so that what's visible matches the rest of the board. ANSI escapes
	// (cursor highlight) are preserved verbatim.
	bodyY := rect.Y + 3
	bodyX := rect.X + 2
	var bodyView string
	if e.Vim != nil && e.Vim.Mode != VimInsert {
		// Normal/visual: vim owns the buffer, so render it here with a
		// block cursor. The textarea cannot show a cursor it cannot
		// place.
		bodyView = vimRender(e.Vim,
			editBodyWidth(rect), editBodyHeight(rect),
			tint.Ink, tint.Paper, tint.Ink)
		bodyView = StyleViewText(bodyView, textMode)
	} else {
		bodyView = StyleViewText(e.Ta.View(), textMode)
	}
	bg = SpliceOverlay(bg, bodyView, bodyX, bodyY)

	// 4. Footer.
	footerText := "esc: place back  •  ctrl+s: save  •  ctrl+y: copy  •  ctrl+p: paste  •  ctrl+e: $EDITOR"
	if e.Vim != nil {
		footerText = vimModeLabel(e.Vim)
	}
	footer := lipgloss.NewStyle().
		Foreground(lipgloss.Color(Footer.Hex())).
		Width(w).Align(lipgloss.Center).
		Render(footerText)

	return bg + "\n" + footer
}

// --- shared helpers ---------------------------------------------------

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
