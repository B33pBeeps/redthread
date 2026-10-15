package app

// extedit.go — open the selected note in $EDITOR.
//
// The board's own editor is deliberately small: a textarea in a card. For
// anything longer than a few lines people want their real editor, with
// their bindings and their syntax highlighting. This hands the note to
// $EDITOR as a markdown file and reads it back.
//
// The file format is the same first-line-is-title convention the built-in
// editor uses, so the two agree about what a note's text means.

import (
	"fmt"
	"os"
	"os/exec"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// resolveEditor returns the user's editor command line. VISUAL wins over
// EDITOR by long-standing convention: EDITOR may be a line editor (ed),
// VISUAL is the full-screen one.
//
// Returns ok=false when neither is set. Falling back to vi would be
// hostile — dropping someone into an editor they may not know how to
// leave, from a TUI they were happily using.
func resolveEditor() (string, bool) {
	for _, key := range []string{"VISUAL", "EDITOR"} {
		if v := strings.TrimSpace(os.Getenv(key)); v != "" {
			return v, true
		}
	}
	return "", false
}

// editorCommand splits the editor setting into a command and arguments and
// appends the path. $EDITOR is a command line, not a bare program name —
// "code -w" and "emacsclient -nw" are both ordinary values.
//
// Fields-splitting does not handle quoted paths with spaces; a user whose
// editor lives at "/My Apps/vim" needs a wrapper script. That is the same
// limitation git and most other tools have.
func editorCommand(path string) (string, []string) {
	line, ok := resolveEditor()
	if !ok {
		return "", nil
	}
	parts := strings.Fields(line)
	if len(parts) == 0 {
		return "", nil
	}
	return parts[0], append(parts[1:], path)
}

// composeNoteFile renders a note as the text handed to the editor: the
// title, a blank line, then the body. A trailing newline because every
// editor expects one and many add it silently.
func composeNoteFile(title, body string) string {
	var b strings.Builder
	b.WriteString(title)
	b.WriteString("\n")
	if body != "" {
		b.WriteString("\n")
		b.WriteString(body)
		if !strings.HasSuffix(body, "\n") {
			b.WriteString("\n")
		}
	}
	return b.String()
}

// parseNoteFile reads the editor's output back into (title, body). It
// delegates to the same splitter the clipboard paste uses so the built-in
// editor, a paste, and an external edit all agree.
func parseNoteFile(s string) (string, string) {
	title, body := splitClipboardText(s)
	// Editors add a trailing newline; keep the body as the user sees it.
	return title, strings.TrimRight(body, "\n")
}

// writeNoteTempFile writes the note to a temp .md file and returns its
// path plus a cleanup func. The .md extension matters: it is what makes
// the user's editor turn on markdown highlighting and wrapping.
//
// cleanup is safe to call more than once.
func writeNoteTempFile(n *Note) (string, func(), error) {
	f, err := os.CreateTemp("", "redthread-*.md")
	if err != nil {
		return "", func() {}, err
	}
	path := f.Name()
	cleanup := func() { _ = os.Remove(path) }

	if _, err := f.WriteString(composeNoteFile(n.Title, n.Body)); err != nil {
		f.Close()
		cleanup()
		return "", func() {}, err
	}
	if err := f.Close(); err != nil {
		cleanup()
		return "", func() {}, err
	}
	return path, cleanup, nil
}

// externalEditDoneMsg reports the outcome of an external edit back into
// the update loop.
type externalEditDoneMsg struct {
	noteID  string
	path    string
	cleanup func()
	err     error
}

// openInEditor builds the command that suspends the TUI, runs the editor,
// and posts an externalEditDoneMsg when it exits. Returns a nil Cmd (and a
// reason) when the edit cannot be started.
func openInEditor(n *Note) (tea.Cmd, string) {
	if n == nil {
		return nil, "select a note first"
	}
	if _, ok := resolveEditor(); !ok {
		return nil, "$EDITOR is not set"
	}

	path, cleanup, err := writeNoteTempFile(n)
	if err != nil {
		return nil, "could not create a temp file: " + err.Error()
	}

	name, args := editorCommand(path)
	if name == "" {
		cleanup()
		return nil, "$EDITOR is not set"
	}

	c := exec.Command(name, args...)
	noteID := n.ID
	return tea.ExecProcess(c, func(err error) tea.Msg {
		return externalEditDoneMsg{noteID: noteID, path: path, cleanup: cleanup, err: err}
	}), ""
}

// applyExternalEdit reads the temp file back into the note. It returns a
// toast describing what happened, and whether the note actually changed.
//
// Every failure path leaves the note untouched: a non-zero editor exit, an
// unreadable file, or a file the user emptied entirely. The last one is
// the important one — `:q!` in vim on a file you did not mean to open
// should not blank the note.
func applyExternalEdit(n *Note, msg externalEditDoneMsg) (toast string, changed bool) {
	if msg.cleanup != nil {
		defer msg.cleanup()
	}
	if n == nil {
		return "the note went away while the editor was open", false
	}
	if msg.err != nil {
		return fmt.Sprintf("editor exited with an error (%v) — note unchanged", msg.err), false
	}

	data, err := os.ReadFile(msg.path)
	if err != nil {
		return "could not read the file back — note unchanged", false
	}

	title, body := parseNoteFile(string(data))
	if title == "" && strings.TrimSpace(body) == "" {
		return "file came back empty — note unchanged", false
	}
	if title == n.Title && body == n.Body {
		return "no changes", false
	}
	n.Title = title
	n.Body = body
	return "updated from $EDITOR", true
}
