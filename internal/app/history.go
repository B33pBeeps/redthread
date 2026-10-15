package app

// history.go — bounded undo/redo over whole-workspace snapshots.
//
// Undo used to be a single slot written only by the delete path, so every
// other mutation — paste over a note, an edit, a tint, a move — was
// unrecoverable, and `u` after an accidental `ctrl+p` reported "nothing to
// undo" while the note's text was already gone from disk.
//
// A workspace is a handful of flat structs; a deep copy costs a few KB,
// so snapshotting the whole thing is simpler and far less error-prone than
// a per-mutation command/diff scheme, and it cannot drift out of sync with
// the mutation sites.

import "time"

// coalesceWindow — pushes with the same label closer together than this
// collapse into one entry. Holding an arrow key fires a nudge per repeat;
// without this the ring fills with 40 one-cell moves and evicts the state
// the user actually wants back.
const coalesceWindow = 700 * time.Millisecond

// defaultHistoryDepth is the number of undo steps kept in memory.
const defaultHistoryDepth = 50

type snapshot struct {
	ws    *Workspace
	label string
	at    time.Time
}

// History is a bounded undo stack with a redo branch. Entries are whole
// *workspace* states as they were before a mutation, so Undo restores the
// previous state and stashes the current one for Redo.
//
// Workspace rather than Board because deleting a board (`D`) destroys far
// more than deleting a note, and a board-level snapshot could not bring
// it back.
type History struct {
	undo  []snapshot
	redo  []snapshot
	limit int
}

func NewHistory(limit int) *History {
	if limit < 1 {
		limit = defaultHistoryDepth
	}
	return &History{limit: limit}
}

// Push records the workspace's current state, to be restored by a later
// Undo. Call it *before* applying a mutation. label describes the mutation
// and is surfaced in the toast ("undo: paste into note").
func (h *History) Push(ws *Workspace, label string) { h.pushAt(ws, label, time.Now()) }

// pushAt is Push with an injectable clock, so coalescing is testable.
func (h *History) pushAt(ws *Workspace, label string, now time.Time) {
	if ws == nil {
		return
	}

	// Coalesce a run of same-label mutations: keep the oldest state (the
	// one before the run started) and just extend its window.
	if n := len(h.undo); n > 0 {
		last := h.undo[n-1]
		if last.label == label && now.Sub(last.at) < coalesceWindow {
			h.undo[n-1].at = now
			h.redo = nil
			return
		}
	}

	h.undo = append(h.undo, snapshot{ws: cloneWorkspace(ws), label: label, at: now})
	if len(h.undo) > h.limit {
		// Drop the oldest. Copying beats a real ring buffer here: the
		// depth is small and this only runs once per mutation.
		h.undo = append(h.undo[:0], h.undo[1:]...)
	}

	// Any new work invalidates the redo branch — keeping it would let a
	// later ctrl+r resurrect a state the user already moved away from.
	h.redo = nil
}

// Undo restores the state before the most recent mutation. current is the
// workspace as it stands now; it is stashed so Redo can return to it.
// Returns the restored workspace, the label of the undone mutation, and
// false when there is nothing to undo.
func (h *History) Undo(current *Workspace) (*Workspace, string, bool) {
	n := len(h.undo)
	if n == 0 {
		return current, "", false
	}
	entry := h.undo[n-1]
	h.undo = h.undo[:n-1]
	h.redo = append(h.redo, snapshot{ws: cloneWorkspace(current), label: entry.label, at: entry.at})
	// Hand back a copy: the caller owns the result and will mutate it, and
	// the stack's own entry must not move underneath it.
	return cloneWorkspace(entry.ws), entry.label, true
}

// Redo re-applies the most recently undone mutation.
func (h *History) Redo(current *Workspace) (*Workspace, string, bool) {
	n := len(h.redo)
	if n == 0 {
		return current, "", false
	}
	entry := h.redo[n-1]
	h.redo = h.redo[:n-1]
	h.undo = append(h.undo, snapshot{ws: cloneWorkspace(current), label: entry.label, at: entry.at})
	return cloneWorkspace(entry.ws), entry.label, true
}

func (h *History) CanUndo() bool { return len(h.undo) > 0 }
func (h *History) CanRedo() bool { return len(h.redo) > 0 }
func (h *History) Len() int      { return len(h.undo) }

// Clear drops both stacks.
func (h *History) Clear() {
	h.undo = nil
	h.redo = nil
}

// cloneWorkspace deep-copies a workspace and every board in it. Background
// and the scalar fields are values, so only the board slice needs work.
func cloneWorkspace(ws *Workspace) *Workspace {
	if ws == nil {
		return nil
	}
	out := *ws
	if ws.Boards != nil {
		out.Boards = make([]*Board, len(ws.Boards))
		for i, b := range ws.Boards {
			out.Boards[i] = cloneBoard(b)
		}
	}
	return &out
}

// cloneBoard deep-copies a board: the note and string slices hold
// pointers, so a shallow copy would alias every note and make undo a
// no-op. Note and StringConn are flat value types, so one level of
// indirection is the whole job.
func cloneBoard(b *Board) *Board {
	if b == nil {
		return nil
	}
	out := *b // copies the scalar fields

	if b.Notes != nil {
		out.Notes = make([]*Note, len(b.Notes))
		for i, n := range b.Notes {
			if n == nil {
				continue
			}
			cp := *n
			out.Notes[i] = &cp
		}
	}
	if b.Strings != nil {
		out.Strings = make([]*StringConn, len(b.Strings))
		for i, s := range b.Strings {
			if s == nil {
				continue
			}
			cp := *s
			out.Strings[i] = &cp
		}
	}
	return &out
}
