package position

import (
	"bytes"
	"fmt"
	"math/bits"
	"strconv"
	"unicode/utf8"
)

type Bitboard uint64

// On sets a specific bit on a bitboard on, i.e. to a 1.
func (b *Bitboard) On(pos uint8) {
	*b |= Bitboard(uint64(1) << pos)
}

// Of sets a specific bit on a bitboard off, i.e. to a 0.
func (b *Bitboard) Off(pos uint8) {
	*b &= Bitboard(^(uint64(1) << pos))
}

// IsOn returns true if that specific bit is on.
func (b *Bitboard) IsOn(pos uint8) bool {
	val := uint64(*b) & (1 << uint64(pos))
	return val > 0
}

// String returns a nice string representation of the bitboard.
func (b *Bitboard) String() string {
	var out bytes.Buffer
	unseperated := fmt.Sprintf("%064s", strconv.FormatUint(uint64(*b), 2))

	for i := 0; i < 8; i++ {
		out.WriteString(reverse(unseperated[i*8 : (i+1)*8]))

		if i != 7 {
			out.WriteString("\n")
		}
	}

	return out.String()
}

// FirstOn returns the index (starting from the least significant bit) of the first bit that is on in the bitboard.
//
func (b *Bitboard) FirstOn() uint8 {
	return uint8(bits.TrailingZeros64(uint64(*b)))
}

// LastOn returns the index (starting from the least significant bit) of the last bit that is on in the bitboard.
// Equals 64 when there are no 1s.
func (b *Bitboard) LastOn() uint8 {
	result := 63 - bits.LeadingZeros64(uint64(*b))

	if result == -1 {
		return uint8(64)
	} else {
		return uint8(result)
	}
}

func reverse(s string) string {
	size := len(s)
	buf := make([]byte, size)
	for start := 0; start < size; {
		r, n := utf8.DecodeRuneInString(s[start:])
		start += n
		utf8.EncodeRune(buf[size-start:], r)
	}
	return string(buf)
}
