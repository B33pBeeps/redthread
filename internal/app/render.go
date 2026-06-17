package app

// render.go — cell grid and ANSI serializer.
//
// The grid never sets a background color. Empty cells have Rune == 0 and
// serialize to a bare space with the terminal's default colors — i.e. the
// user's tmux background shows through.

import (
	"strings"
	"unicode/utf8"
)

// Attr bits for a cell.
const (
	AttrBold   = 1 << 0
	AttrFaint  = 1 << 1
	AttrItalic = 1 << 2
	AttrUnder  = 1 << 3
	AttrStrike = 1 << 4
)

type Cell struct {
	Rune rune
	Fg   *RGB  // nil = terminal default fg (so a space is fully transparent)
	Attr uint8 // Attr* bits
}

// IsEmpty returns true if the cell is the fully-transparent default.
func (c Cell) IsEmpty() bool { return c.Rune == 0 && c.Fg == nil && c.Attr == 0 }

type Canvas struct {
	W, H  int
	cells []Cell // row-major, len == W*H

	// DefaultBg, when non-nil, is the background color emitted for every
	// cell that has no fill of its own — i.e. a solid board background.
	// nil keeps the original fully-transparent behavior (tmux shows through).
	DefaultBg *RGB
}

func NewCanvas(w, h int) *Canvas {
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	return &Canvas{W: w, H: h, cells: make([]Cell, w*h)}
}

func (c *Canvas) Clear() {
	for i := range c.cells {
		c.cells[i] = Cell{}
	}
}

func (c *Canvas) Inside(x, y int) bool {
	return x >= 0 && y >= 0 && x < c.W && y < c.H
}

func (c *Canvas) Get(x, y int) Cell {
	if !c.Inside(x, y) {
		return Cell{}
	}
	return c.cells[y*c.W+x]
}

func (c *Canvas) Set(x, y int, cell Cell) {
	if !c.Inside(x, y) {
		return
	}
	c.cells[y*c.W+x] = cell
}

// SetRune writes just rune+fg+attr without allocation.
func (c *Canvas) SetRune(x, y int, r rune, fg RGB, attr uint8) {
	if !c.Inside(x, y) {
		return
	}
	fgp := fg
	c.cells[y*c.W+x] = Cell{Rune: r, Fg: &fgp, Attr: attr}
}

// SetBlank clears a cell back to transparent (erases cork stars etc).
func (c *Canvas) SetBlank(x, y int) {
	if !c.Inside(x, y) {
		return
	}
	c.cells[y*c.W+x] = Cell{Rune: ' '}
}

// --- serialize ---------------------------------------------------------

