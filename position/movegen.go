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

var (
	// Pre-initialised tables of king and knight moves
	kingMoves   [64]Bitboard = initialiseKingMoves()
	knightMoves [64]Bitboard = initialiseKnightMoves()
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

	pseudolegalMoves = append(pseudolegalMoves, p.MovesKing()...)
	pseudolegalMoves = append(pseudolegalMoves, p.MovesKnights()...)

	return pseudolegalMoves
}

func (p *Position) MovesWhitePawns() []Move {
	moves := []Move{}
	whitePawns := p.Pieces[Pawn] & p.Occupied[White]

	oneStep := (whitePawns << DirN) & ^(p.Occupied[White] | p.Occupied[Black])

	// Deal with pieces blocking pawns advancing two steps
	maskNotBlocked := ^(((p.Occupied[White] | p.Occupied[Black]) & maskRank3) >> DirN)
	twoSteps := ((whitePawns & maskNotBlocked) << (DirN * 2)) & ^(p.Occupied[White] | p.Occupied[Black]) & maskRank4

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

	// Remove moves that would lead to promotion
	oneStep = oneStep & ^maskRank8
	twoSteps = twoSteps & ^maskRank8
	capturesLeft = capturesLeft & ^maskRank8
	capturesRight = capturesRight & ^maskRank8

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

	moves = append(
		moves,
		p.movesFromBitboard(
			WhitePawn,
			func(to uint8) uint8 {
				return to - DirN
			},
			oneStep,
		)...,
	)

	moves = append(
		moves,
		p.movesFromBitboard(
			WhitePawn,
			func(to uint8) uint8 {
				return to - DirN - DirN
			},
			twoSteps,
		)...,
	)

	moves = append(
		moves,
		p.movesFromBitboard(
			WhitePawn,
			func(to uint8) uint8 {
				return to - DirNW
			},
			capturesLeft,
		)...,
	)

	moves = append(
		moves,
		p.movesFromBitboard(
			WhitePawn,
			func(to uint8) uint8 {
				return to - DirNE
			},
			capturesRight,
		)...,
	)

	return moves
}

