package position

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// TestMovesCapturesMatchesPseudolegalSubset checks that the dedicated capture generator produces
// exactly the captures, en-passant captures and promotions that a full pseudolegal generation would,
// across positions exercising each of those cases.
func TestMovesCapturesMatchesPseudolegalSubset(t *testing.T) {
	fens := []string{
		"r3k2r/p1ppqpb1/bn2pnp1/3PN3/1p2P3/2N2Q1p/PPPBBPPP/R3K2R w KQkq - 0 1", // kiwipete: many captures
		"4k3/8/8/3pP3/8/8/8/4K3 w - d6 0 1",                                    // en passant available (exd6)
		"1n2k3/P7/8/8/8/8/8/4K3 w - - 0 1",                                     // push-promotion and capture-promotion
		"r3k2r/p1ppqpb1/bn2pnp1/3PN3/1p2P3/2N2Q1p/PPPBBPPP/R3K2R b KQkq - 0 1", // black to move
		StartingPosition, // no captures or promotions at all
	}

	for _, fen := range fens {
		pos, err := NewPositionFromFEN(fen)
		assert.NoError(t, err)

		expected := map[string]bool{}
		for _, m := range pos.MovesPseudolegal().AsSlice() {
			if m.Captured() != Empty || m.Promotion() != None {
				expected[m.String()] = true
			}
		}

		got := map[string]bool{}
		for _, m := range pos.MovesCaptures().AsSlice() {
			got[m.String()] = true
		}

		assert.Equal(t, expected, got, "MovesCaptures mismatch for %s", fen)
	}
}
