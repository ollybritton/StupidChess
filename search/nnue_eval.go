package search

import (
	"github.com/ollybritton/StupidChess/nnue"
	"github.com/ollybritton/StupidChess/position"
)

// maxAccStack bounds the incremental accumulator stack. It comfortably exceeds the deepest line along
// which the search makes moves (search nodes up to maxPlies, each possibly extended by quiescence), with
// margin to spare; lines past it fall back to a full refresh rather than corrupting the stack.
const maxAccStack = 256

// incEvaluator is the search's view of an incremental NNUE evaluator. Both the classic HalfKP evaluator
// and the bigger HalfKAv2_hm one implement it, so the search is agnostic to which network architecture is
// loaded. Each is per-worker state (an accumulator stack mirroring make/unmake), so a fresh one is
// spawned per Lazy-SMP worker and reset to the root.
type incEvaluator interface {
	// spawn returns a fresh per-worker instance sharing the same network (its own empty accumulator stack).
	spawn() incEvaluator
	// reset rebuilds the base accumulator from pos (the root of a search).
	reset(pos *position.Position)
	// makeMove updates the accumulator for a move just applied to reach pos (the board AFTER the move).
	makeMove(pos *position.Position, m position.Move)
	// undoMove pops the accumulator pushed by the matching makeMove.
	undoMove()
	// eval returns the White-positive static evaluation of the current position (centipawns).
	eval(pos *position.Position) int16
}

// ---------------------------------------------------------------------------
// HalfKP (classic, 256x2) evaluator
// ---------------------------------------------------------------------------

// nnueEvaluator is the incremental evaluator for the classic HalfKP network. It keeps a stack of
// accumulators that mirrors the search's make/unmake, so each leaf evaluation reuses the parent node's
// accumulator updated by only the few features the last move touched, instead of rebuilding all sums from
// scratch. That is the whole point of an "efficiently updatable" network.
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
	return &nnueEvaluator{net: net, stack: make([]nnue.Accumulator, maxAccStack)}
}

func (e *nnueEvaluator) spawn() incEvaluator { return newNNUEEvaluator(e.net) }

func (e *nnueEvaluator) reset(pos *position.Position) {
	e.top = 0
	e.overflow = 0
	e.stack[0].Refresh(e.net, pos)
}

func (e *nnueEvaluator) makeMove(pos *position.Position, m position.Move) {
	if e.top+1 >= len(e.stack) {
		e.overflow++
		return
	}
	e.stack[e.top+1].Update(e.net, &e.stack[e.top], pos, m)
	e.top++
}

func (e *nnueEvaluator) undoMove() {
	if e.overflow > 0 {
		e.overflow--
		return
	}
	e.top--
}

// eval returns the White-positive static evaluation. The network is side-to-move relative, so a
// Black-to-move score is negated into the White-positive convention leafEval expects.
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

// ---------------------------------------------------------------------------
// HalfKAv2_hm (modern, 1024x2) evaluator
// ---------------------------------------------------------------------------

// kaEvaluator is the incremental evaluator for the modern HalfKAv2_hm network. It mirrors nnueEvaluator
// but carries the bigger KAAccumulator (1024-wide transformer plus the 8-bucket PSQT side channel), and
// the forward pass needs the piece count to pick the PSQT bucket and layer stack.
type kaEvaluator struct {
	net      *nnue.KANetwork
	stack    []nnue.KAAccumulator
	top      int
	overflow int
}

func newKAEvaluator(net *nnue.KANetwork) *kaEvaluator {
	return &kaEvaluator{net: net, stack: make([]nnue.KAAccumulator, maxAccStack)}
}

func (e *kaEvaluator) spawn() incEvaluator { return newKAEvaluator(e.net) }

func (e *kaEvaluator) reset(pos *position.Position) {
	e.top = 0
	e.overflow = 0
	e.stack[0].Refresh(e.net, pos)
}

func (e *kaEvaluator) makeMove(pos *position.Position, m position.Move) {
	if e.top+1 >= len(e.stack) {
		e.overflow++
		return
	}
	e.stack[e.top+1].Update(e.net, &e.stack[e.top], pos, m)
	e.top++
}

func (e *kaEvaluator) undoMove() {
	if e.overflow > 0 {
		e.overflow--
		return
	}
	e.top--
}

// eval returns the White-positive static evaluation in centipawns. EvalWith returns the network's
// internal value (side-to-move POV); it is converted to centipawns, flipped to White-positive, and
// clamped well below mate scores so it can never be mistaken for a forced mate.
func (e *kaEvaluator) eval(pos *position.Position) int16 {
	var internal int32
	if e.overflow > 0 {
		internal = e.net.EvalInternal(pos)
	} else {
		internal = e.net.EvalWith(&e.stack[e.top], pos.SideToMove, nnue.KAPieceCount(pos))
	}
	cp := internal * 100 / nnue.KANormalizeToPawn
	if pos.SideToMove == position.Black {
		cp = -cp
	}
	const limit = 20000 // below mateScoreBound; a static eval is never a mate claim
	if cp > limit {
		cp = limit
	} else if cp < -limit {
		cp = -limit
	}
	return int16(cp)
}
