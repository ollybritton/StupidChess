package position

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestValidFENInvariant ensures that loading a chess game from a FEN string and then outputting the game as a FEN string is the same.
// The FEN strings used in this test come from https://gist.github.com/peterellisjones/8c46c28141c162d1d8a0f0badbc9cff9
func TestValidFENInvariant(t *testing.T) {
	tests := []string{
		"r6r/1b2k1bq/8/8/7B/8/8/R3K2R b KQ - 3 2",
		"8/8/8/2k5/2pP4/8/B7/4K3 b - d3 0 3",
		"r1bqkbnr/pppppppp/n7/8/8/P7/1PPPPPPP/RNBQKBNR w KQkq - 2 2",
		"r3k2r/p1pp1pb1/bn2Qnp1/2qPN3/1p2P3/2N5/PPPBBPPP/R3K2R b KQkq - 3 2",
		"2kr3r/p1ppqpb1/bn2Qnp1/3PN3/1p2P3/2N5/PPPBBPPP/R3K2R b KQ - 3 2",
		"rnb2k1r/pp1Pbppp/2p5/q7/2B5/8/PPPQNnPP/RNB1K2R w KQ - 3 9",
		"2r5/3pk3/8/2P5/8/2K5/8/8 w - - 5 4",
		"rnbq1k1r/pp1Pbppp/2p5/8/2B5/8/PPP1NnPP/RNBQK2R w KQ - 1 8",
		"r4rk1/1pp1qppp/p1np1n2/2b1p1B1/2B1P1b1/P1NP1N2/1PP1QPPP/R4RK1 w - - 0 10",
		"3k4/3p4/8/K1P4r/8/8/8/8 b - - 0 1",
		"8/8/4k3/8/2p5/8/B2P2K1/8 w - - 0 1",
		"8/8/1k6/2b5/2pP4/8/5K2/8 b - d3 0 1",
		"5k2/8/8/8/8/8/8/4K2R w K - 0 1",
		"3k4/8/8/8/8/8/8/R3K3 w Q - 0 1",
		"r3k2r/1b4bq/8/8/8/8/7B/R3K2R w KQkq - 0 1",
		"r3k2r/8/3Q4/8/8/5q2/8/R3K2R b KQkq - 0 1",
		"2K2r2/4P3/8/8/8/8/8/3k4 w - - 0 1",
		"8/8/1P2K3/8/2n5/1q6/8/5k2 b - - 0 1",
		"4k3/1P6/8/8/8/8/K7/8 w - - 0 1",
		"8/P1k5/K7/8/8/8/8/8 w - - 0 1",
		"K1k5/8/P7/8/8/8/8/8 w - - 0 1",
		"8/k1P5/8/1K6/8/8/8/8 w - - 0 1",
		"8/8/2k5/5q2/5n2/8/5K2/8 b - - 0 1",
		"rnbqkb1r/pp2pppp/5n2/2pp4/3P1B2/3BP3/PPP2PPP/RN1QK1NR b KQkq - 1 4",
	}

	for _, test := range tests {
		b, err := NewPositionFromFEN(test)
		assert.NoError(t, err)
		assert.Equal(t, test, b.StringFEN(), "expecting identical FEN strings")
	}
}

// TestFENFull checks that parsing a FEN string gives the correct full chessboard, in order.
func TestFENFull(t *testing.T) {
	input := "r6r/1b2k1bq/8/8/7B/8/8/R3K2R b KQ - 3 2"

	// This looks backwards but the array starts at A1.
	output := [64]ColoredPiece{
		WhiteRook, Empty, Empty, Empty, WhiteKing, Empty, Empty, WhiteRook,
		Empty, Empty, Empty, Empty, Empty, Empty, Empty, Empty,
		Empty, Empty, Empty, Empty, Empty, Empty, Empty, Empty,
		Empty, Empty, Empty, Empty, Empty, Empty, Empty, WhiteBishop,
		Empty, Empty, Empty, Empty, Empty, Empty, Empty, Empty,
		Empty, Empty, Empty, Empty, Empty, Empty, Empty, Empty,
		Empty, BlackBishop, Empty, Empty, BlackKing, Empty, BlackBishop, BlackQueen,
		BlackRook, Empty, Empty, Empty, Empty, Empty, Empty, BlackRook,
	}

	position, err := NewPositionFromFEN(input)
	assert.NoError(t, err)
	assert.Equal(t, output, position.Squares)
}

// TestInvalidFEN makes sure that invalid FEN strings are detected and reported as errors.
func TestInvalidFen(t *testing.T) {
	tests := []struct {
		fen string
		why string
	}{
		{"r6r/1b2k1bq/8/8/8/8/R3K2R b KQ - 3 2", "Missing a rank"},
		{"8/8/8/2k5/2pP4/8/B7/4K3 g - d3 0 3", "Uses 'g' instead of 'w' or 'b' for side to move"},
		{"w KQkq - 2 2", "Is missing the ranks section entirely"},
		{"r3k2rp1pp1pb1/8/bn2Qnp1/2qPN3/1p2P3/2N5/PPPBBPPP/R3K2R b KQkq - 3 2", "One rank is too long"},
		{"2kr3r//bn2Qnp1/3PN3/1p2P3/2N5/PPPBBPPP/R3K2R b KQ - 3 2", "Rank is empty"},
		{"5k2/8/8/8/8/8/8/4K2R w  - 0 1", "Castling rights are omitted"},
	}

	for _, test := range tests {
		_, err := NewPositionFromFEN(test.fen)
		assert.Error(t, err, "wanted an error, invalid because %q", test.why)
	}
}

