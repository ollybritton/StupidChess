package engines

import (
	"github.com/ollybritton/StupidChess/position"
	"github.com/ollybritton/StupidChess/search"
)

type EngineTryHard struct{}

func NewEngineTryHard() *EngineTryHard {
	return &EngineTryHard{}
}

func (e *EngineTryHard) Name() string {
	return "try-hard"
}

func (e *EngineTryHard) Author() string {
	return "Olly Britton"
}

func (e *EngineTryHard) Prepare() error {
	return nil
}

func (e *EngineTryHard) NewGame() error {
	return nil
}

func (e *EngineTryHard) Search(pos *position.Position, options search.SearchOptions) (position.Move, error) {
	legalMoves := pos.MovesLegalWithEvaluation(position.EvalSimple)
	legalMoves.Sort()

	return legalMoves.AsSlice()[0], nil
}
