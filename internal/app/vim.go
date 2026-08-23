package app

// vim.go — modal editing for the note card.
//
// The buffer lives here, not in bubbles/textarea. That widget's cursor API
// cannot reliably cross a wrapped line (CursorDown gets stuck when a
// logical line wraps), so anything built on top of it would break on
// exactly the long notes people want vim bindings for. VimState owns the
// text and the cursor; the editor renders from it.
//
// Scope is the muscle memory that matters, not an emulator: motions
// (hjkl w b e 0 $ gg G), counts, insert entry (i a I A o O), operators
// (x d c y with dd cc yy D C, plus operator+motion), put (p P), and
// charwise visual mode. Anything not implemented is ignored rather than
// approximated — a key that silently does the wrong thing is worse than
// one that does nothing.

import "strings"

type VimMode int

const (
	VimNormal VimMode = iota
	VimInsert
	VimVisual
)

func (m VimMode) String() string {
	switch m {
	case VimInsert:
		return "INSERT"
	case VimVisual:
		return "VISUAL"
	default:
		return "NORMAL"
	}
}

// VimState is the buffer plus everything the key handler needs to
// interpret the next keystroke.
type VimState struct {
	// lines holds runes, not bytes: Col is a character index, so a byte
	// slice would split multi-byte runes and corrupt the text. Notes
	// contain bullets and em dashes routinely.
	lines [][]rune // never empty; an empty buffer is one empty line
	Row   int
	Col   int
	Mode  VimMode

	// pending input: a count being typed, an operator awaiting a motion,
	// and the `g` prefix (for gg).
	count    int
	operator string
	gPending bool

	// register holds the last yank or delete. linewise records whether it
	// was whole lines, which decides how p behaves.
	register string
	linewise bool

	// visual anchor — the end of the selection that does not move.
	visRow, visCol int
}

func NewVimState(text string) *VimState {
	return &VimState{lines: vimLines(text), Mode: VimNormal}
}

// vimLines splits text into buffer lines. Distinct from render.go's
// splitLines: an empty buffer is one empty line, not zero lines, so the
// cursor always has somewhere to sit.
func vimLines(text string) [][]rune {
	parts := strings.Split(text, "\n")
	out := make([][]rune, len(parts))
	for i, p := range parts {
		out[i] = []rune(p)
	}
	return out
}

// Text renders the buffer back to a string.
func (v *VimState) Text() string {
	parts := make([]string, len(v.lines))
	for i, l := range v.lines {
		parts[i] = string(l)
	}
	return strings.Join(parts, "\n")
}

// Line returns row i as a string (for rendering).
func (v *VimState) Line(i int) string {
	if i < 0 || i >= len(v.lines) {
		return ""
	}
	return string(v.lines[i])
}

// LineCount is the number of lines in the buffer.
func (v *VimState) LineCount() int { return len(v.lines) }

// SetText replaces the buffer, clamping the cursor into the new content.
func (v *VimState) SetText(text string) {
	v.lines = vimLines(text)
	v.clamp()
}

func (v *VimState) line() []rune { return v.lines[v.Row] }

// clamp keeps (Row, Col) inside the buffer. In normal mode the cursor sits
// *on* a character, so the last valid column is len-1; in insert mode it
// sits between them, so len is valid.
func (v *VimState) clamp() {
	if len(v.lines) == 0 {
		v.lines = [][]rune{{}}
	}
	if v.Row < 0 {
		v.Row = 0
	}
	if v.Row >= len(v.lines) {
		v.Row = len(v.lines) - 1
	}
	maxCol := len(v.lines[v.Row])
	if v.Mode == VimNormal && maxCol > 0 {
		maxCol--
	}
	if v.Col < 0 {
		v.Col = 0
	}
	if v.Col > maxCol {
		v.Col = maxCol
	}
}

// takeCount returns the pending count (default 1) and clears it.
func (v *VimState) takeCount() int {
	n := v.count
	v.count = 0
	if n < 1 {
		return 1
	}
	return n
}

// Insert types text at the cursor (insert mode only).
func (v *VimState) Insert(s string) {
	if v.Mode != VimInsert {
		return
	}
	for _, r := range s {
		if r == '\n' {
			v.splitLine()
			continue
		}
		l := v.line()
		nl := make([]rune, 0, len(l)+1)
		nl = append(nl, l[:v.Col]...)
		nl = append(nl, r)
		nl = append(nl, l[v.Col:]...)
		v.lines[v.Row] = nl
		v.Col++
	}
}

