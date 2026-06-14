package engines

import (
	"math/rand"

	"github.com/ollybritton/StupidChess/position"
	"github.com/ollybritton/StupidChess/search"
)

func NewEngineRandom() *SimpleEngine {
	return NewSimpleEngine(
		"random",
		"Olly Britton",
		"Plays a uniformly random legal move.",
		func(pos *position.Position, searchOptions search.SearchOptions) (position.Move, error) {
			return moveRandom(pos)
		},
	)
}

func moveRandom(pos *position.Position) (position.Move, error) {
	legalMoves := pos.MovesLegal().AsSlice()
	return legalMoves[rand.Intn(len(legalMoves))], nil
}
