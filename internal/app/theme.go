package app

// theme.go — colors, tint palettes, dither tables, character pools,
// and fancy-text (Unicode math-bold / double-struck) helpers.
//
// No background colors are ever returned; we only set fg and rely on the
// terminal's own bg showing through.

import (
	"fmt"
	"strings"
)

// RGB is a 24-bit color.
type RGB struct{ R, G, B uint8 }

func (c RGB) SGR() string {
	return fmt.Sprintf("\x1b[38;2;%d;%d;%dm", c.R, c.G, c.B)
}

// BgSGR emits the background-color SGR for this color. The renderer is
// otherwise foreground-only; this is used for the optional solid board
// background.
func (c RGB) BgSGR() string {
	return fmt.Sprintf("\x1b[48;2;%d;%d;%dm", c.R, c.G, c.B)
}

// ParseHexColor parses "#rrggbb" or "rrggbb" (case-insensitive) into an RGB.
// Returns ok=false for any malformed input.
func ParseHexColor(s string) (RGB, bool) {
	s = strings.TrimSpace(s)
	s = strings.TrimPrefix(s, "#")
	if len(s) != 6 {
		return RGB{}, false
	}
	var vals [3]uint8
	for i := 0; i < 3; i++ {
		hi, ok1 := hexNibble(s[i*2])
		lo, ok2 := hexNibble(s[i*2+1])
		if !ok1 || !ok2 {
			return RGB{}, false
		}
		vals[i] = hi<<4 | lo
	}
	return RGB{R: vals[0], G: vals[1], B: vals[2]}, true
}

func hexNibble(b byte) (uint8, bool) {
	switch {
	case b >= '0' && b <= '9':
		return b - '0', true
	case b >= 'a' && b <= 'f':
		return b - 'a' + 10, true
	case b >= 'A' && b <= 'F':
		return b - 'A' + 10, true
	}
	return 0, false
}

func (c RGB) Hex() string {
	return fmt.Sprintf("#%02x%02x%02x", c.R, c.G, c.B)
}

func (c RGB) Dimmed(f float32) RGB {
	return RGB{uint8(float32(c.R) * f), uint8(float32(c.G) * f), uint8(float32(c.B) * f)}
}

func (c RGB) Brighter(f float32) RGB {
	up := func(v uint8) uint8 {
		x := float32(v) + (255-float32(v))*f
		if x > 255 {
			x = 255
		}
		return uint8(x)
	}
	return RGB{up(c.R), up(c.G), up(c.B)}
}

// Lerp blends two colors linearly (0 → a, 1 → b).
func Lerp(a, b RGB, t float32) RGB {
	if t < 0 {
		t = 0
	}
	if t > 1 {
		t = 1
	}
	return RGB{
		R: uint8(float32(a.R)*(1-t) + float32(b.R)*t),
		G: uint8(float32(a.G)*(1-t) + float32(b.G)*t),
		B: uint8(float32(a.B)*(1-t) + float32(b.B)*t),
	}
}

// Brightness — Rec 601 luma, used by the monochrome dither pass.
func (c RGB) Brightness() float32 {
	return (0.299*float32(c.R) + 0.587*float32(c.G) + 0.114*float32(c.B)) / 255.0
}

// --- cork / background -------------------------------------------------

var (
	CorkDark    = RGB{82, 52, 32}
	CorkMid     = RGB{122, 84, 56}
	CorkLight   = RGB{172, 126, 76}
	CorkRust    = RGB{146, 80, 44}  // warmer reddish-brown
	CorkWarm    = RGB{198, 148, 88} // honey highlight
	CorkPore    = RGB{58, 36, 22}   // rare pores / holes
	CorkBlotch  = RGB{94, 64, 40}   // cork blotches
	CorkPatchFg = RGB{110, 74, 48}  // bigger brown "patch" fg (with `▒` density)

	PinRed   = RGB{230, 60, 60}
	PinHi    = RGB{255, 152, 152}
	PinDark  = RGB{140, 26, 26}
	StringRd = RGB{210, 44, 44}
	StringHi = RGB{245, 100, 100} // brighter red for the pulled-end tip
	ShadowBG = RGB{34, 22, 14}
	ShadowMi = RGB{52, 34, 22}
	Footer   = RGB{154, 123, 90}
	DimText  = RGB{180, 150, 110}
	Flash    = RGB{240, 232, 210} // slightly warm, not pure white
	DarkTint = RGB{18, 12, 8}     // for darken/vignette passes

	// SelBorder — legacy default (first entry of SelBorderChoices).
	// Per-note colors are preferred: Note.BorderColor picks an index.
	SelBorder = SelBorderChoices[0].Color
)

