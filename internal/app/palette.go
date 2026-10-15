package app

// palette.go — the two color palettes and the switch between them.
//
// The renderer only ever sets a foreground color; the terminal's own
// background shows through (see render.go). That makes every color here
// implicitly a statement about what the background looks like. The
// original palette assumed a dark terminal — pastel inks around 0.85
// luma, near-white borders, honey-colored cork — which turns unreadable
// on a light background.
//
// So the palette is data, chosen once at startup by ApplyTheme, and the
// draw sites keep reading the same package-level vars they always did.

// Palette is a complete set of board colors. darkPalette() reproduces the
// colors redthread has always shipped; lightPalette() is its counterpart
// for a pale terminal background.
type Palette struct {
	// Cork texture, scattered directly over the terminal background.
	CorkDark    RGB
	CorkMid     RGB
	CorkLight   RGB
	CorkRust    RGB
	CorkWarm    RGB
	CorkPore    RGB
	CorkBlotch  RGB
	CorkPatchFg RGB

	// Pins and strings.
	PinRed   RGB
	PinHi    RGB
	PinDark  RGB
	StringRd RGB
	StringHi RGB

	// Drop shadow under notes.
	ShadowBG RGB
	ShadowMi RGB

	// Chrome: footer text, dimmed labels, the bright highlight, and the
	// tint used by the darken/vignette passes.
	Footer   RGB
	DimText  RGB
	Flash    RGB
	DarkTint RGB

	Tints            map[string]Tint
	SelBorderChoices []BorderChoice
}

// darkPalette is the original palette, unchanged. Every value here is
// byte-identical to what redthread shipped before the light theme existed,
// and TestDarkPaletteIsUnchanged pins that down.
func darkPalette() Palette {
	return Palette{
		CorkDark:    RGB{82, 52, 32},
		CorkMid:     RGB{122, 84, 56},
		CorkLight:   RGB{172, 126, 76},
		CorkRust:    RGB{146, 80, 44},  // warmer reddish-brown
		CorkWarm:    RGB{198, 148, 88}, // honey highlight
		CorkPore:    RGB{58, 36, 22},   // rare pores / holes
		CorkBlotch:  RGB{94, 64, 40},   // cork blotches
		CorkPatchFg: RGB{110, 74, 48},  // bigger brown "patch" fg

		PinRed:   RGB{230, 60, 60},
		PinHi:    RGB{255, 152, 152},
		PinDark:  RGB{140, 26, 26},
		StringRd: RGB{210, 44, 44},
		StringHi: RGB{245, 100, 100}, // brighter red for the pulled-end tip

		ShadowBG: RGB{34, 22, 14},
		ShadowMi: RGB{52, 34, 22},

		Footer:   RGB{154, 123, 90},
		DimText:  RGB{180, 150, 110},
		Flash:    RGB{240, 232, 210}, // slightly warm, not pure white
		DarkTint: RGB{18, 12, 8},     // darken/vignette passes

		Tints: map[string]Tint{
			"yellow": {
				Name:  "yellow",
				Paper: RGB{246, 220, 120}, Ink: RGB{250, 230, 145},
				Fiber: RGB{204, 176, 85}, Tape: RGB{220, 195, 110}, Edge: RGB{255, 240, 170},
			},
			"pink": {
				Name:  "pink",
				Paper: RGB{245, 175, 200}, Ink: RGB{250, 190, 210},
				Fiber: RGB{205, 130, 160}, Tape: RGB{220, 150, 180}, Edge: RGB{255, 200, 220},
			},
			"blue": {
				Name:  "blue",
				Paper: RGB{175, 210, 240}, Ink: RGB{190, 220, 248},
				Fiber: RGB{130, 170, 205}, Tape: RGB{150, 185, 220}, Edge: RGB{200, 225, 255},
			},
			"green": {
				Name:  "green",
				Paper: RGB{180, 230, 180}, Ink: RGB{200, 240, 200},
				Fiber: RGB{130, 190, 130}, Tape: RGB{155, 210, 155}, Edge: RGB{210, 245, 210},
			},
			"purple": {
				Name:  "purple",
				Paper: RGB{210, 180, 240}, Ink: RGB{225, 195, 248},
				Fiber: RGB{165, 130, 205}, Tape: RGB{190, 160, 225}, Edge: RGB{230, 205, 255},
			},
			"orange": {
				Name:  "orange",
				Paper: RGB{245, 200, 140}, Ink: RGB{250, 210, 155},
				Fiber: RGB{205, 155, 95}, Tape: RGB{222, 178, 120}, Edge: RGB{255, 220, 175},
			},
			"teal": {
				Name:  "teal",
				Paper: RGB{170, 230, 220}, Ink: RGB{190, 240, 230},
				Fiber: RGB{125, 185, 180}, Tape: RGB{150, 210, 200}, Edge: RGB{200, 245, 235},
			},
			"cream": {
				Name:  "cream",
				Paper: RGB{235, 225, 205}, Ink: RGB{245, 235, 215},
				Fiber: RGB{195, 185, 160}, Tape: RGB{215, 205, 185}, Edge: RGB{250, 245, 225},
			},
			"coral": {
				Name:  "coral",
				Paper: RGB{245, 180, 165}, Ink: RGB{250, 195, 180},
				Fiber: RGB{210, 135, 120}, Tape: RGB{225, 155, 140}, Edge: RGB{255, 205, 190},
			},
		},

		SelBorderChoices: []BorderChoice{
			{"warm white", RGB{245, 238, 220}},
			{"cool white", RGB{230, 240, 250}},
			{"bright", RGB{255, 255, 255}},
			{"cyan", RGB{100, 220, 235}},
			{"gold", RGB{220, 180, 80}},
			{"mint", RGB{170, 235, 200}},
			{"lavender", RGB{200, 180, 240}},
			{"amber", RGB{240, 200, 100}},
			{"teal", RGB{80, 200, 180}},
		},
	}
}

