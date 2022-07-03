package board

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestInversion(t *testing.T) {
	assert.Equal(t, "White", Black.Invert().String())
}

func TestPieceFromString(t *testing.T) {
	tests := []struct {
		in       string
		expected ColoredPiece
	}{
		{"P", WhitePawn},
		{"p", BlackPawn},
		{"N", WhiteKnight},
		{"n", BlackKnight},
		{"B", WhiteBishop},
		{"b", BlackBishop},
		{"R", WhiteRook},
		{"r", BlackRook},
		{"Q", WhiteQueen},
		{"q", BlackQueen},
		{"K", WhiteKing},
		{"k", BlackKing},
		{"?", Empty},
	}

	for _, test := range tests {
		assert.Equal(t, test.expected, strToColored(test.in), "wanted %q, got %q", test.expected.String(), strToColored(test.in).String())
	}
}
