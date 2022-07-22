package position

import (
	"fmt"
	"time"
)

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

// MovesLegal generates all legal moves in a position. It does this by generating all pseudolegal moves and then checking
// if it places the king in check.
func (p *Position) MovesLegal() []Move {
	pseudolegalMoves := p.MovesPseudolegal()
	legalMoves := []Move{}

	for _, move := range pseudolegalMoves {
		if p.MakeMove(move) {
			legalMoves = append(legalMoves, move)
			p.UndoMove(move)
		}
	}

	return legalMoves
}

// MovesPseudolegal generates all pseudolegal moves in a position. These are moves that follow all the rules of chess
// with the exception that the king might be in check at the end.
func (p *Position) MovesPseudolegal() []Move {
	pseudolegalMoves := []Move{}

	// Pawn moves need to be handled separately for each side because the logic for generating them depends on the
	// orientation of the board, whereas for the other pieces it doesn't matter.
	if p.SideToMove == White {
		pseudolegalMoves = append(pseudolegalMoves, p.MovesWhitePawns()...)
	} else if p.SideToMove == Black {
		pseudolegalMoves = append(pseudolegalMoves, p.MovesBlackPawns()...)
	}

	// Moves for kings and knights are generated by looking up results in a pre-initialised table.
	pseudolegalMoves = append(pseudolegalMoves, p.MovesKing()...)
	pseudolegalMoves = append(pseudolegalMoves, p.MovesKnights()...)

	// Moves for rooks, bishops and queens are generated using the magic bitboards technique.
	pseudolegalMoves = append(pseudolegalMoves, p.MovesRooks()...)
	pseudolegalMoves = append(pseudolegalMoves, p.MovesBishops()...)
	pseudolegalMoves = append(pseudolegalMoves, p.MovesQueens()...)

	return pseudolegalMoves
}

