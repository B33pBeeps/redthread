package app

// model.go — Bubble Tea MVU. Wires board, zoom, strings, transitions,
// editor, font menu, persistence.

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type Mode int

const (
	ModeBoard Mode = iota
	ModeEdit
)

type tickMsg time.Time

func tickCmd() tea.Cmd {
	return tea.Tick(16*time.Millisecond, func(t time.Time) tea.Msg { return tickMsg(t) })
}

type model struct {
	w, h int
	now  time.Time
	mode Mode

	workspace *Workspace
	board     *Board // always points to workspace.ActiveBoard()
	stars     []Star
	saver     *Saver

	grabID                 string
	grabDx, grabDy         int
	grabStartX, grabStartY int // world-coords at press, used to detect click vs drag

	pull     PullState
	hoverStr int

	transition *Transition

	editor Editor

	lastClickT time.Time
	lastClickN string

	// help panel: slides up from the bottom when toggled with `?`.
	// helpTarget is 0 (closed) or 1 (open); helpAnim eases toward target.
	helpAnim   float32
	helpTarget float32

	fontMenu *FontMenu

	// rename mode: when true, keystrokes edit renameBuffer until enter/esc.
	renaming     bool
	renameBuffer string

	// transient status message shown in the footer
	toast      string
	toastUntil time.Time

	// "press D again" arm window for board deletion
	deleteArmedUntil time.Time

	mouseX, mouseY int
}

func initialModel(ws *Workspace) model {
	m := model{
		mode:      ModeBoard,
		workspace: ws,
		board:     ws.ActiveBoard(),
		now:       time.Now(),
		saver:     NewSaver(ws, 400*time.Millisecond),
		hoverStr:  -1,
	}
	return m
}

// refreshActive points m.board at the workspace's current active board
// and regenerates per-board state (stars + global highlight color).
func (m *model) refreshActive() {
	m.board = m.workspace.ActiveBoard()
	if m.board == nil {
		return
	}
	m.board.ApplyGlobalBorder()
	if m.w > 0 && m.h > 0 {
		m.stars = GenStarsForBoard(m.w, m.h, m.board.GrainSeed)
	}
	m.hoverStr = -1
	m.grabID = ""
	m.pull.Stop()
}

func (m model) Init() tea.Cmd { return tickCmd() }

// --- Update ------------------------------------------------------------

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		seed := int64(0)
		if m.board != nil {
			seed = m.board.GrainSeed
		}
		m.stars = GenStarsForBoard(m.w, m.h, seed)
		if m.mode == ModeEdit {
			m.editor.Resize(m.w, m.h)
		}
		return m, nil

	case tickMsg:
		m.now = time.Time(msg)
		for _, n := range m.board.Notes {
			if n.Bob > 0.02 || n.Bob < -0.02 {
				n.Bob *= 0.82
			} else {
				n.Bob = 0
			}
			if n.BobX > 0.02 || n.BobX < -0.02 {
				n.BobX *= 0.82
			} else {
				n.BobX = 0
			}
			if n.Flash > 0.01 {
				n.Flash *= 0.82
			} else {
				n.Flash = 0
			}
		}
		m.pull.Tick()
		// help panel ease toward target. The snap threshold is generous
		// (0.02) so the exponential tail doesn't leave a stuck "near-zero"
		// frame visible on close.
		if m.helpTarget != m.helpAnim {
			diff := m.helpTarget - m.helpAnim
			m.helpAnim += diff * 0.28
			if absF(diff) < 0.02 {
				m.helpAnim = m.helpTarget
			}
		}
		if m.transition != nil && m.transition.Done(m.now) {
			switch m.transition.Mode {
			case TransitionIn:
				m.mode = ModeEdit
				if n := m.board.Selection(); n != nil {
					m.editor = NewEditor(n, m.w, m.h)
				}
			case TransitionOut:
				m.mode = ModeBoard
				if n := m.findNote(m.transition.NoteID); n != nil {
					// Jiggle on landing — bigger than a cycle bounce so
					// the note feels like it just "thudded" back home.
					n.Bob = 2.0
					n.BobX = 0.7
					n.Flash = 0.5
				}
			}
			m.transition = nil
		}
		return m, tickCmd()

	case tea.KeyMsg:
		return m.handleKey(msg)

	case tea.MouseMsg:
		return m.handleMouse(msg)
	}
	return m, nil
}

