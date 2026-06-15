package search

import (
	"github.com/ollybritton/StupidChess/nnue"
	"github.com/ollybritton/StupidChess/position"
	"github.com/ollybritton/StupidChess/syzygy"
)

type Searcher interface {
	Requests() chan Request
	Responses() chan string
	Root() error
	Stop()
	// PonderHit tells a ponder search that the move it was pondering on was actually played, so the
	// clock should start now.
	PonderHit()
	// SetThreads sets how many search threads to use (Lazy SMP).
	SetThreads(n int)
	// SetEvaluator swaps the hand-crafted evaluation functions (e.g. between personalities).
	SetEvaluator(evalUs, evalThem position.Evaluator)
	// SetNNUE switches evaluation to an incremental NNUE network (nil reverts to the hand-crafted eval).
	SetNNUE(net *nnue.Network)
	// SetTablebases installs (nil clears) the Syzygy endgame tablebases.
	SetTablebases(tb *syzygy.Tablebases)
	// SetParam sets a tunable search parameter by UCI option name (used by the SPSA tuner). Unknown
	// names are ignored.
	SetParam(name string, value int)
}
