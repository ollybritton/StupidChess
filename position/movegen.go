package position

const (
	DirN = +8
	DirE = +1
	DirS = -8
	DirW = -1

	DirNE = +9
	DirNW = +7
	DirSE = -9
	DirSW = -7
)

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
	pseudolegalMoves := []Move{}

	if p.SideToMove == White {
		pseudolegalMoves = append(pseudolegalMoves, p.MovesWhitePawns()...)
	} else if p.SideToMove == Black {
		pseudolegalMoves = append(pseudolegalMoves, p.MovesBlackPawns()...)
	}

	return pseudolegalMoves
}

func (p *Position) MovesWhitePawns() []Move {
	moves := []Move{}
	whitePawns := p.Pieces[Pawn] & p.Occupied[White]

	oneStep := (whitePawns << DirN) & ^(p.Occupied[White] | p.Occupied[Black])
	twoSteps := (whitePawns << (DirN * 2)) & ^(p.Occupied[White] | p.Occupied[Black]) & maskRank3

	capturesLeft := (whitePawns & ^maskFileA) << DirNW & p.Occupied[Black]
	capturesRight := (whitePawns & ^maskFileH) << DirNE & p.Occupied[Black]

	promotions := (oneStep | capturesLeft | capturesRight) & maskRank8
	if promotions != 0 {
		for to := promotions.FirstOn(); to <= promotions.LastOn() && to != 64; to++ {
			if promotions.IsOn(to) {
				if capturesLeft.IsOn(to) {
					from := to - DirNW

					moves = append(moves, NewMove(from, to, WhitePawn, p.Squares[to], Queen, p.Castling, p.EnPassant))
					moves = append(moves, NewMove(from, to, WhitePawn, p.Squares[to], Bishop, p.Castling, p.EnPassant))
					moves = append(moves, NewMove(from, to, WhitePawn, p.Squares[to], Knight, p.Castling, p.EnPassant))
					moves = append(moves, NewMove(from, to, WhitePawn, p.Squares[to], Rook, p.Castling, p.EnPassant))
				}

				if capturesRight.IsOn(to) {
					from := to - DirNE

					moves = append(moves, NewMove(from, to, WhitePawn, p.Squares[to], Queen, p.Castling, p.EnPassant))
					moves = append(moves, NewMove(from, to, WhitePawn, p.Squares[to], Bishop, p.Castling, p.EnPassant))
					moves = append(moves, NewMove(from, to, WhitePawn, p.Squares[to], Knight, p.Castling, p.EnPassant))
					moves = append(moves, NewMove(from, to, WhitePawn, p.Squares[to], Rook, p.Castling, p.EnPassant))
				}

				if oneStep.IsOn(to) {
					from := to - DirN

					moves = append(moves, NewMove(from, to, WhitePawn, p.Squares[to], Queen, p.Castling, p.EnPassant))
					moves = append(moves, NewMove(from, to, WhitePawn, p.Squares[to], Bishop, p.Castling, p.EnPassant))
					moves = append(moves, NewMove(from, to, WhitePawn, p.Squares[to], Knight, p.Castling, p.EnPassant))
					moves = append(moves, NewMove(from, to, WhitePawn, p.Squares[to], Rook, p.Castling, p.EnPassant))
				}
			}
		}
	}

	var enPassantLeft, enPassantRight Bitboard

	if p.EnPassant != 0 {
		enPassant := Bitboard(1) << uint(p.EnPassant)

		enPassantLeft = ((whitePawns & ^maskFileA) << DirNW) & enPassant
		enPassantRight = ((whitePawns & ^maskFileH) << DirNE) & enPassant

		if enPassantLeft != 0 {
			from := p.EnPassant - DirNW
			moves = append(moves, NewMove(from, p.EnPassant, WhitePawn, BlackPawn, None, p.Castling, p.EnPassant))
		}

		if enPassantRight != 0 {
			from := p.EnPassant - DirNE
			moves = append(moves, NewMove(from, p.EnPassant, WhitePawn, BlackPawn, None, p.Castling, p.EnPassant))
		}
	}

	for to := oneStep.FirstOn(); to <= oneStep.LastOn() && to != 64; to++ {
		if oneStep.IsOn(to) {
			from := to - DirN
			moves = append(moves, NewMove(from, to, WhitePawn, Empty, None, p.Castling, p.EnPassant))
		}
	}

	for to := twoSteps.FirstOn(); to <= twoSteps.LastOn() && to != 64; to++ {
		if twoSteps.IsOn(to) {
			from := to - DirN - DirN
			moves = append(moves, NewMove(from, to, WhitePawn, Empty, None, p.Castling, p.EnPassant))
		}
	}

	for to := capturesLeft.FirstOn(); to <= capturesLeft.LastOn() && to != 64; to++ {
		if capturesLeft.IsOn(to) {
			from := to - DirNW
			moves = append(moves, NewMove(from, to, WhitePawn, Empty, None, p.Castling, p.EnPassant))
		}
	}

	for to := capturesRight.FirstOn(); to <= capturesRight.LastOn() && to != 64; to++ {
		if capturesRight.IsOn(to) {
			from := to - DirNE
			moves = append(moves, NewMove(from, to, WhitePawn, Empty, None, p.Castling, p.EnPassant))
		}
	}

	return moves
}

func (p *Position) MovesBlackPawns() []Move {
	return []Move{}
}

// movesFromBitboard returns a list of moves given a piece, a from square and a bitboard representing all possible destinations.
// This function doesn't know anything about promotions.
func (p *Position) movesFromBitboard(piece ColoredPiece, from uint8, bitboard Bitboard) []Move {
	moves := []Move{}

	for to := bitboard.FirstOn(); to <= bitboard.LastOn() && to != 64; to++ {
		if bitboard.IsOn(to) {
			moves = append(moves, NewMove(from, to, piece, p.Squares[to], None, p.Castling, p.EnPassant))
		}
	}

	return moves
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
