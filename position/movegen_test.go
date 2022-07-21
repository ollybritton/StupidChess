package position

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

// TestPerftStartingPosition tests that the number of generated chess games after 1-6 moves agrees with other engines.
// The FEN tested is:
//   rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1
func TestPerftStartingPosition(t *testing.T) {
	testPosition(
		t,
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
	)
}

// TestPerftKiwipete tests that the number of generated chess games after 1-5 moves agrees with other engines.
// This comes from the Chess Programming Wiki: https://www.chessprogramming.org/Perft_Results#Position_2
func TestPerftKiwipete(t *testing.T) {
	testPosition(
		t,
		"kiwipete",
		"r3k2r/p1ppqpb1/bn2pnp1/3PN3/1p2P3/2N2Q1p/PPPBBPPP/R3K2R w KQkq - 0 1",
		[]uint{
			48,
			2039,
			97862,
			4085603,
			193690690,
		},
	)
}

// TestPerftPosition3 tests that the number of generated chess games after 1-7 moves agrees with other engines.
// This comes from the Chess Programming Wiki: https://www.chessprogramming.org/Perft_Results#Position_3
func TestPerftPosition3(t *testing.T) {
	testPosition(
		t,
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
	)
}

func TestPerftPosition4(t *testing.T) {
	testPosition(
		t,
		"position-4",
		"r3k2r/Pppp1ppp/1b3nbN/nP6/BBP1P3/q4N2/Pp1P2PP/R2Q1RK1 w kq - 0 1",
		[]uint{
			6,
			264,
			9467,
			422333,
			15833292,
		},
	)
}

func TestPerftPosition5(t *testing.T) {
	testPosition(
		t,
		"position-5",
		"rnbq1k1r/pp1Pbppp/2p5/8/2B5/8/PPP1NnPP/RNBQK2R w KQ - 1 8",
		[]uint{
			44,
			1486,
			62379,
			2103487,
			89941194,
		},
	)
}

func TestPerftPosition6(t *testing.T) {
	testPosition(
		t,
		"position-6",
		"r4rk1/1pp1qppp/p1np1n2/2b1p1B1/2B1P1b1/P1NP1N2/1PP1QPPP/R4RK1 w - - 0 10",
		[]uint{
			46,
			2079,
			89890,
			3894594,
			164075551,
		},
	)
}

func testPosition(t *testing.T, name, positionFEN string, results []uint) {
	p, err := NewPositionFromFEN(positionFEN)
	assert.NoErrorf(t, err, "wasn't expecting an error parsing the fen")

	if err != nil {
		return
	}

	length := len(results)

	if testing.Short() && length > 1 {
		length -= 1
	}

	for i, expected := range results[:length] {
		t.Logf("%s, depth %d/%d...", name, i+1, length)

		start := time.Now()
		nodes := int(p.Perft(uint(i + 1)))
		duration := time.Since(start)

		t.Logf("%s, depth %d/%d -> got %d nodes, %.2fkn/s", name, i+1, length, nodes, float64(nodes/1000)/duration.Seconds())

		assert.Equal(
			t,
			int(expected),
			nodes,
			"expected perft results to match, position %s, depth %d...",
			positionFEN,
			i+1,
		)
	}
}
