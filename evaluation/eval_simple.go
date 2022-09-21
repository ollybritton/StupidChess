package evaluation

import "github.com/ollybritton/StupidChess/position"

var simpleEvalTable = map[position.Piece]float64{
	position.Pawn:   1,
	position.Knight: 3,
	position.Bishop: 4,
	position.Rook:   5,
	position.Queen:  9,
	position.King:   100_000,
}

// EvalSimple evaluates the position using a simple material count.
// TODO: This could definitely be sped up by using bitboards instead of the pos.Squares.
func EvalSimple(pos position.Position) float64 {
	overall := 0.0

	for i := 0; i < 64; i++ {
		curr := pos.Squares[i]

		switch curr.Color() {
		case position.White:
			overall += simpleEvalTable[curr.Colorless()]
		case position.Black:
			overall -= simpleEvalTable[curr.Colorless()]
		}
	}

	return overall
}
