package main

// draw.go — cork + note + shadow drawing with zoom-aware rect parameter.
// drawNote + drawNoteAt let the same function render at any size so the
// transition and zoom presets share one code path.

import (
	"math/rand"
	"strings"
	"unicode/utf8"
)

// drawCork paints scattered brown ASCII chars across the canvas.
func drawCork(c *Canvas, stars []Star) {
	for _, s := range stars {
		c.SetRune(s.X, s.Y, s.R, s.Color, 0)
	}
}

// shadowRect paints an ASCII-dithered drop shadow under a rect.
func shadowRect(c *Canvas, x0, y0, w, h int, lifted bool) {
	thickness := 2
	if lifted {
		thickness = 4
	}
	densAt := func(d int) float32 {
		return float32(0.82) - float32(d-1)*(0.72/float32(thickness))
	}
	for d := 1; d <= thickness; d++ {
		density := densAt(d)
		for dy := 1; dy < h+thickness; dy++ {
			y := y0 + dy
			x := x0 + w + d - 1
			ch := shadowChar(x, y, density)
			if ch != ' ' {
				col := Lerp(ShadowMi, ShadowBG, density)
				c.SetRune(x, y, ch, col, 0)
			}
		}
	}
	for d := 1; d <= thickness; d++ {
		density := densAt(d)
		for dx := 1; dx < w+thickness; dx++ {
			x := x0 + dx
			y := y0 + h + d - 1
			ch := shadowChar(x, y, density)
			if ch != ' ' {
				col := Lerp(ShadowMi, ShadowBG, density)
				c.SetRune(x, y, ch, col, 0)
			}
		}
	}
}

// drawShadow wraps shadowRect for a Note at board zoom.
func drawShadow(c *Canvas, n *Note, zoom int) {
	d := DimsFor(zoom)
	shadowRect(c, n.EffectiveX(zoom), n.EffectiveY(zoom), d.W, d.H, n.Lifted)
}

// shadowChar picks a character for a shadow cell — dithered base + slash
// sprinkles in the mid-density band for texture.
func shadowChar(x, y int, density float32) rune {
	base := DitherShade(x, y, density)
	if base == ' ' {
		return ' '
	}
	if base == '█' {
		base = '▓'
	}
	if density < 0.6 && density > 0.15 {
		switch {
		case (x*13+y*7)%9 == 0:
			return '╱'
		case (x*11+y*5)%11 == 0:
			return '╲'
		}
	}
	return base
}

// --- borders ---------------------------------------------------------

// vertBorderChar picks the character used for a vertical border cell at
// row-offset dy. Stable seed + row-based modulo = a "woven" pattern.
func vertBorderChar(seed uint32, dy int) rune {
	switch (int(seed)*3 + dy) & 3 {
	case 0:
		return '│'
	case 1:
		return '╏'
	case 2:
		return '│'
	default:
		return '┃'
	}
}

// drawNote renders a Note at the board's zoom level.
func drawNote(c *Canvas, n *Note, selected bool, textMode TextStyleMode, zoom int) {
	d := DimsFor(zoom)
	drawNoteAt(c, Rect{X: n.EffectiveX(zoom), Y: n.EffectiveY(zoom), W: d.W, H: d.H},
		n, selected, textMode)
}

// drawNoteAtWithBorder is drawNoteAt with an explicit border-color override
// (used during the zoom transition to avoid the highlight flashing mid-anim).
func drawNoteAtWithBorder(c *Canvas, rect Rect, n *Note, textMode TextStyleMode, borderColor RGB) {
	drawNoteInternal(c, rect, n, true, textMode, &borderColor)
}

