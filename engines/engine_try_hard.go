package engines

import (
	"fmt"

	"github.com/ollybritton/StupidChess/position"
	"github.com/ollybritton/StupidChess/search"
)

type EngineTryHard struct {
	searcher search.Searcher
}

func NewEngineTryHard() *EngineTryHard {
	requests := make(chan search.Request)
	responses := make(chan string)

	return &EngineTryHard{
		searcher: search.NewAlphaBetaSearch(requests, responses),
	}
}

func (e *EngineTryHard) Name() string {
	return "try-hard"
}

func (e *EngineTryHard) Author() string {
	return "Olly Britton"
}

func (e *EngineTryHard) Prepare() error {
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
