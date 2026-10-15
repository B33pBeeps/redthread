package app

// theme_test.go — guards the light/dark palette contract.
//
// The bug these tests exist for: every color in the original palette was
// tuned for a dark terminal (pastel inks around luma 0.85), so on a light
// background the note text washed out to near-invisible. The luma
// assertions below are the regression test — a light-palette ink that
// drifts bright again fails here.

import "testing"

// maxLightLuma — light-palette ink must be dark enough to read on a pale
// background. 0.5 is the same midpoint termenv uses to classify a
// terminal background as dark.
const maxLightLuma = 0.5

// minDarkLuma — dark-palette ink must stay bright enough to read on black.
const minDarkLuma = 0.5

func TestLightPaletteTintsAreDarkEnough(t *testing.T) {
	p := lightPalette()
	for name, tint := range p.Tints {
		for _, c := range []struct {
			field string
			col   RGB
		}{
			{"Ink", tint.Ink},
			{"Paper", tint.Paper},
			{"Fiber", tint.Fiber},
		} {
			if l := c.col.Brightness(); l >= maxLightLuma {
				t.Errorf("light tint %q %s = %s has luma %.3f; want < %.2f (would wash out on a light background)",
					name, c.field, c.col.Hex(), l, maxLightLuma)
			}
		}
	}
}

func TestDarkPaletteTintsAreBrightEnough(t *testing.T) {
	p := darkPalette()
	for name, tint := range p.Tints {
		for _, c := range []struct {
			field string
			col   RGB
		}{
			{"Ink", tint.Ink},
			{"Paper", tint.Paper},
		} {
			if l := c.col.Brightness(); l <= minDarkLuma {
				t.Errorf("dark tint %q %s = %s has luma %.3f; want > %.2f",
					name, c.field, c.col.Hex(), l, minDarkLuma)
			}
		}
	}
}

func TestLightPaletteBordersAreDarkEnough(t *testing.T) {
	p := lightPalette()
	for i, ch := range p.SelBorderChoices {
		if l := ch.Color.Brightness(); l >= maxLightLuma {
			t.Errorf("light border %d %q = %s has luma %.3f; want < %.2f",
				i, ch.Name, ch.Color.Hex(), l, maxLightLuma)
		}
	}
}

func TestDarkPaletteBordersAreBrightEnough(t *testing.T) {
	p := darkPalette()
	for i, ch := range p.SelBorderChoices {
		if l := ch.Color.Brightness(); l <= minDarkLuma {
			t.Errorf("dark border %d %q = %s has luma %.3f; want > %.2f",
				i, ch.Name, ch.Color.Hex(), l, minDarkLuma)
		}
	}
}

// The chrome colors (footer, dim text, flash highlight) are drawn straight
// onto the terminal background with no note paper behind them, so they are
// the most exposed to a background mismatch.
func TestLightPaletteChromeIsDarkEnough(t *testing.T) {
	p := lightPalette()
	for _, c := range []struct {
		field string
		col   RGB
	}{
		{"Flash", p.Flash},
		{"DimText", p.DimText},
		{"Footer", p.Footer},
		{"PinRed", p.PinRed},
		{"StringRd", p.StringRd},
	} {
		if l := c.col.Brightness(); l >= maxLightLuma {
			t.Errorf("light chrome %s = %s has luma %.3f; want < %.2f",
				c.field, c.col.Hex(), l, maxLightLuma)
		}
	}
}

// Texture (cork specks, shadow dither) is not text: it must not be as dark
// as ink, it must sit the same *distance* from the background as its dark
// counterpart does from black. A speck that merely satisfies "dark enough"
// is far too loud — the whole board reads as noise instead of grain.
//
// darkBgLuma is black; lightBgLuma is a representative pale terminal. The
// tolerance absorbs the rounding to 8-bit channels.
const (
	darkBgLuma  = 0.0
	lightBgLuma = 0.90
	stepTol     = 0.04
)

