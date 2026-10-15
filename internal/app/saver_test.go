package app

import (
	"fmt"
	"testing"
)

// The saver's timer used to marshal the live workspace on its own
// goroutine while the update loop kept mutating it. Touch now snapshots
// on the caller's goroutine, so a zero delay and a tight mutation loop
// must be race-free (this test is meant to run under -race), and the
// state written by Flush must be what ends up on disk.
func TestSaverSnapshotsOnTouch(t *testing.T) {
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	ws := seedWorkspace()
	s := NewSaver(ws, 0)
	t.Cleanup(s.Cancel)
	n := ws.Boards[0].Notes[0]

	for i := 0; i < 200; i++ {
		n.Title = fmt.Sprintf("title %d", i)
		s.Touch()
	}
	n.Title = "final"
	if err := s.Flush(); err != nil {
		t.Fatal(err)
	}

	got, err := LoadWorkspace()
	if err != nil || got == nil {
		t.Fatalf("load: %v", err)
	}
	if title := got.Boards[0].Notes[0].Title; title != "final" {
		t.Errorf("on disk title = %q; want %q (a stale timer write landed after Flush)", title, "final")
	}
}
