package position

func (p *Position) MovesLegal() []Move {
	pseudolegalMoves := p.MovesPseudolegal()
	legalMoves := []Move{}

	for _, move := range pseudolegalMoves {
		if p.MakeMove(move) {
			legalMoves = append(legalMoves, move)
		}
	}

	return legalMoves
}

func (p *Position) MovesPseudolegal() []Move {
	legalMoves := []Move{}

	for i := uint8(0); i < 64; i++ {
		// Skip a square if it's empty or not the correct color.
		if p.Squares[i] == Empty || p.Squares[i].Color() != p.SideToMove {
			continue
		}

		movesFromSquare := p.MovesFromSquare(i)
		legalMoves = append(legalMoves, movesFromSquare...)
	}

	return legalMoves
}

func (p *Position) MovesFromSquare(i uint8) []Move {
	moves := []Move{}

	switch p.Squares[i] {
	case WhitePawn:
		moves = append(moves, p.MovesWhitePawnPushes(i)...)
		moves = append(moves, p.MovesWhitePawnCaptures(i)...)
	case BlackPawn:
		moves = append(moves, p.MovesBlackPawnPushes(i)...)
		moves = append(moves, p.MovesBlackPawnCaptures(i)...)
	}

	return moves
}

func (p *Position) MovesWhitePawnPushes(i uint8) []Move {
	moves := []Move{}

	// The pawn is on the 2nd rank and so it can move two squares.
	if p.OnRank(i, 2) && p.IsEmpty(i+16) {
		moves = append(moves, NewMove(i, i+16, None))
	}

	if p.IsEmpty(i + 8) {
		moves = append(moves, NewMove(i, i+8, None))
	}

	return moves
}

func (p *Position) MovesWhitePawnCaptures(i uint8) []Move {
	moves := []Move{}

	// Capturing upwards to the left.
	if !p.OnFileA(i) && p.IsValid(i+7) && !p.IsEmpty(i+7) && p.Squares[i+7].Color() == p.SideToMove.Invert() {
		moves = append(moves, NewMove(i, i+7, None))
	}

	// Capturing upwards to the right.
	if !p.OnFileH(i) && p.IsValid(i+9) && !p.IsEmpty(i+9) && p.Squares[i+9].Color() == p.SideToMove.Invert() {
		moves = append(moves, NewMove(i, i+9, None))
	}

	// TODO: promotion properly
	// TODO: en passant logic

	return moves
}

func (p *Position) MovesBlackPawnPushes(i uint8) []Move {
	moves := []Move{}

	// The pawn is on the 7th rank and so it can move two squares.
	if p.OnRank(i, 7) && p.IsEmpty(i-16) {
		moves = append(moves, NewMove(i, i-16, None))
	}

	if p.IsEmpty(i - 8) {
		moves = append(moves, NewMove(i, i-8, None))
	}

	return moves
}

func (p *Position) MovesBlackPawnCaptures(i uint8) []Move {
	moves := []Move{}

	// Capturing downwards to the left.
	if !p.OnFileA(i) && p.IsValid(i-7) && !p.IsEmpty(i-7) && p.Squares[i-7].Color() == p.SideToMove.Invert() {
		moves = append(moves, NewMove(i, i-7, None))
	}

	// Capturing downwards to the right.
	if !p.OnFileH(i) && p.IsValid(i-9) && !p.IsEmpty(i-9) && p.Squares[i-9].Color() == p.SideToMove.Invert() {
		moves = append(moves, NewMove(i, i-9, None))
	}

	// TODO: en passant logic

	return moves
}
