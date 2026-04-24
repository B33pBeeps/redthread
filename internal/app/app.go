package app

// app.go — top-level entry. Loads board (or seeds a fresh one), launches
// the Bubble Tea program with alt-screen + full mouse motion.

import (
	"flag"
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"
)

// Run is the package's main entry point. cmd/redthread/main.go calls it.
func Run() {
	var fresh bool
	flag.BoolVar(&fresh, "fresh", false, "start with a fresh seeded board, ignore saved notes")
	flag.Parse()

	var board *Board
	if !fresh {
		b, err := LoadBoard()
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: could not load notes (%v) — starting fresh\n", err)
		}
		board = b
	}
	if board == nil || len(board.Notes) == 0 {
		board = seedBoard()
	}
	board.ApplyGlobalBorder()

	m := initialModel(board)
	p := tea.NewProgram(m, tea.WithAltScreen(), tea.WithMouseAllMotion())
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	_ = SaveBoard(board)
}
