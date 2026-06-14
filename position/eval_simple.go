package position

// Piece values in centipawns (1 pawn = 100). Kings are 0: both sides always have exactly one, so
// they cancel in the material count, and a large value would overflow int16 once scaled.
var simpleEvalTable = map[Piece]int16{
	Pawn:   100,
	Knight: 320,
	Bishop: 330,
	Rook:   500,
	Queen:  900,
	King:   0,
}

// EvalSimple evaluates the position using a simple material count.
// TODO: This could definitely be sped up by using bitboards instead of the pos.Squares.
func EvalSimple(pos *Position) int16 {
	overall := int16(0)

	for i := 0; i < 64; i++ {
		curr := pos.Squares[i]

		switch curr.Color() {
		case White:
			overall += simpleEvalTable[curr.Colorless()]
		case Black:
			overall -= simpleEvalTable[curr.Colorless()]
		}
	}

	return overall
}
