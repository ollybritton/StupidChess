package engines

import (
	"fmt"

	"github.com/ollybritton/StupidChess/position"
	"github.com/ollybritton/StupidChess/search"
)

type EngineTryHard struct {
	searcher search.Searcher
	prepared bool
}

func NewEngineTryHard() *EngineTryHard {
	requests := make(chan search.Request)
	responses := make(chan string)

	return &EngineTryHard{
		searcher: search.NewAlphaBetaSearch(
			requests,
			responses,
			position.EvalComplex,
			position.EvalComplex,
		),
	}
}

func (e *EngineTryHard) Name() string {
	return "try-hard"
}

func (e *EngineTryHard) Author() string {
	return "Olly Britton"
}

func (e *EngineTryHard) Description() string {
	return "A genuine engine: alpha-beta search with quiescence, piece-square evaluation and a transposition table."
}

func (e *EngineTryHard) Prepare() error {
	// Prepare is idempotent: it starts the response pump and search goroutine exactly once. It used to
	// be invoked on every UCI command, which spawned a fresh pair of goroutines each time and left
	// several readers draining the same responses channel (duplicated and interleaved output).
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

func (e *EngineTryHard) NewGame() error {
	return nil
}

func (e *EngineTryHard) Go(pos *position.Position, options search.SearchOptions) error {
	e.searcher.Requests() <- search.NewRequest(pos, options)

	return nil
}

func (e *EngineTryHard) Stop() {
	e.searcher.Stop()
}
