package engines

import (
	"math"

	"github.com/ollybritton/StupidChess/position"
)

type EngineSprinter struct{}

func NewEngineSprinter() *EngineSprinter {
	return &EngineSprinter{}
}

func (e *EngineSprinter) Name() string {
	return "try-hard"
}

func (e *EngineSprinter) Author() string {
	return "Olly Britton"
}

func (e *EngineSprinter) Prepare() error {
	return nil
}

func (e *EngineSprinter) Search(pos *position.Position, searchOptions SearchOptions) (position.Move, error) {
	legalMoves := pos.MovesLegal()

	var bestMove position.Move
	var bestSquaredDist float64

	for _, move := range legalMoves.AsSlice() {
		from := move.From()
		to := move.To()

		fromRank := float64(from / 8)
		fromFile := float64(from % 8)

		toRank := float64(to / 8)
		toFile := float64(to % 8)

		squaredDist := math.Pow(fromRank-toRank, 2) + math.Pow(fromFile-toFile, 2)

		if squaredDist > bestSquaredDist {
			bestMove = move
			bestSquaredDist = squaredDist
		}
	}

	return bestMove, nil
}
