package search

import (
	"github.com/ollybritton/StupidChess/nnue"
	"github.com/ollybritton/StupidChess/position"
)

// maxAccStack bounds the incremental accumulator stack. It comfortably exceeds the deepest line along
// which the search makes moves (search nodes up to maxPlies, each possibly extended by quiescence), with
// margin to spare; lines past it fall back to a full refresh rather than corrupting the stack.
const maxAccStack = 256

// nnueEvaluator is the search's incremental NNUE evaluator. It keeps a stack of accumulators that mirrors
// the search's make/unmake, so each leaf evaluation reuses the parent node's accumulator updated by only
// the few features the last move touched, instead of rebuilding all 256x2 sums from scratch. That is the
// whole point of an "efficiently updatable" network, and it is what lets NNUE run at a sane node rate.
//
// It is per-worker state: each Lazy-SMP worker holds its own clone, so the stacks never race.
type nnueEvaluator struct {
	net   *nnue.Network
	stack []nnue.Accumulator
	top   int

	// overflow counts make() calls beyond the end of the stack (pathologically deep lines). While it is
	// non-zero the evaluator falls back to a full refresh at the leaf, staying correct without growing the
	// stack; undo() unwinds it symmetrically.
	overflow int
}

func newNNUEEvaluator(net *nnue.Network) *nnueEvaluator {
	return &nnueEvaluator{
		net:   net,
		stack: make([]nnue.Accumulator, maxAccStack),
	}
}

// reset re-syncs the evaluator to pos, the root of a fresh search: the stack is emptied and the base
// accumulator rebuilt from scratch.
func (e *nnueEvaluator) reset(pos *position.Position) {
	e.top = 0
	e.overflow = 0
	e.stack[0].Refresh(e.net, pos)
}

// makeMove updates the accumulator for a move just applied to reach pos (the board AFTER the move).
func (e *nnueEvaluator) makeMove(pos *position.Position, m position.Move) {
	if e.top+1 >= len(e.stack) {
		e.overflow++
		return
	}
	e.stack[e.top+1].Update(e.net, &e.stack[e.top], pos, m)
	e.top++
}

// undoMove pops the accumulator pushed by the matching makeMove.
func (e *nnueEvaluator) undoMove() {
	if e.overflow > 0 {
		e.overflow--
		return
	}
	e.top--
}

// eval returns the White-positive static evaluation of the current position. The network is
// side-to-move relative, so a Black-to-move score is negated into the White-positive convention the
// search's leafEval expects (it applies ScoreFromPerspective afterwards).
func (e *nnueEvaluator) eval(pos *position.Position) int16 {
	var v int16
	if e.overflow > 0 {
		v = e.net.Eval(pos) // too deep to track incrementally; refresh from the board
	} else {
		v = e.net.EvalWith(&e.stack[e.top], pos.SideToMove)
	}
	if pos.SideToMove == position.Black {
		return -v
	}
	return v
}

// clone returns an independent copy for another Lazy-SMP worker (its own accumulator stack).
func (e *nnueEvaluator) clone() *nnueEvaluator {
	c := &nnueEvaluator{
		net:      e.net,
		stack:    make([]nnue.Accumulator, len(e.stack)),
		top:      e.top,
		overflow: e.overflow,
	}
	copy(c.stack, e.stack)
	return c
}
