package search

import (
	"strings"
	"testing"
	"time"

	"github.com/ollybritton/StupidChess/position"
	"github.com/ollybritton/StupidChess/syzygy"
)

// applyUCIMove plays the UCI move on pos by matching it against the legal moves, returning false if it is
// not legal.
func applyUCIMove(pos *position.Position, uci string) bool {
	parsed, err := position.ParseMove(uci)
	if err != nil {
		return false
	}
	for _, m := range pos.MovesLegal().AsSlice() {
		if m.From() == parsed.From() && m.To() == parsed.To() && m.Promotion() == parsed.Promotion() {
			return pos.MakeMove(m)
		}
	}
	return false
}

// TestSyzygyConvertsKRvK is the regression test for the "rook and king shuffled and drew" bug. With only
// the win/draw/loss verdict (no DTZ), truncating the search at a tablebase win returns a flat score for
// every winning move, so the engine has no gradient and shuffles to a fifty-move draw. The fix is to not
// truncate wins - let the search convert them. Here the engine plays BOTH sides from a won KR vs K (so
// the defence is as stubborn as the engine can make it) and must deliver checkmate well within the
// fifty-move limit.
func TestSyzygyConvertsKRvK(t *testing.T) {
	tb, err := syzygy.Load("../testdata/syzygy")
	if err != nil || tb.MaxPieces() < 3 {
		t.Skip("no syzygy testdata")
	}

	s := NewAlphaBetaSearch(make(chan Request), make(chan string, 4096), position.EvalComplex, position.EvalComplex)
	s.SetTablebases(tb)
	go s.Root()

	pos, err := position.NewPositionFromFEN("8/8/8/4k3/8/8/8/R3K3 w - - 0 1")
	if err != nil {
		t.Fatal(err)
	}
	history := []uint64{pos.ZobristHash()}

	const maxPly = 100 // fifty full moves: the engine must mate before the fifty-move rule bites
	for ply := 0; ply < maxPly; ply++ {
		if len(pos.MovesLegal().AsSlice()) == 0 {
			if pos.KingInCheck(pos.SideToMove) {
				if pos.SideToMove == position.Black {
					return // black (the lone king) is checkmated: the rook side converted. Pass.
				}
				t.Fatalf("the rook side was checkmated at ply %d", ply)
			}
			t.Fatalf("stalemate at ply %d: engine failed to convert KR vs K", ply)
		}
		if pos.HalfmoveClock >= 100 {
			t.Fatalf("fifty-move draw: engine shuffled instead of mating (KR vs K not converted)")
		}

		opts := NewDeafultOptions()
		opts.Depth = 24
		opts.Nodes = 150000
		opts.MoveTime = time.Hour
		opts.History = history

		s.Requests() <- NewRequest(pos.Clone(), opts)
		best := ""
		for msg := range s.Responses() {
			if strings.HasPrefix(msg, "bestmove ") {
				best = strings.Fields(msg)[1]
				break
			}
		}
		if best == "" || best == "0000" {
			t.Fatalf("no move returned at ply %d", ply)
		}
		if !applyUCIMove(pos, best) {
			t.Fatalf("engine returned illegal move %q at ply %d", best, ply)
		}
		history = append(history, pos.ZobristHash())
	}
	t.Fatalf("KR vs K not mated within fifty moves: the engine is shuffling, not converting")
}
