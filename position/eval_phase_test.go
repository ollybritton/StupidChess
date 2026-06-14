package position

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// evalOf parses a FEN and returns its EvalComplex score (from White's perspective).
func evalOf(t *testing.T, fen string) int16 {
	t.Helper()
	pos, err := NewPositionFromFEN(fen)
	assert.NoError(t, err)
	return EvalComplex(pos)
}

// TestKingEvaluationIsPhaseDependent checks the tapered king evaluation: in the endgame a central king
// is rewarded, while in the middlegame (lots of material) it is penalised in favour of staying home.
// Each pair of positions differs only in the white king's square, so the eval difference is purely the
// (phase-blended) king table.
func TestKingEvaluationIsPhaseDependent(t *testing.T) {
	// Endgame (bare kings, phase 0): the central king should score higher.
	endgameCentral := evalOf(t, "7k/8/8/8/4K3/8/8/8 w - - 0 1") // white Ke4
	endgameBack := evalOf(t, "7k/8/8/8/8/8/8/4K3 w - - 0 1")    // white Ke1
	assert.Greater(t, endgameCentral, endgameBack, "in the endgame a central king should be rewarded")

	// Middlegame (full material, phase 24): the king should prefer the back rank.
	middlegameBack := evalOf(t, StartingPosition)                                              // white Ke1
	middlegameCentral := evalOf(t, "rnbqkbnr/pppppppp/8/8/4K3/8/PPPPPPPP/RNBQ1BNR w kq - 0 1") // white Ke4
	assert.Greater(t, middlegameBack, middlegameCentral, "in the middlegame the king should stay home")
}
