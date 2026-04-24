package main

// strings.go — red-string rendering (between notes or between a note and
// a wall-pin), plus "pull" state with spring-damped free end.

import "math"

// --- PullState --------------------------------------------------------

type PullState struct {
	Active bool
	FromID string // note ID the string starts at

	TargetX int
	TargetY int

	VX float32
	VY float32
}

func (p *PullState) Start(fromID string, fromX, fromY int) {
	p.Active = true
	p.FromID = fromID
	p.TargetX = fromX
	p.TargetY = fromY
	p.VX = float32(fromX)
	p.VY = float32(fromY)
}

func (p *PullState) SetTarget(x, y int) {
	p.TargetX = x
	p.TargetY = y
}

func (p *PullState) Tick() {
	if !p.Active {
		return
	}
	const easing = float32(0.24)
	dx := float32(p.TargetX) - p.VX
	dy := float32(p.TargetY) - p.VY
	p.VX += dx * easing
	p.VY += dy * easing
}

func (p *PullState) Stop() {
	p.Active = false
	p.FromID = ""
}

// --- rendering ---------------------------------------------------------

// drawStringsBehind renders only the strings with InFront=false. Called
// BEFORE notes are drawn so they sit beneath.
func drawStringsBehind(c *Canvas, b *Board, hoverIdx int) {
	drawStringSet(c, b, hoverIdx, false)
}

// drawStringsInFront renders strings with InFront=true plus the in-progress
// pull. Called AFTER notes so they sit on top.
func drawStringsInFront(c *Canvas, b *Board, pull *PullState, hoverIdx int) {
	drawStringSet(c, b, hoverIdx, true)
	if pull != nil && pull.Active {
		from := b.FindNote(pull.FromID)
		if from != nil {
			fx, fy := from.PinPos(b.Zoom)
			vx := int(pull.VX + 0.5)
			vy := int(pull.VY + 0.5)
			drawCurve(c, fx, fy, vx, vy, false, StringRd, AttrBold)
			c.SetRune(fx, fy, '◆', StringRd, AttrBold)
			c.SetRune(vx, vy, '◉', StringHi, AttrBold)
		}
	}
}

// drawAllStrings is the legacy combined renderer (still used by the
// edit-mode backdrop where draw order is just one pass).
func drawAllStrings(c *Canvas, b *Board, pull *PullState, hoverIdx int) {
	drawStringSet(c, b, hoverIdx, false)
	drawStringSet(c, b, hoverIdx, true)
	if pull != nil && pull.Active {
		from := b.FindNote(pull.FromID)
		if from != nil {
			fx, fy := from.PinPos(b.Zoom)
			vx := int(pull.VX + 0.5)
			vy := int(pull.VY + 0.5)
			drawCurve(c, fx, fy, vx, vy, false, StringRd, AttrBold)
			c.SetRune(fx, fy, '◆', StringRd, AttrBold)
			c.SetRune(vx, vy, '◉', StringHi, AttrBold)
		}
	}
}

// drawStringSet is the shared inner loop — draws all strings whose
// InFront matches `inFront`.
func drawStringSet(c *Canvas, b *Board, hoverIdx int, inFront bool) {
	for i, s := range b.Strings {
		if s.InFront != inFront {
			continue
		}
		ax, ay, aok := s.A.Pos(b)
		bx, by, bok := s.B.Pos(b)
		if !aok || !bok {
			continue
		}
		col := StringRd
		attr := uint8(AttrBold)
		tipGlyph := '◆'
		wallGlyph := '◉'
		if i == hoverIdx {
			col = StringHi
			tipGlyph = '◈'
			wallGlyph = '◉'
		}
		drawCurve(c, ax, ay, bx, by, s.Tight, col, attr)
		if s.A.NoteID == "" {
			c.SetRune(ax, ay, wallGlyph, col, AttrBold)
		} else {
			c.SetRune(ax, ay, tipGlyph, col, AttrBold)
		}
		if s.B.NoteID == "" {
			c.SetRune(bx, by, wallGlyph, col, AttrBold)
		} else {
			c.SetRune(bx, by, tipGlyph, col, AttrBold)
		}
	}
}

// drawCurve is the low-level renderer. If `tight`, straight line. Else,
// parabolic sag. Chars chosen with terminal aspect (~2:1) correction so
// diagonals look right.
func drawCurve(c *Canvas, fx, fy, tx, ty int, tight bool, col RGB, attr uint8) {
	dx := float32(tx - fx)
	dy := float32(ty - fy)
	// Use aspect-corrected distance for step count (so vertical spans
	// don't under-sample).
	distVis := float32(math.Sqrt(float64(dx*dx + (dy*2)*(dy*2))))
	if distVis < 1 {
		c.SetRune(fx, fy, '◆', col, attr)
		return
	}

	var sag float32
	if !tight {
		// Sag scales with horizontal span (looks more like a catenary).
		horiz := float32(math.Abs(float64(dx)))
		sag = horiz * 0.12
		if sag > 6 {
			sag = 6
		}
		if sag < 1 && horiz > 4 {
			sag = 1
		}
	}

	steps := int(distVis * 2)
	if steps < 10 {
		steps = 10
	}

	lastX, lastY := -9999, -9999
	dt := float32(1.0) / float32(steps)
	for i := 0; i <= steps; i++ {
		t := float32(i) * dt
		x, y := evalCurve(fx, fy, dx, dy, sag, t)
		x2, y2 := evalCurve(fx, fy, dx, dy, sag, t+dt)
		sx, sy := x2-x, y2-y

		cx, cy := int(x+0.5), int(y+0.5)
		if cx == lastX && cy == lastY {
			continue
		}
		// Don't draw over the exact endpoints (we set knots after).
		if (cx == fx && cy == fy) || (cx == tx && cy == ty) {
			lastX, lastY = cx, cy
			continue
		}
		lastX, lastY = cx, cy

		ch := pickStringChar(sx, sy)
		c.SetRune(cx, cy, ch, col, attr)
	}
}

// evalCurve returns (x,y) at parametric t for a linear interp between
// (fx,fy) and (fx+dx,fy+dy) plus a downward sag = 4*sagAmt*t*(1-t).
func evalCurve(fx, fy int, dx, dy float32, sagAmt float32, t float32) (float32, float32) {
	x := float32(fx) + dx*t
	y := float32(fy) + dy*t + sagAmt*4*t*(1-t)
	return x, y
}

// pickStringChar maps a local (dx, dy) direction to a line character,
// accounting for terminal cell aspect ratio (~2:1 tall:wide). Without this
// correction, near-horizontal lines would often pick diagonal chars.
func pickStringChar(dx, dy float32) rune {
	// Treat one vertical cell as if it were 2 wide for angle calc —
	// matches the visual aspect of the terminal.
	aDy := dy * 2
	ax := absF(dx)
	ay := absF(aDy)
	switch {
	case ax > 1.8*ay:
		return '─'
	case ay > 1.8*ax:
		return '│'
	case dx*dy < 0: // up-right or down-left → `/`
		return '╱'
	default: // down-right or up-left → `\`
		return '╲'
	}
}

func absF(x float32) float32 {
	if x < 0 {
		return -x
	}
	return x
}