// lightPalette is the dark palette's mirror image for a pale terminal.
//
// The inversion is not a simple luma flip. Each tint keeps its hue so a
// "blue" note still reads as blue, but the roles swap: Ink becomes the
// most saturated and darkest value (it is body text, so it needs the most
// contrast), Paper sits a step lighter as the border, and Fiber — a
// decorative fleck — is the lightest of the three while staying under the
// readability threshold. Edge and Tape follow Paper.
func lightPalette() Palette {
	return Palette{
		// Cork: each speck sits the same perceptual distance *below* a pale
		// background as its dark counterpart sits above black, with hue
		// preserved and chroma eased off ~8%. Matching the step rather than
		// just "make it dark" is what keeps the grain reading as texture —
		// uniformly-dark specks turn the board into noise.
		CorkDark:    RGB{203, 161, 134},
		CorkMid:     RGB{175, 127, 92},
		CorkLight:   RGB{120, 90, 57},
		CorkRust:    RGB{193, 115, 73},
		CorkWarm:    RGB{96, 69, 36},
		CorkPore:    RGB{215, 181, 159},
		CorkBlotch:  RGB{192, 151, 118},
		CorkPatchFg: RGB{184, 138, 104},

		// Reds deepened — pure {230,60,60} goes muddy against pale gray.
		PinRed:   RGB{176, 28, 34},
		PinHi:    RGB{206, 62, 68},
		PinDark:  RGB{116, 16, 20},
		StringRd: RGB{164, 24, 30},
		StringHi: RGB{198, 52, 58},

		// Shadow: only a slight step below the background, mirroring how
		// the dark shadow is only a slight step above black. Anything
		// darker reads as a hole punched in the page rather than a shadow.
		ShadowBG: RGB{223, 200, 184},
		ShadowMi: RGB{214, 185, 166},

		Footer:   RGB{104, 88, 66},
		DimText:  RGB{92, 78, 58},
		Flash:    RGB{42, 34, 24}, // the "bright" accent inverts to near-black
		DarkTint: RGB{248, 246, 240},

		Tints: map[string]Tint{
			"yellow": {
				Name:  "yellow",
				Paper: RGB{124, 92, 12}, Ink: RGB{92, 68, 8},
				Fiber: RGB{139, 112, 41}, Tape: RGB{136, 104, 24}, Edge: RGB{78, 58, 6},
			},
			"pink": {
				Name:  "pink",
				Paper: RGB{150, 52, 92}, Ink: RGB{116, 34, 68},
				Fiber: RGB{156, 87, 118}, Tape: RGB{162, 68, 106}, Edge: RGB{98, 26, 58},
			},
			"blue": {
				Name:  "blue",
				Paper: RGB{34, 82, 134}, Ink: RGB{24, 60, 102},
				Fiber: RGB{84, 117, 155}, Tape: RGB{46, 94, 146}, Edge: RGB{18, 48, 86},
			},
			"green": {
				Name:  "green",
				Paper: RGB{38, 100, 46}, Ink: RGB{26, 76, 34},
				Fiber: RGB{85, 130, 89}, Tape: RGB{50, 112, 58}, Edge: RGB{20, 62, 26},
			},
			"purple": {
				Name:  "purple",
				Paper: RGB{92, 58, 142}, Ink: RGB{70, 42, 112},
				Fiber: RGB{123, 97, 158}, Tape: RGB{104, 70, 154}, Edge: RGB{58, 32, 94},
			},
			"orange": {
				Name:  "orange",
				Paper: RGB{146, 78, 20}, Ink: RGB{114, 58, 12},
				Fiber: RGB{153, 102, 55}, Tape: RGB{158, 90, 32}, Edge: RGB{96, 48, 8},
			},
			"teal": {
				Name:  "teal",
				Paper: RGB{18, 98, 96}, Ink: RGB{12, 74, 72},
				Fiber: RGB{73, 129, 127}, Tape: RGB{28, 110, 108}, Edge: RGB{8, 60, 58},
			},
			"cream": {
				Name:  "cream",
				Paper: RGB{104, 92, 68}, Ink: RGB{78, 68, 50},
				Fiber: RGB{120, 110, 91}, Tape: RGB{116, 104, 80}, Edge: RGB{64, 56, 40},
			},
			"coral": {
				Name:  "coral",
				Paper: RGB{158, 62, 44}, Ink: RGB{124, 44, 30},
				Fiber: RGB{161, 92, 77}, Tape: RGB{170, 76, 56}, Edge: RGB{104, 34, 22},
			},
		},

		// Same nine names in the same order — a board's saved
		// highlightColor is an index into this slice, so the order is part
		// of the on-disk format. "warm white" becomes a warm charcoal,
		// "bright" becomes near-black, and the hued entries keep their hue
		// at roughly 40% of their original luma.
		SelBorderChoices: []BorderChoice{
			{"warm white", RGB{68, 58, 44}},
			{"cool white", RGB{48, 58, 72}},
			{"bright", RGB{20, 20, 20}},
			{"cyan", RGB{20, 96, 108}},
			{"gold", RGB{124, 96, 24}},
			{"mint", RGB{40, 108, 78}},
			{"lavender", RGB{86, 68, 132}},
			{"amber", RGB{136, 100, 20}},
			{"teal", RGB{16, 94, 84}},
		},
	}
}

