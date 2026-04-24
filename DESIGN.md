# brainfartadhdfixerupper — DESIGN

*Working-title TUI sticky-note pegboard. Lives in a tmux pane. Mouse + keyboard.
Transparent terminal background. High-quality ASCII with dither/halftone detail.
Obra Dinn-style flashbulb transition when opening a note.*

---

## 1. Context & purpose

Juan wants a personal TUI todo/pinboard that:

- Lives in a ~quarter-screen tmux pane, always available.
- Feels like a brown pegboard seen through the terminal (theme-driven, not solid).
- Treats each task as a sticky note pinned to the board.
- Is mouse-first (drag to reposition, click to select, double-click to zoom)
  but fully keyboard-driven for power use.
- Has **smooth pickup / drop** micro-animations and a **high-contrast, dithered,
  Obra Dinn-style transition** when entering the zoom-to-edit view.
- Uses high-density ASCII (bigger note footprint, smaller per-character area
  via block and box characters) so notes can have paper texture, shading, a
  pin with highlight, and a drop shadow.

Non-goals for v1: multi-user sync, fuzzy search, kanban columns, red-string
linking, undo/redo. All of those are v2 candidates.

---

## 2. Stack & dependencies

- **Go 1.23**, running under `~/.local/go/bin/go` on this machine.
- **[Bubble Tea](https://github.com/charmbracelet/bubbletea)** — MVU event loop.
- **[Lip Gloss](https://github.com/charmbracelet/lipgloss)** — styling primitives,
  used sparingly (we emit most SGR directly to preserve terminal-default bg).
- **[Bubbles](https://github.com/charmbracelet/bubbles)** — `textarea` for edit
  mode.
- Std lib only for persistence (`encoding/json`).

One static binary, no runtime dependencies.

---

## 3. File layout

```
~/code/personal/brainfartadhdfixerupper/
├── DESIGN.md          — this doc
├── README.md
├── run.sh             — dev runner; sets PATH, go run .
├── go.mod / go.sum
├── main.go            — entrypoint, program options
├── model.go           — tea.Model, Update, View, key/mouse routing
├── notes.go           — Note, Board (state, hit-test, z-order)
├── render.go          — cell buffer, ANSI emit, coalesce
├── theme.go           — colors, dither tables, char pools
├── draw.go            — cork bg + note drawing (borders, texture, shadow)
├── anim.go            — animation timelines + Obra Dinn transition state
├── edit.go            — textarea integration for zoom-to-edit
└── storage.go         — JSON load/save, debounced
```

Single Go module `brainfartadhdfixerupper`. All code in package `main`.

---

## 4. Data model

```go
type Note struct {
    ID      string    // short random id
    Title   string    // first line, rendered bold on header strip
    Body    string    // remaining lines, rendered on the paper
    X, Y    int       // top-left cell on the board, board-coords
    Tint    string    // "yellow" | "pink" | "blue" | "green" | "purple"
    Created time.Time
    Updated time.Time
}

type Board struct {
    Notes    []*Note   // back-to-front (last is top)
    Selected string    // note ID, or "" if none
}
```

Board positioning: coordinates are absolute in the terminal window; we don't
scroll/pan in v1 (the tmux pane is the board). If a note goes off-screen, it
clips; the user just drags it back.

---

## 5. Rendering model

### 5.1 Cell buffer

`render.go` defines a `Canvas` — a `w × h` grid of `Cell { Rune; Fg; Attr }`.
**No background color is ever set.** Empty cells have `Rune == 0`, which
serializes to a bare space with fg reset → the terminal background shows.

Coalescer walks each row, groups runs of equal `(Fg, Attr)`, and emits:

- `\x1b[0m` when entering an unstyled run
- `\x1b[38;2;R;G;Bm` for a styled run (+ `1m` if bold, `2m` if faint)
- text
- `\x1b[0m` at end-of-run when style changes

This preserves the user's terminal bg / theme / transparency everywhere we
didn't explicitly draw.

### 5.2 Dither library

`theme.go` provides:

- **`BayerDither(x, y, threshold 0..1) → '█' | '▓' | '▒' | '░' | ' '`**
  Uses a 4×4 Bayer matrix; given a target "density" from 0–1, chooses one of
  5 shades so that large fills look smoothly graduated.
- **`HalftoneChar(level 0..4)`** — simple lookup for when we already know the
  shade level (e.g., pre-baked drop shadow).
- **Paper fiber pool** — sparse `·` `.` `,` `'` at ~3% density over a note's
  body, rendered in a slightly darker tint of the note color to suggest fiber.

### 5.3 Cork / starfield background

Sparse brown ASCII chars (`. , · ' : ; \` ~`) at ~11 % density across the
entire pane, in three brown shades (`#5a3a24`, `#7a5336`, `#a67a4a`). Stable
per-window-size (seeded LCG keyed on `(w,h)`), so it doesn't shimmer frame
to frame.

### 5.4 Note drawing

Default note size: **36 columns × 14 rows** (was 22×7 — almost 4× the cells,
so real room for texture).

```
╭────────────────◉──────────────────╮   ← tape strip (dithered ▓▒), pin centered
│ Buy milk                          │   ← title, bold, left-justified, 1 col pad
├───────────────────────────────────┤   ← separator, dithered fade
│  • whole                          │
│  • 2 gal                     ·    │   ← paper fibers sparsely
│  · for a week                     │
│                 ·                 │
│                                   │
│                            ·      │
│                                   │
│                                   │
╰───────────────────────────────────╯
 ▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒▒ ← drop shadow, 1 row below offset +1 col
```

Rendering breakdown:

1. **Shadow** drawn first, 1 cell offset right + down, rendered with `▒`
   (or `░` if selected = raised higher) in a dark brown (`#2a1a10`). Skipped
   on a grabbed note (it's "in the air").
2. **Outline** drawn with `╭─╮│╰╯` in the note's tint.
3. **Tape strip** (row 0): uses Bayer-dithered half-tone `▓▒` mixed with `─`,
   colored in a darker variant of the tint to feel like cellophane tape.
4. **Pin** at top-center: `◉` in red `#e63c3c`, bold. When selected the pin
   becomes `◈` (diamonded) and gains a faint highlight above it.
5. **Title** on row 1 in tint-foreground, bold.
6. **Separator** on row 2: `┈` or `├┤` variant, half-dithered (alternating
   `─` and `·` or `─` and `┄` to feel like a perforation).
7. **Body** rows 3 … h-2: word-wrapped body text in tint foreground, with
   sparse paper-fiber dots scattered per-cell at ~3% density.
8. **Bottom-right corner glyph** `◜` / `◝` optional (folded corner), skipped
   if note is too small.

Selected notes: border fg becomes red + bold, pin becomes diamond, paper
fibers slightly brighter.

Grabbed notes: drop shadow disappears; note's outline becomes bold; a `~`
ripple on the cork row directly below the lifted note hints "it's floating".

### 5.5 Drop bounce animation

On mouse-up, note's vertical offset is set to `+1.4` cells; then each tick
(16ms) it decays by `× 0.75` until `< 0.01`. Net effect: a ~5-frame soft
settle. Keyboard nudge uses the same field but with `+0.6` on the axis moved,
so even kb-only users feel the impact.

### 5.6 Pickup lift animation

On mouse-down on a note, the note's render offset rises by `-0.8` cells over
2 ticks (eased), and its shadow enlarges from 1-row `▒` to 2-row `▒/░`.
This sells the "lifted" feel immediately.

---

## 6. Obra Dinn zoom transition

The dramatic one. Modeled loosely on the memory-entry effect from
*Return of the Obra Dinn*: stark black+white dither, brief flash, then
the scene resolves.

### 6.1 Phases

Triggered by **Enter** (on selected note) or **double-click** on a note.

| Phase | Duration | What happens |
|------:|---------:|:-----|
| 1. Flash     | 60 ms  | Pane momentarily overwritten with bright white `█` at every cell |
| 2. Dither    | 90 ms  | Cells transition from `█` to white `▓`/`▒`/`░` based on random noise; cork + notes become monochrome dithered silhouettes |
| 3. Resolve   | 140 ms | Selected note's silhouette morphs: its bounding rect linearly scales from (X,Y,36,14) → fullscreen-minus-padding. During morph, pixels outside the morph rect stay as low-density cork-tone dither; pixels inside the morph gain color back. |
| 4. Settle    | 80 ms  | Editable note renders at full size with edge bloom: a 1-char dither band around it in tint color, fading. |
| — total —    | ~370 ms| |

### 6.2 Reverse transition (Esc)

Phases 4→3→2→1, mirrored, but shorter (~280 ms) since context is already
loaded. Final frame returns to board view; bob on the note = 0.8 (tiny
"landed" bump).

### 6.3 Implementation

`anim.go` exposes:

```go
type Transition struct {
    Start   time.Time
    Dur     time.Duration
    From    NoteRect
    Reverse bool
}

func (t *Transition) Frame(now time.Time) TransitionFrame {
    // returns phase + progress 0..1, used by render to compose the frame
}
```

View rendering checks `model.transition`:

- If `nil` → normal board draw.
- If set → draw board dithered at phase-appropriate density, overlay the
  morphing rect, composite the editable note once resolve > 0.6, etc.

### 6.4 Monochrome dither of existing frame

Algorithm during phase 2–3:

1. Render board into cell buffer as normal.
2. For each cell, compute brightness from its fg rgb (`0.299 R + 0.587 G + 0.114 B`).
3. Map brightness × random-jitter-by-phase to 5-level halftone `[' ', '░', '▒', '▓', '█']`.
4. Emit with pure-white fg on terminal-default bg → stark monochrome.

This cheap approximation is what sells the "Obra Dinn feel."

---

## 7. Controls

### 7.1 Global keys

| Key | Action |
|----|--------|
| `q` / `Ctrl+C` | Quit (saves first) |
| `?` | Toggle help overlay (lists keys) |

### 7.2 Board-mode keys

| Key | Action |
|-----|--------|
| `Tab` | Cycle selected note (next) |
| `Shift+Tab` | Cycle previous |
| `← ↑ ↓ →` / `h j k l` | Nudge selected 1 cell |
| `Shift + ← ↑ ↓ →` | Nudge 5 cells |
| `Enter` | Zoom-to-edit (Obra Dinn transition in) |
| `n` | New note at board center, auto-tint + focus |
| `d` | Delete selected (asks `y/n`) |
| `1` `2` `3` `4` `5` | Change selected note's tint |
| `r` | Raise selected note to top |

### 7.3 Mouse

| Event | Action |
|-------|--------|
| Left-click on note | Select + raise + begin grab |
| Drag | Move grabbed note |
| Left-release | Drop (settle bounce) |
| Double-click on note | Zoom-to-edit |
| Click on empty board | Deselect |
| Scroll | (v2 — pan) |

### 7.4 Edit-mode keys

| Key | Action |
|-----|--------|
| `Esc` | Close with reverse transition, save |
| `Ctrl+S` | Save without closing |
| All other keys | Delegated to the textarea |

---

## 8. Persistence

- Storage path:
  - `$XDG_DATA_HOME/brainfartadhdfixerupper/notes.json`, falling back to
    `$HOME/.local/share/brainfartadhdfixerupper/notes.json`.
- On every mutation (move, edit, add, delete) we call `saver.Touch()`,
  which debounces writes: schedule a save in 400 ms, reset the timer on
  subsequent calls, so rapid drags coalesce into one write.
- On quit, force-flush.
- File format: pretty-printed JSON, one top-level object with a `notes`
  array and a `schemaVersion` int.
- If the file is missing or corrupt, the app starts with a small set of
  seed notes so the board isn't empty on day 1.

Schema v1:

```json
{
  "schemaVersion": 1,
  "notes": [
    {
      "id": "nQ7k3",
      "title": "Buy milk",
      "body": "• whole\n• 2 gal",
      "x": 4, "y": 3,
      "tint": "yellow",
      "created": "2026-04-24T01:00:00Z",
      "updated": "2026-04-24T01:00:00Z"
    }
  ]
}
```

---

## 9. Tick / animation loop

- One `tea.Tick(16ms)` command continuously re-armed (≈60 fps).
- On each tick:
  - Decay `bob` on every note with non-zero bob.
  - Advance `transition` if active; clear it when done.
- Rendering is pure from state; no `time.Now()` inside `View()` except what's
  threaded through via messages.

---

## 10. Module-level code map

- `main.go` — parses `--seed` flag (creates demo notes even if file exists),
  builds initial model, launches Bubble Tea with `WithAltScreen`,
  `WithMouseAllMotion`.
- `model.go` — `type model struct` + Init/Update/View. Update routes by mode
  (board vs edit) and by message type.
- `notes.go` — `Note`, `Board`, `genID`, `newNote`, hit-test, raise.
- `draw.go` — `drawCork`, `drawNote`, `drawSelectedNoteOverlay`,
  `drawFooter`, all receiving a `*Canvas`.
- `render.go` — `Canvas`, `Cell`, `(*Canvas).Serialize()` → ANSI string.
- `theme.go` — all colors, tint tables, dither matrices, char pools.
- `anim.go` — `Transition` state machine, easing funcs (`easeOutCubic`,
  `easeInQuad`), the monochrome-dither render helper.
- `edit.go` — wraps `bubbles/textarea.Model`, focus mgmt, keybindings.
- `storage.go` — `Saver` with debounce, `Load`, `Save`, path resolution.

---

## 11. Verification plan

1. `go build .` must succeed cleanly with no warnings.
2. Launch under `script -c` in a tty-emulated env, send a few keypresses,
   snapshot the output to confirm the cork texture + notes render without
   ANSI leaks.
3. Launch interactively (user), run through:
   - drag a note across the pane
   - Tab/Tab/Tab cycle
   - add 2 notes, delete one
   - Enter → observe Obra Dinn transition → edit body → Esc → observe reverse
   - q → confirm JSON file exists at XDG path
   - relaunch → previous state restored

---

## 12. Future (not v1)

- Red string between pinned notes (lipgloss-drawn, tagged in JSON).
- Kanban columns via "lanes" (invisible y-bands with headers).
- Fuzzy search overlay with `bubbles/textinput` + filter.
- Undo/redo ring.
- Pomodoro timer in the pin badge.
- Shared board sync over git-commit pushes.

