package app

// vim_test.go — the modal editing state machine.
//
// Everything here operates on a plain text buffer with a (row, col)
// cursor, deliberately independent of bubbles/textarea: its cursor API
// cannot reliably cross a wrapped line, so vim owns the buffer and the
// textarea is only a display surface for insert mode.
//
// Notation in these tests: the buffer is written as lines, and the cursor
// as (row, col).

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func utf8ValidString(s string) bool { return utf8.ValidString(s) }

// feed sends each key in sequence and returns the final state.
func feed(v *VimState, keys ...string) *VimState {
	for _, k := range keys {
		v.Key(k)
	}
	return v
}

func newVim(text string, row, col int) *VimState {
	v := NewVimState(text)
	v.Row, v.Col = row, col
	return v
}

func (v *VimState) at() (int, int) { return v.Row, v.Col }

// --- motions ----------------------------------------------------------

func TestBasicMotions(t *testing.T) {
	const text = "hello world\nsecond line\nthird"
	for _, tc := range []struct {
		name     string
		keys     []string
		row, col int
		wantR    int
		wantC    int
	}{
		{"l moves right", []string{"l"}, 0, 0, 0, 1},
		{"h moves left", []string{"h"}, 0, 3, 0, 2},
		{"h stops at column 0", []string{"h", "h", "h"}, 0, 0, 0, 0},
		{"l stops at end of line", []string{"l", "l", "l", "l", "l", "l", "l", "l", "l", "l", "l", "l", "l"}, 0, 0, 0, 10},
		{"j moves down", []string{"j"}, 0, 2, 1, 2},
		{"k moves up", []string{"k"}, 1, 2, 0, 2},
		{"j stops at last line", []string{"j", "j", "j", "j"}, 0, 0, 2, 0},
		{"k stops at first line", []string{"k", "k"}, 1, 0, 0, 0},
		{"0 goes to line start", []string{"0"}, 0, 7, 0, 0},
		{"$ goes to line end", []string{"$"}, 0, 0, 0, 10},
		{"gg goes to the first line", []string{"g", "g"}, 2, 3, 0, 0},
		{"G goes to the last line", []string{"G"}, 0, 0, 2, 0},
		// Moving to a shorter line clamps the column.
		{"j clamps onto a shorter line", []string{"j", "j"}, 0, 9, 2, 4},
	} {
		v := newVim(text, tc.row, tc.col)
		feed(v, tc.keys...)
		if r, c := v.at(); r != tc.wantR || c != tc.wantC {
			t.Errorf("%s: from (%d,%d) got (%d,%d); want (%d,%d)",
				tc.name, tc.row, tc.col, r, c, tc.wantR, tc.wantC)
		}
	}
}

func TestWordMotions(t *testing.T) {
	//        0123456789...
	const text = "the quick  brown fox"
	for _, tc := range []struct {
		name string
		keys []string
		col  int
		want int
	}{
		{"w to next word", []string{"w"}, 0, 4},
		{"w skips extra spaces", []string{"w"}, 4, 11},
		{"w from mid-word", []string{"w"}, 5, 11},
		// Past the last word, w clamps to the final character (verified
		// against vim: 5w then x on this line deletes the trailing "x").
		{"w clamps at the end of the last word", []string{"w", "w", "w", "w", "w"}, 0, 19},
		{"b to previous word", []string{"b"}, 11, 4},
		{"b from mid-word goes to its start", []string{"b"}, 6, 4},
		{"b stops at column 0", []string{"b", "b", "b", "b"}, 11, 0},
		{"e to end of word", []string{"e"}, 0, 2},
		{"e from a word end goes to the next", []string{"e"}, 2, 8},
	} {
		v := newVim(text, 0, tc.col)
		feed(v, tc.keys...)
		if _, c := v.at(); c != tc.want {
			t.Errorf("%s: from col %d got %d; want %d", tc.name, tc.col, c, tc.want)
		}
	}
}

