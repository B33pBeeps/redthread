package app

// notes.go — Note, Board (with world-coord notes + zoom scale that
// affects BOTH positions AND sizes), StringConn with wall-pins, cork gen.

import (
	"math/rand"
	"time"
)

// Base (world-coord) note dimensions at zoom 0.
const (
	baseNoteW = 36
	baseNoteH = 14
)

// ZoomMin / ZoomMax — 5 levels total, all clamped here.
const (
	ZoomMin = -3
	ZoomMax = +1
)

// zoomScale returns the render-time scale factor applied to BOTH the note
// dimensions AND the note positions. This is "workspace zoom": at zoom -3
// the whole board is shrunk so more notes fit; at zoom +1 notes are
// bigger and spread further apart.
func zoomScale(z int) float32 {
	switch z {
	case -3:
		return 0.40
	case -2:
		return 0.55
	case -1:
		return 0.75
	case 0:
		return 1.00
	case +1:
		return 1.35
	}
	if z < ZoomMin {
		return zoomScale(ZoomMin)
	}
	return zoomScale(ZoomMax)
}

type NoteDims struct{ W, H int }

// DimsFor returns rendered note size at a zoom level.
func DimsFor(zoom int) NoteDims {
	s := zoomScale(zoom)
	w := int(float32(baseNoteW)*s + 0.5)
	h := int(float32(baseNoteH)*s + 0.5)
	if w < 6 {
		w = 6
	}
	if h < 4 {
		h = 4
	}
	return NoteDims{W: w, H: h}
}

// WorldX / WorldY convert a screen cell coordinate to the equivalent
// world coordinate given the zoom scale.
func WorldX(screenX, zoom int) int {
	s := zoomScale(zoom)
	if s == 0 {
		return screenX
	}
	return int(float32(screenX)/s + 0.5)
}

func WorldY(screenY, zoom int) int { return WorldX(screenY, zoom) }

// ScreenX / ScreenY convert a world cell to screen (no bob applied).
func ScreenX(worldX, zoom int) int {
	return int(float32(worldX)*zoomScale(zoom) + 0.5)
}

func ScreenY(worldY, zoom int) int { return ScreenX(worldY, zoom) }

// ScreenDeltaToWorld converts a 1-cell screen delta into a world delta,
// ensuring a non-zero input always moves at least 1 world cell.
func ScreenDeltaToWorld(d, zoom int) int {
	if d == 0 {
		return 0
	}
	s := zoomScale(zoom)
	if s == 0 {
		return d
	}
	v := int(float32(d)/s + 0.5)
	if v == 0 {
		if d > 0 {
			return 1
		}
		return -1
	}
	return v
}

// --- Note -------------------------------------------------------------

type Note struct {
	ID      string    `json:"id"`
	Title   string    `json:"title"`
	Body    string    `json:"body"`
	X       int       `json:"x"` // WORLD coord
	Y       int       `json:"y"` // WORLD coord
	Tint    string    `json:"tint"`
	Created time.Time `json:"created"`
	Updated time.Time `json:"updated"`

	// Runtime-only:
	Bob    float32 `json:"-"` // SCREEN-cell vertical bob
	BobX   float32 `json:"-"` // SCREEN-cell lateral wobble
	Lifted bool    `json:"-"`
	Flash  float32 `json:"-"`
}

func roundToward(f float32) int {
	if f >= 0 {
		return int(f + 0.5)
	}
	return int(f - 0.5)
}

// EffectiveX: SCREEN X = world→screen projection + bob.
func (n *Note) EffectiveX(zoom int) int {
	return ScreenX(n.X, zoom) + roundToward(n.BobX)
}

// EffectiveY: SCREEN Y = world→screen projection + bob.
func (n *Note) EffectiveY(zoom int) int {
	return ScreenY(n.Y, zoom) + roundToward(n.Bob)
}

// ScreenRect — drawing rect (no bob, stable for shadow/hit-test).
func (n *Note) ScreenRect(zoom int) Rect {
	d := DimsFor(zoom)
	return Rect{X: ScreenX(n.X, zoom), Y: ScreenY(n.Y, zoom), W: d.W, H: d.H}
}

// Hit tests a screen-cell (cx, cy) against the note's current screen rect.
func (n *Note) Hit(cx, cy, zoom int) bool {
	r := n.ScreenRect(zoom)
	return cx >= r.X && cx < r.X+r.W && cy >= r.Y && cy < r.Y+r.H
}

// PinPos — screen position of the pin (top-center) including bob.
func (n *Note) PinPos(zoom int) (int, int) {
	r := n.ScreenRect(zoom)
	return r.X + r.W/2 + roundToward(n.BobX), r.Y + roundToward(n.Bob)
}

// --- String connections ------------------------------------------------

type StringEnd struct {
	NoteID string `json:"note,omitempty"`
	X      int    `json:"x,omitempty"` // WORLD coord (for wall pins)
	Y      int    `json:"y,omitempty"`
}