// MovesWhitePawns generates white pawn moves, including promotions.
func (p *Position) MovesWhitePawns() []Move {
	moves := []Move{}
	whitePawns := p.Pieces[Pawn] & p.Occupied[White]

	oneStep := (whitePawns << DirN) & ^(p.Occupied[White] | p.Occupied[Black])

	// Deal with pieces blocking pawns advancing two steps.
	maskNotBlocked := ^(((p.Occupied[White] | p.Occupied[Black]) & maskRank3) >> DirN)
	twoSteps := ((whitePawns & maskNotBlocked) << (DirN * 2)) & ^(p.Occupied[White] | p.Occupied[Black]) & maskRank4

	capturesLeft := (whitePawns & ^maskFileA) << DirNW & p.Occupied[Black]
	capturesRight := (whitePawns & ^maskFileH) << DirNE & p.Occupied[Black]

	// No need to worry about two step moves leading to a promotion because white pawns can only end up on the 4th rank
	// when going two steps.
	promotions := (oneStep | capturesLeft | capturesRight) & maskRank8

	if promotions != 0 {
		for to := promotions.FirstOn(); to <= promotions.LastOn() && to != 64; to++ {
			if promotions.IsOn(to) {
				var from uint8

				if capturesLeft.IsOn(to) {
					from = to - DirNW
				}

				if capturesRight.IsOn(to) {
					from = to - DirNE
				}

				if oneStep.IsOn(to) {
					from = to - DirN
				}

				moves = append(moves, NewMove(from, to, WhitePawn, p.Squares[to], Queen, p.Castling, p.EnPassant))
				moves = append(moves, NewMove(from, to, WhitePawn, p.Squares[to], Bishop, p.Castling, p.EnPassant))
				moves = append(moves, NewMove(from, to, WhitePawn, p.Squares[to], Knight, p.Castling, p.EnPassant))
				moves = append(moves, NewMove(from, to, WhitePawn, p.Squares[to], Rook, p.Castling, p.EnPassant))
			}
		}
	}

	// Remove moves that would lead to promotion, since these have already been handled.
	oneStep = oneStep & ^maskRank8
	twoSteps = twoSteps & ^maskRank8
	capturesLeft = capturesLeft & ^maskRank8
	capturesRight = capturesRight & ^maskRank8

	var enPassantLeft, enPassantRight Bitboard

	if p.HasEnPassant() {
		enPassant := Bitboard(1) << uint(p.EnPassant) // The bitboard with only the en passant target square highlighted.
		emptySquares := ^(p.Occupied[p.SideToMove] | p.Occupied[p.SideToMove.Invert()])

		enPassantLeft = ((whitePawns & ^maskFileA) << DirNW) & enPassant & emptySquares
		enPassantRight = ((whitePawns & ^maskFileH) << DirNE) & enPassant & emptySquares

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

// MovesBlackPawns generates black pawn moves, including promotions.
func (p *Position) MovesBlackPawns() []Move {
	moves := []Move{}
	blackPawns := p.Pieces[Pawn] & p.Occupied[Black]

	oneStep := (blackPawns >> DirN) & ^(p.Occupied[White] | p.Occupied[Black])

	// Deal with pieces blocking pawns advancing two steps
	maskNotBlocked := ^(((p.Occupied[White] | p.Occupied[Black]) & maskRank6) << DirN)
	twoSteps := ((blackPawns & maskNotBlocked) >> (DirN * 2)) & ^(p.Occupied[White] | p.Occupied[Black]) & maskRank5

	capturesLeft := (blackPawns & ^maskFileA) >> DirNE & p.Occupied[White]
	capturesRight := (blackPawns & ^maskFileH) >> DirNW & p.Occupied[White]

	// No need to worry about two step moves leading to a promotion because black pawns can only end up on the 5th rank
	// when going two steps.
	promotions := (oneStep | capturesLeft | capturesRight) & maskRank1

	if promotions != 0 {
		for to := promotions.FirstOn(); to <= promotions.LastOn() && to != 64; to++ {
			if promotions.IsOn(to) {
				var from uint8

				if capturesLeft.IsOn(to) {
					from = to + DirNE
				}

				if capturesRight.IsOn(to) {
					from = to + DirNW
				}

				if oneStep.IsOn(to) {
					from = to + DirN
				}

				moves = append(moves, NewMove(from, to, BlackPawn, p.Squares[to], Queen, p.Castling, p.EnPassant))
				moves = append(moves, NewMove(from, to, BlackPawn, p.Squares[to], Bishop, p.Castling, p.EnPassant))
				moves = append(moves, NewMove(from, to, BlackPawn, p.Squares[to], Knight, p.Castling, p.EnPassant))
				moves = append(moves, NewMove(from, to, BlackPawn, p.Squares[to], Rook, p.Castling, p.EnPassant))
			}
		}
	}

	// Remove moves that would lead to promotion, since these have already been handled.
	oneStep = oneStep & ^maskRank1
	twoSteps = twoSteps & ^maskRank1
	capturesLeft = capturesLeft & ^maskRank1
	capturesRight = capturesRight & ^maskRank1

	var enPassantLeft, enPassantRight Bitboard

	if p.HasEnPassant() {
		enPassant := Bitboard(1) << uint(p.EnPassant)
		emptySquares := ^(p.Occupied[p.SideToMove] | p.Occupied[p.SideToMove.Invert()])

		enPassantLeft = ((blackPawns & ^maskFileH) >> DirNW) & enPassant & emptySquares
		enPassantRight = ((blackPawns & ^maskFileA) >> DirNE) & enPassant & emptySquares

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

// MovesKing generates the king moves for the current side to move by looking up the current square in a table pre-populated with all the
// king moves from that square.
func (p *Position) MovesKing() []Move {
	currentKingMoves := kingMoves[p.KingLocation[p.SideToMove]] & ^p.Occupied[p.SideToMove]

	moves := p.movesFromBitboard(
		King.OfColor(p.SideToMove),
		func(to uint8) uint8 { return p.KingLocation[p.SideToMove] },
		currentKingMoves,
	)

	moves = append(moves, p.movesCastling()...)

	return moves
}

// movesCastling generates all possible castling moves for the current side to move by checking the stored castling status and determining if
// the king is currently in check or is castling through check.
func (p *Position) movesCastling() []Move {
	moves := []Move{}

	if p.SideToMove == White && !p.KingInCheck(White) {
		// When castling long for white, C1 and D1 can't be occupied or attacked (since the king would move through check), and B1 can't be occupied.
		if p.Castling&longW != 0 &&
			(p.IsEmpty(SquareC1) && !p.IsAttacked(SquareC1, Black)) &&
			(p.IsEmpty(SquareD1) && !p.IsAttacked(SquareD1, Black)) &&
			(p.IsEmpty(SquareB1)) {

			moves = append(
				moves,
				NewMove(SquareE1, SquareC1, WhiteKing, Empty, None, p.Castling, p.EnPassant),
			)
		}

		// When castling short for white, F1 and G1 can't be occupied or attacked.
		if p.Castling&shortW != 0 &&
			(p.IsEmpty(SquareF1) && !p.IsAttacked(SquareF1, Black)) &&
			(p.IsEmpty(SquareG1) && !p.IsAttacked(SquareG1, Black)) {

			moves = append(
				moves,
				NewMove(SquareE1, SquareG1, WhiteKing, Empty, None, p.Castling, p.EnPassant),
			)
		}
	} else if p.SideToMove == Black && !p.KingInCheck(Black) {
		// When castling long for black, C8 and D8 can't be occupied or attacked (since the king would move through check), and B8 can't be occupied.
		if p.Castling&longB != 0 &&
			(p.IsEmpty(SquareC8) && !p.IsAttacked(SquareC8, White)) &&
			(p.IsEmpty(SquareD8) && !p.IsAttacked(SquareD8, White)) &&
			(p.IsEmpty(SquareB8)) {

			moves = append(
				moves,
				NewMove(SquareE8, SquareC8, BlackKing, Empty, None, p.Castling, p.EnPassant),
			)
		}

		// When castling short for black, F8 and G8 can't be occupied or attacked.
		if p.Castling&shortB != 0 &&
			(p.IsEmpty(SquareF8) && !p.IsAttacked(SquareF8, White)) &&
			(p.IsEmpty(SquareG8) && !p.IsAttacked(SquareG8, White)) {

			moves = append(
				moves,
				NewMove(SquareE8, SquareG8, BlackKing, Empty, None, p.Castling, p.EnPassant),
			)
		}
	}

	return moves
}

// MovesKnights generates the knight moves for the current side to move by looking up the current square in a table pre-populated with all the
// knight moves from that square.
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

// MovesRooks generates the rook moves for the current side to move using the magic bitboards algorithm.
//
// In summary, this uses the magic numbers in the magic.go file to quickly look up the available rook moves in a pre-populated table.
func (p *Position) MovesRooks() []Move {
	moves := []Move{}
	rookLocations := p.Pieces[Rook] & p.Occupied[p.SideToMove]

	for from := rookLocations.FirstOn(); from < 64 && from <= rookLocations.LastOn(); from++ {
		if rookLocations.IsOn(from) {
			blockers := (p.Occupied[p.SideToMove.Invert()] | p.Occupied[p.SideToMove]) & rookMasks[from]
			key := (uint64(blockers) * rookMagics[from].multiplier) >> rookMagics[from].shift

			available := rookMoves[from][key] & ^p.Occupied[p.SideToMove]
			moves = append(
				moves,
				p.movesFromBitboard(
					Rook.OfColor(p.SideToMove),
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

// MovesBishops generates the list of rook moves for the current side to move using the magic bitboards algorithm.
//
// In summary, this uses the magic numbers in the magic.go file to quickly look up the available rook moves in a pre-populated table.
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

// MovesQueens generates the list of queen moves for the current side to move by looking up the moves for a rook and a bishop from that square.
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

// initialiseKnightMoves is executed at the start of the program and sets up a table of all the possible knight moves from
// any of the 64 squares, assuming that the other squares are not occupied.
//
// The magic bitboards technique doesn't need to be used for knight moves since they can't be blocked by anything.
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

// initialiseRookMasks generates bitboards for masks that can be used to find all the blockers in a rook's attack path.
// This uses quite a slow method of calculating the masks but this is only run once at the beginning of the program and
// then the results are stored to be reused.
//
// For example, for the square B4:
//
//   0 0 0 0 0 0 0 0
//   0 1 0 0 0 0 0 0
//   0 1 0 0 0 0 0 0
//   0 1 0 0 0 0 0 0
//   0 0 1 1 1 1 1 0
//   0 1 0 0 0 0 0 0
//   0 1 0 0 0 0 0 0
//   0 0 0 0 0 0 0 0
//
// Or the square D5:
//
//   0 0 0 0 0 0 0 0
//   0 0 0 1 0 0 0 0
//   0 0 0 1 0 0 0 0
//   0 1 1 0 1 1 1 0
//   0 0 0 1 0 0 0 0
//   0 0 0 1 0 0 0 0
//   0 0 0 1 0 0 0 0
//   0 0 0 0 0 0 0 0
//
// The bits on the perimeter don't matter because regardless of what's there, it doesn't have any effect on the squares the
// bishop can attack. If there is no piece there, then the attack ends there because the end of the board is reached, and
// if there is a piece there, it will still be able to be attacked and it doesn't block any squares.
//
// This means that 4 bits can be saved for the key to the actual bishop attacks calculated.
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
		bitboard.Off(limitBottom)
		bitboard.Off(limitTop)
		bitboard.Off(limitRight)
		bitboard.Off(limitLeft)
		bitboard.Off(from)

		moves[from] = bitboard
	}

	return moves
}

// getRookMovesFromOccupationSlow returns the rook moves available from a given square with a bitboard representing the blockers
// in its path. It is slow compared to the magic bitboards approach that is used during actual move generation, but is needed because it initialises
// the magic bitboard table.
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

	for i := from; i >= limitBottom; i -= 8 {
		bitboard.On(i)

		if occupiedSquares.IsOn(i) {
			break
		}

		if i < 8 {
			break
		}
	}

	for i := from; i <= limitRight; i++ {
		bitboard.On(i)

		if occupiedSquares.IsOn(i) {
			break
		}
	}

	for i := from; i >= limitLeft; i-- {
		bitboard.On(i)

		if occupiedSquares.IsOn(i) {
			break
		}

		if i < 1 {
			break
		}
	}

	bitboard.Off(from)

	return bitboard
}

// initialiseRookMoves generates the rook moves table that is indexed by the calculated key from the blockers.
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

// initialiseBishopMasks generates bitboards for masks that can be used to find all the blockers in a bishop's attack path.
// This uses quite a slow method of adding or subtracting 7 and 9 until it reaches the end of the board -- not the fastest
// approach but this is only run once at the beginning of the program and then the results are stored to be reused.
//
// For example, for the square B4:
//
//   0 0 0 0 0 0 0 0
//   0 0 0 0 1 0 0 0
//   0 0 0 1 0 0 0 0
//   0 0 1 0 0 0 0 0
//   0 0 0 0 0 0 0 0
//   0 0 1 0 0 0 0 0
//   0 0 0 1 0 0 0 0
//   0 0 0 0 0 0 0 0
//
// Or the square D5:
//
//   0 0 0 0 0 0 0 0
//   0 1 0 0 0 1 0 0
//   0 0 1 0 1 0 0 0
//   0 0 0 0 0 0 0 0
//   0 0 1 0 1 0 0 0
//   0 1 0 0 0 1 0 0
//   0 0 0 0 0 0 1 0
//   0 0 0 0 0 0 0 0
//
// The bits on the perimeter don't matter because regardless of what's there, it doesn't have any effect on the squares the
// bishop can attack. If there is no piece there, then the attack ends there because the end of the board is reached, and
// if there is a piece there, it will still be able to be attacked and it doesn't block any squares.
//
// This means that 4 bits can be saved for the key to the actual bishop attacks calculated.
func initialiseBishopMasks() [64]Bitboard {
	moves := [64]Bitboard{}

	for from := uint8(0); from < 64; from++ {
		bitboard := Bitboard(0)

		// . . *
		// . B .
		// . . .
		for i := from; i < 64; i += 9 {
			if i%8 <= from%8 && i != from {
				break
			}

			bitboard.On(i)
		}

		// The condition on this for loop would be i >= 0, but since it's a uint this will always be true.
		// Instead the i < 9 checks if the subtraction will overflow and breaks inside the for-loop.
		// . . .
		// . B .
		// . . *
		for i := from; ; i -= 9 {
			if i%8 >= from%8 && i != from {
				break
			}

			bitboard.On(i)

			if i < 9 {
				break
			}
		}

		// * . .
		// . B .
		// . . .
		for i := from; i < 64; i += 7 {
			if i%8 >= from%8 && i != from {
				break
			}

			bitboard.On(i)
		}

		// The condition on this for loop would be i >= 0, but since it's a uint this will always be true.
		// Instead the i < 7 checks if the subtraction will overflow and breaks inside the for-loop.
		// . . .
		// . B .
		// * . .
		for i := from; ; i -= 7 {
			if i%8 <= from%8 && i != from {
				break
			}

			bitboard.On(i)

			if i < 7 {
				break
			}
		}

		bitboard.Off(from)
		bitboard &= maskNonPerimeterSquares

		moves[from] = bitboard
	}

	return moves
}

// getBishopMovesFromOccupationSlow returns the bishop moves available from a given square with a bitboard representing the blockers
// in its path. It is slow compared to the magic bitboards approach that is used during actual move generation, but is needed because it initialises
// the magic bitboard table.
func getBishopMovesFromOccupationSlow(from uint8, occupiedSquares Bitboard) Bitboard {
	bitboard := Bitboard(0)

	// . . *
	// . B .
	// . . .
	for i := from; i < 64; i += 9 {
		if i%8 <= from%8 && i != from {
			break
		}

		bitboard.On(i)

		if occupiedSquares.IsOn(i) {
			break
		}
	}

	// The condition on this for loop would be i >= 0, but since it's a uint this will always be true.
	// Instead the i < 9 checks if the subtraction will overflow and breaks inside the for-loop.
	// . . .
	// . B .
	// . . *
	for i := from; ; i -= 9 {
		if i%8 >= from%8 && i != from {
			break
		}

		bitboard.On(i)

		if i < 9 || occupiedSquares.IsOn(i) {
			break
		}
	}

	// * . .
	// . B .
	// . . .
	for i := from; i < 64; i += 7 {
		if i%8 >= from%8 && i != from {
			break
		}

		bitboard.On(i)

		if occupiedSquares.IsOn(i) {
			break
		}
	}

	// The condition on this for loop would be i >= 0, but since it's a uint this will always be true.
	// Instead the i < 7 checks if the subtraction will overflow and breaks inside the for-loop.
	// . . .
	// . B .
	// * . .
	for i := from; ; i -= 7 {
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

// Perft returns the number of possible games after a certain number of moves in the current position.
func (p *Position) Perft(depth uint) uint {
	pseudolegalMoves := p.MovesPseudolegal()
	nodes := uint(0)

	if depth == 0 {
		return 1
	}

	for _, move := range pseudolegalMoves {
		if p.MakeMove(move) {
			nodes += p.Perft(depth - 1)
			p.UndoMove(move)
		}
	}

	return nodes
}

// Divide returns the number of possible games after a certain number of moves in the current position, broken down by the initial move made in
// that position.
// For example, in the starting position, Divide(5) outputs
//
//   a2a3: 181046
//   b2b3: 215255
//   c2c3: 222861
//   d2d3: 328511
//   e2e3: 402988
//   f2f3: 178889
//   g2g3: 217210
//   h2h3: 181044
//   a2a4: 217832
//   b2b4: 216145
//   c2c4: 240082
//   d2d4: 361790
//   e2e4: 405385
//   f2f4: 198473
//   g2g4: 214048
//   h2h4: 218829
//   b1a3: 198572
//   b1c3: 234656
//   g1f3: 233491
//   g1h3: 198502
//
//   total: 4865609
//   speed: 1189.81kn/s
//
func (p *Position) Divide(depth uint) uint {
	pseudolegalMoves := p.MovesPseudolegal()
	nodes := uint(0)

	if depth == 0 {
		return 1
	}

	start := time.Now()

	for _, move := range pseudolegalMoves {
		if p.MakeMove(move) {
			curr := p.Perft(depth - 1)
			nodes += curr
			fmt.Printf("%s: %d\n", move.String(), curr)
			p.UndoMove(move)
		}
	}

	duration := time.Since(start)

	fmt.Println("total:", nodes)
	fmt.Printf("speed: %.2fkn/s\n", float64(nodes/1000)/duration.Seconds())

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
