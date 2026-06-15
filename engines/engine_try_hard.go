package engines

import (
	"fmt"
	"strings"

	"github.com/ollybritton/StupidChess/position"
	"github.com/ollybritton/StupidChess/search"
)

// EngineTryHard is the genuine engine: the shared searching core plus an opening book.
type EngineTryHard struct {
	*searchEngine
	useBook bool
}

func NewEngineTryHard() *EngineTryHard {
	return &EngineTryHard{
		searchEngine: newSearchEngine(
			"try-hard",
			"Olly Britton",
			"A genuine engine: alpha-beta search with quiescence, piece-square evaluation and a transposition table.",
			position.EvalComplex,
			position.EvalComplex,
		),
		useBook: true,
	}
}

// Options exposes the opening-book toggle on top of the base engine's options (Threads).
func (e *EngineTryHard) Options() []EngineOption {
	return append(e.searchEngine.Options(), EngineOption{Name: "OwnBook", Type: "check", Default: "true"})
}

func (e *EngineTryHard) SetOption(name, value string) error {
	if strings.EqualFold(name, "OwnBook") {
		e.useBook = strings.EqualFold(value, "true")
		return nil
	}
	return e.searchEngine.SetOption(name, value) // Threads, etc.
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

	return e.searchEngine.Go(pos, options)
}