func (p *Position) MovesBlackPawns() []Move {
	moves := []Move{}
	blackPawns := p.Pieces[Pawn] & p.Occupied[Black]

	oneStep := (blackPawns >> DirN) & ^(p.Occupied[White] | p.Occupied[Black])

	// Deal with pieces blocking pawns advancing two steps
	maskNotBlocked := ^(((p.Occupied[White] | p.Occupied[Black]) & maskRank6) << DirN)
	twoSteps := ((blackPawns & maskNotBlocked) >> (DirN * 2)) & ^(p.Occupied[White] | p.Occupied[Black]) & maskRank5

	capturesLeft := (blackPawns & ^maskFileA) >> DirNE & p.Occupied[White]
	capturesRight := (blackPawns & ^maskFileH) >> DirNW & p.Occupied[White]

	promotions := (oneStep | capturesLeft | capturesRight) & maskRank1
	if promotions != 0 {
		for to := promotions.FirstOn(); to <= promotions.LastOn() && to != 64; to++ {
			if promotions.IsOn(to) {
				if capturesLeft.IsOn(to) {
					from := to + DirNE

					moves = append(moves, NewMove(from, to, BlackPawn, p.Squares[to], Queen, p.Castling, p.EnPassant))
					moves = append(moves, NewMove(from, to, BlackPawn, p.Squares[to], Bishop, p.Castling, p.EnPassant))
					moves = append(moves, NewMove(from, to, BlackPawn, p.Squares[to], Knight, p.Castling, p.EnPassant))
					moves = append(moves, NewMove(from, to, BlackPawn, p.Squares[to], Rook, p.Castling, p.EnPassant))
				}

				if capturesRight.IsOn(to) {
					from := to + DirNW

					moves = append(moves, NewMove(from, to, BlackPawn, p.Squares[to], Queen, p.Castling, p.EnPassant))
					moves = append(moves, NewMove(from, to, BlackPawn, p.Squares[to], Bishop, p.Castling, p.EnPassant))
					moves = append(moves, NewMove(from, to, BlackPawn, p.Squares[to], Knight, p.Castling, p.EnPassant))
					moves = append(moves, NewMove(from, to, BlackPawn, p.Squares[to], Rook, p.Castling, p.EnPassant))
				}

				if oneStep.IsOn(to) {
					from := to + DirN

					moves = append(moves, NewMove(from, to, BlackPawn, p.Squares[to], Queen, p.Castling, p.EnPassant))
					moves = append(moves, NewMove(from, to, BlackPawn, p.Squares[to], Bishop, p.Castling, p.EnPassant))
					moves = append(moves, NewMove(from, to, BlackPawn, p.Squares[to], Knight, p.Castling, p.EnPassant))
					moves = append(moves, NewMove(from, to, BlackPawn, p.Squares[to], Rook, p.Castling, p.EnPassant))
				}
			}
		}
	}

	// Remove moves that would lead to promotion
	oneStep = oneStep & ^maskRank1
	twoSteps = twoSteps & ^maskRank1
	capturesLeft = capturesLeft & ^maskRank1
	capturesRight = capturesRight & ^maskRank1

	var enPassantLeft, enPassantRight Bitboard

	if p.EnPassant != 0 {
		enPassant := Bitboard(1) << uint(p.EnPassant)

		enPassantLeft = ((blackPawns & ^maskFileA) >> DirNW) & enPassant
		enPassantRight = ((blackPawns & ^maskFileH) >> DirNE) & enPassant

		if enPassantLeft != 0 {
			from := p.EnPassant + DirNW
			moves = append(moves, NewMove(from, p.EnPassant, BlackPawn, WhitePawn, None, p.Castling, p.EnPassant))
		}

		if enPassantRight != 0 {
			from := p.EnPassant + DirNE
			moves = append(moves, NewMove(from, p.EnPassant, BlackPawn, WhitePawn, None, p.Castling, p.EnPassant))
		}
	}

	moves = append(
		moves,
		p.movesFromBitboard(
			BlackPawn,
			func(to uint8) uint8 {
				return to + DirN
			},
			oneStep,
		)...,
	)

	moves = append(
		moves,
		p.movesFromBitboard(
			BlackPawn,
			func(to uint8) uint8 {
				return to + DirN + DirN
			},
			twoSteps,
		)...,
	)

	moves = append(
		moves,
		p.movesFromBitboard(
			BlackPawn,
			func(to uint8) uint8 {
				return to + DirNE
			},
			capturesLeft,
		)...,
	)

	moves = append(
		moves,
		p.movesFromBitboard(
			BlackPawn,
			func(to uint8) uint8 {
				return to + DirNW
			},
			capturesRight,
		)...,
	)

	return moves
}

func (p *Position) MovesKing() []Move {
	bitboard := kingMoves[p.KingLocation[p.SideToMove]] & ^p.Occupied[p.SideToMove]

	return p.movesFromBitboard(
		King.OfColor(p.SideToMove),
		func(to uint8) uint8 { return p.KingLocation[p.SideToMove] },
		bitboard,
	)
}

func (p *Position) MovesKnights() []Move {
	knightLocations := p.Pieces[Knight] & p.Occupied[p.SideToMove]
	moves := []Move{}

	for from := knightLocations.FirstOn(); from < 64 && from <= knightLocations.LastOn(); from++ {
		if knightLocations.IsOn(from) {
			bitboard := knightMoves[from] & ^p.Occupied[p.SideToMove]
			moves = append(
				moves,
				p.movesFromBitboard(
					Knight.OfColor(p.SideToMove),
					func(to uint8) uint8 {
						return from
					},
					bitboard,
				)...,
			)
		}
	}

	return moves
}

