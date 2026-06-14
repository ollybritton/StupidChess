package engines

import (
	"fmt"

	"github.com/ollybritton/StupidChess/position"
	"github.com/ollybritton/StupidChess/search"
)

// SimpleEngine is a type of engine that has no search and instead chooses moves based on the immediate
// consequences of making that move, i.e. a simple material count or the move that makes the board most
// look like a horse.
type SimpleEngine struct {
	name        string
	author      string
	description string
	chooseMove  func(*position.Position, search.SearchOptions) (position.Move, error)
}

// NewSimpleEngine returns a new simple engine from the given parameters.
func NewSimpleEngine(name, author, description string, chooseMove func(*position.Position, search.SearchOptions) (position.Move, error)) *SimpleEngine {
	return &SimpleEngine{
		name:        name,
		author:      author,
		description: description,
		chooseMove:  chooseMove,
	}
}

func (e *SimpleEngine) Name() string        { return e.name }
func (e *SimpleEngine) Author() string      { return e.author }
func (e *SimpleEngine) Description() string { return e.description }
func (e *SimpleEngine) Prepare() error      { return nil }
func (e *SimpleEngine) NewGame() error      { return nil }
func (e *SimpleEngine) Stop()               {}

func (e *SimpleEngine) Go(pos *position.Position, searchOptions search.SearchOptions) error {
	bestMove, err := e.chooseMove(pos, searchOptions)
	if err != nil {
		return err
	}

	fmt.Println("bestmove", bestMove.String())

	return nil
}