// drawNoteFrame draws only the frame (tape, borders, title row, separator,
// inner flourish, fold glyph) — NOT the body. Used in edit mode where a
// live textarea occupies the body. `liveTitle` overrides n.Title if set.
// `borderOverride` — if non-nil, forces the border color (used in edit mode
// to use the note's tint instead of the "selected" red).
func drawNoteFrame(c *Canvas, rect Rect, n *Note, selected bool, textMode TextStyleMode, liveTitle string, borderOverride *RGB) {
	tint := GetTint(n.Tint)
	paper := tint.Paper
	ink := tint.Ink
	tape := tint.Tape

	border := paper.Dimmed(0.8)
	if selected {
		border = PinRed
	}
	if borderOverride != nil {
		border = *borderOverride
	}
	borderAttr := uint8(0)
	if selected {
		borderAttr = AttrBold
	}

	x0, y0, w, h := rect.X, rect.Y, rect.W, rect.H
	if w < 6 || h < 4 {
		return
	}

	// Clear interior
	for dy := 0; dy < h; dy++ {
		for dx := 0; dx < w; dx++ {
			c.SetBlank(x0+dx, y0+dy)
		}
	}

	// Tape
	drawTapeStrip(c, x0, y0, w, tape, border, borderAttr, selected)

	// Vertical borders
	seed := hash(n.ID)
	for dy := 1; dy < h-1; dy++ {
		leftCh := vertBorderChar(seed, dy)
		rightCh := vertBorderChar(seed^0xA5A5, dy)
		c.SetRune(x0, y0+dy, leftCh, border, borderAttr)
		c.SetRune(x0+w-1, y0+dy, rightCh, border, borderAttr)
	}

	// Bottom border
	for dx := 0; dx < w; dx++ {
		x := x0 + dx
		var r rune
		switch {
		case dx == 0:
			r = '╰'
		case dx == w-1:
			r = '╯'
		default:
			switch (dx + x0) % 6 {
			case 0, 3:
				r = '━'
			default:
				r = '─'
			}
		}
		c.SetRune(x, y0+h-1, r, border, borderAttr)
	}

	// Title row
	if h > 3 {
		leadingGlyph := '❖'
		if selected {
			leadingGlyph = '◆'
		}
		c.SetRune(x0+2, y0+1, leadingGlyph, PinRed, AttrBold)

		title := liveTitle
		if title == "" {
			title = n.Title
		}
		if title == "" {
			title = "(untitled)"
		}
		title = StyleText(title, textMode)
		titleAttr := AttrBold | ModeAttr(textMode)

		showDate := w >= 28
		maxTitle := w - 6
		if showDate {
			maxTitle -= 7
		}
		if maxTitle < 4 {
			maxTitle = 4
		}
		if runeLen(title) > maxTitle {
			title = trimRunes(title, maxTitle-1) + "…"
		}
		c.WriteText(x0+4, y0+1, title, ink, titleAttr)

		if showDate {
			dateText := n.Updated.Format("01/02")
			dateX := x0 + w - 1 - 1 - runeLen(dateText)
			c.WriteText(dateX, y0+1, dateText, ink.Dimmed(0.72), AttrFaint)
		}
	}

	// Separator row
	if h > 4 {
		for dx := 0; dx < w; dx++ {
			x := x0 + dx
			var r rune
			switch {
			case dx == 0:
				r = '├'
			case dx == w-1:
				r = '┤'
			default:
				switch (dx + x0) % 3 {
				case 0:
					r = '─'
				case 1:
					r = '┄'
				case 2:
					r = '─'
				}
			}
			col := border
			if dx != 0 && dx != w-1 {
				col = ink.Dimmed(0.85)
			}
			c.SetRune(x, y0+2, r, col, 0)
		}
	}

	// Inner flourish
	if h > 8 {
		decorRow := y0 + h - 2
		for dx := 3; dx < w-3; dx++ {
			if (dx+x0)%3 == 0 {
				continue
			}
			x := x0 + dx
			existing := c.Get(x, decorRow)
			if existing.Rune > ' ' {
				continue
			}
			c.SetRune(x, decorRow, '┈', ink.Dimmed(0.5), AttrFaint)
		}
		c.SetRune(x0+2, decorRow, '◜', ink.Dimmed(0.7), AttrFaint)
		c.SetRune(x0+w-3, decorRow, '◝', ink.Dimmed(0.7), AttrFaint)
	}

	// Fold glyph
	if h > 6 && w > 6 {
		c.SetRune(x0+w-2, y0+h-2, '◞', ink.Dimmed(0.55), 0)
	}
}

// drawNoteAt renders the full note (border, tape, title, body, decor) at
// an arbitrary rect. Used by both normal rendering and the zoom transition.
func drawNoteAt(c *Canvas, rect Rect, n *Note, selected bool, textMode TextStyleMode) {
	drawNoteInternal(c, rect, n, selected, textMode, nil)
}

