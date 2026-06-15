package engines

import (
	"fmt"
	"strings"

	"github.com/ollybritton/StupidChess/position"
	"github.com/ollybritton/StupidChess/search"
)

type EngineTryHard struct {
	searcher search.Searcher
	prepared bool
	useBook  bool
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
		useBook: true,
	}
}

// Options exposes the opening-book toggle as the standard UCI "OwnBook" option.
func (e *EngineTryHard) Options() []EngineOption {
	return []EngineOption{{Name: "OwnBook", Type: "check", Default: "true"}}
}

func (e *EngineTryHard) SetOption(name, value string) error {
	if strings.EqualFold(name, "OwnBook") {
		e.useBook = strings.EqualFold(value, "true")
	}
	return nil
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

// PonderHit forwards to the searcher: the pondered move was played, so the clock starts now.
func (e *EngineTryHard) PonderHit() {
	e.searcher.PonderHit()
}

func (e *EngineTryHard) Go(pos *position.Position, options search.SearchOptions) error {
	// Play instantly from the opening book while still in known theory. Skip it while pondering, since
	// the whole point of pondering is to search on the opponent's clock.
	if e.useBook && !options.Ponder {
		if uci, ok := getBook().lookup(pos); ok {
			if _, legal := bookLegalMove(pos, uci); legal {
				fmt.Println("info string book move")
				fmt.Println("bestmove", uci)
				return nil
			}
		}
	}

	e.searcher.Requests() <- search.NewRequest(pos, options)

	return nil
}

func (e *EngineTryHard) Stop() {
	e.searcher.Stop()
}