func (v *VimState) splitLine() {
	l := v.line()
	head := append([]rune{}, l[:v.Col]...)
	tail := append([]rune{}, l[v.Col:]...)
	v.lines[v.Row] = head
	v.lines = append(v.lines[:v.Row+1], append([][]rune{tail}, v.lines[v.Row+1:]...)...)
	v.Row++
	v.Col = 0
}

// Key feeds one key to the state machine. Keys are bubbletea's names
// ("a", "esc", "backspace", "enter", …).
func (v *VimState) Key(key string) {
	switch v.Mode {
	case VimInsert:
		v.insertKey(key)
	case VimVisual:
		v.visualKey(key)
	default:
		v.normalKey(key)
	}
}

func (v *VimState) insertKey(key string) {
	switch key {
	case "esc":
		v.Mode = VimNormal
		// vim steps left when leaving insert mode.
		if v.Col > 0 {
			v.Col--
		}
		v.clamp()
	case "enter":
		v.splitLine()
	case "backspace":
		v.backspace()
	case "tab":
		v.Insert("  ")
	case "left":
		if v.Col > 0 {
			v.Col--
		}
	case "right":
		if v.Col < len(v.line()) {
			v.Col++
		}
	case "up":
		if v.Row > 0 {
			v.Row--
			v.clamp()
		}
	case "down":
		if v.Row < len(v.lines)-1 {
			v.Row++
			v.clamp()
		}
	default:
		// Single printable runes are text; everything else is ignored.
		if isTypeable(key) {
			v.Insert(key)
		}
	}
}

func (v *VimState) backspace() {
	if v.Col > 0 {
		l := v.line()
		nl := append([]rune{}, l[:v.Col-1]...)
		nl = append(nl, l[v.Col:]...)
		v.lines[v.Row] = nl
		v.Col--
		return
	}
	if v.Row == 0 {
		return
	}
	// Join with the previous line.
	prev := v.lines[v.Row-1]
	cur := v.line()
	v.Col = len(prev)
	v.lines[v.Row-1] = append(append([]rune{}, prev...), cur...)
	v.lines = append(v.lines[:v.Row], v.lines[v.Row+1:]...)
	v.Row--
}

// isTypeable reports whether a key name is a single character to insert.
// Multi-character names are special keys ("ctrl+a", "pgdown", "f1").
func isTypeable(key string) bool {
	r := []rune(key)
	return len(r) == 1 && r[0] >= 0x20 && r[0] != 0x7f
}

func (v *VimState) normalKey(key string) {
	// A digit builds the count, except a leading 0 which is a motion.
	if len(key) == 1 && key[0] >= '0' && key[0] <= '9' {
		if key == "0" && v.count == 0 {
			// fall through to the motion below
		} else {
			v.count = v.count*10 + int(key[0]-'0')
			return
		}
	}

	// `g` prefix: only gg is implemented.
	if v.gPending {
		v.gPending = false
		if key == "g" {
			n := v.takeCount()
			if v.count > 0 || n > 1 {
				v.Row = n - 1
			} else {
				v.Row = 0
			}
			v.Col = 0
			v.clamp()
			return
		}
		// Not a g-command; fall through and treat the key normally.
	}

	// An operator is pending: this key is its motion (or a doubled
	// operator, like the second d in dd).
	if v.operator != "" {
		v.applyOperatorKey(key)
		return
	}

	switch key {
	case "g":
		v.gPending = true
	case "h", "left":
		v.moveCols(-v.takeCount())
	case "l", "right":
		v.moveCols(v.takeCount())
	case "j", "down":
		v.moveRows(v.takeCount())
	case "k", "up":
		v.moveRows(-v.takeCount())
	case "0":
		v.count = 0
		v.Col = 0
	case "$":
		v.count = 0
		v.Col = len(v.line())
		v.clamp()
	case "G":
		n := v.count
		v.count = 0
		if n > 0 {
			v.Row = n - 1
		} else {
			v.Row = len(v.lines) - 1
		}
		v.Col = 0
		v.clamp()
	case "w":
		for n := v.takeCount(); n > 0; n-- {
			v.wordForward()
		}
	case "b":
		for n := v.takeCount(); n > 0; n-- {
			v.wordBack()
		}
	case "e":
		for n := v.takeCount(); n > 0; n-- {
			v.wordEnd()
		}

	case "i":
		v.count = 0
		v.Mode = VimInsert
	case "a":
		v.count = 0
		v.Mode = VimInsert
		if len(v.line()) > 0 {
			v.Col++
		}
	case "I":
		v.count = 0
		v.Mode = VimInsert
		v.Col = 0
	case "A":
		v.count = 0
		v.Mode = VimInsert
		v.Col = len(v.line())
	case "o":
		v.count = 0
		v.openLine(v.Row + 1)
	case "O":
		v.count = 0
		v.openLine(v.Row)

	case "x":
		v.deleteChars(v.takeCount())
	case "D":
		v.count = 0
		at := min(v.Col, len(v.line()))
		v.yankTo(string(v.line()[at:]), false)
		v.lines[v.Row] = append([]rune{}, v.line()[:at]...)
		v.clamp()
	case "C":
		v.count = 0
		at := min(v.Col, len(v.line()))
		v.yankTo(string(v.line()[at:]), false)
		v.lines[v.Row] = append([]rune{}, v.line()[:at]...)
		v.Mode = VimInsert
	case "d", "c", "y":
		v.operator = key
	case "p":
		v.put(true)
	case "P":
		v.put(false)
	case "v":
		v.count = 0
		v.Mode = VimVisual
		v.visRow, v.visCol = v.Row, v.Col
	case "esc":
		v.count = 0
		v.operator = ""
	}
}