func drawNoteInternal(c *Canvas, rect Rect, n *Note, selected bool, textMode TextStyleMode, borderOverride *RGB) {
	tint := GetTint(n.Tint)
	paper := tint.Paper
	ink := tint.Ink
	tape := tint.Tape
	fiberCol := tint.Fiber

	border := paper.Dimmed(0.8)
	if selected {
		border = SelBorder
	}
	if borderOverride != nil {
		border = *borderOverride
	}
	borderAttr := uint8(0)
	if selected {
		borderAttr = AttrBold
	}
	if n.Flash > 0.05 {
		border = Lerp(border, tint.Edge, n.Flash*0.6)
	}

	x0, y0, w, h := rect.X, rect.Y, rect.W, rect.H
	if w < 6 || h < 4 {
		return // too small to render
	}

	// 1. Clear interior.
	for dy := 0; dy < h; dy++ {
		for dx := 0; dx < w; dx++ {
			c.SetBlank(x0+dx, y0+dy)
		}
	}

	// 2. Tape strip (row 0) — simplified: crease pattern + single pin.
	drawTapeStrip(c, x0, y0, w, tape, border, borderAttr, selected)

	// 3. Vertical borders — varied chars per row for ASCII-weave feel.
	seed := hash(n.ID)
	for dy := 1; dy < h-1; dy++ {
		leftCh := vertBorderChar(seed, dy)
		rightCh := vertBorderChar(seed^0xA5A5, dy)
		c.SetRune(x0, y0+dy, leftCh, border, borderAttr)
		c.SetRune(x0+w-1, y0+dy, rightCh, border, borderAttr)
	}

	// 4. Bottom border (row h-1).
	for dx := 0; dx < w; dx++ {
		x := x0 + dx
		var r rune
		switch {
		case dx == 0:
			r = '╰'
		case dx == w-1:
			r = '╯'
		default:
			switch (dx + x0) % 6 {
			case 0, 3:
				r = '━'
			default:
				r = '─'
			}
		}
		c.SetRune(x, y0+h-1, r, border, borderAttr)
	}

	// 5. Title row (row 1).
	titleRow := 1
	if h > 3 {
		leadingGlyph := '❖'
		if selected {
			leadingGlyph = '◆'
		}
		c.SetRune(x0+2, y0+titleRow, leadingGlyph, PinRed, AttrBold)

		title := n.Title
		if title == "" {
			title = "(untitled)"
		}
		title = StyleText(title, textMode)
		titleAttr := AttrBold | ModeAttr(textMode)

		// Reserve date stamp if room allows (>= 28 wide).
		showDate := w >= 28
		titleStart := x0 + 4
		maxTitle := w - 6
		if showDate {
			maxTitle -= 7
		}
		if maxTitle < 4 {
			maxTitle = 4
		}
		if runeLen(title) > maxTitle {
			title = trimRunes(title, maxTitle-1) + "…"
		}
		c.WriteText(titleStart, y0+titleRow, title, ink, titleAttr)

		if showDate {
			dateText := n.Updated.Format("01/02")
			dateX := x0 + w - 1 - 1 - runeLen(dateText)
			c.WriteText(dateX, y0+titleRow, dateText, ink.Dimmed(0.72), AttrFaint)
		}
	}

	// 6. Separator (row 2).
	sepRow := 2
	if h > 4 {
		for dx := 0; dx < w; dx++ {
			x := x0 + dx
			var r rune
			switch {
			case dx == 0:
				r = '├'
			case dx == w-1:
				r = '┤'
			default:
				switch (dx + x0) % 3 {
				case 0:
					r = '─'
				case 1:
					r = '┄'
				case 2:
					r = '─'
				}
			}
			col := border
			if dx != 0 && dx != w-1 {
				col = ink.Dimmed(0.85)
			}
			c.SetRune(x, y0+sepRow, r, col, 0)
		}
	}

	// 7. Body rows.
	bodyTop := 3
	bodyBottom := h - 2
	bodyWidth := w - 4
	if bodyTop < bodyBottom && bodyWidth > 0 {
		styled := StyleText(n.Body, textMode)
		wrapped := wrapLines(styled, bodyWidth)
		bodyAttr := ModeAttr(textMode)
		for i, line := range wrapped {
			if bodyTop+i >= bodyBottom {
				break
			}
			c.WriteText(x0+2, y0+bodyTop+i, line, ink, bodyAttr)
		}
	}

	// 8. Paper fibers (stable per-note seed).
	rng := rand.New(rand.NewSource(int64(seed)))
	bodyRows := bodyBottom - bodyTop
	if bodyRows > 0 && bodyWidth > 0 {
		fiberCount := (bodyWidth * bodyRows) / 22
		if fiberCount < 3 {
			fiberCount = 3
		}
		for i := 0; i < fiberCount; i++ {
			fx := rng.Intn(bodyWidth)
			fy := rng.Intn(bodyRows)
			cx := x0 + 2 + fx
			cy := y0 + bodyTop + fy
			existing := c.Get(cx, cy)
			if existing.Rune > ' ' && existing.Rune != '·' && existing.Rune != '.' && existing.Rune != ',' {
				continue
			}
			fch := FiberChars[rng.Intn(len(FiberChars))]
			c.SetRune(cx, cy, fch, fiberCol, AttrFaint)
		}
	}

	// 9. Inner decorative flourish line at row h-2 (skip on small notes).
	if h > 8 {
		decorRow := y0 + h - 2
		for dx := 3; dx < w-3; dx++ {
			if (dx+x0)%3 == 0 {
				continue
			}
			x := x0 + dx
			existing := c.Get(x, decorRow)
			if existing.Rune > ' ' {
				continue
			}
			c.SetRune(x, decorRow, '┈', ink.Dimmed(0.5), AttrFaint)
		}
		c.SetRune(x0+2, decorRow, '◜', ink.Dimmed(0.7), AttrFaint)
		c.SetRune(x0+w-3, decorRow, '◝', ink.Dimmed(0.7), AttrFaint)
	}

	// 10. Fold glyph.
	if h > 6 && w > 6 {
		c.SetRune(x0+w-2, y0+h-2, '◞', ink.Dimmed(0.55), 0)
	}
}