type StringConn struct {
	A       StringEnd `json:"a"`
	B       StringEnd `json:"b"`
	Tight   bool      `json:"tight"`
	InFront bool      `json:"front,omitempty"`

	// legacy v2 fields (loaded, never written)
	FromID string `json:"from,omitempty"`
	ToID   string `json:"to,omitempty"`
}

func (s *StringConn) normalizeLegacy() {
	if s.A.NoteID == "" && s.A.X == 0 && s.A.Y == 0 && s.FromID != "" {
		s.A.NoteID = s.FromID
	}
	if s.B.NoteID == "" && s.B.X == 0 && s.B.Y == 0 && s.ToID != "" {
		s.B.NoteID = s.ToID
	}
	s.FromID = ""
	s.ToID = ""
}

func (s *StringConn) InvolvesNote(id string) bool {
	return s.A.NoteID == id || s.B.NoteID == id
}

// Pos returns an endpoint's SCREEN position (for drawing).
func (e *StringEnd) Pos(b *Board) (int, int, bool) {
	if e.NoteID == "" {
		// Wall pin — stored in world coords.
		return ScreenX(e.X, b.Zoom), ScreenY(e.Y, b.Zoom), true
	}
	n := b.FindNote(e.NoteID)
	if n == nil {
		return 0, 0, false
	}
	x, y := n.PinPos(b.Zoom)
	return x, y, true
}

// --- Board -------------------------------------------------------------

type Board struct {
	Notes          []*Note
	Strings        []*StringConn
	Selected       string
	TextMode       TextStyleMode
	Zoom           int `json:"zoom,omitempty"`
	HighlightColor int `json:"highlightColor,omitempty"` // index into SelBorderChoices
}

// ApplyGlobalBorder syncs the package-level SelBorder with this board's
// HighlightColor. Call on load and whenever HighlightColor changes.
func (b *Board) ApplyGlobalBorder() {
	idx := b.HighlightColor
	if idx < 0 || idx >= len(SelBorderChoices) {
		idx = 0
	}
	SelBorder = SelBorderChoices[idx].Color
}

func (b *Board) Selection() *Note {
	if b.Selected == "" {
		return nil
	}
	for _, n := range b.Notes {
		if n.ID == b.Selected {
			return n
		}
	}
	return nil
}

func (b *Board) Select(id string) { b.Selected = id }

func (b *Board) Cycle(direction int) {
	if len(b.Notes) == 0 {
		return
	}
	idx := -1
	for i, n := range b.Notes {
		if n.ID == b.Selected {
			idx = i
			break
		}
	}
	idx += direction
	if idx < 0 {
		idx = len(b.Notes) - 1
	}
	if idx >= len(b.Notes) {
		idx = 0
	}
	b.Selected = b.Notes[idx].ID
}

func (b *Board) HitTopmost(cx, cy int) int {
	for i := len(b.Notes) - 1; i >= 0; i-- {
		if b.Notes[i].Hit(cx, cy, b.Zoom) {
			return i
		}
	}
	return -1
}

func (b *Board) Raise(i int) {
	if i < 0 || i >= len(b.Notes) {
		return
	}
	n := b.Notes[i]
	b.Notes = append(b.Notes[:i], b.Notes[i+1:]...)
	b.Notes = append(b.Notes, n)
	b.Selected = n.ID
}

// NewNote places a note at screen-centre converted to world coords, so it
// appears centred at whatever zoom level the user is on.
func (b *Board) NewNote(screenW, screenH int) *Note {
	now := time.Now().UTC()
	tint := TintOrder[len(b.Notes)%len(TintOrder)]
	d := DimsFor(b.Zoom)
	screenCx := screenW/2 - d.W/2
	screenCy := screenH/2 - d.H/2
	offset := (len(b.Notes) % 5) * 2
	worldX := WorldX(screenCx+offset, b.Zoom)
	worldY := WorldY(screenCy+offset, b.Zoom)
	n := &Note{
		ID:      genID(),
		Title:   "new note",
		Body:    "write it here.\nenter to zoom, esc to place back.",
		X:       worldX,
		Y:       worldY,
		Tint:    tint,
		Created: now,
		Updated: now,
	}
	b.Notes = append(b.Notes, n)
	b.Selected = n.ID
	return n
}

func (b *Board) Delete(id string) {
	for i, n := range b.Notes {
		if n.ID == id {
			b.Notes = append(b.Notes[:i], b.Notes[i+1:]...)
			break
		}
	}
	kept := b.Strings[:0]
	for _, s := range b.Strings {
		if !s.InvolvesNote(id) {
			kept = append(kept, s)
		}
	}
	b.Strings = kept
	if b.Selected == id {
		b.Selected = ""
		if len(b.Notes) > 0 {
			b.Selected = b.Notes[len(b.Notes)-1].ID
		}
	}
}