// w at the end of a line moves to the next line, as in vim.
func TestWordMotionCrossesLines(t *testing.T) {
	v := newVim("one two\nthree", 0, 4)
	feed(v, "w")
	if r, c := v.at(); r != 1 || c != 0 {
		t.Errorf("w at end of line went to (%d,%d); want (1,0)", r, c)
	}
	v = newVim("one\ntwo", 1, 0)
	feed(v, "b")
	if r, c := v.at(); r != 0 || c != 0 {
		t.Errorf("b at line start went to (%d,%d); want (0,0)", r, c)
	}
}

// --- counts -----------------------------------------------------------

func TestCounts(t *testing.T) {
	const text = "abcdefghij\nsecond\nthird\nfourth\nfifth"
	for _, tc := range []struct {
		name  string
		keys  []string
		wantR int
		wantC int
	}{
		{"3l", []string{"3", "l"}, 0, 3},
		{"5l", []string{"5", "l"}, 0, 5},
		{"2j", []string{"2", "j"}, 2, 0},
		{"12l is a two-digit count", []string{"1", "2", "l"}, 0, 9}, // clamped to line end
		{"3G goes to line 3", []string{"3", "G"}, 2, 0},
	} {
		v := newVim(text, 0, 0)
		feed(v, tc.keys...)
		if r, c := v.at(); r != tc.wantR || c != tc.wantC {
			t.Errorf("%s: got (%d,%d); want (%d,%d)", tc.name, r, c, tc.wantR, tc.wantC)
		}
	}
}

// A leading 0 is the line-start motion, not the start of a count.
func TestZeroIsAMotionNotACount(t *testing.T) {
	v := newVim("hello world", 0, 6)
	feed(v, "0")
	if _, c := v.at(); c != 0 {
		t.Errorf("0 moved to col %d; want 0", c)
	}
	// But 10l is a count of ten.
	v = newVim("abcdefghijklmno", 0, 0)
	feed(v, "1", "0", "l")
	if _, c := v.at(); c != 10 {
		t.Errorf("10l moved to col %d; want 10", c)
	}
}

// --- mode changes -----------------------------------------------------

func TestEnteringInsertMode(t *testing.T) {
	const text = "hello\nworld"
	for _, tc := range []struct {
		name     string
		key      string
		row, col int
		wantR    int
		wantC    int
		wantText string
	}{
		{"i inserts before the cursor", "i", 0, 2, 0, 2, text},
		{"a inserts after the cursor", "a", 0, 2, 0, 3, text},
		{"I goes to the first column", "I", 0, 3, 0, 0, text},
		{"A goes past the end", "A", 0, 1, 0, 5, text},
		{"o opens a line below", "o", 0, 2, 1, 0, "hello\n\nworld"},
		{"O opens a line above", "O", 1, 2, 1, 0, "hello\n\nworld"},
		{"o on the last line appends", "o", 1, 0, 2, 0, "hello\nworld\n"},
	} {
		v := newVim(text, tc.row, tc.col)
		feed(v, tc.key)
		if v.Mode != VimInsert {
			t.Errorf("%s: mode = %v; want insert", tc.name, v.Mode)
		}
		if r, c := v.at(); r != tc.wantR || c != tc.wantC {
			t.Errorf("%s: cursor (%d,%d); want (%d,%d)", tc.name, r, c, tc.wantR, tc.wantC)
		}
		if got := v.Text(); got != tc.wantText {
			t.Errorf("%s: text %q; want %q", tc.name, got, tc.wantText)
		}
	}
}

// esc returns to normal mode and steps left, as vim does.
func TestEscapeLeavesInsertMode(t *testing.T) {
	v := newVim("hello", 0, 0)
	feed(v, "a", "esc")
	if v.Mode != VimNormal {
		t.Fatalf("mode = %v; want normal", v.Mode)
	}
	if _, c := v.at(); c != 0 {
		t.Errorf("after esc, col = %d; want 0 (stepped back from 1)", c)
	}
}

// --- operators --------------------------------------------------------