// drawTapeStrip — simplified: tape crease pattern + a single red pin.
func drawTapeStrip(c *Canvas, x0, y0, w int, tape RGB, border RGB, borderAttr uint8, selected bool) {
	pinX := x0 + w/2

	for dx := 0; dx < w; dx++ {
		x := x0 + dx
		var r rune
		col := tape
		attr := uint8(0)
		switch {
		case dx == 0:
			r, col, attr = '╭', border, borderAttr
		case dx == w-1:
			r, col, attr = '╮', border, borderAttr
		case x == pinX:
			r = '━' // placeholder, overwritten with pin
		default:
			idx := (dx + x0) % 8
			switch idx {
			case 0, 4:
				r = '━'
			case 1, 5:
				r = '╍'
			case 2, 6:
				r = '━'
			case 3, 7:
				r = '─'
			}
		}
		c.SetRune(x, y0, r, col, attr)
	}

	// Single red pin at center (no flanking highlights, no decorations).
	pinGlyph := '◉'
	if selected {
		pinGlyph = '◈'
	}
	c.SetRune(pinX, y0, pinGlyph, PinRed, AttrBold)
}

// drawFooter writes a bottom status bar.
func drawFooter(c *Canvas, text string) {
	if c.H < 1 {
		return
	}
	y := c.H - 1
	for x := 0; x < c.W; x++ {
		c.SetBlank(x, y)
	}
	pad := (c.W - runeLen(text)) / 2
	if pad < 0 {
		pad = 0
	}
	c.WriteText(pad, y, text, Footer, 0)
}

// --- helpers ----------------------------------------------------------

func runeLen(s string) int { return utf8.RuneCountInString(s) }

func trimRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

func wrapLines(body string, width int) []string {
	if width <= 0 {
		return []string{""}
	}
	var out []string
	for _, para := range strings.Split(body, "\n") {
		if para == "" {
			out = append(out, "")
			continue
		}
		words := strings.Fields(para)
		if len(words) == 0 {
			out = append(out, "")
			continue
		}
		cur := ""
		for _, w := range words {
			if cur == "" {
				if runeLen(w) > width {
					runes := []rune(w)
					for len(runes) > width {
						out = append(out, string(runes[:width]))
						runes = runes[width:]
					}
					cur = string(runes)
					continue
				}
				cur = w
				continue
			}
			if runeLen(cur)+1+runeLen(w) <= width {
				cur += " " + w
			} else {
				out = append(out, cur)
				cur = w
			}
		}
		if cur != "" {
			out = append(out, cur)
		}
	}
	return out
}

func hash(s string) uint32 {
	var h uint32 = 2166136261
	for i := 0; i < len(s); i++ {
		h ^= uint32(s[i])
		h *= 16777619
	}
	return h
}
