package engines

import (
	"math/rand"

	"github.com/ollybritton/StupidChess/position"
	"github.com/ollybritton/StupidChess/search"
)

type EngineRandom struct{}

func NewEngineRandom() *EngineRandom {
	return &EngineRandom{}
}

func (e *EngineRandom) Name() string {
	return "random"
}

func (e *EngineRandom) Author() string {
	return "Olly Britton"
}

func (e *EngineRandom) NewGame() error {
	return nil
}

func (e *EngineRandom) Prepare() error {
	return nil
}

func (e *EngineRandom) Search(pos *position.Position, searchOptions search.SearchOptions) (position.Move, error) {
	legalMoves := pos.MovesLegal().AsSlice()
	return legalMoves[rand.Intn(len(legalMoves))], nil
}
