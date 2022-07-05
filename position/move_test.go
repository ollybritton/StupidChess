package position

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestParseMoveValid tests that moves can correctly be parsed.
func TestParseMoveValid(t *testing.T) {
	tests := []struct {
		in                string
		expectedFrom      uint8
		expectedTo        uint8
		expectedPromotion Piece
	}{
		{"e2e4", SquareE2, SquareE4, None},
		{"a7a5", SquareA7, SquareA5, None},
		{"e1g1", SquareE1, SquareG1, None},
		{"e7e8q", SquareE7, SquareE8, Queen},
		{"b2b1r", SquareB2, SquareB1, Rook},
	}

	for _, test := range tests {
		move, err := ParseMove(test.in)

		assert.NoError(t, err)
		assert.Equal(t, test.expectedFrom, move.From)
		assert.Equal(t, test.expectedTo, move.To)
		assert.Equal(t, test.expectedPromotion, move.Promotion)
	}
}

// TestParseMoveInvalid tests that an invalid move will cause an error.
func TestParseMoveInvalid(t *testing.T) {
	tests := []string{
		// "beans",
		"",
		// "e9b1p",
	}

	for _, test := range tests {
		_, err := ParseMove(test)
		assert.Error(t, err, "expected invalid move for %s", test)
	}
}