// Serialize walks the grid, coalescing adjacent cells that share style.
// Returns a string containing ANSI SGR + text, suitable for Bubble Tea's
// View() return value.
func (c *Canvas) Serialize() string {
	var b strings.Builder
	b.Grow(c.W * c.H * 2)

	const reset = "\x1b[0m"
	bg := c.DefaultBg // constant across the canvas

	for y := 0; y < c.H; y++ {
		curFg := (*RGB)(nil)
		curAttr := uint8(255) // sentinel != any real attr so first cell forces write
		styled := false
		for x := 0; x < c.W; x++ {
			cell := c.cells[y*c.W+x]
			r := cell.Rune
			if r == 0 {
				r = ' '
			}
			// compute target style
			sameFg := (cell.Fg == nil && curFg == nil) ||
				(cell.Fg != nil && curFg != nil && *cell.Fg == *curFg)
			sameAttr := cell.Attr == curAttr
			if !(sameFg && sameAttr) {
				// close previous run
				if styled {
					b.WriteString(reset)
					styled = false
				}
				// A solid background means even fg-less cells need styling.
				if cell.Fg != nil || cell.Attr != 0 || bg != nil {
					if cell.Attr&AttrBold != 0 {
						b.WriteString("\x1b[1m")
					}
					if cell.Attr&AttrFaint != 0 {
						b.WriteString("\x1b[2m")
					}
					if cell.Attr&AttrItalic != 0 {
						b.WriteString("\x1b[3m")
					}
					if cell.Attr&AttrUnder != 0 {
						b.WriteString("\x1b[4m")
					}
					if cell.Attr&AttrStrike != 0 {
						b.WriteString("\x1b[9m")
					}
					if bg != nil {
						b.WriteString(bg.BgSGR())
					}
					if cell.Fg != nil {
						b.WriteString(cell.Fg.SGR())
					}
					styled = true
				}
				curFg = cell.Fg
				curAttr = cell.Attr
			}
			b.WriteRune(r)
		}
		if styled {
			b.WriteString(reset)
		}
		if y < c.H-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// FillRect writes a single rune + style across a rectangular area,
// overwriting whatever was there (useful for drop shadows, clears, etc).
func (c *Canvas) FillRect(x, y, w, h int, r rune, fg *RGB, attr uint8) {
	for dy := 0; dy < h; dy++ {
		for dx := 0; dx < w; dx++ {
			px, py := x+dx, y+dy
			if !c.Inside(px, py) {
				continue
			}
			c.cells[py*c.W+px] = Cell{Rune: r, Fg: fg, Attr: attr}
		}
	}
}

// BlankRect clears a rectangle back to transparent.
func (c *Canvas) BlankRect(x, y, w, h int) {
	for dy := 0; dy < h; dy++ {
		for dx := 0; dx < w; dx++ {
			px, py := x+dx, y+dy
			if !c.Inside(px, py) {
				continue
			}
			c.cells[py*c.W+px] = Cell{Rune: ' '}
		}
	}
}

// WriteText writes a single-line string cell-by-cell starting at (x, y),
// wrapping handled by the caller.
func (c *Canvas) WriteText(x, y int, s string, fg RGB, attr uint8) {
	col := x
	for _, r := range s {
		if col >= c.W {
			break
		}
		if c.Inside(col, y) {
			f := fg
			c.cells[y*c.W+col] = Cell{Rune: r, Fg: &f, Attr: attr}
		}
		col++
	}
}

// BlitAt copies every cell from `src` into this canvas at offset (dx, dy).
// Cells that fall outside this canvas are clipped. Source-empty cells
// (Rune == 0 && Fg == nil) overwrite the destination just the same — the
// caller is expected to pass a freshly-allocated dst canvas if it wants
// the off-screen area to read as terminal default.
func (c *Canvas) BlitAt(src *Canvas, dx, dy int) {
	for sy := 0; sy < src.H; sy++ {
		ty := sy + dy
		if ty < 0 || ty >= c.H {
			continue
		}
		for sx := 0; sx < src.W; sx++ {
			tx := sx + dx
			if tx < 0 || tx >= c.W {
				continue
			}
			c.cells[ty*c.W+tx] = src.cells[sy*src.W+sx]
		}
	}
}

// Dim applies a multiplier to every cell's foreground color, preserving
// transparency on bare cells. Used for blurring the backdrop during edit.
func (c *Canvas) Dim(f float32) {
	for i := range c.cells {
		if c.cells[i].Fg == nil {
			continue
		}
		d := c.cells[i].Fg.Dimmed(f)
		c.cells[i].Fg = &d
	}
	// Dim the solid background too so the edit backdrop / crossfade darken
	// uniformly instead of leaving a full-brightness field behind dim text.
	if c.DefaultBg != nil {
		d := c.DefaultBg.Dimmed(f)
		c.DefaultBg = &d
	}
}

// --- ANSI-aware string splicing -------------------------------------

// splitAtVisual cuts `s` at visible cell column `col`. CSI escape
// sequences (`\x1b[...{letter}`) are passed through unchanged and don't
// count toward `col`.
func splitAtVisual(s string, col int) (string, string) {
	visible := 0
	i := 0
	for i < len(s) {
		if s[i] == '\x1b' && i+1 < len(s) {
			j := i + 2 // skip ESC + next byte (usually '[')
			for j < len(s) {
				ch := s[j]
				if (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') {
					j++
					break
				}
				j++
			}
			if j <= len(s) {
				i = j
				continue
			}
			break
		}
		if visible >= col {
			break
		}
		_, size := utf8Decode(s[i:])
		if size == 0 {
			i++
			continue
		}
		i += size
		visible++
	}
	return s[:i], s[i:]
}

// visibleWidth returns the visible-cell width of a string, ignoring CSI.
func visibleWidth(s string) int {
	w := 0
	i := 0
	for i < len(s) {
		if s[i] == '\x1b' && i+1 < len(s) {
			j := i + 2
			for j < len(s) {
				ch := s[j]
				if (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') {
					j++
					break
				}
				j++
			}
			i = j
			continue
		}
		_, size := utf8Decode(s[i:])
		if size == 0 {
			i++
			continue
		}
		i += size
		w++
	}
	return w
}

// utf8Decode is a thin wrapper. Kept separate so future width-aware
// logic (e.g. CJK double-width) can slot in here.
func utf8Decode(s string) (rune, int) {
	if len(s) == 0 {
		return 0, 0
	}
	r, size := utf8.DecodeRuneInString(s)
	return r, size
}

// effectiveStyleAt returns the SGR sequence active at visual column `col`
// within string `s` — i.e., the last CSI seen before that column. An
// empty return means no style was active (terminal default).
func effectiveStyleAt(s string, col int) string {
	last := ""
	visible := 0
	i := 0
	for i < len(s) {
		if s[i] == '\x1b' && i+1 < len(s) {
			j := i + 2
			for j < len(s) {
				ch := s[j]
				if (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') {
					j++
					break
				}
				j++
			}
			last = s[i:j]
			i = j
			continue
		}
		if visible >= col {
			break
		}
		_, size := utf8Decode(s[i:])
		if size == 0 {
			i++
			continue
		}
		i += size
		visible++
	}
	return last
}

// SpliceOverlay splices `overlay` onto `bg` at visual cell position
// (x, y). Both strings are newline-separated; ANSI CSI escapes are
// preserved. Style state active at the cut point is re-emitted before
// the suffix so chars just past the overlay keep their original styling
// (otherwise they'd render in terminal default, often appearing white).
func SpliceOverlay(bg, overlay string, x, y int) string {
	bgLines := splitLines(bg)
	ovLines := splitLines(overlay)
	const reset = "\x1b[0m"
	for i, ov := range ovLines {
		row := y + i
		if row < 0 || row >= len(bgLines) {
			continue
		}
		ovW := visibleWidth(ov)
		prefix, rest := splitAtVisual(bgLines[row], x)
		_, suffix := splitAtVisual(rest, ovW)
		// Find style that was active at the position AFTER the overlay.
		styleAfter := effectiveStyleAt(bgLines[row], x+ovW)
		if styleAfter == reset {
			styleAfter = ""
		}
		bgLines[row] = prefix + reset + ov + reset + styleAfter + suffix
	}
	return joinLines(bgLines)
}

func splitLines(s string) []string {
	out := []string{}
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	out = append(out, s[start:])
	return out
}

func joinLines(lines []string) string {
	n := 0
	for _, l := range lines {
		n += len(l) + 1
	}
	b := make([]byte, 0, n)
	for i, l := range lines {
		if i > 0 {
			b = append(b, '\n')
		}
		b = append(b, l...)
	}
	return string(b)
}
