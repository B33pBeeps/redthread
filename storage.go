package main

// storage.go — loads and saves the board (notes + strings + text mode)
// to a JSON file under the XDG data directory, with debounced writes.

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const schemaVersion = 3

type diskFile struct {
	SchemaVersion int           `json:"schemaVersion"`
	TextMode      TextStyleMode `json:"textMode,omitempty"`
	Notes         []*Note       `json:"notes"`
	Strings       []*StringConn `json:"strings,omitempty"`
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
	// Migrate the pre-naming save file once, on first launch under the
	// new name. If the legacy folder has a notes.json and the new folder
	// doesn't yet, move the file over.
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

func LoadBoard() (*Board, error) {
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
	// Migrate legacy v2 strings (FromID/ToID → A/B endpoints) in-place.
	for _, s := range f.Strings {
		s.normalizeLegacy()
	}
	b := &Board{Notes: f.Notes, Strings: f.Strings, TextMode: f.TextMode}
	if len(b.Notes) > 0 {
		b.Selected = b.Notes[len(b.Notes)-1].ID
	}
	return b, nil
}

func SaveBoard(b *Board) error {
	path, err := DataPath()
	if err != nil {
		return err
	}
	f := diskFile{
		SchemaVersion: schemaVersion,
		TextMode:      b.TextMode,
		Notes:         b.Notes,
		Strings:       b.Strings,
	}
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
	board *Board
	mu    sync.Mutex
	timer *time.Timer
	delay time.Duration
}

func NewSaver(b *Board, delay time.Duration) *Saver {
	return &Saver{board: b, delay: delay}
}

func (s *Saver) Touch() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.timer != nil {
		s.timer.Stop()
	}
	s.timer = time.AfterFunc(s.delay, func() {
		_ = SaveBoard(s.board)
	})
}

func (s *Saver) Flush() error {
	s.mu.Lock()
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
	s.mu.Unlock()
	return SaveBoard(s.board)
}
