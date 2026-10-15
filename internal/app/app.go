package app

// app.go — top-level entry. Loads workspace (or seeds a fresh one) and
// launches the Bubble Tea program.

import (
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

// Run is the package's main entry point. cmd/redthread/main.go calls it.
func Run() {
	var fresh bool
	var themeFlag string
	flag.BoolVar(&fresh, "fresh", false, "start with a fresh seeded workspace, ignore saved notes")
	flag.StringVar(&themeFlag, "theme", "",
		"color palette: auto (detect the terminal background), light, or dark. Also settable with RT_THEME.")
	flag.Parse()

	if themeFlag != "" {
		if _, err := ParseThemeMode(themeFlag); err != nil {
			fmt.Fprintf(os.Stderr, "warning: %v — ignoring it\n", err)
		}
	}

	var ws *Workspace
	if !fresh {
		w, err := LoadWorkspace()
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not load notes (%v) — starting fresh\n", err)
		}
		ws = w
	}
	if ws == nil || len(ws.Boards) == 0 {
		ws = seedWorkspace()
	}

	// Pick the palette before anything renders. The flag beats RT_THEME,
	// which beats the value saved in notes.json, which beats auto-detection.
	// The detection itself writes an OSC 11 query to the terminal, so it has
	// to happen before Bubble Tea takes over the screen.
	//
	// `--theme=auto` is an override in its own right: it asks to re-detect
	// and to forget any saved choice. Only an absent flag and env var defer
	// to what the workspace saved.
	mode, explicit := themeModeFrom(themeFlag, os.Getenv("RT_THEME"))
	if !explicit {
		mode = ws.ThemeMode()
	}
	ApplyTheme(ResolveTheme(mode))

	// Record the choice: an explicit light/dark so the next run skips the
	// probe, an explicit auto so the next run performs it.
	if explicit {
		ws.SetThemeMode(mode)
	}

	if active := ws.ActiveBoard(); active != nil {
		active.ApplyGlobalBorder()
	}

	m := initialModel(ws)
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseAllMotion())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	_ = SaveWorkspace(ws)
}
