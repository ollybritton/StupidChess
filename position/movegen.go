package position

import "fmt"

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
	kingMoves   [64]Bitboard
	knightMoves [64]Bitboard

	// Pre-initialised tables of rook and bishop masks to find the blockers on the board from a particular square
	rookMasks   [64]Bitboard
	bishopMasks [64]Bitboard

	// Pre-initialised tables of rook and bishop moves from a square given an index calculated using a magic number.
	rookMoves   [64][4096]Bitboard
	bishopMoves [64][512]Bitboard
)

func (p *Position) MovesLegal() []Move {
	pseudolegalMoves := p.MovesPseudolegal()
	legalMoves := []Move{}

	for _, move := range pseudolegalMoves {
		if p.MakeMove(move) { // If it doesn't put the king in check, add it back.
			legalMoves = append(legalMoves, move)
			p.UndoMove(move)
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

	pseudolegalMoves = append(pseudolegalMoves, p.MovesRooks()...)
	pseudolegalMoves = append(pseudolegalMoves, p.MovesBishops()...)
	pseudolegalMoves = append(pseudolegalMoves, p.MovesQueens()...)

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

	moves := p.movesFromBitboard(
		King.OfColor(p.SideToMove),
		func(to uint8) uint8 { return p.KingLocation[p.SideToMove] },
		bitboard,
	)

	moves = append(moves, p.MovesCastling()...)

	return moves
}

func (p *Position) MovesCastling() []Move {
	moves := []Move{}

	if p.SideToMove == White && !p.KingInCheck(White) {
		if p.Castling&longW != 0 {
			squares := []uint8{SquareB1, SquareC1, SquareD1}

			failed := false

			for _, square := range squares {
				if !p.IsEmpty(square) || p.IsAttacked(square, Black) {
					failed = true
					break
				}
			}

			if !failed {
				moves = append(
					moves,
					NewMove(SquareE1, SquareC1, WhiteKing, Empty, None, p.Castling, p.EnPassant),
				)
			}
		}

		if p.Castling&shortW != 0 {
			squares := []uint8{SquareF1, SquareG1}

			failed := false

			for _, square := range squares {
				if !p.IsEmpty(square) || p.IsAttacked(square, Black) {
					failed = true
					break
				}
			}

			if !failed {
				moves = append(
					moves,
					NewMove(SquareE1, SquareG1, WhiteKing, Empty, None, p.Castling, p.EnPassant),
				)
			}
		}
	} else if p.SideToMove == Black && !p.KingInCheck(Black) {
		if p.Castling&longB != 0 {
			squares := []uint8{SquareB8, SquareC8, SquareD8}

			failed := false

			for _, square := range squares {
				if !p.IsEmpty(square) || p.IsAttacked(square, White) {
					failed = true
					break
				}
			}

			if !failed {
				moves = append(
					moves,
					NewMove(SquareE8, SquareC8, BlackKing, Empty, None, p.Castling, p.EnPassant),
				)
			}
		}

		if p.Castling&shortW != 0 {
			squares := []uint8{SquareF8, SquareG8}

			failed := false

			for _, square := range squares {
				if !p.IsEmpty(square) || p.IsAttacked(square, Black) {
					failed = true
					break
				}
			}

			if !failed {
				moves = append(
					moves,
					NewMove(SquareE8, SquareG8, BlackKing, Empty, None, p.Castling, p.EnPassant),
				)
			}
		}
	}

	return moves
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

func (p *Position) MovesRooks() []Move {
	moves := []Move{}
	rookLocations := p.Pieces[Rook] & p.Occupied[p.SideToMove]

	for from := rookLocations.FirstOn(); from < 64 && from <= rookLocations.LastOn(); from++ {
		if rookLocations.IsOn(from) {
			blockers := (p.Occupied[p.SideToMove.Invert()] | p.Occupied[p.SideToMove]) & rookMasks[from]
			key := (uint64(blockers) * rookMagics[from].multiplier) >> rookMagics[from].shift

			moves = append(
				moves,
				p.movesFromBitboard(
					Rook.OfColor(p.SideToMove),
					func(to uint8) uint8 {
						return from
					},
					rookMoves[from][key] & ^p.Occupied[p.SideToMove],
				)...,
			)
		}
	}

	return moves
}

func (p *Position) MovesBishops() []Move {
	moves := []Move{}
	bishopLocations := p.Pieces[Bishop] & p.Occupied[p.SideToMove]

	for from := bishopLocations.FirstOn(); from < 64 && from <= bishopLocations.LastOn(); from++ {
		if bishopLocations.IsOn(from) {
			blockers := (p.Occupied[p.SideToMove.Invert()] | p.Occupied[p.SideToMove]) & bishopMasks[from]
			key := (uint64(blockers) * bishopMagics[from].multiplier) >> bishopMagics[from].shift

			available := bishopMoves[from][key] & ^p.Occupied[p.SideToMove]

			moves = append(
				moves,
				p.movesFromBitboard(
					Bishop.OfColor(p.SideToMove),
					func(to uint8) uint8 {
						return from
					},
					available,
				)...,
			)
		}
	}

	return moves
}

func (p *Position) MovesQueens() []Move {
	moves := []Move{}
	queenLocations := p.Pieces[Queen] & p.Occupied[p.SideToMove]

	for from := queenLocations.FirstOn(); from < 64 && from <= queenLocations.LastOn(); from++ {
		if queenLocations.IsOn(from) {
			bishopBlockers := (p.Occupied[p.SideToMove.Invert()] | p.Occupied[p.SideToMove]) & bishopMasks[from]
			bishopKey := (uint64(bishopBlockers) * bishopMagics[from].multiplier) >> bishopMagics[from].shift
			bishopAvailable := bishopMoves[from][bishopKey] & ^p.Occupied[p.SideToMove]

			rookBlockers := (p.Occupied[p.SideToMove.Invert()] | p.Occupied[p.SideToMove]) & rookMasks[from]
			rookKey := (uint64(rookBlockers) * rookMagics[from].multiplier) >> rookMagics[from].shift
			rookAvailable := rookMoves[from][rookKey] & ^p.Occupied[p.SideToMove]

			available := bishopAvailable | rookAvailable

			moves = append(
				moves,
				p.movesFromBitboard(
					Queen.OfColor(p.SideToMove),
					func(to uint8) uint8 {
						return from
					},
					available,
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
				bitboard.On(from - 9)
			}

			if file != 8 {
				// . . .
				// . K .
				// . . *
				bitboard.On(from - 7)
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

// initiailiseRookMasks pre-populates the table of rook masks representing the possible squares a rook can travel to on an empty board
// from a given square
func initialiseRookMasks() [64]Bitboard {
	moves := [64]Bitboard{}

	for from := uint8(0); from < 64; from++ {
		bitboard := Bitboard(0)

		rank := (from / 8) + 1
		file := (from % 8) + 1

		limitBottom := file - 1
		limitTop := 7*8 + (file - 1)

		limitLeft := (rank - 1) * 8
		limitRight := limitLeft + 7

		for i := limitBottom; i <= limitTop; i += 8 {
			bitboard.On(i)
		}

		for i := limitLeft; i <= limitRight; i++ {
			bitboard.On(i)
		}

		// This may not be the most efficient way to do this, but since this only runs once at the start of the
		// program it is fine for now.
		// TODO: clean up rook mask generation code
		bitboard.Off(limitBottom)
		bitboard.Off(limitTop)
		bitboard.Off(limitRight)
		bitboard.Off(limitLeft)
		bitboard.Off(from)

		moves[from] = bitboard
	}

	return moves
}

// getRookMovesFromOccupationSlow returns the rook moves available from a given square with a bitboard representing the occupied squares
// in its path.
// TODO: clean up this code and make it more efficient
func getRookMovesFromOccupationSlow(from uint8, occupiedSquares Bitboard) Bitboard {
	bitboard := Bitboard(0)

	rank := (from / 8) + 1
	file := (from % 8) + 1

	limitBottom := file - 1
	limitTop := 7*8 + (file - 1)

	limitLeft := (rank - 1) * 8
	limitRight := limitLeft + 7

	for i := from; i <= limitTop; i += 8 {
		bitboard.On(i)

		if occupiedSquares.IsOn(i) {
			break
		}
	}

	for i := from; i >= limitBottom && i >= 8; i -= 8 {
		bitboard.On(i)

		if occupiedSquares.IsOn(i) {
			break
		}
	}

	for i := from; i <= limitRight; i++ {
		bitboard.On(i)

		if occupiedSquares.IsOn(i) {
			break
		}
	}

	for i := from; i >= limitLeft && i >= 1; i-- {
		bitboard.On(i)

		if occupiedSquares.IsOn(i) {
			break
		}
	}

	bitboard.Off(from)

	return bitboard
}

func initialiseRookMoves() [64][4096]Bitboard {
	moves := [64][4096]Bitboard{}

	for from := uint8(0); from < 64; from++ {
		subset := Bitboard(0)
		i := 0

		// This code comes from
		//   https://github.com/nkarve/surge/blob/c4ea4e2655cc938632011672ddc880fefe7d02a6/src/tables.cpp#L148
		// Which was found on the question
		//   https://chess.stackexchange.com/questions/34453/how-do-i-complete-this-implementation-of-magic-bitboards
		// This sort of reverse-engineers the magic numbers to come up with all possible blocker bits so we can initialise the table more
		// quickly.
		for subset != 0 || i == 0 {
			index := subset
			index = index * Bitboard(rookMagics[from].multiplier)
			index = index >> rookMagics[from].shift
			moves[from][index] = getRookMovesFromOccupationSlow(from, subset)

			subset = (subset - rookMasks[from]) & rookMasks[from]
			i++
		}
	}

	return moves
}

func initialiseBishopMasks() [64]Bitboard {
	moves := [64]Bitboard{}

	// TODO: clean up and comment this code. Efficiency isn't important here as this is only initialising table values
	// but it's not the easiest to understand like this
	for from := uint8(0); from < 64; from++ {
		bitboard := Bitboard(0)

		for i := from; i < 64; i += 9 {
			if i%8 <= from%8 && i != from {
				break
			}

			bitboard.On(i)
		}

		for i := from; i >= 0; i -= 9 {
			if i%8 >= from%8 && i != from {
				break
			}

			bitboard.On(i)

			if i < 9 {
				break
			}
		}

		for i := from; i < 64; i += 7 {
			if i%8 >= from%8 && i != from {
				break
			}

			bitboard.On(i)
		}

		for i := from; i >= 0 && i < 64; i -= 7 {
			if i%8 <= from%8 && i != from {
				break
			}

			bitboard.On(i)

			if i < 7 {
				break
			}
		}

		bitboard.Off(from)
		bitboard &= maskNonPerimiterSquares

		moves[from] = bitboard
	}

	return moves
}

func getBishopMovesFromOccupationSlow(from uint8, occupiedSquares Bitboard) Bitboard {
	bitboard := Bitboard(0)

	for i := from; i < 64; i += 9 {
		if i%8 <= from%8 && i != from {
			break
		}

		bitboard.On(i)

		if occupiedSquares.IsOn(i) {
			break
		}
	}

	for i := from; i >= 0; i -= 9 {
		if i%8 >= from%8 && i != from {
			break
		}

		bitboard.On(i)

		if i < 9 || occupiedSquares.IsOn(i) {
			break
		}
	}

	for i := from; i < 64; i += 7 {
		if i%8 >= from%8 && i != from {
			break
		}

		bitboard.On(i)

		if occupiedSquares.IsOn(i) {
			break
		}
	}

	for i := from; i >= 0 && i < 64; i -= 7 {
		if i%8 <= from%8 && i != from {
			break
		}

		bitboard.On(i)

		if i < 7 || occupiedSquares.IsOn(i) {
			break
		}
	}

	bitboard.Off(from)

	return bitboard
}

func initialiseBishopMoves() [64][512]Bitboard {
	moves := [64][512]Bitboard{}

	for from := uint8(0); from < 64; from++ {
		subset := Bitboard(0)
		i := 0

		// This code comes from
		//   https://github.com/nkarve/surge/blob/c4ea4e2655cc938632011672ddc880fefe7d02a6/src/tables.cpp#L148
		// Which was found on the question
		//   https://chess.stackexchange.com/questions/34453/how-do-i-complete-this-implementation-of-magic-bitboards
		// This sort of reverse-engineers the magic numbers to come up with all possible blocker bits so we can initialise the table more
		// quickly.
		for subset != 0 || i == 0 {
			index := subset
			index = index * Bitboard(bishopMagics[from].multiplier)
			index = index >> bishopMagics[from].shift
			moves[from][index] = getBishopMovesFromOccupationSlow(from, subset)

			subset = (subset - bishopMasks[from]) & bishopMasks[from]
			i++
		}
	}

	return moves
}

func (p *Position) Perft(depth uint) uint {
	nodes := uint(0)

	if depth == 0 {
		return 1
	}

	moves := p.MovesLegal()

	for _, move := range moves {
		p.MakeMove(move)
		nodes += p.Perft(depth - 1)
		p.UndoMove(move)
	}

	return nodes
}

func (p *Position) Divide(depth uint) uint {
	nodes := uint(0)

	if depth == 0 {
		return 1
	}

	moves := p.MovesLegal()

	for _, move := range moves {
		p.MakeMove(move)
		curr := p.Perft(depth - 1)
		nodes += curr
		fmt.Printf("%s: %d\n", move.String(), curr)
		p.UndoMove(move)
	}

	fmt.Println("total:", nodes)

	return nodes
}

func init() {
	// Pre-initialised tables of king and knight moves
	kingMoves = initialiseKingMoves()
	knightMoves = initialiseKnightMoves()

	// Pre-initialised tables of rook and bishop masks to find the blockers on the board from a particular square
	rookMasks = initialiseRookMasks()
	bishopMasks = initialiseBishopMasks()

	// Pre-initialised tables of rook and bishop moves from a square given an index calculated using a magic number.
	rookMoves = initialiseRookMoves()
	bishopMoves = initialiseBishopMoves()
}
