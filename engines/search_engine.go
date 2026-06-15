package engines

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/ollybritton/StupidChess/nnue"
	"github.com/ollybritton/StupidChess/position"
	"github.com/ollybritton/StupidChess/search"
	"github.com/ollybritton/StupidChess/syzygy"
)

// searchEngine is the shared core of every engine that plays real, searching chess (try-hard,
// fortress, and any custom-evaluation engine you add). It wires an alpha-beta searcher to the UCI
// lifecycle, leaving the personality in the pair of evaluators handed to it.
//
// To make a new searching engine with a different style, build one with a custom evaluator from
// position.MakeEvaluator — for example one that loves king safety:
//
//	func NewEngineCoward() Engine {
//		params := position.DefaultEvalParams
//		params.KingShield *= 4
//		return newSearchEngine("coward", "Olly Britton", "Hides its king.", position.MakeEvaluator(params), position.EvalComplex)
//	}
//
// Every search engine also exposes UCI options to use all cores (Threads), Syzygy endgame tablebases
// (SyzygyPath), and an NNUE network in place of the hand-crafted evaluation (EvalFile).
type searchEngine struct {
	name        string
	author      string
	description string
	searcher    search.Searcher
	prepared    bool

	// The evaluators this engine was built with, kept so EvalFile can switch to NNUE and back.
	defaultEvalUs   position.Evaluator
	defaultEvalThem position.Evaluator
}

func newSearchEngine(name, author, description string, evalUs, evalThem position.Evaluator) *searchEngine {
	requests := make(chan search.Request)
	responses := make(chan string)

	return &searchEngine{
		name:            name,
		author:          author,
		description:     description,
		searcher:        search.NewAlphaBetaSearch(requests, responses, evalUs, evalThem),
		defaultEvalUs:   evalUs,
		defaultEvalThem: evalThem,
	}
}

func (e *searchEngine) Name() string        { return e.name }
func (e *searchEngine) Author() string      { return e.author }
func (e *searchEngine) Description() string { return e.description }
func (e *searchEngine) NewGame() error      { return nil }
func (e *searchEngine) Stop()               { e.searcher.Stop() }

// Options exposes the search engine's standard UCI options. Engines that add their own (e.g. try-hard's
// OwnBook) should append to these.
func (e *searchEngine) Options() []EngineOption {
	return []EngineOption{
		{Name: "Threads", Type: "spin", Default: "1"},
		{Name: "SyzygyPath", Type: "string", Default: ""},
		{Name: "EvalFile", Type: "string", Default: ""},
	}
}

func (e *searchEngine) SetOption(name, value string) error {
	switch {
	case strings.EqualFold(name, "Threads"):
		if n, err := strconv.Atoi(value); err == nil {
			e.searcher.SetThreads(n)
		}
	case strings.EqualFold(name, "SyzygyPath"):
		e.setSyzygy(value)
	case strings.EqualFold(name, "EvalFile"):
		e.setEvalFile(value)
	}
	return nil
}

// setSyzygy installs (or, for an empty path, clears) the Syzygy tablebases. A load failure is reported
// but does not break the engine, which simply continues without tablebases.
func (e *searchEngine) setSyzygy(path string) {
	if path == "" {
		e.searcher.SetTablebases(nil)
		return
	}
	tb, err := syzygy.Load(path)
	if err != nil {
		fmt.Printf("info string could not load syzygy from %q: %v\n", path, err)
		return
	}
	e.searcher.SetTablebases(tb)
	fmt.Printf("info string syzygy loaded from %s (up to %d pieces)\n", path, tb.MaxPieces())
}

// setEvalFile switches the evaluation to an NNUE network (or, for an empty path, back to the engine's
// own hand-crafted evaluation). The network is evaluated incrementally by the search, so it is handed to
// the searcher whole rather than wrapped as a stateless function.
func (e *searchEngine) setEvalFile(path string) {
	if path == "" {
		e.searcher.SetNNUE(nil)
		e.searcher.SetEvaluator(e.defaultEvalUs, e.defaultEvalThem)
		return
	}
	net, err := nnue.Load(path)
	if err != nil {
		fmt.Printf("info string could not load nnue from %q: %v\n", path, err)
		return
	}
	e.searcher.SetNNUE(net)
	fmt.Printf("info string nnue loaded from %s (hash ok: %v)\n", path, net.HashOK)
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