// TestValidMoves tests that performing valid moves on the position gives the expected FEN string.
func TestValidMoves(t *testing.T) {
	tests := []struct {
		startingFEN string
		moves       []string
		expectedFEN string
	}{
		{
			"rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1",
			[]string{
				"e2e4",
				"e7e5",
			},
			"rnbqkbnr/pppp1ppp/8/4p3/4P3/8/PPPP1PPP/RNBQKBNR w KQkq e6 0 2",
		},
		{
			"4k3/1P6/8/8/8/8/8/4K3 w - - 0 1",
			[]string{
				"b7b8q",
			},
			"1Q2k3/8/8/8/8/8/8/4K3 b - - 0 1",
		},
		{
			"rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1",
			[]string{
				"e2e4",
				"e7e5",
				"f1c4",
				"f8c5",
				"g1f3",
				"g8f6",
				"d1e2",
				"d8e7",
				"b2b3",
				"b7b6",
				"c1b2",
				"c8b7",
				"b1a3",
				"b8a6",
			},
			"r3k2r/pbppqppp/np3n2/2b1p3/2B1P3/NP3N2/PBPPQPPP/R3K2R w KQkq - 4 8",
		},
		{
			"r3k2r/pbppqppp/np3n2/2b1p3/2B1P3/NP3N2/PBPPQPPP/R3K2R w KQkq - 4 8",
			[]string{
				"e1d1",
			},
			"r3k2r/pbppqppp/np3n2/2b1p3/2B1P3/NP3N2/PBPPQPPP/R2K3R b kq - 5 8",
		},
		{
			"r3k2r/pbppqppp/np3n2/2b1p3/2B1P3/NP3N2/PBPPQPPP/R3K2R w KQkq - 4 8",
			[]string{
				"e1c1",
			},
			"r3k2r/pbppqppp/np3n2/2b1p3/2B1P3/NP3N2/PBPPQPPP/2KR3R b kq - 5 8",
		},
		{
			"r3k2r/pbppqppp/np3n2/2b1p3/2B1P3/NP3N2/PBPPQPPP/R3K2R w KQkq - 4 8",
			[]string{
				"e1g1",
			},
			"r3k2r/pbppqppp/np3n2/2b1p3/2B1P3/NP3N2/PBPPQPPP/R4RK1 b kq - 5 8",
		},
		{
			"r3k2r/pbppqppp/np3n2/2b1p3/2B1P3/NP3N2/PBPPQPPP/R3K2R w KQkq - 4 8",
			[]string{
				"a1b1",
			},
			"r3k2r/pbppqppp/np3n2/2b1p3/2B1P3/NP3N2/PBPPQPPP/1R2K2R b Kkq - 5 8",
		},
		{
			"r3k2r/pbppqppp/np3n2/2b1p3/2B1P3/NP3N2/PBPPQPPP/R3K2R w KQkq - 4 8",
			[]string{
				"h1f1",
			},
			"r3k2r/pbppqppp/np3n2/2b1p3/2B1P3/NP3N2/PBPPQPPP/R3KR2 b Qkq - 5 8",
		},
		{
			"r3k2r/pbppqppp/np3n2/2b1p3/2B1P3/NP1P1N2/PBP1QPPP/R3K2R b KQkq - 0 8",
			[]string{
				"e8f8",
			},
			"r4k1r/pbppqppp/np3n2/2b1p3/2B1P3/NP1P1N2/PBP1QPPP/R3K2R w KQ - 1 9",
		},
		{
			"r3k2r/pbppqppp/np3n2/2b1p3/2B1P3/NP1P1N2/PBP1QPPP/R3K2R b KQkq - 0 8",
			[]string{
				"e8g8",
			},
			"r4rk1/pbppqppp/np3n2/2b1p3/2B1P3/NP1P1N2/PBP1QPPP/R3K2R w KQ - 1 9",
		},
		{
			"r3k2r/pbppqppp/np3n2/2b1p3/2B1P3/NP1P1N2/PBP1QPPP/R3K2R b KQkq - 0 8",
			[]string{
				"e8c8",
			},
			"2kr3r/pbppqppp/np3n2/2b1p3/2B1P3/NP1P1N2/PBP1QPPP/R3K2R w KQ - 1 9",
		},
	}

	for _, test := range tests {
		position, err := NewPositionFromFEN(test.startingFEN)
		assert.NoError(t, err, "wasn't expecting an error parsing the start position when testing if moves are valid")

		for _, move := range test.moves {
			parsed, err := ParseMove(move)
			assert.NoError(t, err, "wasn't expecting an error parsing valid moves")

			position.MakeMove(parsed)
		}

		assert.Equal(t, test.expectedFEN, position.StringFEN(), "expected FEN to match after moves have been played")
	}
}

// TODO: write tests for generating moves and then unmaking them
