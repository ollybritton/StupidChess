package engines

import (
	"fmt"
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
	return "sprinter"
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

func (e *EngineSprinter) Go(pos *position.Position, searchOptions search.SearchOptions) error {
	legalMoves := pos.MovesLegal()

	newMoves := pos.MovesLegal().Copy()
	newMoves.Filter(func(m position.Move) bool {
		return m.Moved().Colorless() != e.prevPiece
	})

	// Distance is calculated using the "maximum metric" (https://chris3606.github.io/GoRogue/articles/grid_components/measuring-distance.html#chebyshev-distance).

	var bestMove position.Move
	var bestDist float64 = -1

	for _, move := range newMoves.AsSlice() {
		from := move.From()
		to := move.To()

		fromRank := float64(from / 8)
		fromFile := float64(from % 8)

		toRank := float64(to / 8)
		toFile := float64(to % 8)

		dist := math.Max(fromRank-toRank, fromFile-toFile)

		if dist > bestDist {
			bestMove = move
			bestDist = dist
		}
	}

	if bestMove != position.Move(0) {
		e.prevPiece = bestMove.Moved().Colorless()

		fmt.Println("bestmove", bestMove.String())
		return nil
	}

	for _, move := range legalMoves.AsSlice() {
		from := move.From()
		to := move.To()

		fromRank := float64(from / 8)
		fromFile := float64(from % 8)

		toRank := float64(to / 8)
		toFile := float64(to % 8)

		dist := math.Max(fromRank-toRank, fromFile-toFile)

		if dist > bestDist {
			bestMove = move
			bestDist = dist
		}
	}

	e.prevPiece = bestMove.Moved().Colorless()

	fmt.Println("bestmove", bestMove.String())

	return nil
}

func (e *EngineSprinter) Stop() {}
