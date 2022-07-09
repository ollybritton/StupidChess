package position

import (
	"bytes"
	"fmt"
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
