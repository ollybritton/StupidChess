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

// Knight moves and king moves can be found by looking up a table of each square on the board to the corresponding bitboard.
// This table is initialised at the beginning of the program.
//
// It would be nice to have a table to look up bishop and rook attacks (and queens, which are their combination), but this would require a table with
// around 64*2^64 entries, for each square on the board and all possible other pieces on the board that could block its path.
// The solution to this is to use magic bitboards, which is where you find all the "blockers" in the way of a rook or bishop's path, and multiply
// it by a magic number that turns this into the index of a pre-populated table. This reduces the amount of information you need to store.
// This pre-populated table should be generated and stored at the start of the program.
// I still don't understand this very well -- you can probably tell from the half-description. Hopefully writing the implementation will mean I understand
// it better.
//
// Moves for pawns should be generated seperately for black and white, and then have the MovesPseudolegal function pick the correct
// ones depending on whose side to move it is. This is because pawn moves aren't symmetrical like the other moves.
// There's a way you can do this effeciently with bitwise operations.
//
// TODO: Write a function for generating a list of moves given a from square and a bitboard of all possible destinations.

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