// --- key handlers ------------------------------------------------------

func (m model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	if key == "ctrl+c" {
		_ = m.saver.Flush()
		return m, tea.Quit
	}
	if m.transition != nil {
		return m, nil
	}
	if m.renaming {
		return m.handleRenameKey(msg, key)
	}
	if m.fontMenu != nil {
		return m.handleFontMenuKey(key)
	}
	if m.pull.Active && key == "esc" {
		m.pull.Stop()
		return m, nil
	}
	switch m.mode {
	case ModeBoard:
		return m.handleBoardKey(key)
	case ModeEdit:
		return m.handleEditKey(msg, key)
	}
	return m, nil
}

// handleRenameKey intercepts every keystroke while the user is editing a
// board's name in the tab bar. Enter commits, esc cancels.
func (m model) handleRenameKey(msg tea.KeyMsg, key string) (tea.Model, tea.Cmd) {
	switch key {
	case "esc":
		m.renaming = false
		m.renameBuffer = ""
		return m, nil
	case "enter":
		name := m.renameBuffer
		if name == "" {
			name = "board " + intToStr(m.workspace.ActiveIdx+1)
		}
		if m.board != nil {
			m.board.Name = name
			m.saver.Touch()
		}
		m.renaming = false
		m.renameBuffer = ""
		return m, nil
	case "backspace":
		if r := []rune(m.renameBuffer); len(r) > 0 {
			m.renameBuffer = string(r[:len(r)-1])
		}
		return m, nil
	}
	// Plain rune input
	if len(msg.Runes) > 0 {
		// Reasonable cap so the tab bar doesn't blow up
		if len([]rune(m.renameBuffer)) < 24 {
			m.renameBuffer += string(msg.Runes)
		}
	}
	return m, nil
}

func (m model) handleFontMenuKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "esc":
		// Revert to the mode that was active when the menu opened.
		m.board.TextMode = m.fontMenu.SavedMode
		m.fontMenu = nil
		return m, nil
	case "up", "k":
		m.fontMenu.Move(-1)
		m.board.TextMode = m.fontMenu.Selected()
		return m, nil
	case "down", "j":
		m.fontMenu.Move(1)
		m.board.TextMode = m.fontMenu.Selected()
		return m, nil
	case "enter":
		// Already applied on cursor move; just close + persist.
		m.board.TextMode = m.fontMenu.Selected()
		m.saver.Touch()
		m.fontMenu = nil
		return m, nil
	}
	return m, nil
}