func TestDeleteOperators(t *testing.T) {
	for _, tc := range []struct {
		name     string
		text     string
		row, col int
		keys     []string
		wantText string
		wantR    int
		wantC    int
	}{
		{"x deletes a character", "hello", 0, 1, []string{"x"}, "hllo", 0, 1},
		{"x at line end steps back", "hi", 0, 1, []string{"x"}, "h", 0, 0},
		{"3x deletes three", "hello", 0, 0, []string{"3", "x"}, "lo", 0, 0},
		{"dd deletes the line", "one\ntwo\nthree", 1, 1, []string{"d", "d"}, "one\nthree", 1, 0},
		{"dd on the last line moves up", "one\ntwo", 1, 0, []string{"d", "d"}, "one", 0, 0},
		{"dd on the only line empties it", "solo", 0, 2, []string{"d", "d"}, "", 0, 0},
		{"2dd deletes two lines", "a\nb\nc\nd", 0, 0, []string{"2", "d", "d"}, "c\nd", 0, 0},
		{"D deletes to end of line", "hello world", 0, 5, []string{"D"}, "hello", 0, 4},
		{"dw deletes a word", "the quick brown", 0, 0, []string{"d", "w"}, "quick brown", 0, 0},
		{"dw mid-word deletes to word end", "the quick brown", 0, 4, []string{"d", "w"}, "the brown", 0, 4},
		{"d$ deletes to end", "hello world", 0, 5, []string{"d", "$"}, "hello", 0, 4},
		{"d0 deletes to start", "hello world", 0, 6, []string{"d", "0"}, "world", 0, 0},
	} {
		v := newVim(tc.text, tc.row, tc.col)
		feed(v, tc.keys...)
		if got := v.Text(); got != tc.wantText {
			t.Errorf("%s: text %q; want %q", tc.name, got, tc.wantText)
		}
		if r, c := v.at(); r != tc.wantR || c != tc.wantC {
			t.Errorf("%s: cursor (%d,%d); want (%d,%d)", tc.name, r, c, tc.wantR, tc.wantC)
		}
	}
}

func TestChangeOperators(t *testing.T) {
	for _, tc := range []struct {
		name     string
		text     string
		row, col int
		keys     []string
		wantText string
	}{
		// cw is ce: it leaves the space after the word. Verified against
		// real vim, which also gives " quick" here.
		{"cw changes a word, keeping the space", "the quick", 0, 0, []string{"c", "w"}, " quick"},
		{"cc clears the line", "one\ntwo", 0, 1, []string{"c", "c"}, "\ntwo"},
		{"C changes to end of line", "hello world", 0, 5, []string{"C"}, "hello"},
	} {
		v := newVim(tc.text, tc.row, tc.col)
		feed(v, tc.keys...)
		if got := v.Text(); got != tc.wantText {
			t.Errorf("%s: text %q; want %q", tc.name, got, tc.wantText)
		}
		if v.Mode != VimInsert {
			t.Errorf("%s: mode = %v; want insert after a change", tc.name, v.Mode)
		}
	}
}

// --- yank and put -----------------------------------------------------

func TestYankAndPut(t *testing.T) {
	v := newVim("one\ntwo\nthree", 0, 0)
	feed(v, "y", "y") // yank "one"
	feed(v, "j")      // to line 1
	feed(v, "p")      // put below
	if got, want := v.Text(), "one\ntwo\none\nthree"; got != want {
		t.Errorf("yy then p gave %q; want %q", got, want)
	}
	if r := v.Row; r != 2 {
		t.Errorf("after p, row = %d; want 2 (on the pasted line)", r)
	}
}

func TestPutAboveWithCapitalP(t *testing.T) {
	v := newVim("one\ntwo", 0, 0)
	feed(v, "y", "y", "j", "P")
	if got, want := v.Text(), "one\none\ntwo"; got != want {
		t.Errorf("yy then P gave %q; want %q", got, want)
	}
}

// dd puts the deleted line in the register, so ddp swaps two lines — the
// canonical test that delete and yank share a register.
func TestDeleteThenPutSwapsLines(t *testing.T) {
	v := newVim("first\nsecond", 0, 0)
	feed(v, "d", "d", "p")
	if got, want := v.Text(), "second\nfirst"; got != want {
		t.Errorf("ddp gave %q; want %q", got, want)
	}
}

