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
)

func (c Piece) String() string {
	return string("PNBRQK?"[c])
}
