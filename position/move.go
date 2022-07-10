package position

import "fmt"

// Move represents a chess move.
// TODO: switch to a more compact binary format.
type Move struct {
	From uint8
	To   uint8

	Moved     ColoredPiece
	Captured  ColoredPiece
	Promotion Piece

	PriorCastling        CastlingAvailability
	PriorEnPassantTarget uint8
}

// NewMove returns a new move from a from square to a to square.
func NewMove(from, to uint8, moved, captured ColoredPiece, promotion Piece, priorCastling CastlingAvailability, priorEnPassant uint8) Move {
	return Move{
		From:      from,
		To:        to,
		Moved:     moved,
		Captured:  captured,
		Promotion: promotion,

		PriorCastling:        priorCastling,
		PriorEnPassantTarget: priorEnPassant,
	}
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
		return Move{}, fmt.Errorf("invalid move")

	}

	return Move{
		From:      StringToSquare(fromString),
		To:        StringToSquare(toString),
		Promotion: promotion,
	}, nil
}

// String returns the long algebraic notation representation of the move.
func (m Move) String() string {
	return SquareToString(m.From) + SquareToString(m.To)
}
