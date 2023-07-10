package position

// EvalPawnStarUs evaluates the position from the perspective of the engine that really, really cares about its own pawns.
func EvalPawnStarUs(pos *Position) int16 {
	overall := int16(0)

	for i := 0; i < 64; i++ {
		curr := pos.Squares[i]

		switch curr.Color() {
		case White:
			overall += simpleEvalTable[curr.Colorless()]
		case Black:
			overall -= simpleEvalTable[curr.Colorless()]
		}

		if pos.SideToMove == White && curr == WhitePawn {
			overall += 15
		} else if pos.SideToMove == Black && curr == BlackPawn {
			overall -= 15
		}
	}

	return overall
}

// EvalPawnStarThem evaluates the position from the perspective of the player, that sort of wants the engine's pawns.
func EvalPawnStarThem(pos *Position) int16 {
	overall := int16(0)

	for i := 0; i < 64; i++ {
		curr := pos.Squares[i]

		switch curr.Color() {
		case White:
			overall += simpleEvalTable[curr.Colorless()]
		case Black:
			overall -= simpleEvalTable[curr.Colorless()]
		}

		if pos.SideToMove == White && curr == BlackPawn {
			overall -= 5
		} else if pos.SideToMove == Black && curr == WhitePawn {
			overall += 5
		}
	}

	return overall
}