func (m model) handleBoardKey(key string) (tea.Model, tea.Cmd) {
	switch key {
	case "q":
		_ = m.saver.Flush()
		return m, tea.Quit
	case ">", ".":
		// Next board
		m.workspace.CycleActive(1)
		m.refreshActive()
		m.saver.Touch()
		m.setToast("board: " + m.board.Name)
		return m, nil
	case "<", ",":
		// Previous board
		m.workspace.CycleActive(-1)
		m.refreshActive()
		m.saver.Touch()
		m.setToast("board: " + m.board.Name)
		return m, nil
	case "B":
		// Create a new board with a unique grain seed.
		nb := m.workspace.AddBoard("")
		m.refreshActive()
		m.saver.Touch()
		// Drop straight into rename mode so the user can name it.
		m.renaming = true
		m.renameBuffer = nb.Name
		return m, nil
	case "R":
		// Rename the active board.
		if m.board != nil {
			m.renaming = true
			m.renameBuffer = m.board.Name
		}
		return m, nil
	case "D":
		// Two-press delete with a ~2s arm window. First press warns;
		// second press (within the window) actually deletes.
		if m.now.Before(m.deleteArmedUntil) {
			name := m.board.Name
			if m.workspace.DeleteBoard(m.workspace.ActiveIdx) {
				m.refreshActive()
				m.saver.Touch()
				m.setToast("deleted board: " + name)
			} else {
				m.setToast("can't delete the last board")
			}
			m.deleteArmedUntil = time.Time{}
		} else {
			m.deleteArmedUntil = m.now.Add(2 * time.Second)
			m.setToast("press D again to delete '" + m.board.Name + "'")
		}
		return m, nil
	case "?":
		// Toggle the bottom-anchored help panel; tick eases the height.
		if m.helpTarget < 0.5 {
			m.helpTarget = 1
		} else {
			m.helpTarget = 0
		}
		return m, nil
	case "a":
		m.fontMenu = NewFontMenu(m.board.TextMode)
		return m, nil
	case "-", "_":
		m.zoomStep(-1)
		return m, nil
	case "=", "+":
		m.zoomStep(+1)
		return m, nil
	case "0":
		m.board.Zoom = 0
		m.saver.Touch()
		return m, nil
	case "tab":
		m.board.Cycle(1)
		m.hoverStr = -1
		return m.flashSel()
	case "shift+tab":
		m.board.Cycle(-1)
		m.hoverStr = -1
		return m.flashSel()
	case "left", "h":
		return m.nudgeSel(-1, 0)
	case "right", "l":
		return m.nudgeSel(1, 0)
	case "up", "k":
		return m.nudgeSel(0, -1)
	case "down", "j":
		return m.nudgeSel(0, 1)
	case "shift+left":
		return m.nudgeSel(-5, 0)
	case "shift+right":
		return m.nudgeSel(5, 0)
	case "shift+up":
		return m.nudgeSel(0, -5)
	case "shift+down":
		return m.nudgeSel(0, 5)
	case "enter":
		if m.pull.Active {
			m.pull.Stop()
			return m, nil
		}
		if n := m.board.Selection(); n != nil {
			m.transition = NewTransitionIn(n, m.board.Zoom)
		}
		return m, nil
	case "n":
		n := m.board.NewNote(m.w, m.h)
		n.Bob = 1.2
		n.Flash = 1.0
		m.saver.Touch()
		return m, nil
	case "d":
		if n := m.board.Selection(); n != nil {
			m.board.Delete(n.ID)
			m.hoverStr = -1
			m.saver.Touch()
		}
		return m, nil
	case "r":
		if n := m.board.Selection(); n != nil {
			for i, nn := range m.board.Notes {
				if nn.ID == n.ID {
					m.board.Raise(i)
					break
				}
			}
			m.saver.Touch()
		}
		return m, nil
	case "s":
		if m.pull.Active {
			m.pull.Stop()
			return m, nil
		}
		if n := m.board.Selection(); n != nil {
			px, py := n.PinPos(m.board.Zoom)
			m.pull.Start(n.ID, px, py)
			if m.mouseX != 0 || m.mouseY != 0 {
				m.pull.SetTarget(m.mouseX, m.mouseY)
			}
		}
		return m, nil
	case "[":
		m.cycleHoverStr(-1)
		return m, nil
	case "]":
		m.cycleHoverStr(1)
		return m, nil
	case "t":
		if m.hoverStr >= 0 && m.hoverStr < len(m.board.Strings) {
			m.board.Strings[m.hoverStr].Tight = !m.board.Strings[m.hoverStr].Tight
			m.saver.Touch()
		}
		return m, nil
	case "f":
		// Toggle the hovered string between in-front / behind notes.
		if m.hoverStr >= 0 && m.hoverStr < len(m.board.Strings) {
			m.board.Strings[m.hoverStr].InFront = !m.board.Strings[m.hoverStr].InFront
			m.saver.Touch()
		}
		return m, nil
	case "x":
		// Cut the hovered string only.
		if m.hoverStr >= 0 && m.hoverStr < len(m.board.Strings) {
			if m.board.DeleteStringAt(m.hoverStr) {
				m.hoverStr = -1
				m.saver.Touch()
			}
		}
		return m, nil
	case "X":
		// Cut every string on the selected note (bulk).
		if n := m.board.Selection(); n != nil {
			if m.board.DeleteStringsTouching(n.ID) > 0 {
				m.hoverStr = -1
				m.saver.Touch()
			}
		}
		return m, nil
	case "1", "2", "3", "4", "5", "6", "7", "8", "9":
		// Pick a tint (sticky-note paper color) by index.
		if n := m.board.Selection(); n != nil {
			idx := int(key[0] - '1')
			if idx >= 0 && idx < len(TintOrder) {
				n.Tint = TintOrder[idx]
				n.Updated = time.Now().UTC()
				n.Flash = 1.0
				m.setToast("tint: " + n.Tint)
				m.saver.Touch()
			}
		}
		return m, nil
	case "c":
		// Cycle the GLOBAL highlight color — the selection border for
		// every note. (Red pin + leading glyph stay red.)
		m.board.HighlightColor = (m.board.HighlightColor + 1) % len(SelBorderChoices)
		if m.board.HighlightColor < 0 {
			m.board.HighlightColor = 0
		}
		m.board.ApplyGlobalBorder()
		m.setToast("highlight: " + SelBorderChoices[m.board.HighlightColor].Name)
		if n := m.board.Selection(); n != nil {
			n.Flash = 0.8
		}
		m.saver.Touch()
		return m, nil
	}
	return m, nil
}

