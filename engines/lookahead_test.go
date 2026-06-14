package engines

import (
	"testing"

	"github.com/ollybritton/StupidChess/position"
	"github.com/stretchr/testify/assert"
)

// firstNonKingMove returns a legal non-king move if one exists, so the opponent in a test can move
// without relocating its king. Falls back to any legal move.
func firstNonKingMove(pos *position.Position) position.Move {
	moves := pos.MovesLegal().AsSlice()
	for _, m := range moves {
		if m.Moved().Colorless() != position.King {
			return m
		}
	}
	if len(moves) > 0 {
		return moves[0]
	}
	return position.NoMove
}

// TestSuicideKingMarchesKingForward checks the suicide king plans: its king starts boxed in by its own
// pieces (a knight and pawns), so a one-ply engine could only shuffle. The look-ahead should clear the
// way and march the king toward the enemy, shrinking the king-to-king distance. Black is kept passive
// (it only shuffles a rook) so the improvement is purely the suicide king's doing.
func TestSuicideKingMarchesKingForward(t *testing.T) {
	pos, err := position.NewPositionFromFEN("r6k/8/8/8/8/8/PP6/KN6 w - - 0 1")
	assert.NoError(t, err)

	start := kingDistanceSquared(pos)

	for i := 0; i < 6; i++ {
		white := bestPositionalMove(pos, suicideKingDepth, position.White, suicideKingEval)
		assert.NotEqual(t, position.NoMove, white)
		assert.True(t, pos.MakeMove(white), "engine produced an illegal move: %s", white.String())

		black := firstNonKingMove(pos)
		if black == position.NoMove {
			break
		}
		pos.MakeMove(black)
	}

	assert.Less(t, kingDistanceSquared(pos), start, "the suicide king should have marched its king closer")
}

// TestSprinterPrefersLongMoves: from the start, every move except a knight's covers only one square, so
// the sprinter should pick a (two-square) knight move.
func TestSprinterPrefersLongMoves(t *testing.T) {
	pos, err := position.NewPositionFromFEN(position.StartingPosition)
	assert.NoError(t, err)

	m := bestSprinterMove(pos, sprinterDepth, position.None)
	assert.GreaterOrEqual(t, moveDistance(m), 2, "expected a long (knight) move, got %s", m.String())
}

// TestSprinterAvoidsRepeatingPiece: when the previous move was a knight, the sprinter should move a
// different piece if it can.
func TestSprinterAvoidsRepeatingPiece(t *testing.T) {
	pos, err := position.NewPositionFromFEN(position.StartingPosition)
	assert.NoError(t, err)

	m := bestSprinterMove(pos, sprinterDepth, position.Knight)
	assert.NotEqual(t, position.Knight, m.Moved().Colorless(), "should not move a knight again, got %s", m.String())
}