// applyOperatorKey handles the key following d, c, or y.
func (v *VimState) applyOperatorKey(key string) {
	op := v.operator
	v.operator = ""
	n := v.takeCount()

	// Doubled operator = linewise on n lines (dd, cc, yy).
	if key == op {
		v.linewiseOp(op, n)
		return
	}

	// Otherwise the key is a motion; operate over the span it covers.
	startRow, startCol := v.Row, v.Col

	// vim's one documented irregularity: on a non-blank, cw behaves like
	// ce — changing to the end of the word rather than to the start of the
	// next one, so it does not swallow the space between them.
	//
	// The exception is a single-character word, where ce would advance to
	// the *next* word's end. cw must change only that character, so the
	// span is handled directly rather than via a motion.
	if op == "c" && key == "w" {
		l := v.line()
		if v.Col < len(l) && !isSpace(l[v.Col]) {
			if v.Col+1 >= len(l) || isSpace(l[v.Col+1]) {
				// One-character word: change just it.
				v.yankTo(string(l[v.Col:v.Col+1]), false)
				nl := append([]rune{}, l[:v.Col]...)
				nl = append(nl, l[v.Col+1:]...)
				v.lines[v.Row] = nl
				v.Mode = VimInsert
				v.clamp()
				return
			}
			key = "e"
		}
	}

	switch key {
	case "w":
		for i := 0; i < n; i++ {
			v.wordForwardForOperator()
		}
	case "b":
		for i := 0; i < n; i++ {
			v.wordBack()
		}
	case "e":
		for i := 0; i < n; i++ {
			v.wordEnd()
		}
		if v.Col < len(v.line()) {
			v.Col++ // e is inclusive
		}
	case "$":
		v.Col = len(v.line())
	case "0":
		v.Col = 0
	default:
		// Not a motion we support: abandon the operation rather than
		// guessing. The buffer is untouched.
		v.Row, v.Col = startRow, startCol
		return
	}

	if v.Row != startRow {
		// Multi-line operator motions are not supported; keep it simple
		// and safe rather than half-right.
		v.Row, v.Col = startRow, startCol
		return
	}

	lo, hi := startCol, v.Col
	if lo > hi {
		lo, hi = hi, lo
	}
	l := v.line()
	lo, hi = clampInt(lo, 0, len(l)), clampInt(hi, 0, len(l))
	text := string(l[lo:hi])

	switch op {
	case "y":
		v.yankTo(text, false)
		v.Row, v.Col = startRow, lo
	case "d", "c":
		v.yankTo(text, false)
		nl := append([]rune{}, l[:lo]...)
		nl = append(nl, l[hi:]...)
		v.lines[v.Row] = nl
		v.Col = lo
		if op == "c" {
			v.Mode = VimInsert
		}
	}
	v.clamp()
}