// setToast schedules a transient status message in the footer.
func (m *model) setToast(msg string) {
	m.toast = msg
	m.toastUntil = m.now.Add(1500 * time.Millisecond)
}

// zoomStep clamps within the full [ZoomMin, ZoomMax] range.
func (m *model) zoomStep(delta int) {
	z := m.board.Zoom + delta
	if z < ZoomMin {
		z = ZoomMin
	}
	if z > ZoomMax {
		z = ZoomMax
	}
	m.board.Zoom = z
	m.saver.Touch()
}

func (m *model) cycleHoverStr(delta int) {
	if len(m.board.Strings) == 0 {
		m.hoverStr = -1
		return
	}
	sel := m.board.Selected
	if sel == "" {
		m.hoverStr += delta
		if m.hoverStr < 0 {
			m.hoverStr = len(m.board.Strings) - 1
		}
		if m.hoverStr >= len(m.board.Strings) {
			m.hoverStr = 0
		}
		return
	}
	idxs := m.board.StringsTouching(sel)
	if len(idxs) == 0 {
		m.hoverStr = -1
		return
	}
	cur := -1
	for i, idx := range idxs {
		if idx == m.hoverStr {
			cur = i
			break
		}
	}
	cur += delta
	if cur < 0 {
		cur = len(idxs) - 1
	}
	if cur >= len(idxs) {
		cur = 0
	}
	m.hoverStr = idxs[cur]
}

func (m model) flashSel() (tea.Model, tea.Cmd) {
	if n := m.board.Selection(); n != nil {
		n.Flash = 0.8
		// Small jiggle so cycling with tab/S-tab feels tactile.
		n.Bob = 1.4
		n.BobX = 0.6
	}
	return m, nil
}