func (b *Board) FindNote(id string) *Note {
	for _, n := range b.Notes {
		if n.ID == id {
			return n
		}
	}
	return nil
}

func (b *Board) Connect(fromID, toID string) *StringConn {
	if fromID == toID || fromID == "" || toID == "" {
		return nil
	}
	for _, s := range b.Strings {
		if (s.A.NoteID == fromID && s.B.NoteID == toID) ||
			(s.A.NoteID == toID && s.B.NoteID == fromID) {
			return s
		}
	}
	s := &StringConn{
		A:     StringEnd{NoteID: fromID},
		B:     StringEnd{NoteID: toID},
		Tight: false,
	}
	b.Strings = append(b.Strings, s)
	return s
}

// ConnectToWall creates a string from a note to a WORLD-coord wall-pin.
// The caller is responsible for converting a mouse click (screen) to
// world before calling this.
func (b *Board) ConnectToWall(fromID string, worldX, worldY int) *StringConn {
	if fromID == "" {
		return nil
	}
	s := &StringConn{
		A:     StringEnd{NoteID: fromID},
		B:     StringEnd{X: worldX, Y: worldY},
		Tight: false,
	}
	b.Strings = append(b.Strings, s)
	return s
}

func (b *Board) StringsTouching(id string) []int {
	var out []int
	for i, s := range b.Strings {
		if s.InvolvesNote(id) {
			out = append(out, i)
		}
	}
	return out
}

func (b *Board) DeleteStringAt(i int) bool {
	if i < 0 || i >= len(b.Strings) {
		return false
	}
	b.Strings = append(b.Strings[:i], b.Strings[i+1:]...)
	return true
}

func (b *Board) DeleteStringsTouching(id string) int {
	kept := b.Strings[:0]
	removed := 0
	for _, s := range b.Strings {
		if s.InvolvesNote(id) {
			removed++
			continue
		}
		kept = append(kept, s)
	}
	b.Strings = kept
	return removed
}

// --- id generator ------------------------------------------------------

var idRNG = rand.New(rand.NewSource(time.Now().UnixNano()))

const idAlphabet = "abcdefghijkmnpqrstuvwxyzABCDEFGHJKLMNPQRSTUVWXYZ23456789"

func genID() string {
	b := make([]byte, 6)
	for i := range b {
		b[i] = idAlphabet[idRNG.Intn(len(idAlphabet))]
	}
	return string(b)
}

// --- cork texture ------------------------------------------------------

type Star struct {
	X, Y  int
	R     rune
	Color RGB
}

// GenStars returns a stable cork-texture for (w,h). Dense ~34% with
// four tiers: flecks, patches, pores, blotches. (w,h) seeded so stable.
func GenStars(w, h int) []Star {
	out := make([]Star, 0, w*h/3)
	s := uint64(w)*73856093 ^ uint64(h)*19349663 ^ 0x12345678
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			s = s*6364136223846793005 + 1442695040888963407
			n := s >> 33
			switch {
			case n%230 == 0:
				ch := BlotchChars[(s>>17)%uint64(len(BlotchChars))]
				out = append(out, Star{X: x, Y: y, R: ch, Color: CorkBlotch})
			case n%80 == 0:
				out = append(out, Star{X: x, Y: y, R: '▒', Color: CorkPatchFg})
			case n%42 == 0:
				ch := PoreChars[(s>>17)%uint64(len(PoreChars))]
				out = append(out, Star{X: x, Y: y, R: ch, Color: CorkPore})
			case n%3 == 0:
				ch := StarChars[(s>>17)%uint64(len(StarChars))]
				col := CorkShades[(s>>23)%uint64(len(CorkShades))]
				out = append(out, Star{X: x, Y: y, R: ch, Color: col})
			}
		}
	}
	return out
}

// --- seed ---------------------------------------------------------

func seedBoard() *Board {
	now := time.Now().UTC()
	b := &Board{}
	// Positions are world-coords at the unit scale (zoom 0).
	b.Notes = []*Note{
		{
			ID: genID(), Title: "welcome", Body: "• drag with the mouse\n• tab cycles\n• enter zooms\n• s pulls a string\n• - = 0 workspace zoom",
			X: 3, Y: 1, Tint: "yellow", Created: now, Updated: now,
		},
		{
			ID: genID(), Title: "write rfc", Body: "- auth spec\n- sync protocol\n- review tuesday",
			X: 44, Y: 6, Tint: "pink", Created: now, Updated: now,
		},
		{
			ID: genID(), Title: "call mom", Body: "after 5pm.\nask about the trip.",
			X: 22, Y: 16, Tint: "blue", Created: now, Updated: now,
		},
	}
	if len(b.Notes) > 0 {
		b.Selected = b.Notes[0].ID
		b.Connect(b.Notes[0].ID, b.Notes[1].ID)
	}
	return b
}
