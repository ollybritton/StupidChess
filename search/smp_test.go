package search

import (
	"strings"
	"testing"
	"time"

	"github.com/ollybritton/StupidChess/position"
)

// searchBestMoveThreads is searchBestMove with a configurable thread count.
func searchBestMoveThreads(t *testing.T, fen string, depth uint, threads int) string {
	t.Helper()
	pos, err := position.NewPositionFromFEN(fen)
	if err != nil {
		t.Fatalf("bad fen: %v", err)
	}
	s := NewAlphaBetaSearch(make(chan Request), make(chan string, 4096), position.EvalComplex, position.EvalComplex)
	s.SetThreads(threads)
	go s.Root()

	opts := NewDeafultOptions()
	opts.Depth = depth
	opts.MoveTime = time.Hour
	s.Requests() <- NewRequest(pos, opts)

	for msg := range s.Responses() {
		if strings.HasPrefix(msg, "bestmove ") {
			return strings.Fields(msg)[1]
		}
	}
	return ""
}

// TestParallelSearchFindsTactics: with several threads sharing the transposition table, the search must
// still find the tactical-suite moves. Run under -race, this also exercises the lock-free table.
func TestParallelSearchFindsTactics(t *testing.T) {
	for _, tc := range tacticalSuite {
		got := searchBestMoveThreads(t, tc.fen, tc.depth, 4)
		if got != tc.best {
			t.Errorf("%s: got %s, want %s (4 threads)", tc.name, got, tc.best)
		}
	}
}
