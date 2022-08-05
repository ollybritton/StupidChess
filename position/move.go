package position

import (
	"fmt"
	"strings"
)

// The binary format used for moves is taken from the GoBit engine:
// https://github.com/carokanns/GoBit
const (
	maskFrom      = 0x0000003f // 0000 0000  0000 0000  0000 0000  0011 1111
	maskTo        = 0x00000fd0 // 0000 0000  0000 0000  0000 1111  1100 0000
	maskMoved     = 0x0000f000 // 0000 0000  0000 0000  1111 0000  0000 0000
	maskCaptured  = 0x000f0000 // 0000 0000  0000 1111  0000 0000  0000 0000
	maskPromotion = 0x00f00000 // 0000 0000  1111 0000  0000 0000  0000 0000
	maskEnPassant = 0x0f000000 // 0000 1111  0000 0000  0000 0000  0000 0000
	maskCastling  = 0xf0000000 // 1111 0000  0000 0000  0000 0000  0000 0000

	shiftFrom      = 0
	shiftTo        = 6
	shiftMoved     = 12 // 6 + 6
	shiftCaptured  = 16 // 6 + 6 + 4
	shiftPromotion = 20 // 6 + 6 + 4 + 4
	shiftEnPassant = 24 // 6 + 6 + 4 + 4 + 4
	shiftCastling  = 28 // 6 + 6 + 4 + 4 + 4 + 4
)

// Move represents a chess move.
type Move uint64

// NewMove returns a new move from a from square to a to square.
func NewMove(from, to uint8,
	moved, captured ColoredPiece,
	promotion Piece,
	priorCastling CastlingAvailability,
	priorEnPassant uint8,
) Move {
	enPassantFile := uint8(0)
	if priorEnPassant != NoEnPassant {
		enPassantFile = (priorEnPassant % 8) + 1
	}

	return Move(
		(uint64(from) << shiftFrom) |
			(uint64(to) << shiftTo) |
			(uint64(moved) << shiftMoved) |
			(uint64(captured) << shiftCaptured) |
			(uint64(promotion) << shiftPromotion) |
			(uint64(enPassantFile) << shiftEnPassant) |
			(uint64(priorCastling) << shiftCastling))
}

// ParseMove parses a UCI-style long algebraic notation move into a Move.
func ParseMove(str string) (Move, error) {
	var fromString, toString string
	var promotion Piece = None

	switch len(str) {
	case 4:
		fromString = str[0:2]
		toString = str[2:4]

	case 5:
		fromString = str[0:2]
		toString = str[2:4]
		promotionString := string(str[4])
		promotion = strToPiece(promotionString)

	default:
		return Move(0), fmt.Errorf("invalid move")

	}

	return NewMove(
		StringToSquare(fromString),
		StringToSquare(toString),
		Empty,
		Empty,
		promotion,
		CastlingAvailability(0),
		NoEnPassant,
	), nil
}

// From returns the from square in the move.
func (m Move) From() uint8 {
	return uint8(uint64(m&maskFrom) >> shiftFrom)
}

// To returns the to square in the move.
func (m Move) To() uint8 {
	return uint8(uint64(m&maskTo) >> shiftTo)
}

// Moved returns the moved piece in the move.
func (m Move) Moved() ColoredPiece {
	return ColoredPiece(uint64(m&maskMoved) >> shiftMoved)
}

// Captured returns the captured piece in the move.
func (m Move) Captured() ColoredPiece {
	return ColoredPiece(uint64(m&maskCaptured) >> shiftCaptured)
}

// Promotion returns the promoted piece if there is one.
func (m Move) Promotion() Piece {
	return Piece(uint64(m&maskPromotion) >> shiftPromotion)
}

// PriorCastling returns the castling status prior to the move being completed.
func (m Move) PriorCastling() CastlingAvailability {
	return CastlingAvailability((m & maskCastling) >> shiftCastling)
}

// PriorEnPassantTarget returns the en passant target for the move.
func (m Move) PriorEnPassantTarget() uint8 {
	enPassantFile := uint64(m&maskEnPassant) >> shiftEnPassant

	if enPassantFile == 0 {
		return NoEnPassant
	}

	if m.Moved().Color() == White {
		return uint8(enPassantFile+5*8) - 1
	} else {
		return uint8(enPassantFile+2*8) - 1
	}
}

// String returns the long algebraic notation representation of the move.
func (m Move) String() string {
	if m.Promotion() == None {
		return SquareToString(m.From()) + SquareToString(m.To())
	} else {
		return SquareToString(m.From()) + SquareToString(m.To()) + strings.ToLower(m.Promotion().String())
	}
}

// FullString returns a string containing all information about the move, e.g. the captured piece, the prior en passant target, etc.
func (m Move) FullString() string {
	return fmt.Sprintf(
		"%s (from=%s to=%s promotion=%s moved-piece=%s captured-piece=%s castling=%s en-passant=%d)",
		m.String(),
		SquareToString(m.From()),
		SquareToString(m.To()),
		m.Promotion().String(),
		m.Moved().String(),
		m.Captured().String(),
		m.PriorCastling().String(),
		m.PriorEnPassantTarget(),
	)
}

// MoveList represents a list of moves that can be sorted and filtered.
type MoveList struct {
	moves []Move
}

// NewMoveList returns a new list of moves of a given size.
func NewMoveList(size uint) *MoveList {
	return &MoveList{moves: make([]Move, 0, size)}
}

// Append add a move to the move list.
func (l *MoveList) Append(move Move) {
	l.moves = append(l.moves, move)
}

// AppendMany adds multiple moves to the moves list.
func (l *MoveList) AppendMany(moves []Move) {
	l.moves = append(l.moves, moves...)
}

// AppendFromBitboard
func (l *MoveList) AppendFromBitboard(piece ColoredPiece,
	fromFunc func(uint8) uint8,
	bitboard Bitboard,
	squares []ColoredPiece,
	castling CastlingAvailability,
	enPassantTarget uint8,
) {
	for to := bitboard.FirstOn(); to <= bitboard.LastOn() && to != 64; to++ {
		if bitboard.IsOn(to) {
			l.moves = append(l.moves, (NewMove(fromFunc(to), to, piece, squares[to], None, castling, enPassantTarget)))
		}
	}
}

// AsSlice returns the moves in the move list as a slice.
func (l *MoveList) AsSlice() []Move {
	return l.moves
}

// Copy returns a copied version of the move list.
func (l *MoveList) Copy() *MoveList {
	newMoves := []Move{}

	for _, move := range l.moves {
		newMoves = append(newMoves, move)
	}

	return &MoveList{newMoves}
}

// Length returns the number of moves in the move list.
func (l *MoveList) Length() int {
	return len(l.moves)
}

// Filter removes moves in the move list according to a function that evaluates a move and says whether it is allowed in the
// list or not.
func (l *MoveList) Filter(allowedFunc func(Move) bool) {
	out := make([]Move, 0, len(l.moves))

	for _, move := range l.moves {
		if allowedFunc(move) {
			out = append(out, move)
		}
	}

	l.moves = out
}
