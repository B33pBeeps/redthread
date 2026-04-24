# redthread

A sticky-note pegboard for your terminal. Mouse-first, keyboard-friendly,
transparent over your tmux theme, ASCII-textured, with a workspace zoom
and physics-y red strings you can dangle between notes.

![screenshot](docs/screenshot.png)

---

## What it is

- A small TUI meant to live in a tmux pane, always open.
- A cork pegboard rendered entirely in ASCII. Your terminal background
  shows through — no solid fill.
- Sticky notes you drag with the mouse or nudge with the keyboard.
  Colored paper, red pin, ornate borders, paper-fiber flecks, drop
  shadow, fold glyph.
- **Red strings** between notes (or pinned to bare cork). Slack or tight.
  In-front-of or behind the notes. A weighted "swoop" while you pull
  the free end around with the mouse.
- **Workspace zoom** — 5 levels that scale note sizes *and* positions,
  so you can pack many stickies into an overview or lean in on a few.
- **3D zoom-to-edit** — a smooth scale + lift + shadow-grow animation
  that settles into a full-screen card with a live textarea.
- **Multiple tints + global highlight color** — 9 paper colors, 9 border
  colors.
- **9 text styles** via Unicode math alphabets — plain, bold, italic,
  script, fraktur, double-struck, monospace, fullwidth.
- Persists to `$XDG_DATA_HOME/redthread/notes.json` with debounced writes.

## Install

**Requires Go 1.23+**.

```bash
# clone + build
git clone https://github.com/B33pBeeps/redthread.git
cd redthread
go build -o redthread ./cmd/redthread
./redthread
```

Or install straight from GitHub:

```bash
go install github.com/B33pBeeps/redthread/cmd/redthread@latest
redthread
```

On first run with no saved file, you'll get a small demo board.
Run `redthread --fresh` any time to ignore the save and load the demo.

## Controls

### Mouse

| | |
|---|---|
| **click** a note | select + raise |
| **drag** | move the note (drops with a bounce) |
| **double-click** | zoom-to-edit |
| **click empty cork (during `s` pull)** | place a wall-pin |

### Keyboard — board mode

| key | action |
|---|---|
| `tab` / `shift+tab` | cycle selection |
| `← ↑ ↓ →` or `hjkl` | nudge (overshoots in the motion direction) |
| `shift+arrows` | big nudge |
| `enter` | zoom-to-edit |
| `n` / `d` / `r` | new / delete / raise selected |
| `1` – `9` | pick a tint (yellow, pink, blue, green, purple, orange, teal, cream, coral) |
| `c` | cycle the global highlight (selected-border) color |
| `a` | open the font menu (arrow keys preview, enter to commit, esc to cancel) |
| `-` / `=` / `0` | zoom out / in / reset (five levels total, −3..+1) |
| `s` | start pulling a red string from the selected note's pin |
| `[` / `]` | cycle the hovered string |
| `t` | toggle tight / slack on the hovered string |
| `f` | toggle the hovered string between in-front-of and behind notes |
| `x` | cut the hovered string |
| `X` | cut every string on the selected note |
| `?` | toggle the help overlay |
| `q` / `ctrl+c` | quit (saves) |

### Keyboard — edit mode

| key | action |
|---|---|
| typing | edit body (first non-empty line is the title) |
| `esc` | close with reverse transition, saves |
| `ctrl+s` | save without closing |

## Data

Notes live in a single JSON file:

- `$XDG_DATA_HOME/redthread/notes.json`, or
- `$HOME/.local/share/redthread/notes.json` by default.

Writes are debounced (400 ms), so rapid drags coalesce into one flush.
A legacy `brainfartadhdfixerupper/` directory is auto-migrated on first
launch under the new name.

Schema (v3):

```json
{
  "schemaVersion": 3,
  "zoom": 0,
  "highlightColor": 0,
  "textMode": 0,
  "notes":   [{ "id": "…", "title": "…", "body": "…", "x": 3, "y": 1,
                "tint": "yellow", "created": "…", "updated": "…" }],
  "strings": [{ "a": { "note": "idA" }, "b": { "note": "idB" },
                "tight": false, "front": false }]
}
```

## Customizing

The color palettes and text styles live in `theme.go`:

- `Tints` + `TintOrder` — the 9 paper-color presets.
- `SelBorderChoices` — the 9 highlight colors.
- `StarChars` / `PoreChars` / `BlotchChars` — cork-texture character pools.
- `TextModes` — the Unicode-alphabet font options.

Edit any of those, rebuild, and the whole app picks up the new values
(help text + menu + cycle included).

## File layout

```
cmd/redthread/
  main.go          thin entry — calls internal/app.Run()

internal/app/
  app.go           Run(): flag parsing, load/seed, launch Bubble Tea
  model.go         MVU (Model/Update/View, key + mouse routing)
  notes.go         Note, Board, StringConn, zoom scale, starfield gen
  draw.go          cork + note rendering (shadow, tape, borders, text, fibers)
  wire.go          red-string curve rendering + pull physics
  render.go        cell grid + coalesced ANSI emitter (transparent bg)
  theme.go         colors, tints, Bayer dither, halftone, text-style mapping
  anim.go          zoom transition timeline + easings
  edit.go          canvas-drawn edit frame + bubbles/textarea splice
  menu.go          font-picker popup
  storage.go       XDG JSON persistence with debounced saver

docs/
  screenshot.png

DESIGN.md          earlier design notes (historical)
```

## Text preview

Colors won't render in GitHub's Markdown, but here's a rough shape of
what the board looks like at default zoom:

```
  ╭━╍━─━╍━─━◈━─━╍━─━╍━╮
  ┃ ◆ welcome          │
  ├┄──┄──┄──┄──┄──┄──┄┤
  ╏ • drag with mouse  │
  ╏ • tab cycles       │
  │ • enter zooms      │
  ┃ • s pulls a string │
  │ ◜ ┈┈ ┈┈ ┈┈ ┈┈ ┈┈ ◝◞│
  ╰─━──━──━──━──━──━──╯
```

## License

Personal project. MIT-friendly — use it, fork it, whatever.
