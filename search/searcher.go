package search

import (
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
	// SetEvaluator swaps the evaluation functions (e.g. hand-crafted eval <-> NNUE).
	SetEvaluator(evalUs, evalThem position.Evaluator)
	// SetTablebases installs (nil clears) the Syzygy endgame tablebases.
	SetTablebases(tb *syzygy.Tablebases)
}
