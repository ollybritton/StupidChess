package position

var simpleEvalTable = map[Piece]int16{
	Pawn:   1,
	Knight: 3,
	Bishop: 4,
	Rook:   5,
	Queen:  9,
	King:   1_000,
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
