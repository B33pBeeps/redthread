package app

// storage.go — loads and saves the workspace (multiple boards) to a JSON
// file under the XDG data directory, with debounced writes. Schema v4
// adds the workspace envelope; v3 single-board files migrate forward.

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const schemaVersion = 6

// diskFile is the v4 envelope. v3 fields (TextMode, Notes, Strings, Zoom,
// HighlightColor) are kept here as optional fallbacks — when a v3 file is
// loaded, those fields are wrapped into a single board. v6 adds the theme
// preference; v5 files simply lack the key and load as auto.
type diskFile struct {
	SchemaVersion int         `json:"schemaVersion"`
	ActiveIdx     int         `json:"activeIdx,omitempty"`
	Background    *Background `json:"background,omitempty"`
	Theme         string      `json:"theme,omitempty"`
	Vim           bool        `json:"vim,omitempty"`
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

	// v4/v5/v6: workspace with boards.
	if len(f.Boards) > 0 {
		ws := &Workspace{Boards: f.Boards, ActiveIdx: f.ActiveIdx, Theme: f.Theme, Vim: f.Vim}
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

// SaveWorkspace writes the workspace as a v4 file (workspace envelope),
// rotating the previous content into the backup directory first.
func SaveWorkspace(w *Workspace) error {
	path, err := DataPath()
	if err != nil {
		return err
	}
	f := diskFile{
		SchemaVersion: schemaVersion,
		ActiveIdx:     w.ActiveIdx,
		Theme:         w.Theme,
		Vim:           w.Vim,
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
	// Snapshot what is on disk before overwriting it. Best-effort: a
	// failure to back up must never stop the save itself, or a full disk
	// would cost the user their live work as well as their history.
	rotateBackup(path, data)

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// backupKeep is how many previous versions of notes.json are retained.
// Only saves that actually change the content rotate, so this is a window
// of the last 20 distinct states rather than 20 timer ticks.
const backupKeep = 20

// BackupDir returns the directory holding rotated copies of notes.json,
// creating it on demand.
func BackupDir() (string, error) {
	path, err := DataPath()
	if err != nil {
		return "", err
	}
	dir := filepath.Join(filepath.Dir(path), "backups")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

// rotateBackup copies the current notes.json into the backup directory
// before it is overwritten, unless its content already matches what is
// about to be written. Errors are swallowed by design — see SaveWorkspace.
func rotateBackup(path string, next []byte) {
	prev, err := os.ReadFile(path)
	if err != nil {
		return // nothing on disk yet (first run), or unreadable
	}
	if bytes.Equal(prev, next) {
		return // the debounce fires on idle too; do not churn the ring
	}

	dir, err := BackupDir()
	if err != nil {
		return
	}

	// Timestamped so the ring sorts chronologically, at nanosecond
	// resolution: the debounce can fire twice inside a millisecond, and a
	// colliding name would silently overwrite a distinct earlier version.
	// The fixed-width fractional part keeps lexical order == time order.
	stamp := time.Now().UTC().Format("20060102-150405.000000000")
	name := filepath.Join(dir, "notes-"+stamp+".json")
	if err := os.WriteFile(name, prev, 0o644); err != nil {
		return
	}
	pruneBackups(dir, backupKeep)
}

// pruneBackups keeps the newest `keep` files and removes the rest. Names
// are timestamped, so lexical order is chronological order.
func pruneBackups(dir string, keep int) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		n := e.Name()
		if strings.HasPrefix(n, "notes-") && strings.HasSuffix(n, ".json") {
			names = append(names, n)
		}
	}
	if len(names) <= keep {
		return
	}
	sort.Strings(names)
	for _, n := range names[:len(names)-keep] {
		_ = os.Remove(filepath.Join(dir, n))
	}
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

// Cancel drops a pending write without performing it. The timer fires on
// its own goroutine and marshals the workspace, so anything that tears
// down a model — notably tests — needs a way to stop it before mutating
// the workspace again.
func (s *Saver) Cancel() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
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
