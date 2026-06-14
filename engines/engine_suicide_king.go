package engines

import (
	"github.com/ollybritton/StupidChess/position"
	"github.com/ollybritton/StupidChess/search"
)

// suicideKingDepth is how many plies ahead the suicide king plans. Depth 3 (its move, the reply, its
// next move) is enough to make preparatory moves — e.g. shift a blocking pawn now so the king can step
// forward next move — while staying fast.
const suicideKingDepth = 3

func NewEngineSuicideKing() *SimpleEngine {
	return NewSimpleEngine(
		"suicide-king",
		"Olly Britton",
		"Walks its own king toward the enemy king, planning a few moves ahead to clear the way.",
		func(pos *position.Position, _ search.SearchOptions) (position.Move, error) {
			move := bestPositionalMove(pos, suicideKingDepth, pos.SideToMove, suicideKingEval)
			return move, nil
		},
	)
}

// suicideKingEval rewards the kings being close together (it is the same for either colour, since the
// distance is symmetric). The look-ahead maximises it for the suicide king and assumes the opponent
// tries to keep its king away.
func suicideKingEval(pos *position.Position) int {
	return -kingDistanceSquared(pos)
}