// nudgeSel moves the selected note by SCREEN-cell deltas (converted to
// world) and triggers a directional overshoot bounce. This keeps nudges
// feeling like "move one cell" at any zoom level.
func (m model) nudgeSel(dx, dy int) (tea.Model, tea.Cmd) {
	n := m.board.Selection()
	if n == nil {
		return m, nil
	}
	wdx := ScreenDeltaToWorld(dx, m.board.Zoom)
	wdy := ScreenDeltaToWorld(dy, m.board.Zoom)
	n.X += wdx
	n.Y += wdy
	mag := float32(2.0)
	if abs(dx) >= 5 || abs(dy) >= 5 {
		mag = 3.5
	}
	// Overshoot in screen-direction of motion.
	if dx < 0 {
		n.BobX = -mag
	} else if dx > 0 {
		n.BobX = mag
	}
	if dy < 0 {
		n.Bob = -mag
	} else if dy > 0 {
		n.Bob = mag
	}
	n.Updated = time.Now().UTC()
	m.saver.Touch()
	return m, nil
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func (m model) handleEditKey(msg tea.KeyMsg, key string) (tea.Model, tea.Cmd) {
	switch key {
	case "esc":
		if n := m.findNote(m.editor.NoteID); n != nil {
			title, body := m.editor.Split()
			n.Title = title
			n.Body = body
			n.Updated = time.Now().UTC()
			m.saver.Touch()
			m.transition = NewTransitionOut(n, m.board.Zoom)
		} else {
			m.mode = ModeBoard
		}
		return m, nil
	case "ctrl+s":
		if n := m.findNote(m.editor.NoteID); n != nil {
			title, body := m.editor.Split()
			n.Title = title
			n.Body = body
			n.Updated = time.Now().UTC()
			_ = m.saver.Flush()
		}
		return m, nil
	}
	cmd := m.editor.Update(msg)
	return m, cmd
}

// --- mouse handlers ---------------------------------------------------

func (m model) handleMouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if m.transition != nil {
		return m, nil
	}
	if m.fontMenu != nil {
		return m, nil
	}
	if m.mode != ModeBoard {
		cmd := m.editor.Update(msg)
		return m, cmd
	}

	m.mouseX, m.mouseY = msg.X, msg.Y
	if m.pull.Active {
		m.pull.SetTarget(msg.X, msg.Y)
	}

	if msg.Button != tea.MouseButtonLeft {
		return m, nil
	}

	switch msg.Action {
	case tea.MouseActionPress:
		if m.pull.Active {
			idx := m.board.HitTopmost(msg.X, msg.Y)
			if idx >= 0 {
				target := m.board.Notes[idx]
				if target.ID != m.pull.FromID {
					if s := m.board.Connect(m.pull.FromID, target.ID); s != nil {
						target.Flash = 0.8
						m.saver.Touch()
					}
				}
			} else {
				// Wall-pin: store in WORLD coords so it scales with zoom.
				wx := WorldX(msg.X, m.board.Zoom)
				wy := WorldY(msg.Y, m.board.Zoom)
				if s := m.board.ConnectToWall(m.pull.FromID, wx, wy); s != nil {
					m.saver.Touch()
				}
			}
			m.pull.Stop()
			return m, nil
		}
		idx := m.board.HitTopmost(msg.X, msg.Y)
		if idx < 0 {
			m.board.Select("")
			m.hoverStr = -1
			return m, nil
		}
		m.board.Raise(idx)
		n := m.board.Notes[len(m.board.Notes)-1]
		m.grabID = n.ID
		// Grab offset is stored in WORLD coords — so dragging stays accurate
		// regardless of zoom.
		wx := WorldX(msg.X, m.board.Zoom)
		wy := WorldY(msg.Y, m.board.Zoom)
		m.grabDx = wx - n.X
		m.grabDy = wy - n.Y
		m.grabStartX = n.X
		m.grabStartY = n.Y
		n.Lifted = true
		n.Flash = 0.6
		m.hoverStr = -1
		if m.lastClickN == n.ID && m.now.Sub(m.lastClickT) < 350*time.Millisecond {
			m.transition = NewTransitionIn(n, m.board.Zoom)
			m.grabID = ""
			n.Lifted = false
		}
		m.lastClickT = m.now
		m.lastClickN = n.ID

	case tea.MouseActionMotion:
		if m.grabID != "" {
			if n := m.findNote(m.grabID); n != nil {
				wx := WorldX(msg.X, m.board.Zoom)
				wy := WorldY(msg.Y, m.board.Zoom)
				n.X = wx - m.grabDx
				n.Y = wy - m.grabDy
			}
		}

	case tea.MouseActionRelease:
		if m.grabID != "" {
			if n := m.findNote(m.grabID); n != nil {
				moved := n.X != m.grabStartX || n.Y != m.grabStartY
				if moved {
					// Drop bounce after an actual move
					n.Bob = 2.2
					n.Updated = time.Now().UTC()
					m.saver.Touch()
				} else {
					// Click without move → small cycle-style jiggle
					n.Bob = 1.4
					n.BobX = 0.6
				}
				n.Lifted = false
			}
			m.grabID = ""
		}
	}
	return m, nil
}

// --- helpers ----------------------------------------------------------

func (m model) findNote(id string) *Note { return m.board.FindNote(id) }

// --- View -------------------------------------------------------------

func (m model) View() string {
	if m.w == 0 || m.h == 0 {
		return ""
	}

	if m.mode == ModeEdit && m.transition == nil {
		if n := m.findNote(m.editor.NoteID); n != nil {
			base := m.editor.View(m.w, m.h, n, m.stars, m.board.TextMode, m.board)
			if m.fontMenu != nil {
				return m.overlayFontMenuOnString(base)
			}
			return base
		}
	}

	c := NewCanvas(m.w, m.h)
	if m.transition != nil {
		m.drawTransitionView(c)
	} else {
		m.drawBoardView(c)
	}
	m.drawTabBar(c)
	m.drawFooterView(c)
	if m.helpAnim > 0.001 {
		drawHelpPanel(c, m.helpAnim)
	}
	if m.fontMenu != nil {
		drawFontMenu(c, m.fontMenu)
	}
	return c.Serialize()
}

