package position

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBitboardFirstOn(t *testing.T) {
	bitboard := Bitboard(0b00100000_00001000_00000000_00000000_00000000_00010000_01000000_00000000)
	assert.Equal(t, uint8(14), bitboard.FirstOn())

	bitboard = Bitboard(0b0)
	assert.Equal(t, uint8(64), bitboard.FirstOn())
}

func TestBitboardLastOn(t *testing.T) {
	bitboard := Bitboard(0b00100000_00001000_00000000_00000000_00000000_00010000_01000000_00000000)
	assert.Equal(t, uint8(61), bitboard.LastOn())

	bitboard = Bitboard(0b0)
	assert.Equal(t, uint8(64), bitboard.LastOn())
}

func TestBitboardIsOn(t *testing.T) {
	bitboard := Bitboard(0b00100000_00001000_00000000_00000000_00000000_00010000_01000000_00000000)
	assert.Equal(t, true, bitboard.IsOn(14))
	assert.Equal(t, false, bitboard.IsOn(0))
}
