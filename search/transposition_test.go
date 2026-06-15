package search

import (
	"testing"

	"github.com/ollybritton/StupidChess/position"
	"github.com/stretchr/testify/assert"
)

func fixedDepthScore(t *testing.T, fen string, depth uint, useTT bool) int16 {
	t.Helper()
	pos, err := position.NewPositionFromFEN(fen)
	assert.NoError(t, err)
	s := newTestSearch()
	if !useTT {
		s.tt = nil
	}
	var pv pvList
	return s.search(position.MinEval, position.MaxEval, depth, 0, &pv, pos)
}

// TestTranspositionTableDoesNotCorruptScore: with the table on or off, a fixed-depth search must
// return essentially the same score. They are not bit-identical because the selective pruning
// (null-move, reverse-futility, futility) depends on the alpha/beta bounds, which the table tightens
// via cutoffs, so a pruning decision can shift by a few centipawns. A large divergence, though, would
// mean the table is returning unsound scores, which is what this guards against.
func TestTranspositionTableDoesNotCorruptScore(t *testing.T) {
	fens := []string{
		position.StartingPosition,
		"r1bqkbnr/pppp1ppp/2n5/4p3/2B1P3/5N2/PPPP1PPP/RNBQK2R w KQkq - 4 4",
		"r3k2r/p1ppqpb1/bn2pnp1/3PN3/1p2P3/2N2Q1p/PPPBBPPP/R3K2R w KQkq - 0 1",
		"8/2p5/3p4/KP5r/1R3p1k/8/4P1P1/8 w - - 0 1",
		"4k3/8/4p3/3q4/3Q4/8/8/4K3 w - - 0 1",
	}

	const tolerance = 30 // centipawns; selective-pruning jitter, far below a real corruption
	for _, fen := range fens {
		with := fixedDepthScore(t, fen, 4, true)
		without := fixedDepthScore(t, fen, 4, false)
		assert.InDelta(t, without, with, tolerance, "transposition table changed the depth-4 score for %s", fen)
	}
}

// TestZobristDistinguishesPositions sanity-checks the hash: equal positions hash equally, a move
// changes the hash, and undoing it restores the hash. The move comes from the legal-move generator
// (so it carries the prior castling/en-passant state UndoMove needs); a move from ParseMove does not.
func TestZobristDistinguishesPositions(t *testing.T) {
	a, _ := position.NewPositionFromFEN(position.StartingPosition)
	b, _ := position.NewPositionFromFEN(position.StartingPosition)
	assert.Equal(t, a.ZobristHash(), b.ZobristHash(), "identical positions must hash equally")

	var e2e4 position.Move
	for _, mv := range a.MovesLegal().AsSlice() {
		if mv.String() == "e2e4" {
			e2e4 = mv
			break
		}
	}

	a.MakeMove(e2e4)
	assert.NotEqual(t, a.ZobristHash(), b.ZobristHash(), "a move must change the hash")
	a.UndoMove(e2e4)
	assert.Equal(t, a.ZobristHash(), b.ZobristHash(), "undoing the move must restore the hash")
}
