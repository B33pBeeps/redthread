package app

import "testing"

func TestParseThemeMode(t *testing.T) {
	for _, tc := range []struct {
		in      string
		want    ThemeMode
		wantErr bool
	}{
		{"auto", ThemeAuto, false},
		{"", ThemeAuto, false},
		{"AUTO", ThemeAuto, false},
		{"  auto  ", ThemeAuto, false},
		{"light", ThemeLight, false},
		{"Light", ThemeLight, false},
		{"dark", ThemeDark, false},
		{"DARK", ThemeDark, false},
		{"blue", ThemeAuto, true},
		{"lite", ThemeAuto, true},
	} {
		got, err := ParseThemeMode(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("ParseThemeMode(%q) = %v, nil; want an error", tc.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseThemeMode(%q) returned error %v", tc.in, err)
			continue
		}
		if got != tc.want {
			t.Errorf("ParseThemeMode(%q) = %v; want %v", tc.in, got, tc.want)
		}
	}
}

func TestThemeModeString(t *testing.T) {
	for mode, want := range map[ThemeMode]string{
		ThemeAuto:  "auto",
		ThemeDark:  "dark",
		ThemeLight: "light",
	} {
		if got := mode.String(); got != want {
			t.Errorf("ThemeMode(%d).String() = %q; want %q", mode, got, want)
		}
	}
}

// An explicit mode must never consult the terminal — that is the whole
// point of the override, since the OSC 11 probe is what fails under tmux
// and over ssh.
func TestResolveThemeHonorsExplicitMode(t *testing.T) {
	probed := false
	probe := func() bool {
		probed = true
		return true // claim "dark" regardless
	}

	if got := resolveTheme(ThemeLight, probe); !got {
		t.Error("resolveTheme(ThemeLight) = false; want true (light)")
	}
	if probed {
		t.Error("resolveTheme(ThemeLight) probed the terminal; an explicit mode must not")
	}

	probed = false
	if got := resolveTheme(ThemeDark, probe); got {
		t.Error("resolveTheme(ThemeDark) = true; want false (dark)")
	}
	if probed {
		t.Error("resolveTheme(ThemeDark) probed the terminal; an explicit mode must not")
	}
}

func TestResolveThemeAutoUsesProbe(t *testing.T) {
	if got := resolveTheme(ThemeAuto, func() bool { return true }); got {
		t.Error("resolveTheme(ThemeAuto) with a dark terminal = true; want false (dark)")
	}
	if got := resolveTheme(ThemeAuto, func() bool { return false }); !got {
		t.Error("resolveTheme(ThemeAuto) with a light terminal = false; want true (light)")
	}
}

// When the probe cannot reach a terminal it reports a zero-value (black)
// background, i.e. dark. That is the pre-existing behavior and the safe
// fallback: an unreadable-but-familiar dark board beats guessing light.
func TestResolveThemeAutoFallsBackToDark(t *testing.T) {
	if got := resolveTheme(ThemeAuto, hasDarkBackground); got {
		// hasDarkBackground on a non-TTY (as under `go test`) reports dark.
		t.Error("resolveTheme(ThemeAuto) on a non-TTY = true; want false (dark fallback)")
	}
}

func TestThemeModeFromEnv(t *testing.T) {
	for _, tc := range []struct {
		env          string
		flag         string
		want         ThemeMode
		wantExplicit bool
	}{
		{"", "", ThemeAuto, false},
		{"light", "", ThemeLight, true},
		{"dark", "", ThemeDark, true},
		{"garbage", "", ThemeAuto, false},   // invalid env is ignored, not fatal
		{"dark", "light", ThemeLight, true}, // an explicit flag beats the env
		{"light", "dark", ThemeDark, true},
		{"", "light", ThemeLight, true},

		// "auto" typed by hand is an override — it asks to re-detect — and
		// must be distinguishable from saying nothing at all.
		{"", "auto", ThemeAuto, true},
		{"", "AUTO", ThemeAuto, true},
		{"auto", "", ThemeAuto, true},
		{"light", "auto", ThemeAuto, true}, // the flag wins, and means re-detect

		// A malformed flag falls through to the env rather than being read
		// as a request for auto.
		{"light", "garbage", ThemeLight, true},
		{"", "garbage", ThemeAuto, false},
	} {
		got, explicit := themeModeFrom(tc.flag, tc.env)
		if got != tc.want || explicit != tc.wantExplicit {
			t.Errorf("themeModeFrom(flag=%q, env=%q) = (%v, %v); want (%v, %v)",
				tc.flag, tc.env, got, explicit, tc.want, tc.wantExplicit)
		}
	}
}

// The bug this guards: `--theme=auto` returned ThemeAuto, which Run() read
// as "no override" and replaced with the saved theme — so an explicitly
// requested re-detect silently kept using the saved light/dark choice.
func TestExplicitAutoOverridesASavedTheme(t *testing.T) {
	ws := &Workspace{}
	ws.SetThemeMode(ThemeLight) // the user pressed T at some point

	// resolveMode mirrors what Run() does with themeModeFrom's results.
	resolveMode := func(flagVal, envVal string) ThemeMode {
		mode, explicit := themeModeFrom(flagVal, envVal)
		if !explicit {
			return ws.ThemeMode()
		}
		return mode
	}

	if got := resolveMode("auto", ""); got != ThemeAuto {
		t.Errorf("--theme=auto with a saved light choice = %v; want auto (re-detect)", got)
	}
	if got := resolveMode("", "auto"); got != ThemeAuto {
		t.Errorf("RT_THEME=auto with a saved light choice = %v; want auto (re-detect)", got)
	}
	// With nothing specified, the saved choice still wins.
	if got := resolveMode("", ""); got != ThemeLight {
		t.Errorf("no flag or env = %v; want the saved light choice", got)
	}
	// And an explicit dark still beats the saved light.
	if got := resolveMode("dark", ""); got != ThemeDark {
		t.Errorf("--theme=dark with a saved light choice = %v; want dark", got)
	}
}

// An explicit auto must also clear the saved value, so the *next* launch
// detects too rather than reverting.
func TestExplicitAutoClearsTheSavedTheme(t *testing.T) {
	ws := &Workspace{}
	ws.SetThemeMode(ThemeLight)

	mode, explicit := themeModeFrom("auto", "")
	if explicit {
		ws.SetThemeMode(mode)
	}
	if ws.Theme != "" {
		t.Errorf("after --theme=auto, saved theme = %q; want it cleared so the next run detects", ws.Theme)
	}
}
