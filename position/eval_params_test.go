package position

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEvalParamsReweightable: an engine can build a custom evaluator that values king safety more than
// the default. With a full piece complement (so the middlegame king-safety term is not tapered away),
// White's king is stranded on e4 while Black is castled behind f7/g7/h7. A king-safety-loving evaluator
// should judge White's position more harshly than the default one does.
func TestEvalParamsReweightable(t *testing.T) {
	pos, err := NewPositionFromFEN("rnbq1rk1/ppppbppp/8/8/4K3/8/PPPP1PPP/RNBQ1BNR w - - 0 1")
	require.NoError(t, err)

	coward := DefaultEvalParams
	coward.KingShield *= 5
	coward.KingOpenFile *= 5

	base := EvalWith(pos, &DefaultEvalParams)
	loud := EvalWith(pos, &coward)
	assert.Less(t, loud, base, "valuing king safety more should judge the exposed white king more harshly")

	// MakeEvaluator must produce the same scores as EvalWith with the same params.
	assert.Equal(t, loud, MakeEvaluator(coward)(pos))
}
