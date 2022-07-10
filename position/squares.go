package position

// SquareToString converts a position on the chess board into its corresponding algebraic chess position.
// E.g. SquareE4 -> "e4"
// The function for the opposite is StringToSquare.
func SquareToString(square uint8) string {
	if square > 63 {
		return "?"
	}

	return squareStringMap[square]
}

// StringToSquare converts an algebraic chess position into its corresponding numerical position on the chess board.
// E.g. "e4" -> SquareE4
// The function for the opposite is SquareToString.
func StringToSquare(str string) uint8 {
	return stringSquareMap[str]
}

const (
	maskRank1 = Bitboard(0x00000000000000FF)
	maskRank2 = Bitboard(0x000000000000FF00)
	maskRank3 = Bitboard(0x0000000000FF0000)
	maskRank4 = Bitboard(0x00000000FF000000)
	maskRank5 = Bitboard(0x000000FF00000000)
	maskRank6 = Bitboard(0x0000FF0000000000)
	maskRank7 = Bitboard(0x00FF000000000000)
	maskRank8 = Bitboard(0xFF00000000000000)
	maskFileA = Bitboard(0x0101010101010101)
	maskFileB = Bitboard(0x0202020202020202)
	maskFileG = Bitboard(0x4040404040404040)
	maskFileH = Bitboard(0x8080808080808080)
)

// Square names as variables in the program.
const (
	SquareA1 uint8 = iota
	SquareB1
	SquareC1
	SquareD1
	SquareE1
	SquareF1
	SquareG1
	SquareH1

	SquareA2
	SquareB2
	SquareC2
	SquareD2
	SquareE2
	SquareF2
	SquareG2
	SquareH2

	SquareA3
	SquareB3
	SquareC3
	SquareD3
	SquareE3
	SquareF3
	SquareG3
	SquareH3

	SquareA4
	SquareB4
	SquareC4
	SquareD4
	SquareE4
	SquareF4
	SquareG4
	SquareH4

	SquareA5
	SquareB5
	SquareC5
	SquareD5
	SquareE5
	SquareF5
	SquareG5
	SquareH5

	SquareA6
	SquareB6
	SquareC6
	SquareD6
	SquareE6
	SquareF6
	SquareG6
	SquareH6

	SquareA7
	SquareB7
	SquareC7
	SquareD7
	SquareE7
	SquareF7
	SquareG7
	SquareH7

	SquareA8
	SquareB8
	SquareC8
	SquareD8
	SquareE8
	SquareF8
	SquareG8
	SquareH8
)

var squareStringMap map[uint8]string = map[uint8]string{
	SquareA1: "a1",
	SquareB1: "b1",
	SquareC1: "c1",
	SquareD1: "d1",
	SquareE1: "e1",
	SquareF1: "f1",
	SquareG1: "g1",
	SquareH1: "h1",

	SquareA2: "a2",
	SquareB2: "b2",
	SquareC2: "c2",
	SquareD2: "d2",
	SquareE2: "e2",
	SquareF2: "f2",
	SquareG2: "g2",
	SquareH2: "h2",

	SquareA3: "a3",
	SquareB3: "b3",
	SquareC3: "c3",
	SquareD3: "d3",
	SquareE3: "e3",
	SquareF3: "f3",
	SquareG3: "g3",
	SquareH3: "h3",

	SquareA4: "a4",
	SquareB4: "b4",
	SquareC4: "c4",
	SquareD4: "d4",
	SquareE4: "e4",
	SquareF4: "f4",
	SquareG4: "g4",
	SquareH4: "h4",

	SquareA5: "a5",
	SquareB5: "b5",
	SquareC5: "c5",
	SquareD5: "d5",
	SquareE5: "e5",
	SquareF5: "f5",
	SquareG5: "g5",
	SquareH5: "h5",

	SquareA6: "a6",
	SquareB6: "b6",
	SquareC6: "c6",
	SquareD6: "d6",
	SquareE6: "e6",
	SquareF6: "f6",
	SquareG6: "g6",
	SquareH6: "h6",

	SquareA7: "a7",
	SquareB7: "b7",
	SquareC7: "c7",
	SquareD7: "d7",
	SquareE7: "e7",
	SquareF7: "f7",
	SquareG7: "g7",
	SquareH7: "h7",

	SquareA8: "a8",
	SquareB8: "b8",
	SquareC8: "c8",
	SquareD8: "d8",
	SquareE8: "e8",
	SquareF8: "f8",
	SquareG8: "g8",
	SquareH8: "h8",
}

var stringSquareMap map[string]uint8 = map[string]uint8{
	"a1": SquareA1,
	"b1": SquareB1,
	"c1": SquareC1,
	"d1": SquareD1,
	"e1": SquareE1,
	"f1": SquareF1,
	"g1": SquareG1,
	"h1": SquareH1,

	"a2": SquareA2,
	"b2": SquareB2,
	"c2": SquareC2,
	"d2": SquareD2,
	"e2": SquareE2,
	"f2": SquareF2,
	"g2": SquareG2,
	"h2": SquareH2,

	"a3": SquareA3,
	"b3": SquareB3,
	"c3": SquareC3,
	"d3": SquareD3,
	"e3": SquareE3,
	"f3": SquareF3,
	"g3": SquareG3,
	"h3": SquareH3,

	"a4": SquareA4,
	"b4": SquareB4,
	"c4": SquareC4,
	"d4": SquareD4,
	"e4": SquareE4,
	"f4": SquareF4,
	"g4": SquareG4,
	"h4": SquareH4,

	"a5": SquareA5,
	"b5": SquareB5,
	"c5": SquareC5,
	"d5": SquareD5,
	"e5": SquareE5,
	"f5": SquareF5,
	"g5": SquareG5,
	"h5": SquareH5,

	"a6": SquareA6,
	"b6": SquareB6,
	"c6": SquareC6,
	"d6": SquareD6,
	"e6": SquareE6,
	"f6": SquareF6,
	"g6": SquareG6,
	"h6": SquareH6,

	"a7": SquareA7,
	"b7": SquareB7,
	"c7": SquareC7,
	"d7": SquareD7,
	"e7": SquareE7,
	"f7": SquareF7,
	"g7": SquareG7,
	"h7": SquareH7,

	"a8": SquareA8,
	"b8": SquareB8,
	"c8": SquareC8,
	"d8": SquareD8,
	"e8": SquareE8,
	"f8": SquareF8,
	"g8": SquareG8,
	"h8": SquareH8,
}
