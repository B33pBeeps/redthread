package app

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWorkspaceThemeModeRoundTrip(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	ws := seedWorkspace()
	ws.SetThemeMode(ThemeLight)
	if err := SaveWorkspace(ws); err != nil {
		t.Fatalf("SaveWorkspace: %v", err)
	}

	got, err := LoadWorkspace()
	if err != nil {
		t.Fatalf("LoadWorkspace: %v", err)
	}
	if got == nil {
		t.Fatal("LoadWorkspace returned nil")
	}
	if got.ThemeMode() != ThemeLight {
		t.Errorf("round-tripped theme = %v; want light", got.ThemeMode())
	}
}

// A workspace that has never chosen a theme must resolve to auto, so
// existing users keep getting detection rather than a frozen palette.
func TestWorkspaceThemeModeDefaultsToAuto(t *testing.T) {
	ws := &Workspace{}
	if got := ws.ThemeMode(); got != ThemeAuto {
		t.Errorf("empty workspace theme = %v; want auto", got)
	}

	ws.Theme = "nonsense"
	if got := ws.ThemeMode(); got != ThemeAuto {
		t.Errorf("workspace with an unparseable theme = %v; want auto", got)
	}
}

func TestSetThemeModeAutoClearsTheField(t *testing.T) {
	ws := &Workspace{}
	ws.SetThemeMode(ThemeDark)
	if ws.Theme != "dark" {
		t.Errorf("Theme = %q; want \"dark\"", ws.Theme)
	}
	ws.SetThemeMode(ThemeAuto)
	if ws.Theme != "" {
		t.Errorf("Theme after SetThemeMode(auto) = %q; want empty so the key is omitted", ws.Theme)
	}
}

// A v5 file predates the theme key entirely. It must load without error
// and land on auto.
func TestLoadV5FileWithoutThemeKey(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("XDG_DATA_HOME", dir)

	path := filepath.Join(dir, "redthread", "notes.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	const v5 = `{
	  "schemaVersion": 5,
	  "boards": [{"name": "main", "notes": [], "grainSeed": 42}],
	  "background": {"cork": true}
	}`
	if err := os.WriteFile(path, []byte(v5), 0o644); err != nil {
		t.Fatal(err)
	}

	ws, err := LoadWorkspace()
	if err != nil {
		t.Fatalf("LoadWorkspace on a v5 file: %v", err)
	}
	if ws == nil {
		t.Fatal("LoadWorkspace returned nil for a valid v5 file")
	}
	if got := ws.ThemeMode(); got != ThemeAuto {
		t.Errorf("v5 file theme = %v; want auto", got)
	}
	if !ws.CorkOn() {
		t.Error("v5 background did not survive the load")
	}
}

// Toggling from the board must flip the live palette and record the choice,
// so the next launch does not fall back to probing.
func TestToggleThemeSwitchesPaletteAndPersists(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Cleanup(func() { ApplyTheme(false) })

	ApplyTheme(false)
	ws := seedWorkspace()

	ws.ToggleTheme()
	if got := ws.ThemeMode(); got != ThemeLight {
		t.Fatalf("after one toggle from dark, theme = %v; want light", got)
	}
	if l := GetTint("blue").Ink.Brightness(); l >= maxLightLuma {
		t.Errorf("blue ink luma after toggling to light = %.3f; want < %.2f", l, maxLightLuma)
	}

	ws.ToggleTheme()
	if got := ws.ThemeMode(); got != ThemeDark {
		t.Fatalf("after two toggles, theme = %v; want dark", got)
	}
	if l := GetTint("blue").Ink.Brightness(); l <= minDarkLuma {
		t.Errorf("blue ink luma after toggling back to dark = %.3f; want > %.2f", l, minDarkLuma)
	}
}

// Auto must not be written to disk — an omitted key is how "no preference"
// is represented, and writing "auto" would make every save churn the file.
func TestSaveOmitsAutoTheme(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())

	ws := seedWorkspace()
	ws.SetThemeMode(ThemeAuto)
	if err := SaveWorkspace(ws); err != nil {
		t.Fatalf("SaveWorkspace: %v", err)
	}

	path, err := DataPath()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), `"theme"`) {
		t.Errorf("saved file contains a theme key for auto:\n%s", data)
	}

	var f map[string]any
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatalf("saved file is not valid JSON: %v", err)
	}
	if got := f["schemaVersion"]; got != float64(schemaVersion) {
		t.Errorf("schemaVersion = %v; want %d", got, schemaVersion)
	}
}