// Weighted shade pool — duplicates bias the random pick toward common
// mid tones, with occasional brighter/rustier accents.
// BorderChoice pairs a display name with an RGB for the border palette.
type BorderChoice struct {
	Name  string
	Color RGB
}

// SelBorderChoices — the 9 border colors available for notes. Keys 1-9
// map to these in order; `c` cycles through them.
var SelBorderChoices = []BorderChoice{
	{"warm white", RGB{245, 238, 220}},
	{"cool white", RGB{230, 240, 250}},
	{"bright", RGB{255, 255, 255}},
	{"cyan", RGB{100, 220, 235}},
	{"gold", RGB{220, 180, 80}},
	{"mint", RGB{170, 235, 200}},
	{"lavender", RGB{200, 180, 240}},
	{"amber", RGB{240, 200, 100}},
	{"teal", RGB{80, 200, 180}},
}

// --- background choices ------------------------------------------------

// BackgroundColor is one fill option in the background picker. Hex "" means
// transparent — the terminal/tmux background shows through (the default).
type BackgroundColor struct {
	Name string
	Hex  string
}

// BackgroundColors — the fill colors offered in the background menu, in
// order. "terminal" keeps today's transparency; the rest are the background
// colors of the dark editor/terminal themes developers actually use, so the
// picker feels familiar. The menu appends a "custom hex" entry after these.
// Cork is an independent toggle, not a list entry.
var BackgroundColors = []BackgroundColor{
	{"terminal (transparent)", ""},
	{"true black", "#000000"},
	{"github dark", "#0d1117"},
	{"tokyo night", "#1a1b26"},
	{"catppuccin", "#1e1e2e"},
	{"one dark", "#282c34"},
	{"dracula", "#282a36"},
	{"gruvbox", "#282828"},
	{"nord", "#2e3440"},
	{"solarized", "#002b36"},
}

var CorkShades = []RGB{
	CorkDark, CorkDark,
	CorkMid, CorkMid, CorkMid,
	CorkLight, CorkLight,
	CorkRust,
	CorkWarm,
}

// Chars used to texture the cork surface — main pool. More variety gives
// the cork a busier, more ASCII-art feel.
var StarChars = []rune{
	'.', ',', '·', '\'', ':', ';', '`', '~', '˙',
	'ˏ', 'ˎ', '‚', '„', '•',
}

// Rare bigger cork pores / pinholes.
var PoreChars = []rune{'∘', '°', '◦', '○'}

// Extremely rare cork blotches (imperfections).
var BlotchChars = []rune{'░', '▒'}

// Chars used for the note's own paper-fiber flecks.
var FiberChars = []rune{'.', '·', ',', '\'', '˙'}

// Chars used inside the drop shadow for texture.
var ShadowChars = []rune{'▒', '░', '▓', '▞', '▚'}

// --- note tints --------------------------------------------------------

// A Tint is a named color set for a sticky-note look.
type Tint struct {
	Name  string
	Paper RGB // main ink / border color
	Ink   RGB // body text color (slightly lighter or darker for contrast)
	Fiber RGB // paper-fiber dot color
	Tape  RGB // tape strip color (tape on top of the note)
	Edge  RGB // edge highlight when selected
}

var Tints = map[string]Tint{
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
}

// TintOrder — 1-9 in order.
var TintOrder = []string{
	"yellow", "pink", "blue", "green", "purple",
	"orange", "teal", "cream", "coral",
}

