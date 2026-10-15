package app

import (
	"os"
	"testing"
)

// TestMain points the data directory at a throwaway temp dir for the whole
// package. The Saver's timer fires on its own goroutine 400 ms after a
// mutation; any test that drives a key or mouse handler arms it, and a slow
// run (-race) keeps the binary alive long enough for it to overwrite the
// developer's real notes.json. Tests that need their own dir still call
// t.Setenv, which restores this value afterwards.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "redthread-test-")
	if err != nil {
		panic(err)
	}
	os.Setenv("XDG_DATA_HOME", dir)
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}