// themeIsLight tracks which palette ApplyTheme last installed, so a
// workspace still on auto can flip away from whatever is actually on
// screen rather than from a guess.
var themeIsLight bool

// currentThemeIsLight reports which palette is currently installed.
func currentThemeIsLight() bool { return themeIsLight }

// ApplyTheme installs a palette into the package-level color vars that
// every draw site reads. Call it once at startup, before the first render.
func ApplyTheme(light bool) {
	p := darkPalette()
	if light {
		p = lightPalette()
	}
	themeIsLight = light

	CorkDark = p.CorkDark
	CorkMid = p.CorkMid
	CorkLight = p.CorkLight
	CorkRust = p.CorkRust
	CorkWarm = p.CorkWarm
	CorkPore = p.CorkPore
	CorkBlotch = p.CorkBlotch
	CorkPatchFg = p.CorkPatchFg

	PinRed = p.PinRed
	PinHi = p.PinHi
	PinDark = p.PinDark
	StringRd = p.StringRd
	StringHi = p.StringHi

	ShadowBG = p.ShadowBG
	ShadowMi = p.ShadowMi

	Footer = p.Footer
	DimText = p.DimText
	Flash = p.Flash
	DarkTint = p.DarkTint

	Tints = p.Tints
	SelBorderChoices = p.SelBorderChoices
	SelBorder = SelBorderChoices[0].Color

	// The weighted shade pool is derived, so it has to be rebuilt whenever
	// the cork colors change. Duplicates bias the pick toward mid tones.
	CorkShades = []RGB{
		CorkDark, CorkDark,
		CorkMid, CorkMid, CorkMid,
		CorkLight, CorkLight,
		CorkRust,
		CorkWarm,
	}
}

// init keeps the package usable before Run() picks a theme — notably in
// tests, which construct models directly without going through app.Run.
func init() { ApplyTheme(false) }
