package app

// vimedit.go — connects VimState to the note card.
//
// The two editors split the work by mode. In insert mode the textarea
// does what it is good at: typing, wrapping, its own cursor. In normal
// and visual mode VimState owns the buffer and this file renders it,
// because the textarea's cursor cannot be positioned reliably across
// wrapped lines (see the note at the top of vim.go).
//
// The buffer is handed across on every mode change, so both sides always
// agree about the text.

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// vimRender draws the buffer with a block cursor, sized to the card's
// body area. Returns the lines to splice into the frame.
func vimRender(v *VimState, width, height int, ink, cursorFg, cursorBg RGB) string {
	if width < 1 {
		width = 1
	}
	// Index by rune: a byte index would split multi-byte characters and
	// render mojibake, and Col is a character offset anyway.
	rawLines := strings.Split(v.Text(), "\n")
	lines := make([][]rune, len(rawLines))
	for i, l := range rawLines {
		lines[i] = []rune(l)
	}

	// Scroll so the cursor row is visible.
	top := 0
	if height > 0 && v.Row >= height {
		top = v.Row - height + 1
	}

	selR1, selC1, selR2, selC2 := -1, -1, -1, -1
	if v.Mode == VimVisual {
		selR1, selC1, selR2, selC2 = v.visualSpan()
	}

	inkStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(ink.Hex()))
	curStyle := lipgloss.NewStyle().
		Foreground(lipgloss.Color(cursorFg.Hex())).
		Background(lipgloss.Color(cursorBg.Hex()))

	var out []string
	for i := top; i < len(lines) && (height <= 0 || len(out) < height); i++ {
		line := lines[i]
		var b strings.Builder
		// Render at least one cell so an empty line can still show the
		// cursor.
		n := len(line)
		if n == 0 {
			n = 1
		}
		for col := 0; col < n; col++ {
			ch := " "
			if col < len(line) {
				ch = string(line[col])
			}
			switch {
			case i == v.Row && col == v.Col:
				b.WriteString(curStyle.Render(ch))
			case inVisualSpan(i, col, selR1, selC1, selR2, selC2):
				b.WriteString(curStyle.Render(ch))
			default:
				b.WriteString(inkStyle.Render(ch))
			}
		}
		out = append(out, b.String())
	}
	return strings.Join(out, "\n")
}

func inVisualSpan(row, col, r1, c1, r2, c2 int) bool {
	if r1 < 0 {
		return false
	}
	if row < r1 || row > r2 {
		return false
	}
	if row == r1 && col < c1 {
		return false
	}
	if row == r2 && col > c2 {
		return false
	}
	return true
}

// vimModeLabel is the footer hint for the current mode.
func vimModeLabel(v *VimState) string {
	switch v.Mode {
	case VimInsert:
		return "-- INSERT --  esc: normal  •  ctrl+s: save"
	case VimVisual:
		return "-- VISUAL --  d/c/y: operate  •  esc: cancel"
	default:
		return "-- NORMAL --  i: insert  •  esc: place back  •  ctrl+s: save"
	}
}