func TestLightTextureMirrorsDarkContrastSteps(t *testing.T) {
	d, l := darkPalette(), lightPalette()

	for _, c := range []struct {
		field      string
		dark, lite RGB
	}{
		{"CorkDark", d.CorkDark, l.CorkDark},
		{"CorkMid", d.CorkMid, l.CorkMid},
		{"CorkLight", d.CorkLight, l.CorkLight},
		{"CorkRust", d.CorkRust, l.CorkRust},
		{"CorkWarm", d.CorkWarm, l.CorkWarm},
		{"CorkPore", d.CorkPore, l.CorkPore},
		{"CorkBlotch", d.CorkBlotch, l.CorkBlotch},
		{"CorkPatchFg", d.CorkPatchFg, l.CorkPatchFg},
		{"ShadowBG", d.ShadowBG, l.ShadowBG},
		{"ShadowMi", d.ShadowMi, l.ShadowMi},
	} {
		darkStep := c.dark.Brightness() - darkBgLuma
		liteStep := lightBgLuma - c.lite.Brightness()
		if diff := darkStep - liteStep; diff > stepTol || diff < -stepTol {
			t.Errorf("%s: light step from background = %.3f, dark step = %.3f (diff %.3f > %.2f). "+
				"Texture must sit the same distance from the background in both themes.",
				c.field, liteStep, darkStep, diff, stepTol)
		}
		// And it must actually be *below* the light background, not above.
		if c.lite.Brightness() >= lightBgLuma {
			t.Errorf("%s = %s is not darker than the light background", c.field, c.lite.Hex())
		}
	}
}

// CorkShades is derived from the cork colors, so ApplyTheme has to rebuild
// it — a stale pool would keep painting the old theme's specks.
func TestApplyThemeRebuildsCorkShades(t *testing.T) {
	t.Cleanup(func() { ApplyTheme(false) })

	ApplyTheme(true)
	if len(CorkShades) == 0 {
		t.Fatal("CorkShades is empty after ApplyTheme(true)")
	}
	light := lightPalette()
	for i, col := range CorkShades {
		if col.Brightness() >= lightBgLuma {
			t.Errorf("light CorkShades[%d] = %s is not darker than the background", i, col.Hex())
		}
		if col != light.CorkDark && col != light.CorkMid && col != light.CorkLight &&
			col != light.CorkRust && col != light.CorkWarm {
			t.Errorf("light CorkShades[%d] = %s is not one of the light cork colors", i, col.Hex())
		}
	}

	ApplyTheme(false)
	if CorkShades[0] != CorkDark {
		t.Errorf("CorkShades[0] = %s; want CorkDark %s", CorkShades[0].Hex(), CorkDark.Hex())
	}
}

// Saved boards store a highlight color as an index into SelBorderChoices,
// and tints by name. Both palettes must expose the same shape or a saved
// notes.json breaks when the user switches themes.
func TestPalettesHaveMatchingShape(t *testing.T) {
	d, l := darkPalette(), lightPalette()

	if got, want := len(l.SelBorderChoices), len(d.SelBorderChoices); got != want {
		t.Fatalf("light SelBorderChoices has %d entries; want %d (saved highlightColor indexes into this)", got, want)
	}
	for i := range d.SelBorderChoices {
		if got, want := l.SelBorderChoices[i].Name, d.SelBorderChoices[i].Name; got != want {
			t.Errorf("SelBorderChoices[%d] name = %q in light, %q in dark; names must match across palettes", i, got, want)
		}
	}

	if got, want := len(l.Tints), len(d.Tints); got != want {
		t.Fatalf("light palette has %d tints; want %d", got, want)
	}
	for name := range d.Tints {
		if _, ok := l.Tints[name]; !ok {
			t.Errorf("light palette is missing tint %q", name)
		}
	}
	for _, name := range TintOrder {
		if _, ok := l.Tints[name]; !ok {
			t.Errorf("light palette is missing TintOrder tint %q", name)
		}
		if _, ok := d.Tints[name]; !ok {
			t.Errorf("dark palette is missing TintOrder tint %q", name)
		}
	}
}

// Every Tint must name itself correctly, since GetTint falls back by name.
func TestPaletteTintNamesMatchKeys(t *testing.T) {
	for label, p := range map[string]Palette{"dark": darkPalette(), "light": lightPalette()} {
		for key, tint := range p.Tints {
			if tint.Name != key {
				t.Errorf("%s palette: tint under key %q has Name %q", label, key, tint.Name)
			}
		}
	}
}

