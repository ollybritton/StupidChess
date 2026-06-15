package search

import (
	"testing"
	"time"

	"github.com/ollybritton/StupidChess/position"
	"github.com/stretchr/testify/assert"
)

func newTestSearch() *AlphaBetaSearch {
	s := NewAlphaBetaSearch(make(chan Request), make(chan string, 256), position.EvalComplex, position.EvalComplex)
	s.us = position.White
	s.options = NewDeafultOptions()
	s.options.MoveTime = time.Hour // don't let the time check abort these tiny searches
	s.startTime = time.Now()
	return s
}

func searchScore(t *testing.T, fen string, depth uint) int16 {
	t.Helper()
	pos, err := position.NewPositionFromFEN(fen)
	assert.NoError(t, err)
	var pv pvList
	return newTestSearch().search(position.MinEval, position.MaxEval, depth, 0, &pv, pos, position.NoMove, position.NoMove)
}

// TestQuiescenceSeesRecapture: a depth-1 search must not believe that capturing a DEFENDED queen wins
// material. Black's queen on d5 is defended by the e6 pawn, so White's Qxd5 is met by exd5 and the
// position is roughly level (in fact White is down a pawn). Without quiescence the depth-1 leaf would
// score Qxd5 as if it had won a whole queen.
func TestQuiescenceSeesRecapture(t *testing.T) {
	score := searchScore(t, "4k3/8/4p3/3q4/3Q4/8/8/4K3 w - - 0 1", 1)
	assert.True(t, score < 300, "expected ~level material after the recapture, got %d (a queen is 900)", score)
}

// TestQuiescenceTakesFreeMaterial: quiescence must not suppress genuinely free captures. Here Black's
// queen is undefended, so a depth-1 search should see it is winning a queen.
func TestQuiescenceTakesFreeMaterial(t *testing.T) {
	score := searchScore(t, "4k3/8/8/3q4/3Q4/8/8/4K3 w - - 0 1", 1)
	assert.True(t, score > 700, "expected to win the undefended queen, got %d", score)
}