// --- visual mode ------------------------------------------------------

func TestVisualModeSelectionAndDelete(t *testing.T) {
	v := newVim("hello world", 0, 0)
	feed(v, "v", "l", "l", "l", "d") // select 4 chars, delete
	if got, want := v.Text(), "o world"; got != want {
		t.Errorf("visual delete gave %q; want %q", got, want)
	}
	if v.Mode != VimNormal {
		t.Errorf("mode after visual delete = %v; want normal", v.Mode)
	}
}

func TestVisualModeYank(t *testing.T) {
	v := newVim("hello world", 0, 0)
	feed(v, "v", "l", "l", "l", "l", "y") // yank "hello"
	if v.Mode != VimNormal {
		t.Errorf("mode after visual yank = %v; want normal", v.Mode)
	}
	feed(v, "$", "p")
	if !strings.Contains(v.Text(), "hello") {
		t.Errorf("put after visual yank gave %q", v.Text())
	}
}

func TestEscapeLeavesVisualMode(t *testing.T) {
	v := newVim("hello", 0, 0)
	feed(v, "v", "l", "esc")
	if v.Mode != VimNormal {
		t.Errorf("mode = %v; want normal", v.Mode)
	}
	if v.Text() != "hello" {
		t.Errorf("esc from visual changed the text: %q", v.Text())
	}
}

// --- insert-mode typing -----------------------------------------------

func TestTypingInInsertMode(t *testing.T) {
	v := newVim("hllo", 0, 1)
	feed(v, "i")
	v.Insert("e")
	if got, want := v.Text(), "hello"; got != want {
		t.Errorf("typing gave %q; want %q", got, want)
	}
	if _, c := v.at(); c != 2 {
		t.Errorf("cursor col = %d; want 2 (after the inserted rune)", c)
	}
}

func TestBackspaceInInsertMode(t *testing.T) {
	v := newVim("hexllo", 0, 3)
	feed(v, "i")
	v.Key("backspace")
	if got, want := v.Text(), "hello"; got != want {
		t.Errorf("backspace gave %q; want %q", got, want)
	}
}

// Backspace at column 0 joins with the previous line.
func TestBackspaceJoinsLines(t *testing.T) {
	v := newVim("one\ntwo", 1, 0)
	feed(v, "i")
	v.Key("backspace")
	if got, want := v.Text(), "onetwo"; got != want {
		t.Errorf("backspace at col 0 gave %q; want %q", got, want)
	}
	if r, c := v.at(); r != 0 || c != 3 {
		t.Errorf("cursor (%d,%d); want (0,3)", r, c)
	}
}

func TestEnterSplitsTheLine(t *testing.T) {
	v := newVim("hello world", 0, 5)
	feed(v, "i")
	v.Key("enter")
	if got, want := v.Text(), "hello\n world"; got != want {
		t.Errorf("enter gave %q; want %q", got, want)
	}
	if r, c := v.at(); r != 1 || c != 0 {
		t.Errorf("cursor (%d,%d); want (1,0)", r, c)
	}
}

// --- robustness -------------------------------------------------------

// An unknown key in normal mode must be ignored, not inserted — the whole
// point of normal mode is that stray keys do not edit the buffer.
func TestUnknownNormalKeysAreIgnored(t *testing.T) {
	v := newVim("hello", 0, 0)
	feed(v, "z", "q", "!", "ctrl+z", "f1")
	if got := v.Text(); got != "hello" {
		t.Errorf("unknown keys changed the text to %q", got)
	}
}

// A pending operator followed by something meaningless must reset rather
// than leaving the machine wedged.
func TestDanglingOperatorResets(t *testing.T) {
	v := newVim("hello", 0, 0)
	feed(v, "d", "z") // dz is not an operation
	if got := v.Text(); got != "hello" {
		t.Errorf("dz changed the text to %q", got)
	}
	feed(v, "x") // must still work
	if got := v.Text(); got != "ello" {
		t.Errorf("after a dangling operator, x gave %q; want %q", got, "ello")
	}
}

