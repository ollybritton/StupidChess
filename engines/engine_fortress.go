package engines

import (
	"fmt"

	"github.com/ollybritton/StupidChess/position"
	"github.com/ollybritton/StupidChess/search"
)

// EngineFortress is the ultra-defensive engine. Unlike the other personality engines it plays real,
// searching chess and is genuinely trying to win, but through a defensive lens: its evaluation
// (EvalFortressUs) favours closed positions with its pawns and pieces kept back and huddled together,
// so it grinds from behind a wall rather than charging out. The strength comes from the same
// alpha-beta search, quiescence and transposition table as try-hard; only the evaluation differs.
type EngineFortress struct {
	noOptions
	searcher search.Searcher
	prepared bool
}

func NewEngineFortress() *EngineFortress {
	requests := make(chan search.Request)
	responses := make(chan string)

	return &EngineFortress{
		searcher: search.NewAlphaBetaSearch(
			requests,
			responses,
			position.EvalFortressUs,
			position.EvalFortressThem,
		),
	}
}

func (e *EngineFortress) Name() string   { return "fortress" }
func (e *EngineFortress) Author() string { return "Olly Britton" }
func (e *EngineFortress) Description() string {
	return "Plays to win but hates open positions: a full alpha-beta search wrapped around a defensive evaluation that keeps its pawns and pieces back and huddled, grinding away from behind a closed wall."
}

func (e *EngineFortress) Prepare() error {
	// Idempotent: start the response pump and search goroutine exactly once (see EngineTryHard.Prepare).
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

func (e *EngineFortress) NewGame() error {
	return nil
}

// PonderHit forwards to the searcher: the pondered move was played, so the clock starts now.
func (e *EngineFortress) PonderHit() {
	e.searcher.PonderHit()
}

func (e *EngineFortress) Go(pos *position.Position, options search.SearchOptions) error {
	e.searcher.Requests() <- search.NewRequest(pos, options)
	return nil
}

func (e *EngineFortress) Stop() {
	e.searcher.Stop()
}
