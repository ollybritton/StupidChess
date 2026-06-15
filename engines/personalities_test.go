package engines

import (
	"testing"

	"github.com/ollybritton/StupidChess/position"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestSoloistKeepsOnePiece: the soloist should keep moving the same piece as long as that piece has a
// legal move available, and only ever switch when it cannot.
func TestSoloistKeepsOnePiece(t *testing.T) {
	e := NewEngineSoloist()
	pos, err := position.NewPositionFromFEN(position.StartingPosition)
	require.NoError(t, err)

	prevActive := uint8(0)
	havePrev := false

	for ply := 0; ply < 8; ply++ {
		// If the previously active piece still has a legal move, the soloist must play it.
		if havePrev {
			activeHasMove := false
			for _, m := range pos.MovesLegal().AsSlice() {
				if m.From() == prevActive {
					activeHasMove = true
					break
				}
			}

			move := e.chooseMove(pos)
			require.NotEqual(t, position.NoMove, move)
			if activeHasMove {
				assert.Equal(t, prevActive, move.From(),
					"ply %d: active piece could move but soloist switched", ply)
			}
			require.True(t, pos.MakeMove(move))
		} else {
			move := e.chooseMove(pos)
			require.NotEqual(t, position.NoMove, move)
			require.True(t, pos.MakeMove(move))
		}
		prevActive, havePrev = e.active, true

		// Opponent replies with its first legal move to hand the turn back to the soloist.
		opp := pos.MovesLegal().AsSlice()
		require.NotEmpty(t, opp)
		require.True(t, pos.MakeMove(opp[0]))
	}
}
