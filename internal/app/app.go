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
	flag.BoolVar(&fresh, "fresh", false, "start with a fresh seeded workspace, ignore saved notes")
	flag.Parse()

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