func (m model) overlayFontMenuOnString(base string) string {
	rect := FontMenuRect(m.w, m.h-1)
	c := NewCanvas(rect.W, rect.H)
	drawFontMenuAt(c, 0, 0, m.fontMenu)
	return SpliceOverlay(base, c.Serialize(), rect.X, rect.Y)
}

func (m model) drawBoardView(c *Canvas) {
	drawCork(c, m.stars)

	// 1) Behind-strings first (under notes). Strings touching the
	//    grabbed note are skipped here and drawn on top in step 3 so
	//    the attachment stays visible while dragging.
	drawStringsBehind(c, m.board, m.hoverStr, m.grabID, nil)

	// 2) Notes (back-to-front, skipping grabbed so we can draw it on top)
	for _, n := range m.board.Notes {
		if n.ID == m.grabID {
			continue
		}
		drawShadow(c, n, m.board.Zoom)
		drawNote(c, n, n.ID == m.board.Selected, m.board.TextMode, m.board.Zoom)
	}
	if m.grabID != "" {
		if n := m.findNote(m.grabID); n != nil {
			drawShadow(c, n, m.board.Zoom)
			drawNote(c, n, true, m.board.TextMode, m.board.Zoom)
		}
	}

	// 3) In-front-strings + strings touching the grabbed note + the
	//    active pull overlay.
	drawStringsInFront(c, m.board, &m.pull, m.hoverStr, m.grabID, nil)
}

// drawTransitionView — clean 3D zoom with an animated backdrop fade.
// The cork + background notes fade from full brightness (at transition
// start) down to the edit-mode dim factor (~0.38) as progress approaches 1.
// The focused note is drawn on top AFTER the dim pass, so it stays sharp.
func (m model) drawTransitionView(c *Canvas) {
	progress := m.transition.Progress(m.now)
	target := TargetRect(c.W, c.H)
	morphing := lerpRect(m.transition.Source, target, progress)
	lift := riseOffset(progress)
	extraShadow := shadowGrowth(progress)

	// Base board (cork + background notes + strings) — drawn first, then
	// dimmed together so the fade feels unified.
	drawCork(c, m.stars)

	focusID := m.transition.NoteID
	// During the transition, the focused note's pin lives at the
	// morphing rect — strings should follow.
	override := &PinOverride{
		NoteID: focusID,
		X:      morphing.X + morphing.W/2,
		Y:      morphing.Y + lift,
	}
	// Strings touching the focused note are deferred to the in-front
	// pass so they sit on top of the floating card.
	drawStringsBehind(c, m.board, -1, focusID, override)
	for _, n := range m.board.Notes {
		if n.ID == focusID {
			continue
		}
		drawShadow(c, n, m.board.Zoom)
		drawNote(c, n, false, m.board.TextMode, m.board.Zoom)
	}
	drawStringsInFront(c, m.board, nil, -1, focusID, override)

	// Animated fade: 1.0 (no dim) → 0.38 (edit-mode dim).
	const editDim = float32(0.38)
	dim := 1.0 - (1.0-editDim)*progress
	c.Dim(dim)

	focus := m.findNote(focusID)
	if focus == nil {
		return
	}
	rect := Rect{X: morphing.X, Y: morphing.Y + lift, W: morphing.W, H: morphing.H}

	lifted := extraShadow > 0
	if lifted {
		focus.Lifted = true
	}
	shadowRect(c, rect.X, rect.Y, rect.W, rect.H, lifted)
	focus.Lifted = false

	tint := GetTint(focus.Tint)
	drawNoteAtWithBorder(c, rect, focus, m.board.TextMode, tint.Paper)
}

