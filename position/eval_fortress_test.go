package position

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFortressPenalisesAdvancing: the defensive overlay (the gap between the fortress evaluation and
// the plain one) is zero when everything is at home and negative once pawns and pieces push forward.
// EvalComplex itself rewards development, so the personality has to be tested on the overlay, not on
// the raw score.
func TestFortressPenalisesAdvancing(t *testing.T) {
	home, err := NewPositionFromFEN(StartingPosition)
	require.NoError(t, err)
	overlayHome := int(EvalFortressUs(home)) - int(EvalComplex(home))
	assert.Equal(t, 0, overlayHome, "a home setup incurs no defensive penalty")

	// White (side to move) has shoved two centre pawns and developed both knights.
	advanced, err := NewPositionFromFEN("rnbqkbnr/pppppppp/8/8/3PP3/2N2N2/PPP2PPP/R1BQKB1R w KQkq - 0 1")
	require.NoError(t, err)
	overlayAdvanced := int(EvalFortressUs(advanced)) - int(EvalComplex(advanced))
	assert.Less(t, overlayAdvanced, overlayHome, "advancing should cost the fortress overlay points")
}

// TestFortressOverlayBounded: the defensive overlay must never be worth as much as a pawn, so the
// search never sacrifices material for it. Compare the fortress and plain evaluations of a developed
// position; the difference is the overlay.
func TestFortressOverlayBounded(t *testing.T) {
	pos, err := NewPositionFromFEN("r1bqkb1r/pppp1ppp/2n2n2/4p3/2B1P3/3P1N2/PPP2PPP/RNBQK2R w KQkq - 0 1")
	require.NoError(t, err)

	overlay := int(EvalFortressUs(pos)) - int(EvalComplex(pos))
	assert.LessOrEqual(t, overlay, 0, "the overlay is a penalty, never a bonus")
	assert.GreaterOrEqual(t, overlay, -fortressMaxOverlay, "the overlay is capped")
}

// TestFortressThemIsPlain: the opponent is assumed to play ordinary chess.
func TestFortressThemIsPlain(t *testing.T) {
	pos, err := NewPositionFromFEN("r1bqkb1r/pppp1ppp/2n2n2/4p3/2B1P3/3P1N2/PPP2PPP/RNBQK2R w KQkq - 0 1")
	require.NoError(t, err)
	assert.Equal(t, EvalComplex(pos), EvalFortressThem(pos))
}

// TestFortressRanksAdvanced sanity-checks the advancement helpers for both colours.
func TestFortressRanksAdvanced(t *testing.T) {
	assert.Equal(t, 0, pawnRanksAdvanced(White, 1)) // home rank 2
	assert.Equal(t, 2, pawnRanksAdvanced(White, 3)) // pushed to rank 4
	assert.Equal(t, 0, pawnRanksAdvanced(Black, 6)) // home rank 7
	assert.Equal(t, 2, pawnRanksAdvanced(Black, 4)) // pushed to rank 5

	assert.Equal(t, 0, pieceRanksAdvanced(White, 0)) // back rank
	assert.Equal(t, 2, pieceRanksAdvanced(White, 2)) // out to rank 3
	assert.Equal(t, 0, pieceRanksAdvanced(Black, 7)) // back rank
	assert.Equal(t, 2, pieceRanksAdvanced(Black, 5)) // out to rank 6
}