func (v *VimState) linewiseOp(op string, n int) {
	end := v.Row + n
	if end > len(v.lines) {
		end = len(v.lines)
	}
	seg := make([]string, 0, end-v.Row)
	for _, l := range v.lines[v.Row:end] {
		seg = append(seg, string(l))
	}
	text := strings.Join(seg, "\n")

	switch op {
	case "y":
		v.yankTo(text, true)
	case "d":
		v.yankTo(text, true)
		v.lines = append(v.lines[:v.Row], v.lines[end:]...)
		if len(v.lines) == 0 {
			v.lines = [][]rune{{}}
		}
		v.Col = 0
		v.clamp()
	case "c":
		v.yankTo(text, true)
		// cc clears the line but keeps it, then enters insert.
		v.lines = append(v.lines[:v.Row], append([][]rune{{}}, v.lines[end:]...)...)
		v.Col = 0
		v.Mode = VimInsert
	}
}

func (v *VimState) yankTo(text string, linewise bool) {
	v.register = text
	v.linewise = linewise
}

// put inserts the register. after=true is p, false is P.
func (v *VimState) put(after bool) {
	v.count = 0
	if v.register == "" {
		return
	}
	if v.linewise {
		at := v.Row
		if after {
			at++
		}
		newLines := vimLines(v.register)
		tail := append([][]rune{}, v.lines[at:]...)
		v.lines = append(v.lines[:at], append(newLines, tail...)...)
		v.Row = at
		v.Col = 0
		v.clamp()
		return
	}
	l := v.line()
	at := v.Col
	if after && len(l) > 0 {
		at++
	}
	at = clampInt(at, 0, len(l))
	reg := []rune(v.register)
	nl := append([]rune{}, l[:at]...)
	nl = append(nl, reg...)
	nl = append(nl, l[at:]...)
	v.lines[v.Row] = nl
	v.Col = at + len(reg) - 1
	v.clamp()
}

func (v *VimState) openLine(at int) {
	tail := append([][]rune{}, v.lines[at:]...)
	v.lines = append(v.lines[:at], append([][]rune{{}}, tail...)...)
	v.Row = at
	v.Col = 0
	v.Mode = VimInsert
}

func (v *VimState) deleteChars(n int) {
	l := v.line()
	if len(l) == 0 {
		return
	}
	end := v.Col + n
	if end > len(l) {
		end = len(l)
	}
	v.yankTo(string(l[v.Col:end]), false)
	nl := append([]rune{}, l[:v.Col]...)
	nl = append(nl, l[end:]...)
	v.lines[v.Row] = nl
	v.clamp()
}

func (v *VimState) moveCols(delta int) {
	v.Col += delta
	v.clamp()
}

func (v *VimState) moveRows(delta int) {
	v.Row += delta
	v.clamp()
}

// --- word motions -----------------------------------------------------
//
// One class of "word" only: runs of non-space. vim's distinction between
// word and WORD earns its keep in code, less so in a sticky note.

func isSpace(r rune) bool { return r == ' ' || r == '\t' }

// wordForwardForOperator is `w` as an operator target. It differs from
// the motion in one way: at the last word of a line it advances to the
// line end rather than parking on that word's first character, so `dw`
// deletes the final word instead of nothing.
func (v *VimState) wordForwardForOperator() {
	l := v.line()
	i := v.Col
	for i < len(l) && !isSpace(l[i]) {
		i++
	}
	for i < len(l) && isSpace(l[i]) {
		i++
	}
	v.Col = i // may be len(l); the operator span clamps it
}

func (v *VimState) wordForward() {
	l := v.line()
	i := v.Col
	// Step off the current word...
	for i < len(l) && !isSpace(l[i]) {
		i++
	}
	// ...then over the gap.
	for i < len(l) && isSpace(l[i]) {
		i++
	}
	if i >= len(l) {
		// Past the end: move to the next line, as vim does.
		if v.Row < len(v.lines)-1 {
			v.Row++
			v.Col = 0
			// Land on the first non-space.
			nl := v.line()
			j := 0
			for j < len(nl) && isSpace(nl[j]) {
				j++
			}
			v.Col = j
			v.clamp()
			return
		}
		// Last line, no further word: vim parks on the final character.
		i = len(l)
		if i > 0 {
			i--
		}
	}
	v.Col = i
	v.clamp()
}

func (v *VimState) wordBack() {
	l := v.line()
	i := v.Col
	if i == 0 {
		if v.Row > 0 {
			v.Row--
			v.Col = len(v.line())
			v.clamp()
			// Land on the start of the last word.
			v.wordBackWithin()
		}
		return
	}
	i--
	for i > 0 && isSpace(l[i]) {
		i--
	}
	for i > 0 && !isSpace(l[i-1]) {
		i--
	}
	v.Col = i
	v.clamp()
}