func GetTint(name string) Tint {
	if t, ok := Tints[name]; ok {
		return t
	}
	return Tints["yellow"]
}

// --- dither ------------------------------------------------------------

// Bayer 4x4 ordered-dither matrix (0..15).
var Bayer4x4 = [4][4]int{
	{0, 8, 2, 10},
	{12, 4, 14, 6},
	{3, 11, 1, 9},
	{15, 7, 13, 5},
}

var HalftoneLadder = []rune{' ', '░', '▒', '▓', '█'}

// DitherShade returns a halftone char for a given (x,y) and density 0..1.
func DitherShade(x, y int, density float32) rune {
	if density <= 0 {
		return ' '
	}
	if density >= 1 {
		return '█'
	}
	threshold := float32(Bayer4x4[((y%4)+4)%4][((x%4)+4)%4]) / 16.0
	adj := density + (threshold-0.5)*0.25
	switch {
	case adj < 0.2:
		return ' '
	case adj < 0.4:
		return '░'
	case adj < 0.6:
		return '▒'
	case adj < 0.8:
		return '▓'
	default:
		return '█'
	}
}

// HalftoneChar is a simple level lookup (0..4).
func HalftoneChar(level int) rune {
	if level < 0 {
		level = 0
	}
	if level >= len(HalftoneLadder) {
		level = len(HalftoneLadder) - 1
	}
	return HalftoneLadder[level]
}

// --- text styles --------------------------------------------------------
//
// Maps ASCII letters/digits to Unicode math-alphabet variants. Visually
// the title/body look like different fonts in the user's terminal; the
// actual Unicode codepoints are rendered by the terminal's font.

type TextStyleMode int

const (
	TextPlain   TextStyleMode = iota // plain
	TextBold                         // 𝗕𝗼𝗹𝗱 — math sans-serif bold
	TextItalic                       // 𝘐𝘵𝘢𝘭𝘪𝘤 — math sans-serif italic
	TextBoldIt                       // 𝘽𝙤𝙡𝙙 𝙄𝙩 — math sans-serif bold italic
	TextScript                       // 𝓢𝓬𝓻𝓲𝓹𝓽 — math bold script
	TextFraktur                      // 𝕱𝖗𝖆𝖐𝖙𝖚𝖗 — math bold fraktur
	TextDouble                       // 𝔻𝕠𝕦𝕓𝕝𝕖 — math double-struck
	TextMono                         // 𝙼𝚘𝚗𝚘 — math monospace
)

var TextModes = []TextStyleMode{
	TextPlain, TextBold, TextItalic, TextBoldIt,
	TextScript, TextFraktur, TextDouble, TextMono,
}

func (m TextStyleMode) Name() string {
	switch m {
	case TextBold:
		return "bold"
	case TextItalic:
		return "italic"
	case TextBoldIt:
		return "bold italic"
	case TextScript:
		return "script"
	case TextFraktur:
		return "fraktur"
	case TextDouble:
		return "double-struck"
	case TextMono:
		return "monospace"
	default:
		return "plain"
	}
}

func (m TextStyleMode) Sample() string { return StyleText("Aa Xx 12", m) }

// ModeAttr returns attribute bits to OR into cell renders. Unicode-math
// modes provide no ANSI attributes of their own (the visual "boldness"
// comes from the codepoint itself), so this always returns 0.
func ModeAttr(_ TextStyleMode) uint8 { return 0 }

// StyleText maps ASCII letters/digits to the mode's Unicode variant.
func StyleText(s string, mode TextStyleMode) string {
	if mode == TextPlain {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) * 4)
	for _, r := range s {
		b.WriteRune(styleRune(r, mode))
	}
	return b.String()
}