func TestEmptyBufferIsSafe(t *testing.T) {
	v := newVim("", 0, 0)
	feed(v, "x", "d", "d", "j", "k", "l", "h", "w", "b", "e", "$", "0", "G", "g", "g", "D", "p")
	if got := v.Text(); got != "" {
		t.Errorf("operations on an empty buffer produced %q", got)
	}
}

// The cursor must never point outside the buffer, whatever the sequence.
func TestCursorStaysInBounds(t *testing.T) {
	v := newVim("ab\n\ncde", 0, 0)
	for _, k := range []string{"G", "$", "j", "l", "l", "l", "x", "x", "x", "x", "k", "$", "d", "d", "d", "d", "d", "d", "$", "l"} {
		v.Key(k)
		lines := strings.Split(v.Text(), "\n")
		if v.Row < 0 || v.Row >= len(lines) {
			t.Fatalf("after %q, row %d is outside 0..%d (text %q)", k, v.Row, len(lines)-1, v.Text())
		}
		if v.Col < 0 || v.Col > len(lines[v.Row]) {
			t.Fatalf("after %q, col %d is outside 0..%d (line %q)", k, v.Col, len(lines[v.Row]), lines[v.Row])
		}
	}
}

// Arrow keys work in every mode — people reach for them even with vim
// bindings on, and in insert mode they are the only way to move.
func TestArrowKeysInInsertMode(t *testing.T) {
	v := newVim("hello\nworld", 0, 0)
	feed(v, "i", "right", "right", "down")
	if r, c := v.at(); r != 1 || c != 2 {
		t.Errorf("arrows in insert put the cursor at (%d,%d); want (1,2)", r, c)
	}
	feed(v, "up", "left")
	if r, c := v.at(); r != 0 || c != 1 {
		t.Errorf("after up+left, cursor (%d,%d); want (0,1)", r, c)
	}
	v.Insert("X")
	if got, want := v.Text(), "hXello\nworld"; got != want {
		t.Errorf("typing after arrows gave %q; want %q", got, want)
	}
}

func TestArrowKeysInNormalMode(t *testing.T) {
	v := newVim("hello\nworld", 0, 0)
	feed(v, "right", "right", "down")
	if r, c := v.at(); r != 1 || c != 2 {
		t.Errorf("arrows in normal put the cursor at (%d,%d); want (1,2)", r, c)
	}
}

// Tab in a note should indent, not move focus.
func TestTabIndentsInInsertMode(t *testing.T) {
	v := newVim("x", 0, 0)
	feed(v, "i", "tab")
	if got, want := v.Text(), "  x"; got != want {
		t.Errorf("tab gave %q; want %q", got, want)
	}
}

// Special keys must never leak into the buffer as literal text.
func TestSpecialKeysDoNotInsertLiterals(t *testing.T) {
	v := newVim("", 0, 0)
	feed(v, "i")
	for _, k := range []string{"ctrl+a", "pgdown", "f1", "shift+tab", "home", "delete"} {
		v.Key(k)
	}
	if got := v.Text(); got != "" {
		t.Errorf("special keys inserted %q into the buffer", got)
	}
}

func TestVisualAcrossLines(t *testing.T) {
	v := newVim("one\ntwo\nthree", 0, 1)
	feed(v, "v", "j", "l", "d") // from (0,1) to (1,2) inclusive
	if got, want := v.Text(), "o\nthree"; got != want {
		t.Errorf("multi-line visual delete gave %q; want %q", got, want)
	}
}

// Selecting backwards works, and the anchor character is included —
// charwise visual is inclusive at both ends, as in vim. Anchoring on the
// space at index 5 and running back to 0 removes "hello ".
func TestVisualBackwardSelection(t *testing.T) {
	v := newVim("hello world", 0, 5)
	feed(v, "v", "h", "h", "h", "h", "h", "d")
	if got, want := v.Text(), "world"; got != want {
		t.Errorf("backward visual delete gave %q; want %q", got, want)
	}
}

