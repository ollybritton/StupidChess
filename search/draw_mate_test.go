package search

import (
	"testing"

	"github.com/ollybritton/StupidChess/position"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMateScoreTTRoundTrip: rewriting a mate score for storage and back must be lossless at any ply,
// and ordinary scores must pass through untouched.
func TestMateScoreTTRoundTrip(t *testing.T) {
	for _, ply := range []int{0, 1, 5, 20} {
		win := position.MaxEval - 3   // mate we deliver
		lose := position.MinEval + 3  // mate against us
		assert.Equal(t, win, scoreFromTT(scoreToTT(win, ply), ply), "win mate ply %d", ply)
		assert.Equal(t, lose, scoreFromTT(scoreToTT(lose, ply), ply), "lose mate ply %d", ply)
		assert.Equal(t, int16(42), scoreToTT(42, ply), "normal score unchanged")
		assert.Equal(t, int16(42), scoreFromTT(42, ply), "normal score unchanged")
	}
}

// mateInOne is Re1-e8#: Black's king is boxed in by its own pawns on the back rank.
const mateInOne = "6k1/5ppp/8/8/8/8/8/4R1K1 w - - 0 1"

// TestFindsMate: the search reports a forced mate from a mate-in-one position.
func TestFindsMate(t *testing.T) {
	score := searchScore(t, mateInOne, 3)
	assert.Greater(t, score, mateScoreBound, "should see the forced mate (got %d)", score)
}

// TestMateScoreIsCached is the regression test for the caching bug: mate scores used to be skipped by
// the transposition table, so the engine forgot mates it had already found. Now the position's result
// must be stored.
func TestMateScoreIsCached(t *testing.T) {
	s := newTestSearch()
	pos, err := position.NewPositionFromFEN(mateInOne)
	require.NoError(t, err)

	var pv pvList
	score := s.search(position.MinEval, position.MaxEval, 3, 0, &pv, pos, position.NoMove, position.NoMove)
	require.Greater(t, score, mateScoreBound, "should have found the mate")

	_, ttScore, _, _, ok := s.tt.probe(pos.ZobristHash())
	require.True(t, ok, "the mate position must now be cached")
	assert.Greater(t, scoreFromTT(ttScore, 0), mateScoreBound, "the cached score must still be a mate")
}

// TestIsRepetitionScanAndWindow checks the repetition scan over the search path and the game history,
// and that it respects the fifty-move window.
func TestIsRepetitionScanAndWindow(t *testing.T) {
	s := newTestSearch()
	s.gameHistory = []uint64{0xAAAA, 0xBBBB}
	s.pathHashes[0] = 0x1111
	s.pathHashes[1] = 0x2222

	// At ply 2 with a generous window, a hash that appears in the path or the history is a repetition.
	assert.True(t, s.isRepetition(0x1111, 2, 100), "should match a path ancestor")
	assert.True(t, s.isRepetition(0xBBBB, 2, 100), "should match a history entry")
	assert.False(t, s.isRepetition(0x9999, 2, 100), "a novel position is not a repetition")

	// A window of 1 only reaches one ply back (pathHashes[1]), so a deeper match is out of range.
	assert.False(t, s.isRepetition(0xAAAA, 2, 1), "fifty-move window must bound the scan")
}

// TestFiftyMoveRuleIsDraw: a node whose halfmove clock has reached 100 is a draw, even when a side is
// up a whole queen.
func TestFiftyMoveRuleIsDraw(t *testing.T) {
	s := newTestSearch()
	s.pathHashes[0] = 0xdead

	pos, err := position.NewPositionFromFEN("4k3/8/8/8/8/8/8/3QK3 w - - 100 80")
	require.NoError(t, err)

	var pv pvList
	score := s.search(position.MinEval, position.MaxEval, 4, 1, &pv, pos, position.NoMove, position.NoMove)
	assert.Equal(t, drawScore, score, "the fifty-move rule should make this a draw despite the extra queen")
}