// StyleViewText applies styleRune to a pre-rendered string while passing
// through ANSI CSI escape sequences untouched. Useful when post-processing
// the output of a third-party widget (e.g. bubbles/textarea) so the visible
// glyphs reflect the board's font without breaking cursor highlighting.
func StyleViewText(s string, mode TextStyleMode) string {
	if mode == TextPlain {
		return s
	}
	var b strings.Builder
	b.Grow(len(s) * 4)
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
			b.WriteString(s[i:j])
			i = j
			continue
		}
		r, size := utf8Decode(s[i:])
		if size == 0 {
			b.WriteByte(s[i])
			i++
			continue
		}
		b.WriteRune(styleRune(r, mode))
		i += size
	}
	return b.String()
}

// styleRune — Unicode block offsets:
//
//	sans-serif bold:         A..Z=0x1D5D4  a..z=0x1D5EE  0..9=0x1D7EC
//	sans-serif italic:       A..Z=0x1D608  a..z=0x1D622  (digits: bold)
//	sans-serif bold italic:  A..Z=0x1D63C  a..z=0x1D656  (digits: bold)
//	bold script:             A..Z=0x1D4D0  a..z=0x1D4EA  (digits: bold)
//	fraktur bold:            A..Z=0x1D56C  a..z=0x1D586  (digits: bold)
//	double-struck:           A..Z=0x1D538  a..z=0x1D552  0..9=0x1D7D8
//	  (holes: C H N P Q R Z at BMP: 0x2102 0x210D 0x2115 0x2119 0x211A 0x211D 0x2124)
//	monospace:               A..Z=0x1D670  a..z=0x1D68A  0..9=0x1D7F6
func styleRune(r rune, mode TextStyleMode) rune {
	switch mode {
	case TextBold:
		switch {
		case r >= 'A' && r <= 'Z':
			return 0x1D5D4 + (r - 'A')
		case r >= 'a' && r <= 'z':
			return 0x1D5EE + (r - 'a')
		case r >= '0' && r <= '9':
			return 0x1D7EC + (r - '0')
		}
	case TextItalic:
		switch {
		case r >= 'A' && r <= 'Z':
			return 0x1D608 + (r - 'A')
		case r >= 'a' && r <= 'z':
			return 0x1D622 + (r - 'a')
		case r >= '0' && r <= '9':
			return 0x1D7EC + (r - '0')
		}
	case TextBoldIt:
		switch {
		case r >= 'A' && r <= 'Z':
			return 0x1D63C + (r - 'A')
		case r >= 'a' && r <= 'z':
			return 0x1D656 + (r - 'a')
		case r >= '0' && r <= '9':
			return 0x1D7EC + (r - '0')
		}
	case TextScript:
		switch {
		case r >= 'A' && r <= 'Z':
			return 0x1D4D0 + (r - 'A')
		case r >= 'a' && r <= 'z':
			return 0x1D4EA + (r - 'a')
		case r >= '0' && r <= '9':
			return 0x1D7EC + (r - '0')
		}
	case TextFraktur:
		switch {
		case r >= 'A' && r <= 'Z':
			return 0x1D56C + (r - 'A')
		case r >= 'a' && r <= 'z':
			return 0x1D586 + (r - 'a')
		case r >= '0' && r <= '9':
			return 0x1D7EC + (r - '0')
		}
	case TextDouble:
		switch r {
		case 'C':
			return 0x2102
		case 'H':
			return 0x210D
		case 'N':
			return 0x2115
		case 'P':
			return 0x2119
		case 'Q':
			return 0x211A
		case 'R':
			return 0x211D
		case 'Z':
			return 0x2124
		}
		switch {
		case r >= 'A' && r <= 'Z':
			return 0x1D538 + (r - 'A')
		case r >= 'a' && r <= 'z':
			return 0x1D552 + (r - 'a')
		case r >= '0' && r <= '9':
			return 0x1D7D8 + (r - '0')
		}
	case TextMono:
		switch {
		case r >= 'A' && r <= 'Z':
			return 0x1D670 + (r - 'A')
		case r >= 'a' && r <= 'z':
			return 0x1D68A + (r - 'a')
		case r >= '0' && r <= '9':
			return 0x1D7F6 + (r - '0')
		}
	}
	return r
}
