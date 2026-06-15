package search

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/ollybritton/StupidChess/nnue"
	"github.com/ollybritton/StupidChess/position"
)

// realNetPath points at the genuine HalfKP net shared with the nnue package tests. It is gitignored, so
// the NNUE search tests skip when it is absent rather than failing on a clean checkout.
const realNetPath = "../nnue/testdata/net.nnue"

func loadNetOrSkip(t *testing.T) *nnue.Network {
	t.Helper()
	if _, err := os.Stat(realNetPath); err != nil {
		t.Skip("no real net at " + realNetPath)
	}
	net, err := nnue.Load(realNetPath)
	if err != nil {
		t.Fatalf("load net: %v", err)
	}
	return net
}

// searchWithNNUE runs a fixed-depth search through the full Root pipeline using either the incremental
// accumulator (SetNNUE) or a refresh-per-leaf wrapper (SetEvaluator), and returns the best move plus the
// score reported at the deepest completed depth.
func searchWithNNUE(t *testing.T, fen string, depth uint, net *nnue.Network, incremental bool) (best, score string) {
	t.Helper()

	pos, err := position.NewPositionFromFEN(fen)
	if err != nil {
		t.Fatalf("bad fen %q: %v", fen, err)
	}

	s := NewAlphaBetaSearch(make(chan Request), make(chan string, 4096), position.EvalComplex, position.EvalComplex)
	if incremental {
		s.SetNNUE(net)
	} else {
		// The old path: a stateless evaluator that rebuilds the accumulator from scratch every call,
		// wrapped to the White-positive convention. Both paths must give identical evals at every leaf.
		refresh := func(p *position.Position) int16 {
			v := net.Eval(p)
			if p.SideToMove == position.Black {
				return -v
			}
			return v
		}
		s.SetEvaluator(refresh, refresh)
	}
	go s.Root()

	opts := NewDeafultOptions()
	opts.Depth = depth
	opts.MoveTime = time.Hour // depth-bounded, not time-bounded, so every depth completes deterministically

	s.Requests() <- NewRequest(pos, opts)
	for msg := range s.Responses() {
		fields := strings.Fields(msg)
		if len(fields) >= 4 && fields[0] == "info" && fields[1] == "depth" {
			for i, f := range fields {
				if f == "score" && i+2 < len(fields) {
					score = fields[i+1] + " " + fields[i+2]
				}
			}
		}
		if strings.HasPrefix(msg, "bestmove ") {
			return fields[1], score
		}
	}
	return "", score
}

// TestNNUEIncrementalMatchesRefreshInSearch is the integration guard for the incremental accumulator:
// because Update is bit-identical to Refresh (proved in the nnue package), a single-threaded search must
// reach exactly the same best move and score whether the leaves are evaluated incrementally or by a full
// refresh. Any slip in the make/unmake bracketing of the accumulator stack would diverge the scores.
func TestNNUEIncrementalMatchesRefreshInSearch(t *testing.T) {
	net := loadNetOrSkip(t)

	positions := []struct {
		name  string
		fen   string
		depth uint
	}{
		{"start", position.StartingPosition, 7},
		{"kiwipete", "r3k2r/p1ppqpb1/bn2pnp1/3PN3/1p2P3/2N2Q1p/PPPBBPPP/R3K2R w KQkq - 0 1", 6},
		{"endgame", "8/2p5/3p4/KP5r/1R3p1k/8/4P1P1/8 w - - 0 1", 9},
		{"tactical", "r1bqkb1r/pppp1ppp/2n2n2/4p3/2B1P3/5N2/PPPP1PPP/RNBQK2R w KQkq - 4 4", 7},
		{"castling", "rnbqk2r/ppp1bppp/3p1n2/4p3/2B1P3/2N2N2/PPPP1PPP/R1BQK2R w KQkq - 0 5", 6},
	}

	for _, p := range positions {
		t.Run(p.name, func(t *testing.T) {
			incBest, incScore := searchWithNNUE(t, p.fen, p.depth, net, true)
			refBest, refScore := searchWithNNUE(t, p.fen, p.depth, net, false)
			if incBest != refBest || incScore != refScore {
				t.Fatalf("incremental vs refresh diverged: incremental {%s, %s} != refresh {%s, %s}",
					incBest, incScore, refBest, refScore)
			}
		})
	}
}
