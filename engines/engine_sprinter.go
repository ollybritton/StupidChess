package engines

import (
	"math"

	"github.com/ollybritton/StupidChess/position"
	"github.com/ollybritton/StupidChess/search"
)

type EngineSprinter struct {
	prevPiece position.Piece
}

func NewEngineSprinter() *EngineSprinter {
	return &EngineSprinter{prevPiece: position.None}
}

func (e *EngineSprinter) Name() string {
	return "try-hard"
}

func (e *EngineSprinter) Author() string {
	return "Olly Britton"
}

func (e *EngineSprinter) NewGame() error {
	e.prevPiece = position.None
	return nil
}

func (e *EngineSprinter) Prepare() error {
	return nil
}

func (e *EngineSprinter) Search(pos *position.Position, searchOptions search.SearchOptions) (position.Move, error) {
	legalMoves := pos.MovesLegal()

	newMoves := pos.MovesLegal().Copy()
	newMoves.Filter(func(m position.Move) bool {
		return m.Moved().Colorless() != e.prevPiece
	})

	var bestMove position.Move
	var bestSquaredDist float64

	for _, move := range newMoves.AsSlice() {
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

	if bestMove != position.Move(0) {
		e.prevPiece = bestMove.Moved().Colorless()
		return bestMove, nil
	}

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

	e.prevPiece = bestMove.Moved().Colorless()
	return bestMove, nil
}