// ApplyTheme swaps the package-level vars every draw site reads.
func TestApplyThemeSwapsPackageVars(t *testing.T) {
	t.Cleanup(func() { ApplyTheme(false) })

	ApplyTheme(false)
	darkInk := GetTint("blue").Ink
	darkFooter := Footer

	ApplyTheme(true)
	lightInk := GetTint("blue").Ink
	lightFooter := Footer

	if darkInk == lightInk {
		t.Error("ApplyTheme(true) did not change the blue tint ink")
	}
	if darkFooter == lightFooter {
		t.Error("ApplyTheme(true) did not change Footer")
	}
	if l := lightInk.Brightness(); l >= maxLightLuma {
		t.Errorf("after ApplyTheme(true) blue ink luma = %.3f; want < %.2f", l, maxLightLuma)
	}
	if l := darkInk.Brightness(); l <= minDarkLuma {
		t.Errorf("after ApplyTheme(false) blue ink luma = %.3f; want > %.2f", l, minDarkLuma)
	}

	ApplyTheme(false)
	if GetTint("blue").Ink != darkInk {
		t.Error("ApplyTheme(false) did not restore the dark blue ink")
	}
	if Footer != darkFooter {
		t.Error("ApplyTheme(false) did not restore Footer")
	}
}

// ApplyTheme must also re-point SelBorder, which ApplyGlobalBorder and the
// draw path both read.
func TestApplyThemeResetsSelBorder(t *testing.T) {
	t.Cleanup(func() { ApplyTheme(false) })

	ApplyTheme(true)
	if SelBorder != SelBorderChoices[0].Color {
		t.Errorf("SelBorder = %s; want the first light border choice %s",
			SelBorder.Hex(), SelBorderChoices[0].Color.Hex())
	}
	if l := SelBorder.Brightness(); l >= maxLightLuma {
		t.Errorf("light SelBorder luma = %.3f; want < %.2f", l, maxLightLuma)
	}
}

// The dark palette must remain byte-identical to the colors shipped before
// this change, so existing users see no difference at all.
func TestDarkPaletteIsUnchanged(t *testing.T) {
	ApplyTheme(false)
	t.Cleanup(func() { ApplyTheme(false) })

	for _, tc := range []struct {
		name string
		got  RGB
		want RGB
	}{
		{"CorkDark", CorkDark, RGB{82, 52, 32}},
		{"CorkMid", CorkMid, RGB{122, 84, 56}},
		{"CorkLight", CorkLight, RGB{172, 126, 76}},
		{"CorkRust", CorkRust, RGB{146, 80, 44}},
		{"CorkWarm", CorkWarm, RGB{198, 148, 88}},
		{"CorkPore", CorkPore, RGB{58, 36, 22}},
		{"CorkBlotch", CorkBlotch, RGB{94, 64, 40}},
		{"CorkPatchFg", CorkPatchFg, RGB{110, 74, 48}},
		{"PinRed", PinRed, RGB{230, 60, 60}},
		{"PinHi", PinHi, RGB{255, 152, 152}},
		{"PinDark", PinDark, RGB{140, 26, 26}},
		{"StringRd", StringRd, RGB{210, 44, 44}},
		{"StringHi", StringHi, RGB{245, 100, 100}},
		{"ShadowBG", ShadowBG, RGB{34, 22, 14}},
		{"ShadowMi", ShadowMi, RGB{52, 34, 22}},
		{"Footer", Footer, RGB{154, 123, 90}},
		{"DimText", DimText, RGB{180, 150, 110}},
		{"Flash", Flash, RGB{240, 232, 210}},
		{"DarkTint", DarkTint, RGB{18, 12, 8}},
	} {
		if tc.got != tc.want {
			t.Errorf("dark %s = %s; want %s (dark palette must not change)",
				tc.name, tc.got.Hex(), tc.want.Hex())
		}
	}

	yellow := GetTint("yellow")
	if yellow.Paper != (RGB{246, 220, 120}) || yellow.Ink != (RGB{250, 230, 145}) {
		t.Errorf("dark yellow tint = %+v; want the original paper #f6dc78 / ink #fae691", yellow)
	}
}
