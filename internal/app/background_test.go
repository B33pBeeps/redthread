package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBackgroundPersistenceRoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dir)

	ws := seedWorkspace()
	ws.Background = Background{Cork: true, Color: "#1c2233"}
	if err := SaveWorkspace(ws); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := LoadWorkspace()
	if err != nil || got == nil {
		t.Fatalf("load: %v (ws=%v)", err, got)
	}
	if got.Background != ws.Background {
		t.Errorf("round-trip background = %+v; want %+v", got.Background, ws.Background)
	}
}

func TestV4FileLoadsAsCork(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dir)

	// A v4 file has boards but no "background" key.
	v4 := `{"schemaVersion":4,"activeIdx":0,"boards":[{"name":"main","notes":[{"id":"abc","x":1,"y":1,"tint":"yellow"}]}]}`
	path := filepath.Join(dir, "redthread", "notes.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(v4), 0o644); err != nil {
		t.Fatal(err)
	}
	ws, err := LoadWorkspace()
	if err != nil || ws == nil {
		t.Fatalf("load v4: %v", err)
	}
	if !ws.CorkOn() {
		t.Error("v4 file without background should load as cork")
	}
}

func TestParseHexColor(t *testing.T) {
	cases := []struct {
		in   string
		ok   bool
		want RGB
	}{
		{"#1c2233", true, RGB{0x1c, 0x22, 0x33}},
		{"1c2233", true, RGB{0x1c, 0x22, 0x33}},
		{"#FFFFFF", true, RGB{255, 255, 255}},
		{"  #00ff00 ", true, RGB{0, 255, 0}},
		{"#12345", false, RGB{}},
		{"#nothex", false, RGB{}},
		{"", false, RGB{}},
	}
	for _, c := range cases {
		got, ok := ParseHexColor(c.in)
		if ok != c.ok || (ok && got != c.want) {
			t.Errorf("ParseHexColor(%q) = %v,%v; want %v,%v", c.in, got, ok, c.want, c.ok)
		}
	}
}

func TestWorkspaceBackgroundHelpers(t *testing.T) {
	// A freshly seeded workspace keeps the original look: cork on, transparent.
	if !seedWorkspace().CorkOn() {
		t.Error("seeded workspace should default to cork on")
	}

	w := &Workspace{}

	// Cork and color are independent axes.
	w.Background = Background{Cork: true} // cork over transparent
	if !w.CorkOn() {
		t.Error("Cork:true should draw cork")
	}
	if _, ok := w.BgColor(); ok {
		t.Error("empty color should be transparent")
	}

	w.Background = Background{Cork: false, Color: "#102030"} // solid, no cork
	if w.CorkOn() {
		t.Error("Cork:false should not draw cork")
	}
	col, ok := w.BgColor()
	if !ok || *col != (RGB{0x10, 0x20, 0x30}) {
		t.Errorf("BgColor = %v,%v; want #102030", col, ok)
	}

	w.Background = Background{Cork: true, Color: "#102030"} // cork over solid
	if !w.CorkOn() {
		t.Error("cork-over-color should still draw cork")
	}
	if _, ok := w.BgColor(); !ok {
		t.Error("cork-over-color should still have a fill color")
	}

	// Invalid hex → no color (renders transparent), no crash.
	w.Background = Background{Color: "nope"}
	if _, ok := w.BgColor(); ok {
		t.Error("invalid hex should yield no color")
	}
}

func TestBackgroundNormalizeLegacy(t *testing.T) {
	cases := []struct {
		in       Background
		wantCork bool
		wantCol  string
	}{
		{Background{Mode: BgCork}, true, ""},
		{Background{Mode: BgTransparent}, false, ""},
		{Background{Mode: BgSolid, Color: "#112233"}, false, "#112233"},
	}
	for _, c := range cases {
		b := c.in
		b.normalizeLegacy()
		if b.Cork != c.wantCork || b.Color != c.wantCol || b.Mode != "" {
			t.Errorf("normalizeLegacy(%v) = %+v; want cork=%v color=%q mode=\"\"",
				c.in, b, c.wantCork, c.wantCol)
		}
	}
}

func TestSerializeBackgroundSGR(t *testing.T) {
	c := NewCanvas(3, 1)
	// No DefaultBg → no background SGR emitted.
	if got := c.Serialize(); strings.Contains(got, "\x1b[48;2;") {
		t.Errorf("transparent canvas should not emit bg SGR, got %q", got)
	}
	bg := RGB{10, 20, 30}
	c.DefaultBg = &bg
	out := c.Serialize()
	if !strings.Contains(out, "\x1b[48;2;10;20;30m") {
		t.Errorf("solid canvas should emit bg SGR, got %q", out)
	}
}

func TestViewRendersAcrossBackgroundModes(t *testing.T) {
	for _, bg := range []Background{
		{Cork: true},                    // cork over transparent (default)
		{Cork: false},                   // clean terminal
		{Cork: true, Color: "#1c2233"},  // cork over a solid color
		{Cork: false, Color: "#1c2233"}, // solid only
		{Cork: false, Color: "bad"},     // invalid hex → graceful fallback
	} {
		ws := seedWorkspace()
		ws.Background = bg
		m := initialModel(ws)
		m.w, m.h = 80, 24
		m.stars = GenStarsForBoard(m.w, m.h, ws.ActiveBoard().GrainSeed)
		// Board view.
		if out := m.View(); out == "" {
			t.Errorf("empty board view for bg %v", bg)
		}
		// With the background menu open.
		m.bgMenu = NewBackgroundMenu(bg)
		if out := m.View(); out == "" {
			t.Errorf("empty menu view for bg %v", bg)
		}
	}
}

func TestBackgroundMenuSelection(t *testing.T) {
	// Open on a cork-on, transparent background.
	m := NewBackgroundMenu(Background{Cork: true})
	if got := m.Selected(); !got.Cork || got.Color != "" {
		t.Errorf("initial selection = %+v; want cork on, transparent", got)
	}

	// Cork is an independent toggle.
	m.Cursor = 0
	if !m.OnCork() {
		t.Fatal("expected cork row at index 0")
	}
	m.ToggleCork()
	if m.Selected().Cork {
		t.Error("ToggleCork should turn cork off")
	}
	m.ToggleCork() // back on

	// Move to the custom row and type a hex value — cork stays as-is.
	m.Cursor = len(BackgroundColors) + 1
	if !m.OnCustom() {
		t.Fatal("expected custom row")
	}
	m.Hex = "#"
	m.HexInput([]rune("1a2b3c"))
	got := m.Selected()
	if got.Color != "#1a2b3c" || !got.Cork {
		t.Errorf("custom selection = %+v; want #1a2b3c with cork on", got)
	}

	// Backspace shortens but keeps the leading '#'.
	for i := 0; i < 10; i++ {
		m.HexBackspace()
	}
	if m.Hex != "#" {
		t.Errorf("after backspacing all, Hex = %q; want \"#\"", m.Hex)
	}

	// A palette color lands on its color row, not the custom row.
	paletteHex := BackgroundColors[2].Hex
	m2 := NewBackgroundMenu(Background{Color: paletteHex})
	if m2.OnCustom() || m2.OnCork() {
		t.Errorf("palette color should select its color row (cursor=%d)", m2.Cursor)
	}
	if m2.Selected().Color != paletteHex {
		t.Errorf("palette selection color = %q; want %q", m2.Selected().Color, paletteHex)
	}
}