func TestVisualChangeEntersInsert(t *testing.T) {
	v := newVim("hello world", 0, 0)
	feed(v, "v", "l", "l", "l", "l", "c")
	if v.Mode != VimInsert {
		t.Errorf("mode after visual c = %v; want insert", v.Mode)
	}
	if got, want := v.Text(), " world"; got != want {
		t.Errorf("visual change gave %q; want %q", got, want)
	}
}

// vw selects up to and including the first character of the next word,
// because charwise visual includes the cursor position. This is vim's
// behavior, and the reason `vwd` and `dw` differ by one character.
func TestVisualWordMotion(t *testing.T) {
	v := newVim("the quick brown", 0, 0)
	feed(v, "v", "w", "d")
	if got, want := v.Text(), "uick brown"; got != want {
		t.Errorf("visual w then d gave %q; want %q", got, want)
	}
}

// SetText is how the editor pushes external changes in; the cursor has to
// survive landing in a smaller buffer.
func TestSetTextClampsTheCursor(t *testing.T) {
	v := newVim("long line here\nsecond\nthird", 2, 4)
	v.SetText("hi")
	if r, c := v.at(); r != 0 || c > 1 {
		t.Errorf("after SetText the cursor is (%d,%d); want it clamped into %q", r, c, "hi")
	}
}

func TestModeStrings(t *testing.T) {
	for mode, want := range map[VimMode]string{
		VimNormal: "NORMAL",
		VimInsert: "INSERT",
		VimVisual: "VISUAL",
	} {
		if got := mode.String(); got != want {
			t.Errorf("mode %d prints %q; want %q", mode, got, want)
		}
	}
}

// --- checked against real vim -----------------------------------------
//
// These expectations were produced by running the same keys through
// `vim -Nu NONE -es -c 'normal! gg0<keys>'` rather than from memory. Where
// this implementation and vim disagree, vim is right.
func TestMatchesRealVim(t *testing.T) {
	for _, tc := range []struct {
		text string
		keys []string
		want string
	}{
		{"one\ntwo\nthree", []string{"d", "d"}, "two\nthree"},
		{"one\ntwo", []string{"d", "d", "p"}, "two\none"},
		{"hello world", []string{"5", "l", "D"}, "hello"},
		{"one two three", []string{"w", "d", "w"}, "one three"},
		{"hello", []string{"2", "x"}, "llo"},
		{"hello", []string{"x"}, "ello"},
		{"hello", []string{"3", "x"}, "lo"},
		{"one two three", []string{"d", "w"}, "two three"},
		{"one two three", []string{"2", "d", "w"}, "three"},
		{"hello", []string{"$", "x"}, "hell"},
		{"the quick", []string{"w", "d", "w"}, "the "},
		{"hello world", []string{"5", "l", "v", "5", "h", "d"}, "world"},
		{"the quick brown", []string{"v", "w", "d"}, "uick brown"},
	} {
		v := newVim(tc.text, 0, 0)
		feed(v, tc.keys...)
		if got := v.Text(); got != tc.want {
			t.Errorf("%q + %v = %q; real vim gives %q",
				tc.text, tc.keys, got, tc.want)
		}
	}
}

// cw is vim's notable special case: it changes to the *end of the word*,
// not to the start of the next one, so it does not eat the space.
func TestChangeWordDoesNotEatTheSpace(t *testing.T) {
	v := newVim("the quick brown", 0, 4)
	feed(v, "c", "w")
	v.Insert("X")
	if got, want := v.Text(), "the X brown"; got != want {
		t.Errorf("wcwX gave %q; real vim gives %q", got, want)
	}
}