// drawTabBar draws row 0: " redthread  • work │  personal │  ideas ".
// The active board has a "•" prefix and brighter color. While renaming,
// the active board's slot becomes an inline editor.
func (m model) drawTabBar(c *Canvas) {
	if c.W < 4 {
		return
	}
	for x := 0; x < c.W; x++ {
		c.SetBlank(x, 0)
	}
	cur := 1
	c.WriteText(cur, 0, "redthread", PinRed, AttrBold)
	cur += runeLen("redthread") + 1
	c.SetRune(cur, 0, '│', Footer, 0)
	cur += 2
	for i, b := range m.workspace.Boards {
		if i > 0 {
			c.SetRune(cur, 0, '│', Footer, 0)
			cur += 2
		}
		isActive := i == m.workspace.ActiveIdx
		name := b.Name
		if name == "" {
			name = "board " + intToStr(i+1)
		}
		// Rename slot: replace the active name with the live buffer + caret.
		if m.renaming && isActive {
			disp := m.renameBuffer + "▏"
			c.WriteText(cur, 0, "✎ ", PinRed, AttrBold)
			cur += 2
			c.WriteText(cur, 0, disp, Flash, AttrBold)
			cur += runeLen(disp) + 1
			continue
		}
		marker := "  "
		col := DimText
		attr := uint8(0)
		if isActive {
			marker = "● "
			col = Flash
			attr = AttrBold
		}
		c.WriteText(cur, 0, marker+name, col, attr)
		cur += runeLen(marker+name) + 1
		if cur >= c.W-1 {
			break
		}
	}
}

func (m model) drawFooterView(c *Canvas) {
	// Transient toast takes priority when active.
	if !m.toastUntil.IsZero() && m.now.Before(m.toastUntil) {
		drawFooter(c, m.toast)
		return
	}
	switch {
	case m.fontMenu != nil:
		drawFooter(c, "↑↓ preview  •  enter set  •  esc cancel")
		return
	case m.pull.Active:
		drawFooter(c, "pull mode • click note to connect • click empty cork for wall-pin • esc cancel")
		return
	case m.transition != nil:
		drawFooter(c, "…")
		return
	case m.mode == ModeEdit:
		drawFooter(c, "esc: place back  •  ctrl+s: save")
		return
	}
	// Default: a quiet "? help" tucked into the bottom-right. Hidden
	// when the help panel is already open (no need for the nudge).
	y := c.H - 1
	for x := 0; x < c.W; x++ {
		c.SetBlank(x, y)
	}
	if m.helpAnim < 0.5 {
		hint := "? help"
		c.WriteText(c.W-runeLen(hint)-1, y, hint, Footer, 0)
	}
}

// --- help panel (bottom slide-up) -------------------------------------

// helpEntry is one (key, description) row inside a help column.
type helpEntry struct{ key, desc string }

// helpColumn is a labeled vertical group of entries.
type helpColumn struct {
	title   string
	entries []helpEntry
}

// helpData drives the panel. Columns are rendered side-by-side; each
// column's keys and descriptions are aligned to fixed widths derived
// from the longest item, so columns never drift.
var helpData = []helpColumn{
	{"MOUSE", []helpEntry{
		{"drag", "move"},
		{"click", "select"},
		{"dblclk", "zoom"},
	}},
	{"KEYS", []helpEntry{
		{"arrows", "nudge"},
		{"hjkl", "nudge"},
		{"S-arr", "big nudge"},
		{"tab", "next"},
		{"S-tab", "prev"},
	}},
	{"NOTES", []helpEntry{
		{"enter", "zoom-edit"},
		{"n", "new"},
		{"d", "delete"},
		{"r", "raise"},
		{"1-9", "tint"},
	}},
	{"STRINGS", []helpEntry{
		{"s", "pull"},
		{"[ ]", "cycle"},
		{"t", "tight/slack"},
		{"f", "front/behind"},
		{"x", "cut"},
		{"X", "cut all"},
	}},
	{"BOARDS", []helpEntry{
		{">", "next"},
		{"<", "prev"},
		{"B", "new"},
		{"R", "rename"},
		{"D", "delete"},
	}},
	{"VIEW", []helpEntry{
		{"-/=/0", "zoom"},
		{"a", "font"},
		{"c", "highlight"},
		{"?", "toggle"},
		{"q", "quit"},
	}},
}

