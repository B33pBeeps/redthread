package main

// menu.go — font picker popup (tmux-menu style). Right-anchored, lists
// all TextStyleMode options with a live preview rendered using each
// mode's actual ANSI attribute bits.

// FontMenu is the stateful popup. nil on model = closed.
type FontMenu struct {
	Cursor    int           // index into TextModes
	SavedMode TextStyleMode // mode at time of opening; restored on Esc
}

func NewFontMenu(current TextStyleMode) *FontMenu {
	i := 0
	for idx, m := range TextModes {
		if m == current {
			i = idx
			break
		}
	}
	return &FontMenu{Cursor: i, SavedMode: current}
}

func (m *FontMenu) Move(delta int) {
	m.Cursor += delta
	if m.Cursor < 0 {
		m.Cursor = len(TextModes) - 1
	}
	if m.Cursor >= len(TextModes) {
		m.Cursor = 0
	}
}

func (m *FontMenu) Selected() TextStyleMode { return TextModes[m.Cursor] }

// --- rendering --------------------------------------------------------

func FontMenuRect(w, h int) Rect {
	maxName := 0
	maxSample := 0
	for _, m := range TextModes {
		if runeLen(m.Name()) > maxName {
			maxName = runeLen(m.Name())
		}
		if runeLen(m.Sample()) > maxSample {
			maxSample = runeLen(m.Sample())
		}
	}
	mw := maxName + maxSample + 10
	if mw < 32 {
		mw = 32
	}
	mh := len(TextModes) + 4
	mx := w - mw - 2
	if mx < 2 {
		mx = 2
	}
	my := (h - mh) / 2
	if my < 1 {
		my = 1
	}
	return Rect{X: mx, Y: my, W: mw, H: mh}
}

func drawFontMenu(c *Canvas, menu *FontMenu) {
	rect := FontMenuRect(c.W, c.H)
	drawFontMenuAt(c, rect.X, rect.Y, menu)
}

func drawFontMenuAt(c *Canvas, x, y int, menu *FontMenu) {
	maxName := 0
	maxSample := 0
	names := make([]string, len(TextModes))
	samples := make([]string, len(TextModes))
	for i, m := range TextModes {
		names[i] = m.Name()
		samples[i] = m.Sample()
		if runeLen(names[i]) > maxName {
			maxName = runeLen(names[i])
		}
		if runeLen(samples[i]) > maxSample {
			maxSample = runeLen(samples[i])
		}
	}
	w := maxName + maxSample + 10
	if w < 32 {
		w = 32
	}
	h := len(TextModes) + 4

	c.BlankRect(x, y, w, h)

	bdCol := PinRed
	for dx := 0; dx < w; dx++ {
		c.SetRune(x+dx, y, '─', bdCol, 0)
		c.SetRune(x+dx, y+h-1, '─', bdCol, 0)
	}
	for dy := 0; dy < h; dy++ {
		c.SetRune(x, y+dy, '│', bdCol, 0)
		c.SetRune(x+w-1, y+dy, '│', bdCol, 0)
	}
	c.SetRune(x, y, '╭', bdCol, AttrBold)
	c.SetRune(x+w-1, y, '╮', bdCol, AttrBold)
	c.SetRune(x, y+h-1, '╰', bdCol, AttrBold)
	c.SetRune(x+w-1, y+h-1, '╯', bdCol, AttrBold)

	title := " ◈ pick a font "
	c.WriteText(x+2, y, title, PinRed, AttrBold)

	for i, mode := range TextModes {
		row := y + 1 + i
		selected := i == menu.Cursor
		arrow := ' '
		if selected {
			arrow = '›'
		}
		c.SetRune(x+2, row, arrow, PinRed, AttrBold)

		nameCol := DimText
		if selected {
			nameCol = Flash
		}
		nameAttr := uint8(0)
		if selected {
			nameAttr = AttrBold
		}
		c.WriteText(x+4, row, names[i], nameCol, nameAttr)

		// Preview: render with the mode's actual attributes so the user
		// sees exactly how their note will look.
		sample := samples[i]
		sampleCol := DimText
		if selected {
			sampleCol = Flash
		}
		sampleAttr := ModeAttr(mode)
		sx := x + w - 2 - runeLen(sample)
		c.WriteText(sx, row, sample, sampleCol, sampleAttr)
	}

	hint := " ↑↓ pick  enter set  esc cancel "
	hintX := x + (w-runeLen(hint))/2
	c.WriteText(hintX, y+h-1, hint, Footer, AttrFaint)
}
