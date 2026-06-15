package engines

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/ollybritton/StupidChess/position"
	"github.com/ollybritton/StupidChess/search"
)

// searchEngine is the shared core of every engine that plays real, searching chess (try-hard,
// fortress, and any custom-evaluation engine you add). It wires an alpha-beta searcher to the UCI
// lifecycle, leaving the personality entirely in the pair of evaluators handed to it.
//
// To make a new searching engine with a different style, just build one with a custom evaluator from
// position.MakeEvaluator — for example one that loves king safety:
//
//	func NewEngineCoward() Engine {
//		params := position.DefaultEvalParams
//		params.KingShield *= 4
//		params.KingOpenFile *= 4
//		eval := position.MakeEvaluator(params)
//		return newSearchEngine("coward", "Olly Britton", "Hides its king at all costs.", eval, position.EvalComplex)
//	}
//
// then register it in EngineInfo. The "us"/"them" split (see the pawnstar memory) lets the engine
// pursue its own goal while assuming the opponent plays ordinary chess; pass the same evaluator twice
// for a symmetric engine.
type searchEngine struct {
	name        string
	author      string
	description string
	searcher    search.Searcher
	prepared    bool
}

func newSearchEngine(name, author, description string, evalUs, evalThem position.Evaluator) *searchEngine {
	requests := make(chan search.Request)
	responses := make(chan string)

	return &searchEngine{
		name:        name,
		author:      author,
		description: description,
		searcher:    search.NewAlphaBetaSearch(requests, responses, evalUs, evalThem),
	}
}

func (e *searchEngine) Name() string        { return e.name }
func (e *searchEngine) Author() string      { return e.author }
func (e *searchEngine) Description() string { return e.description }
func (e *searchEngine) NewGame() error      { return nil }
func (e *searchEngine) Stop()               { e.searcher.Stop() }

// Options exposes the standard UCI "Threads" option (Lazy SMP). Engines that add their own options
// (e.g. try-hard's OwnBook) should append to these.
func (e *searchEngine) Options() []EngineOption {
	return []EngineOption{{Name: "Threads", Type: "spin", Default: "1"}}
}

func (e *searchEngine) SetOption(name, value string) error {
	if strings.EqualFold(name, "Threads") {
		if n, err := strconv.Atoi(value); err == nil {
			e.searcher.SetThreads(n)
		}
	}
	return nil
}

// PonderHit forwards to the searcher: the pondered move was played, so the clock starts now.
func (e *searchEngine) PonderHit() { e.searcher.PonderHit() }

// Prepare is idempotent: it starts the response pump and the search goroutine exactly once.
func (e *searchEngine) Prepare() error {
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

func (e *searchEngine) Go(pos *position.Position, options search.SearchOptions) error {
	e.searcher.Requests() <- search.NewRequest(pos, options)
	return nil
}
