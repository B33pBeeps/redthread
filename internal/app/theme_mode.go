package app

// theme_mode.go — choosing between the light and dark palettes.
//
// Detection is a best-effort OSC 11 query to the terminal ("what is your
// background color?"). Plenty of setups swallow it — tmux without
// allow-passthrough, some ssh sessions, anything that is not a TTY — so
// the answer is always overridable by flag or environment variable, and
// the fallback when the probe learns nothing is the dark palette, which is
// what redthread has always rendered.

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// ThemeMode is the user's stated preference for palette selection.
type ThemeMode int

const (
	ThemeAuto  ThemeMode = iota // probe the terminal
	ThemeDark                   // force the dark palette
	ThemeLight                  // force the light palette
)

func (m ThemeMode) String() string {
	switch m {
	case ThemeDark:
		return "dark"
	case ThemeLight:
		return "light"
	default:
		return "auto"
	}
}

// ParseThemeMode reads "auto" / "light" / "dark", case-insensitively.
// The empty string means auto.
func ParseThemeMode(s string) (ThemeMode, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "auto":
		return ThemeAuto, nil
	case "dark":
		return ThemeDark, nil
	case "light":
		return ThemeLight, nil
	default:
		return ThemeAuto, fmt.Errorf("unknown theme %q (want auto, light, or dark)", s)
	}
}

// themeModeFrom resolves the effective mode from the --theme flag and the
// RT_THEME environment variable, in that order of precedence. An
// unparseable value is ignored rather than fatal — a typo in a shell rc
// should not stop the board from opening.
//
// The second return distinguishes "the user asked for auto" from "the user
// said nothing". Both yield ThemeAuto, but only the former should override
// a theme saved in the workspace: `--theme=auto` means *re-detect*, and
// without this the saved value would silently win.
func themeModeFrom(flagVal, envVal string) (mode ThemeMode, explicit bool) {
	if flagVal != "" {
		if m, err := ParseThemeMode(flagVal); err == nil {
			return m, true
		}
		// A malformed flag falls through to the env var, then to the saved
		// choice — it must not read as an explicit request for auto.
	}
	if envVal != "" {
		if m, err := ParseThemeMode(envVal); err == nil {
			return m, true
		}
	}
	return ThemeAuto, false
}

// hasDarkBackground asks the terminal for its background color. On a
// non-TTY, or when the terminal ignores the query, lipgloss reports a
// zero-value (black) background, so this returns true — dark.
func hasDarkBackground() bool { return lipgloss.HasDarkBackground() }

// resolveTheme turns a mode into "should we use the light palette?".
// probe is injected so the decision is testable without a terminal.
func resolveTheme(mode ThemeMode, probe func() bool) bool {
	switch mode {
	case ThemeLight:
		return true
	case ThemeDark:
		return false
	default:
		return !probe()
	}
}

// ResolveTheme is resolveTheme against the real terminal.
func ResolveTheme(mode ThemeMode) bool { return resolveTheme(mode, hasDarkBackground) }
