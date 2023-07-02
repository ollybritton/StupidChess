package engines

import (
	"math"
	"math/rand"

	"github.com/ollybritton/StupidChess/position"
	"github.com/ollybritton/StupidChess/search"
)

func NewEngineSuicideKing() *SimpleEngine {
	return NewSimpleEngine(
		"suicide-king",
		"Olly Britton",
		func(pos *position.Position, searchOptions search.SearchOptions) (position.Move, error) {
			return moveMinimiseKingDistance(pos)
		},
	)
}

func moveMinimiseKingDistance(pos *position.Position) (position.Move, error) {
	legalMoves := pos.MovesLegal().AsSlice()

	var bestMoves []position.Move
	var lowestScore float64 = 100

	for _, move := range legalMoves {
		pos.MakeMove(move)

		ourKing := pos.KingLocation[pos.SideToMove.Invert()]
		theirKing := pos.KingLocation[pos.SideToMove]

		pos.UndoMove(move)

		ourKingRank := float64(ourKing / 8)
		ourKingFile := float64(ourKing % 8)

		theirKingRank := float64(theirKing / 8)
		theirKingFile := float64(theirKing % 8)

		score := math.Pow(theirKingFile-ourKingFile, 2) + math.Pow(theirKingRank-ourKingRank, 2)

		if score == lowestScore {
			bestMoves = append(bestMoves, move)
		} else if score < lowestScore {
			bestMoves = []position.Move{move}
			lowestScore = score
		}
	}

	return bestMoves[rand.Intn(len(bestMoves))], nil
}
