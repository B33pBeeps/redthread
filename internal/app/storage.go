package app

// storage.go — loads and saves the workspace (multiple boards) to a JSON
// file under the XDG data directory, with debounced writes. Schema v4
// adds the workspace envelope; v3 single-board files migrate forward.

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const schemaVersion = 5

// diskFile is the v4 envelope. v3 fields (TextMode, Notes, Strings, Zoom,
// HighlightColor) are kept here as optional fallbacks — when a v3 file is
// loaded, those fields are wrapped into a single board.
type diskFile struct {
	SchemaVersion int         `json:"schemaVersion"`
	ActiveIdx     int         `json:"activeIdx,omitempty"`
	Background    *Background `json:"background,omitempty"`
	Boards        []*Board    `json:"boards,omitempty"`

	// Legacy v3 fields — read on load, never written.
	LegacyTextMode       TextStyleMode `json:"textMode,omitempty"`
	LegacyZoom           int           `json:"zoom,omitempty"`
	LegacyHighlightColor int           `json:"highlightColor,omitempty"`
	LegacyNotes          []*Note       `json:"notes,omitempty"`
	LegacyStrings        []*StringConn `json:"strings,omitempty"`
}

func DataPath() (string, error) {
	var dir, legacyDir string
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		dir = filepath.Join(xdg, "redthread")
		legacyDir = filepath.Join(xdg, "brainfartadhdfixerupper")
	} else {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(home, ".local", "share", "redthread")
		legacyDir = filepath.Join(home, ".local", "share", "brainfartadhdfixerupper")
	}
	newNotes := filepath.Join(dir, "notes.json")
	legacyNotes := filepath.Join(legacyDir, "notes.json")
	if _, err := os.Stat(newNotes); errors.Is(err, os.ErrNotExist) {
		if _, err := os.Stat(legacyNotes); err == nil {
			_ = os.MkdirAll(dir, 0o755)
			_ = os.Rename(legacyNotes, newNotes)
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return newNotes, nil
}

// LoadWorkspace reads notes.json, migrating older schemas on the way in.
// Returns (nil, nil) when the file simply doesn't exist yet.
func LoadWorkspace() (*Workspace, error) {
	path, err := DataPath()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	var f diskFile
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, err
	}

	// v4/v5: workspace with boards.
	if len(f.Boards) > 0 {
		ws := &Workspace{Boards: f.Boards, ActiveIdx: f.ActiveIdx}
		if f.Background != nil {
			ws.Background = *f.Background
			ws.Background.normalizeLegacy()
		} else {
			// Files written before the background option: cork on, transparent.
			ws.Background = Background{Cork: true}
		}
		for _, b := range ws.Boards {
			normalizeLoadedBoard(b)
		}
		return ws, nil
	}

	// v3 (or older): single board at the root. Wrap into a workspace.
	if len(f.LegacyNotes) > 0 {
		b := &Board{
			Name:           "main",
			GrainSeed:      time.Now().UnixNano(),
			Notes:          f.LegacyNotes,
			Strings:        f.LegacyStrings,
			TextMode:       f.LegacyTextMode,
			Zoom:           f.LegacyZoom,
			HighlightColor: f.LegacyHighlightColor,
		}
		normalizeLoadedBoard(b)
		return &Workspace{Boards: []*Board{b}, Background: Background{Cork: true}}, nil
	}

	// Empty file or unknown shape — let caller seed.
	return nil, nil
}

func normalizeLoadedBoard(b *Board) {
	for _, s := range b.Strings {
		s.normalizeLegacy()
	}
	if b.GrainSeed == 0 {
		b.GrainSeed = time.Now().UnixNano()
	}
	if b.Name == "" {
		b.Name = "main"
	}
	if len(b.Notes) > 0 && b.Selected == "" {
		b.Selected = b.Notes[len(b.Notes)-1].ID
	}
}

// SaveWorkspace writes the workspace as a v4 file (workspace envelope).
func SaveWorkspace(w *Workspace) error {
	path, err := DataPath()
	if err != nil {
		return err
	}
	f := diskFile{
		SchemaVersion: schemaVersion,
		ActiveIdx:     w.ActiveIdx,
		Boards:        w.Boards,
	}
	// Always persist the background — cork=false is meaningful, and the
	// legacy Mode field must never be written.
	bg := w.Background
	bg.Mode = ""
	f.Background = &bg
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// --- debouncer --------------------------------------------------------

type Saver struct {
	ws    *Workspace
	mu    sync.Mutex
	timer *time.Timer
	delay time.Duration
}

func NewSaver(w *Workspace, delay time.Duration) *Saver {
	return &Saver{ws: w, delay: delay}
}

func (s *Saver) Touch() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.timer != nil {
		s.timer.Stop()
	}
	s.timer = time.AfterFunc(s.delay, func() {
		_ = SaveWorkspace(s.ws)
	})
}

func (s *Saver) Flush() error {
	s.mu.Lock()
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
	s.mu.Unlock()
	return SaveWorkspace(s.ws)
}