// drawHelpPanel renders the bordered slide-up panel. anim ∈ [0,1]
// controls its height. Columns are rendered at fixed x-offsets so all
// keys/descriptions align visually regardless of their text length.
func drawHelpPanel(c *Canvas, anim float32) {
	if anim <= 0 || c.H < 4 {
		return
	}

	// Per-column widths.
	keyW := make([]int, len(helpData))
	descW := make([]int, len(helpData))
	for i, col := range helpData {
		kw := runeLen(col.title)
		dw := 0
		for _, e := range col.entries {
			if runeLen(e.key) > kw {
				kw = runeLen(e.key)
			}
			if runeLen(e.desc) > dw {
				dw = runeLen(e.desc)
			}
		}
		keyW[i] = kw
		descW[i] = dw
	}
	const colGap = 4 // horizontal space between columns
	const keyDescGap = 1
	colTotal := make([]int, len(helpData))
	contentW := 0
	for i := range helpData {
		colTotal[i] = keyW[i] + keyDescGap + descW[i]
		contentW += colTotal[i]
		if i < len(helpData)-1 {
			contentW += colGap
		}
	}

	maxRows := 0
	for _, col := range helpData {
		if len(col.entries) > maxRows {
			maxRows = len(col.entries)
		}
	}
	fullContentRows := 1 + maxRows // header + entries
	fullPanelH := fullContentRows + 2

	panelH := int(anim*float32(fullPanelH) + 0.5)
	if panelH > fullPanelH {
		panelH = fullPanelH
	}
	if panelH > c.H-2 {
		panelH = c.H - 2
	}
	if panelH < 1 {
		// Panel has fully retracted — don't draw anything (avoids the
		// "stuck thin sliver" frame at the tail of the close animation).
		return
	}

	panelW := contentW + 6 // 1 border + 2 padding each side
	if panelW > c.W {
		panelW = c.W
	}
	if panelW < 30 {
		panelW = 30
	}
	panelX := (c.W - panelW) / 2
	bottomY := c.H - 2
	topY := bottomY - panelH + 1
	if topY < 0 {
		topY = 0
	}

	// Clear interior.
	for dy := 0; dy < panelH; dy++ {
		for dx := 0; dx < panelW; dx++ {
			c.SetBlank(panelX+dx, topY+dy)
		}
	}

	// Borders.
	drawHB := func(y int, left, right rune) {
		for dx := 0; dx < panelW; dx++ {
			r := '─'
			switch dx {
			case 0:
				r = left
			case panelW - 1:
				r = right
			}
			c.SetRune(panelX+dx, y, r, Footer, 0)
		}
	}
	drawHB(topY, '╭', '╮')
	if panelH >= 2 {
		drawHB(bottomY, '╰', '╯')
	}
	for dy := 1; dy < panelH-1; dy++ {
		c.SetRune(panelX, topY+dy, '│', Footer, 0)
		c.SetRune(panelX+panelW-1, topY+dy, '│', Footer, 0)
	}
	if panelW > 12 {
		c.WriteText(panelX+2, topY, " help ", DimText, AttrBold)
	}

	// Column x-offsets.
	colX := make([]int, len(helpData))
	cur := panelX + 3 // 1 border + 2 padding
	for i := range helpData {
		colX[i] = cur
		cur += colTotal[i] + colGap
	}

	contentTop := topY + 1
	contentBot := bottomY - 1
	// Layout assuming full panel: header sits N rows above contentBot.
	// Anything that lands above contentTop gets clipped — that's the
	// reveal-from-bottom-up animation.
	headerY := contentBot - maxRows

	if headerY >= contentTop && headerY <= contentBot {
		for i, col := range helpData {
			c.WriteText(colX[i], headerY, col.title, Flash, AttrBold)
		}
	}
	for r := 0; r < maxRows; r++ {
		rowY := headerY + 1 + r
		if rowY < contentTop || rowY > contentBot {
			continue
		}
		for i, col := range helpData {
			if r >= len(col.entries) {
				continue
			}
			e := col.entries[r]
			c.WriteText(colX[i], rowY, e.key, DimText, AttrBold)
			descX := colX[i] + keyW[i] + keyDescGap
			c.WriteText(descX, rowY, e.desc, DimText, 0)
		}
	}
}