func (v *VimState) wordBackWithin() {
	l := v.line()
	i := v.Col
	for i > 0 && isSpace(l[min(i, len(l)-1)]) {
		i--
	}
	for i > 0 && !isSpace(l[i-1]) {
		i--
	}
	v.Col = i
	v.clamp()
}

func (v *VimState) wordEnd() {
	l := v.line()
	if len(l) == 0 {
		return
	}
	i := v.Col
	// Move at least one, so repeated e advances; but never past the end.
	if i < len(l)-1 {
		i++
	}
	for i < len(l)-1 && isSpace(l[i]) {
		i++
	}
	for i < len(l)-1 && !isSpace(l[i+1]) {
		i++
	}
	v.Col = i
	v.clamp()
}

// --- visual mode ------------------------------------------------------

func (v *VimState) visualKey(key string) {
	switch key {
	case "esc":
		v.count = 0
		v.Mode = VimNormal
		v.clamp()
	case "h", "left":
		v.moveCols(-v.takeCount())
	case "l", "right":
		v.moveCols(v.takeCount())
	case "j", "down":
		v.moveRows(v.takeCount())
	case "k", "up":
		v.moveRows(-v.takeCount())
	case "0":
		v.Col = 0
	case "$":
		v.Col = len(v.line())
		v.clamp()
	case "w":
		for n := v.takeCount(); n > 0; n-- {
			v.wordForward()
		}
	case "b":
		for n := v.takeCount(); n > 0; n-- {
			v.wordBack()
		}
	case "e":
		for n := v.takeCount(); n > 0; n-- {
			v.wordEnd()
		}
	case "d", "x":
		v.visualDelete(false)
	case "c":
		v.visualDelete(true)
	case "y":
		v.visualYank()
	default:
		if len(key) == 1 && key[0] >= '1' && key[0] <= '9' {
			v.count = v.count*10 + int(key[0]-'0')
		}
	}
}

// visualSpan returns the ordered selection bounds, inclusive of the end
// character (vim's charwise visual includes the cursor).
func (v *VimState) visualSpan() (r1, c1, r2, c2 int) {
	r1, c1 = v.visRow, v.visCol
	r2, c2 = v.Row, v.Col
	if r1 > r2 || (r1 == r2 && c1 > c2) {
		r1, c1, r2, c2 = r2, c2, r1, c1
	}
	return
}

func (v *VimState) visualText() string {
	r1, c1, r2, c2 := v.visualSpan()
	if r1 == r2 {
		l := v.lines[r1]
		lo := clampInt(c1, 0, len(l))
		hi := clampInt(c2+1, 0, len(l))
		return string(l[lo:hi])
	}
	var b strings.Builder
	first := v.lines[r1]
	b.WriteString(string(first[clampInt(c1, 0, len(first)):]))
	for r := r1 + 1; r < r2; r++ {
		b.WriteString("\n")
		b.WriteString(string(v.lines[r]))
	}
	b.WriteString("\n")
	last := v.lines[r2]
	b.WriteString(string(last[:clampInt(c2+1, 0, len(last))]))
	return b.String()
}

func (v *VimState) visualYank() {
	v.yankTo(v.visualText(), false)
	r1, c1, _, _ := v.visualSpan()
	v.Mode = VimNormal
	v.Row, v.Col = r1, c1
	v.clamp()
}

func (v *VimState) visualDelete(change bool) {
	r1, c1, r2, c2 := v.visualSpan()
	v.yankTo(v.visualText(), false)

	if r1 == r2 {
		l := v.lines[r1]
		lo := clampInt(c1, 0, len(l))
		hi := clampInt(c2+1, 0, len(l))
		nl := append([]rune{}, l[:lo]...)
		nl = append(nl, l[hi:]...)
		v.lines[r1] = nl
	} else {
		first := v.lines[r1]
		last := v.lines[r2]
		joined := append([]rune{}, first[:clampInt(c1, 0, len(first))]...)
		joined = append(joined, last[clampInt(c2+1, 0, len(last)):]...)
		v.lines = append(v.lines[:r1], append([][]rune{joined}, v.lines[r2+1:]...)...)
	}
	v.Row, v.Col = r1, c1
	if change {
		v.Mode = VimInsert
	} else {
		v.Mode = VimNormal
	}
	v.clamp()
}
