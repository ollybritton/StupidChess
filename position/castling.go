package position

// CastlingAvailability stores information about whether either player can castle in the current position.
//   0b0000XYZW
// X is black castling long.
// Y is black castling short.
// Z is white castling long.
// W is white castling short.
type CastlingAvailability uint8

const (
	shortW = CastlingAvailability(0x1)
	longW  = CastlingAvailability(0x2)
	shortB = CastlingAvailability(0x4)
	longB  = CastlingAvailability(0x8)
)

// String returns the FEN-string for castling availability.
// E.g. "KQkq" -> Both sides can castle long or short.
// 		"Kq" -> White can castle short, black can castle long.
func (c CastlingAvailability) String() string {
	var out string

	if c&shortW != 0 {
		out += "K"
	}

	if c&longW != 0 {
		out += "Q"
	}

	if c&shortB != 0 {
		out += "k"
	}

	if c&longB != 0 {
		out += "q"
	}

	if out == "" {
		out = "-"
	}

	return out
}

// off disables a type of castling.
func (c *CastlingAvailability) off(castlingType CastlingAvailability) {
	(*c) &= ^castlingType
}

// castlingAvailabilityFromString returns a CastlingAvailability from a FEN-formatted castling string.
func castlingAvailabilityFromString(str string) CastlingAvailability {
	var out CastlingAvailability

	for _, char := range str {
		switch char {
		case 'K':
			out |= shortW
		case 'Q':
			out |= longW
		case 'k':
			out |= shortB
		case 'q':
			out |= longB
		}
	}

	return out
}
