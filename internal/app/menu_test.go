package app

import "testing"

// The background picker has no scrolling, so every row has to fit on a
// terminal people actually use. 24 rows is the classic floor.
func TestBackgroundMenuFitsAStandardTerminal(t *testing.T) {
	const minRows = 24
	rect := BackgroundMenuRect(120, minRows)
	if rect.Y+rect.H > minRows {
		t.Errorf("background menu occupies rows %d..%d on a %d-row terminal; the last %d row(s) are cut off",
			rect.Y, rect.Y+rect.H, minRows, rect.Y+rect.H-minRows)
	}
}

// Clipping on a genuinely tiny pane is acceptable, but it must not panic.
func TestBackgroundMenuDrawsOnATinyCanvas(t *testing.T) {
	for _, size := range [][2]int{{20, 6}, {40, 10}, {80, 24}, {200, 60}} {
		c := NewCanvas(size[0], size[1])
		menu := NewBackgroundMenu(Background{Cork: true})
		drawBackgroundMenu(c, menu) // must not panic or write out of bounds
		_ = c.Serialize()
	}
}

// Every fill in the picker must parse — a typo'd hex would silently render
// as transparent instead of the named color.
func TestBackgroundColorsAllParse(t *testing.T) {
	for i, ch := range BackgroundColors {
		if ch.Hex == "" {
			continue // the transparent entry
		}
		if _, ok := ParseHexColor(ch.Hex); !ok {
			t.Errorf("BackgroundColors[%d] %q has unparseable hex %q", i, ch.Name, ch.Hex)
		}
	}
}

// The picker should offer fills for both kinds of terminal, or a light-theme
// user has nothing but dark fills to choose from.
func TestBackgroundColorsCoverLightAndDark(t *testing.T) {
	var light, dark int
	for _, ch := range BackgroundColors {
		if ch.Hex == "" {
			continue
		}
		col, ok := ParseHexColor(ch.Hex)
		if !ok {
			continue
		}
		if col.Brightness() > 0.5 {
			light++
		} else {
			dark++
		}
	}
	if light == 0 {
		t.Error("BackgroundColors offers no light fills")
	}
	if dark == 0 {
		t.Error("BackgroundColors offers no dark fills")
	}
}

// Cursor arithmetic indexes into BackgroundColors, so it has to stay in
// range no matter how many fills the list grows to.
func TestBackgroundMenuCursorStaysInRange(t *testing.T) {
	m := NewBackgroundMenu(Background{Cork: true})
	for i := 0; i < 3*len(BackgroundColors)+10; i++ {
		m.Move(1)
		if idx := m.colorIndex(); idx >= len(BackgroundColors) {
			t.Fatalf("colorIndex() = %d after %d moves; out of range for %d colors",
				idx, i+1, len(BackgroundColors))
		}
		_ = m.Selected()
		_ = m.Label()
	}
	for i := 0; i < 3*len(BackgroundColors)+10; i++ {
		m.Move(-1)
		if idx := m.colorIndex(); idx >= len(BackgroundColors) {
			t.Fatalf("colorIndex() = %d after backward move %d; out of range", idx, i+1)
		}
	}
}
