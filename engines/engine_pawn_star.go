package engines

import (
	"fmt"

	"github.com/ollybritton/StupidChess/position"
	"github.com/ollybritton/StupidChess/search"
)

type EnginePawnStar struct {
	noOptions
	searcher search.Searcher
	prepared bool
}

func NewEnginePawnStar() *EnginePawnStar {
	requests := make(chan search.Request)
	responses := make(chan string)

	return &EnginePawnStar{
		searcher: search.NewAlphaBetaSearch(
			requests,
			responses,
			position.EvalPawnStarUs,
			position.EvalPawnStarThem,
		),
	}
}

func (e *EnginePawnStar) Name() string {
	return "pawn-star"
}

func (e *EnginePawnStar) Author() string {
	return "Olly Britton"
}

func (e *EnginePawnStar) Description() string {
	return "Values its own pawns enormously, treating each as worth far more than normal."
}

func (e *EnginePawnStar) Prepare() error {
	// Idempotent; see EngineTryHard.Prepare for why.
	if e.prepared {
		return nil
	}
	e.prepared = true

	go func() {
		for msg := range e.searcher.Responses() {
			fmt.Println(msg)
		}
	}()

	go e.searcher.Root()

	return nil
}

func (e *EnginePawnStar) NewGame() error {
	return nil
}

func (e *EnginePawnStar) Go(pos *position.Position, options search.SearchOptions) error {
	e.searcher.Requests() <- search.NewRequest(pos, options)

	return nil
}

func (e *EnginePawnStar) Stop() {
	e.searcher.Stop()
}