// movesFromBitboard returns a list of moves given a piece, a from square and a bitboard representing all possible destinations.
// fromFunc takes a function to calculate the from square from the to square. E.g. if it's a white pawn advance of one square, the function
// that needs to be passed will subtract 8.
// This function doesn't know anything about promotions.
func (p *Position) movesFromBitboard(piece ColoredPiece, fromFunc func(uint8) uint8, bitboard Bitboard) []Move {
	moves := []Move{}

	for to := bitboard.FirstOn(); to <= bitboard.LastOn() && to != 64; to++ {
		if bitboard.IsOn(to) {
			moves = append(moves, NewMove(fromFunc(to), to, piece, p.Squares[to], None, p.Castling, p.EnPassant))
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

// initialiseKingMoves is executed at the start of the program and sets up a table of all the possible king moves from any of the 64 squares,
// assuming that the other squares are not occupied.
func initialiseKingMoves() [64]Bitboard {
	moves := [64]Bitboard{}

	for from := uint8(0); from < 64; from++ {
		bitboard := Bitboard(0)

		rank := (from / 8) + 1
		file := (from % 8) + 1 // A-file is 1, B-file is 2, etc.

		if rank != 1 {
			// . . .
			// . K .
			// . * .
			bitboard.On(from - DirN)

			if file != 1 {
				// . . .
				// . K .
				// * . .
				bitboard.On(from - 7)
			}

			if file != 8 {
				// . . .
				// . K .
				// . . *
				bitboard.On(from - 9)
			}
		}

		// Not on the last rank, so can move forwards
		if rank != 8 {
			// . * .
			// . K .
			// . . .
			bitboard.On(from + DirN)

			if file != 1 {
				// * . .
				// . K .
				// . . .
				bitboard.On(from + 7)
			}

			if file != 8 {
				// . . *
				// . K .
				// . . .
				bitboard.On(from + 9)
			}
		}

		if file != 1 {
			// . . .
			// * K .
			// . . .
			bitboard.On(from - 1)
		}

		if file != 8 {
			// . . .
			// . K *
			// . . .
			bitboard.On(from + 1)
		}

		moves[from] = bitboard
	}

	return moves
}

// initialiseKnightMoves is executed at the start of the program and sets up a table of all the possible knight moves from any of the 64 squares,
// assuming that the other squares are not occupied.
func initialiseKnightMoves() [64]Bitboard {
	moves := [64]Bitboard{}

	// Possible knight moves from a given square:
	// . * . * .
	// * . . . *
	// . . N . .
	// * . . . *
	// . * . * .
	// Care needs to be taken not to set bits out of the range.

	for from := uint8(0); from < 64; from++ {
		bitboard := Bitboard(0)

		rank := (from / 8) + 1
		file := (from % 8) + 1 // A-file is 1, B-file is 2, etc.

		if file > 2 && rank < 8 {
			// . . . . .
			// * . . . .
			// . . N . .
			// . . . . .
			// . . . . .
			bitboard.On(from + 8 - 1 - 1)
		}

		if file < 7 && rank < 8 {
			// . . . . .
			// . . . . *
			// . . N . .
			// . . . . .
			// . . . . .
			bitboard.On(from + 8 + 1 + 1)
		}

		if file > 1 && rank < 7 {
			// . * . . .
			// . . . . .
			// . . N . .
			// . . . . .
			// . . . . .
			bitboard.On(from + 8 + 8 - 1)
		}

		if file < 8 && rank < 7 {
			// . . . * .
			// . . . . .
			// . . N . .
			// . . . . .
			// . . . . .
			bitboard.On(from + 8 + 8 + 1)
		}

		if file > 2 && rank > 1 {
			// . . . . .
			// . . . . .
			// . . N . .
			// * . . . .
			// . . . . .
			bitboard.On(from - 8 - 1 - 1)
		}

		if file < 7 && rank > 1 {
			// . . . . .
			// . . . . .
			// . . N . .
			// . . . . *
			// . . . . .
			bitboard.On(from - 8 + 1 + 1)
		}

		if file > 1 && rank > 2 {
			// . . . . .
			// . . . . .
			// . . N . .
			// . . . . .
			// . * . . .
			bitboard.On(from - 8 - 8 - 1)
		}

		if file < 8 && rank > 2 {
			// . . . . .
			// . . . . .
			// . . N . .
			// . . . . .
			// . . . * .
			bitboard.On(from - 8 - 8 + 1)
		}

		moves[from] = bitboard

	}

	return moves
}
