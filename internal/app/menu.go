package app

// menu.go — font picker popup (tmux-menu style). Right-anchored, lists
// all TextStyleMode options with a live preview rendered using each
// mode's actual ANSI attribute bits. Also holds the background picker.

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

// --- background picker -------------------------------------------------

// BackgroundMenu is the stateful background picker. nil on model = closed.
// Two independent axes: a cork on/off toggle (row 0) and the fill color
// (the BackgroundColors rows, then a custom-hex row). Row layout:
//
//	0                     → cork toggle
//	1 .. len(colors)      → color rows (color index = cursor-1)
//	len(colors)+1         → custom hex
type BackgroundMenu struct {
	Cursor int
	Saved  Background // background at open time; restored on esc

	Cork  bool   // working cork toggle
	Color string // working fill color (hex, or "" for transparent)
	Hex   string // custom-hex buffer, e.g. "#1c2233"
}

func NewBackgroundMenu(current Background) *BackgroundMenu {
	m := &BackgroundMenu{
		Saved: current,
		Cork:  current.Cork,
		Color: current.Color,
		Hex:   "#",
		// Default the cursor to the cork toggle.
	}
	// If the current fill matches a palette color, land on that row; if it's
	// a custom color, land on the custom row and seed its buffer.
	matched := false
	for i, ch := range BackgroundColors {
		if ch.Hex == current.Color {
			m.Cursor = 1 + i
			matched = true
			break
		}
	}
	if !matched && current.Color != "" {
		m.Cursor = len(BackgroundColors) + 1
		m.Hex = current.Color
	}
	return m
}

func (m *BackgroundMenu) count() int { return len(BackgroundColors) + 2 }

// OnCork / OnCustom report which special row the cursor is on.
func (m *BackgroundMenu) OnCork() bool   { return m.Cursor == 0 }
func (m *BackgroundMenu) OnCustom() bool { return m.Cursor == len(BackgroundColors)+1 }

// colorIndex returns the BackgroundColors index for the cursor, or -1.
func (m *BackgroundMenu) colorIndex() int {
	if m.Cursor >= 1 && m.Cursor <= len(BackgroundColors) {
		return m.Cursor - 1
	}
	return -1
}

func (m *BackgroundMenu) Move(delta int) {
	m.Cursor += delta
	if m.Cursor < 0 {
		m.Cursor = m.count() - 1
	}
	if m.Cursor >= m.count() {
		m.Cursor = 0
	}
	m.syncColor()
}

// syncColor live-previews the fill as the cursor lands on a color/custom row;
// on the cork row the fill is left unchanged.
func (m *BackgroundMenu) syncColor() {
	if i := m.colorIndex(); i >= 0 {
		m.Color = BackgroundColors[i].Hex
	} else if m.OnCustom() {
		m.Color = m.Hex
	}
}

// ToggleCork flips the cork overlay (used by left/right/space on the cork row).
func (m *BackgroundMenu) ToggleCork() { m.Cork = !m.Cork }

// HexInput appends valid hex characters to the custom buffer (single leading
// '#'), capping at "#rrggbb", and live-previews the result.
func (m *BackgroundMenu) HexInput(runes []rune) {
	for _, r := range runes {
		if r == '#' {
			continue
		}
		if _, ok := hexNibble(byte(r)); !ok {
			continue
		}
		if runeLen(m.Hex) >= 7 { // "#" + 6 digits
			break
		}
		m.Hex += string(r)
	}
	m.Color = m.Hex
}

func (m *BackgroundMenu) HexBackspace() {
	r := []rune(m.Hex)
	if len(r) > 1 { // keep the leading '#'
		m.Hex = string(r[:len(r)-1])
	}
	m.Color = m.Hex
}

// Selected returns the Background described by the working state.
func (m *BackgroundMenu) Selected() Background {
	return Background{Cork: m.Cork, Color: m.Color}
}

// Label returns a human-readable summary for the toast.
func (m *BackgroundMenu) Label() string {
	cork := "cork off"
	if m.Cork {
		cork = "cork on"
	}
	color := "transparent"
	if m.Color != "" {
		color = m.Color
		for _, ch := range BackgroundColors {
			if ch.Hex == m.Color {
				color = ch.Name
				break
			}
		}
	}
	return cork + " · " + color
}

func BackgroundMenuRect(w, h int) Rect {
	maxName := runeLen("terminal (transparent)")
	for _, ch := range BackgroundColors {
		if runeLen(ch.Name) > maxName {
			maxName = runeLen(ch.Name)
		}
	}
	mw := maxName + 18 // name + gap + swatch + padding
	if mw < 36 {
		mw = 36
	}
	// rows: cork + spacer + colors + custom, plus top/bottom borders.
	mh := len(BackgroundColors) + 5
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

func drawBackgroundMenu(c *Canvas, menu *BackgroundMenu) {
	rect := BackgroundMenuRect(c.W, c.H)
	drawBackgroundMenuAt(c, rect.X, rect.Y, menu)
}

func drawBackgroundMenuAt(c *Canvas, x, y int, menu *BackgroundMenu) {
	rect := BackgroundMenuRect(c.W, c.H)
	w, h := rect.W, rect.H

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

	title := " ◈ background "
	c.WriteText(x+2, y, title, PinRed, AttrBold)

	swatchX := x + w - 9
	drawRow := func(row int, selected bool, name string, nameCol RGB, right func()) {
		arrow := ' '
		if selected {
			arrow = '›'
		}
		c.SetRune(x+2, row, arrow, PinRed, AttrBold)
		col := nameCol
		attr := uint8(0)
		if selected {
			col = Flash
			attr = AttrBold
		}
		c.WriteText(x+4, row, name, col, attr)
		if right != nil {
			right()
		}
	}
	solidSwatch := func(row int, col RGB) func() {
		return func() {
			for k := 0; k < 6; k++ {
				c.SetRune(swatchX+k, row, '█', col, 0)
			}
		}
	}

	// Row 0: cork toggle.
	corkRow := y + 1
	{
		val := "‹ off ›"
		valCol := Footer
		if menu.Cork {
			val = "‹ on  ›"
			valCol = Flash
		}
		drawRow(corkRow, menu.OnCork(), "cork texture", DimText, func() {
			c.WriteText(x+w-2-runeLen(val), corkRow, val, valCol, AttrBold)
		})
	}

	// Color rows (after a one-line spacer).
	firstColorRow := corkRow + 2
	for i, ch := range BackgroundColors {
		row := firstColorRow + i
		selected := menu.Cursor == 1+i
		var right func()
		if ch.Hex == "" {
			right = func() { c.WriteText(swatchX, row, "▁▁▁▁▁▁", Footer, AttrFaint) }
		} else if col, ok := ParseHexColor(ch.Hex); ok {
			right = solidSwatch(row, col)
		}
		drawRow(row, selected, ch.Name, DimText, right)
	}

	// Custom hex row.
	customRow := firstColorRow + len(BackgroundColors)
	caret := ""
	if menu.OnCustom() {
		caret = "▏"
	}
	label := "custom " + menu.Hex + caret
	var right func()
	if col, ok := ParseHexColor(menu.Hex); ok {
		right = solidSwatch(customRow, col)
	} else {
		right = func() { c.WriteText(swatchX, customRow, "??????", Footer, AttrFaint) }
	}
	drawRow(customRow, menu.OnCustom(), label, DimText, right)

	hint := " ↑↓ move  ←→ cork  enter set  esc "
	hintX := x + (w-runeLen(hint))/2
	c.WriteText(hintX, y+h-1, hint, Footer, AttrFaint)
}