// TestDifferentialAgainstVim compares this implementation against real
// vim over a grid of buffers and key sequences. The expectations were
// generated by running each case through
//
//	vim -Nu NONE -es -c 'normal! gg0<keys>' -c 'wq' file
//
// and are baked in here so the comparison runs anywhere, not only on a
// machine with vim installed. Regenerating: see docs in the PR.
//
// Five real bugs turned up this way that hand-written tests had agreed
// with: `dw` on a line's last word deleting nothing, `cw` eating the
// trailing space, `cw` on a single-character word over-reaching, and `w`
// and `e` failing to clamp at the end of the last word. That is why the
// grid is here rather than a handful of cases picked from memory.
//
// Excluded: sequences where a motion fails at a buffer edge (`bx` at
// column 0, `wwx` on a single-word line). Inside one `normal!` command
// vim aborts the whole sequence at the failed motion, so the oracle
// reports "no change"; issued as separate commands it agrees with this
// implementation. That is a property of vim's command batching, not of
// the motions.
func TestDifferentialAgainstVim(t *testing.T) {
	for _, tc := range []struct {
		text string
		keys []string
		want string
	}{
		{"hello world", []string{"x"}, "ello world"},
		{"hello world", []string{"2", "x"}, "llo world"},
		{"hello world", []string{"3", "x"}, "lo world"},
		{"hello world", []string{"D"}, ""},
		{"hello world", []string{"d", "w"}, "world"},
		{"hello world", []string{"2", "d", "w"}, ""},
		{"hello world", []string{"d", "d"}, ""},
		{"hello world", []string{"c", "w"}, " world"},
		{"hello world", []string{"c", "e"}, " world"},
		{"hello world", []string{"d", "$"}, ""},
		{"hello world", []string{"d", "0"}, "hello world"},
		{"hello world", []string{"y", "y", "p"}, "hello world\nhello world"},
		{"hello world", []string{"y", "y", "P"}, "hello world\nhello world"},
		{"hello world", []string{"w"}, "hello world"},
		{"hello world", []string{"b"}, "hello world"},
		{"hello world", []string{"e"}, "hello world"},
		{"hello world", []string{"$"}, "hello world"},
		{"hello world", []string{"0"}, "hello world"},
		{"hello world", []string{"w", "x"}, "hello orld"},
		{"hello world", []string{"w", "w", "x"}, "hello worl"},
		{"hello world", []string{"3", "l", "x"}, "helo world"},
		{"hello world", []string{"e", "x"}, "hell world"},
		{"the quick brown fox", []string{"x"}, "he quick brown fox"},
		{"the quick brown fox", []string{"2", "x"}, "e quick brown fox"},
		{"the quick brown fox", []string{"3", "x"}, " quick brown fox"},
		{"the quick brown fox", []string{"D"}, ""},
		{"the quick brown fox", []string{"d", "w"}, "quick brown fox"},
		{"the quick brown fox", []string{"2", "d", "w"}, "brown fox"},
		{"the quick brown fox", []string{"d", "d"}, ""},
		{"the quick brown fox", []string{"c", "w"}, " quick brown fox"},
		{"the quick brown fox", []string{"c", "e"}, " quick brown fox"},
		{"the quick brown fox", []string{"d", "$"}, ""},
		{"the quick brown fox", []string{"d", "0"}, "the quick brown fox"},
		{"the quick brown fox", []string{"y", "y", "p"}, "the quick brown fox\nthe quick brown fox"},
		{"the quick brown fox", []string{"y", "y", "P"}, "the quick brown fox\nthe quick brown fox"},
		{"the quick brown fox", []string{"w"}, "the quick brown fox"},
		{"the quick brown fox", []string{"b"}, "the quick brown fox"},
		{"the quick brown fox", []string{"e"}, "the quick brown fox"},
		{"the quick brown fox", []string{"$"}, "the quick brown fox"},
		{"the quick brown fox", []string{"0"}, "the quick brown fox"},
		{"the quick brown fox", []string{"w", "x"}, "the uick brown fox"},
		{"the quick brown fox", []string{"w", "w", "x"}, "the quick rown fox"},
		{"the quick brown fox", []string{"3", "l", "x"}, "thequick brown fox"},
		{"the quick brown fox", []string{"e", "x"}, "th quick brown fox"},
		{"a bb ccc", []string{"x"}, " bb ccc"},
		{"a bb ccc", []string{"2", "x"}, "bb ccc"},
		{"a bb ccc", []string{"3", "x"}, "b ccc"},
		{"a bb ccc", []string{"D"}, ""},
		{"a bb ccc", []string{"d", "w"}, "bb ccc"},
		{"a bb ccc", []string{"2", "d", "w"}, "ccc"},
		{"a bb ccc", []string{"d", "d"}, ""},
		{"a bb ccc", []string{"c", "w"}, " bb ccc"},
		{"a bb ccc", []string{"c", "e"}, " ccc"},
		{"a bb ccc", []string{"d", "$"}, ""},
		{"a bb ccc", []string{"d", "0"}, "a bb ccc"},
		{"a bb ccc", []string{"y", "y", "p"}, "a bb ccc\na bb ccc"},
		{"a bb ccc", []string{"y", "y", "P"}, "a bb ccc\na bb ccc"},
		{"a bb ccc", []string{"w"}, "a bb ccc"},
		{"a bb ccc", []string{"b"}, "a bb ccc"},
		{"a bb ccc", []string{"e"}, "a bb ccc"},
		{"a bb ccc", []string{"$"}, "a bb ccc"},
		{"a bb ccc", []string{"0"}, "a bb ccc"},
		{"a bb ccc", []string{"w", "x"}, "a b ccc"},
		{"a bb ccc", []string{"w", "w", "x"}, "a bb cc"},
		{"a bb ccc", []string{"3", "l", "x"}, "a b ccc"},
		{"a bb ccc", []string{"e", "x"}, "a b ccc"},
		{"one", []string{"x"}, "ne"},
		{"one", []string{"2", "x"}, "e"},
		{"one", []string{"3", "x"}, ""},
		{"one", []string{"D"}, ""},
		{"one", []string{"d", "w"}, ""},
		{"one", []string{"2", "d", "w"}, ""},
		{"one", []string{"d", "d"}, ""},
		{"one", []string{"c", "w"}, ""},
		{"one", []string{"c", "e"}, ""},
		{"one", []string{"d", "$"}, ""},
		{"one", []string{"d", "0"}, "one"},
		{"one", []string{"y", "y", "p"}, "one\none"},
		{"one", []string{"y", "y", "P"}, "one\none"},
		{"one", []string{"w"}, "one"},
		{"one", []string{"b"}, "one"},
		{"one", []string{"e"}, "one"},
		{"one", []string{"$"}, "one"},
		{"one", []string{"0"}, "one"},
		{"one", []string{"w", "x"}, "on"},
		{"one", []string{"3", "l", "x"}, "on"},
		{"one", []string{"e", "x"}, "on"},
	} {
		v := newVim(tc.text, 0, 0)
		feed(v, tc.keys...)
		if got := v.Text(); got != tc.want {
			t.Errorf("%q + %v\n got %q\nvim %q", tc.text, tc.keys, got, tc.want)
		}
	}
}

