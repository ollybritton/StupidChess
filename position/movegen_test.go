package position

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestPerft tests that the total number of moves possible in a certain position matches previous results from other
// engines.
// The positions for this test come from https://www.chessprogramming.org/Perft_Results.
func TestPerft(t *testing.T) {
	t.Run(
		"starting position",
		newPerftTest(
			"starting position",
			StartingPosition,
			[]uint{
				20,
				400,
				8902,
				197281,
				4865609,
				119060324,
			},
		),
	)

	t.Run(
		"kiwipete",
		newPerftTest(
			"kiwipete",
			"r3k2r/p1ppqpb1/bn2pnp1/3PN3/1p2P3/2N2Q1p/PPPBBPPP/R3K2R w KQkq - 0 1",
			[]uint{
				48,
				2039,
				97862,
				4085603,
				193690690,
			},
		),
	)

	t.Run(
		"position-3",
		newPerftTest(
			"position-3",
			"8/2p5/3p4/KP5r/1R3p1k/8/4P1P1/8 w - - 0 1",
			[]uint{
				14,
				191,
				2812,
				43238,
				674624,
				11030083,
				178633661,
			},
		),
	)

	t.Run(
		"position-4",
		newPerftTest(
			"position-4",
			"r3k2r/Pppp1ppp/1b3nbN/nP6/BBP1P3/q4N2/Pp1P2PP/R2Q1RK1 w kq - 0 1",
			[]uint{
				6,
				264,
				9467,
				422333,
				15833292,
			},
		),
	)

	t.Run(
		"position-5",
		newPerftTest(
			"position-5",
			"rnbq1k1r/pp1Pbppp/2p5/8/2B5/8/PPP1NnPP/RNBQK2R w KQ - 1 8",
			[]uint{
				44,
				1486,
				62379,
				2103487,
				89941194,
			},
		),
	)

	t.Run(
		"position-6",
		newPerftTest(
			"position-6",
			"r4rk1/1pp1qppp/p1np1n2/2b1p1B1/2B1P1b1/P1NP1N2/1PP1QPPP/R4RK1 w - - 0 10",
			[]uint{
				46,
				2079,
				89890,
				3894594,
				164075551,
			},
		),
	)
}

func newPerftTest(name, positionFEN string, results []uint) func(t *testing.T) {
	return func(t *testing.T) {
		p, err := NewPositionFromFEN(positionFEN)
		assert.NoErrorf(t, err, "wasn't expecting an error parsing the fen")

		if err != nil {
			return
		}

		for i, expected := range results {
			t.Logf("%s, depth %d/%d", name, i+1, len(results))
			assert.Equal(
				t,
				int(expected),
				int(p.Perft(uint(i+1))),
				"expected perft results to match, position %s, depth %d...",
				positionFEN,
				i+1,
			)
		}
	}
}
