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

// TestFortressEvalPrefersHuddle: with identical material, a compact position should score higher for
// the fortress than a spread-out one.
func TestFortressEvalPrefersHuddle(t *testing.T) {
	// Both have K + two knights; in "huddle" they sit next to the king, in "spread" they're in the
	// far corners.
	huddle, err := position.NewPositionFromFEN("8/8/8/8/8/8/4k3/3NKN2 w - - 0 1")
	require.NoError(t, err)
	spread, err := position.NewPositionFromFEN("N6N/8/8/8/8/8/4k3/4K3 w - - 0 1")
	require.NoError(t, err)

	assert.Greater(t, fortressEval(huddle, position.White), fortressEval(spread, position.White),
		"huddled pieces should evaluate better than scattered ones")
}

// TestFortressEvalKeepsMaterial: losing a piece must hurt the fortress far more than any positional
// term, so it never throws material away to tidy its formation.
func TestFortressEvalKeepsMaterial(t *testing.T) {
	full, err := position.NewPositionFromFEN("8/8/8/8/8/8/4k3/3NKN2 w - - 0 1")
	require.NoError(t, err)
	downAKnight, err := position.NewPositionFromFEN("8/8/8/8/8/8/4k3/4KN2 w - - 0 1")
	require.NoError(t, err)

	assert.Greater(t, fortressEval(full, position.White), fortressEval(downAKnight, position.White),
		"keeping a knight should beat any tidiness gained by losing it")
}

// TestPawnAdvancement sanity-checks the helper for both colours.
func TestPawnAdvancement(t *testing.T) {
	assert.Equal(t, 0, pawnAdvancement(position.White, 1)) // white pawn at home (rank 2)
	assert.Equal(t, 2, pawnAdvancement(position.White, 3)) // white pawn pushed to rank 4
	assert.Equal(t, 0, pawnAdvancement(position.Black, 6)) // black pawn at home (rank 7)
	assert.Equal(t, 2, pawnAdvancement(position.Black, 4)) // black pawn pushed to rank 5
}