// The buffer is indexed by byte throughout, which is fine for ASCII and
// wrong for anything else: a multi-byte rune gets split and renders as
// mojibake. Notes contain bullets and em dashes routinely.
func TestMultiByteRunesSurviveEditing(t *testing.T) {
	const text = "• bullet — dash\nsecond ✻ line"
	v := newVim(text, 0, 0)

	// Navigation must not corrupt anything.
	feed(v, "j", "k", "l", "l", "h", "w", "b", "e", "$", "0")
	if got := v.Text(); got != text {
		t.Errorf("navigation corrupted multi-byte text:\n got %q\nwant %q", got, text)
	}

	// x must delete a whole rune, not one byte of it.
	v = newVim(text, 0, 0)
	feed(v, "x")
	if got := v.Text(); !strings.HasPrefix(got, " bullet") {
		t.Errorf("x on a multi-byte rune gave %q; want the whole rune removed", got)
	}
	if !utf8ValidString(v.Text()) {
		t.Errorf("x produced invalid UTF-8: %q", v.Text())
	}
}

func TestMultiByteRenderingIsIntact(t *testing.T) {
	v := newVim("• drag with the mouse", 0, 0)
	out := vimRender(v, 40, 5, RGB{200, 200, 200}, RGB{0, 0, 0}, RGB{255, 255, 255})
	if !utf8ValidString(out) {
		t.Errorf("vimRender produced invalid UTF-8: %q", out)
	}
	if !strings.Contains(out, "•") {
		t.Errorf("the bullet did not survive rendering: %q", out)
	}
}
