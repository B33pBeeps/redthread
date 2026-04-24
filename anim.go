package main

// anim.go — smooth 3D zoom transition (no flash, no vignette, no dither).
// The note rect interpolates from source to a centered card rect. A half-
// sine Y-lift during the middle sells a subtle 3D "coming toward you"
// hint; the shadow grows with it.

import (
	"math"
	"time"
)

type TransitionMode int

const (
	TransitionIn TransitionMode = iota
	TransitionOut
)

type Rect struct{ X, Y, W, H int }

// TargetRect returns the centered edit-card rect for a given canvas size.
func TargetRect(w, h int) Rect {
	cw := 60
	ch := 20
	if cw > w-6 {
		cw = w - 6
	}
	if ch > h-4 {
		ch = h - 4
	}
	if cw < 22 {
		cw = w
	}
	if ch < 10 {
		ch = h - 1
	}
	return Rect{
		X: (w - cw) / 2,
		Y: (h - ch) / 2,
		W: cw,
		H: ch,
	}
}

type Transition struct {
	Mode     TransitionMode
	Start    time.Time
	Source   Rect
	NoteID   string
	Duration time.Duration
}

// RectFromNote builds the source rect for a note at the board's zoom
// (SCREEN coords, no bob — transitions should start from the stable
// resting position, not mid-bounce).
func RectFromNote(n *Note, zoom int) Rect {
	return n.ScreenRect(zoom)
}

func NewTransitionIn(n *Note, zoom int) *Transition {
	return &Transition{
		Mode:     TransitionIn,
		Start:    time.Now(),
		Source:   RectFromNote(n, zoom),
		NoteID:   n.ID,
		Duration: 320 * time.Millisecond,
	}
}

func NewTransitionOut(n *Note, zoom int) *Transition {
	return &Transition{
		Mode:     TransitionOut,
		Start:    time.Now(),
		Source:   RectFromNote(n, zoom),
		NoteID:   n.ID,
		Duration: 260 * time.Millisecond,
	}
}

// Progress: 0 = at source, 1 = at target. In runs 0→1; Out runs 1→0.
func (t *Transition) Progress(now time.Time) float32 {
	e := now.Sub(t.Start)
	if e < 0 {
		e = 0
	}
	if e > t.Duration {
		e = t.Duration
	}
	p := float32(e) / float32(t.Duration)
	if t.Mode == TransitionOut {
		p = 1 - p
	}
	return p
}

func (t *Transition) Done(now time.Time) bool {
	return now.Sub(t.Start) >= t.Duration
}

// --- easings ---------------------------------------------------------

func easeOutCubic(t float32) float32 {
	u := 1 - t
	return 1 - u*u*u
}

func easeInOutCubic(t float32) float32 {
	if t < 0.5 {
		return 4 * t * t * t
	}
	u := -2*t + 2
	return 1 - (u*u*u)/2
}

func lerpI(a, b int, t float32) int { return a + int(float32(b-a)*t+0.5) }

// lerpRect eases rect a toward b by t with easeInOutCubic — gives a
// nicer "glide" than a linear or ease-out-only curve.
func lerpRect(a, b Rect, t float32) Rect {
	e := easeInOutCubic(t)
	return Rect{
		X: lerpI(a.X, b.X, e),
		Y: lerpI(a.Y, b.Y, e),
		W: lerpI(a.W, b.W, e),
		H: lerpI(a.H, b.H, e),
	}
}

// riseOffset returns the Y-offset (negative = up) applied during the
// morph to sell the 3D lift. Peaks at the middle of the transition with
// amplitude ~3 cells, returning to 0 on arrival.
func riseOffset(progress float32) int {
	f := float32(math.Sin(float64(progress) * math.Pi))
	if f < 0 {
		f = 0
	}
	return -int(f*3 + 0.5)
}

// shadowGrowth returns how many extra cells of shadow thickness to add
// during the morph. Peaks mid-transition.
func shadowGrowth(progress float32) int {
	f := float32(math.Sin(float64(progress) * math.Pi))
	if f < 0 {
		f = 0
	}
	return int(f*3 + 0.5)
}
