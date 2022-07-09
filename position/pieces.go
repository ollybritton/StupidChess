package position

import "strings"

// Color means white or black. In this representation, white is zero and black is one.
type Color uint8

const (
	White Color = iota
	Black
)

func (c Color) Invert() Color {
	if c == White {
		return Black
	}

	return White
}

func (c Color) String() string {
	if c == 0 {
		return "White"
	}

	return "Black"
}

// ColoredPiece represents a piece (including pawns) with an associated color.
// White pieces are even, and black pieces are odd.
type ColoredPiece uint8

const (
	WhitePawn ColoredPiece = iota
	BlackPawn

	WhiteKnight
	BlackKnight

	WhiteBishop
	BlackBishop

	WhiteRook
	BlackRook

	WhiteQueen
	BlackQueen

	WhiteKing
	BlackKing
	Empty
)

func (c ColoredPiece) String() string {
	return string("PpNnBbRrQqKk?"[c])
}

func (c ColoredPiece) Color() Color {
	return Color(c & 0b00000001)
}

func (c ColoredPiece) Colorless() Piece {
	switch c {
	case WhitePawn, BlackPawn:
		return Pawn
	case WhiteKnight, BlackKnight:
		return Knight
	case WhiteBishop, BlackBishop:
		return Bishop
	case WhiteRook, BlackRook:
		return Rook
	case WhiteQueen, BlackQueen:
		return Queen
	case WhiteKing, BlackKing:
		return King
	}

	return None
}

func strToColored(str string) ColoredPiece {
	return ColoredPiece(strings.IndexAny("PpNnBbRrQqKk?", str))
}

// Piece represents a piece (including pawns) of any color.
type Piece uint8

const (
	Pawn Piece = iota
	Knight
	Bishop
	Rook
	Queen
	King

	None
)

func (c Piece) String() string {
	return string("PNBRQK?"[c])
}

// OfColor returns the colored version of the piece.
// E.g. Pawn -> WhitePawn
//      King -> BlackKing
func (c Piece) OfColor(color Color) ColoredPiece {
	if color == White {
		switch c {
		case Pawn:
			return WhitePawn
		case Knight:
			return WhiteKnight
		case Bishop:
			return WhiteBishop
		case Rook:
			return WhiteRook
		case Queen:
			return WhiteQueen
		case King:
			return WhiteKing
		}
	} else {
		switch c {
		case Pawn:
			return BlackPawn
		case Knight:
			return BlackKnight
		case Bishop:
			return BlackBishop
		case Rook:
			return BlackRook
		case Queen:
			return BlackQueen
		case King:
			return BlackKing
		}
	}

	return Empty
}

// strToPiece returns the associated Piece given a string
func strToPiece(str string) Piece {
	str = strings.ToLower(str)
	return Piece(strings.IndexAny("pnbrqk?", str))
}
