package syzygy

import (
	"testing"

	"github.com/ollybritton/StupidChess/position"
)

// TestProbeRoot asserts ProbeRoot picks the optimal (lowest-DTZ winning) move,
// with the expected candidate set and best score, verified by replaying
// tbprobe.c's probe_root logic against python-chess on the same testdata (see
// the task notes). For each case the chosen best move must be one of the
// optimal-DTZ candidates and the whole legal-move set must be ranked.
func TestProbeRoot(t *testing.T) {
	tb := loadTB(t)

	cases := []struct {
		name string
		fen  string
		// best is the set of moves that share the optimal score (the engine may
		// pick any of them; ordering decides which). bestScore is that score.
		best      map[string]bool
		bestScore int
	}{
		{
			name:      "KRvK win e5/a1",
			fen:       "8/8/8/4k3/8/8/8/R3K3 w - - 0 1",
			best:      map[string]bool{"e1e2": true, "a1a5": true},
			bestScore: 27,
		},
		{
			name:      "KQvK win",
			fen:       "7k/8/8/8/8/1Q6/8/K7 w - - 0 1",
			best:      map[string]bool{"b3b7": true, "b3g3": true},
			bestScore: 13,
		},
		{
			name:      "KRvK win b3",
			fen:       "7k/8/8/8/8/1R6/8/K7 w - - 0 1",
			best:      map[string]bool{"b3b7": true, "b3g3": true},
			bestScore: 21,
		},
		{
			name:      "KQvKR win unique",
			fen:       "8/2Q5/1K6/8/8/3k4/8/6r1 w - - 0 1",
			best:      map[string]bool{"c7f4": true},
			bestScore: 43,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p, err := position.NewPositionFromFEN(c.fen)
			if err != nil {
				t.Fatalf("bad FEN %q: %v", c.fen, err)
			}
			best, ranking, ok := tb.ProbeRoot(p)
			if !ok {
				t.Fatalf("ProbeRoot returned ok=false for %q", c.fen)
			}
			if !c.best[best.String()] {
				t.Errorf("ProbeRoot best = %s, want one of %v", best.String(), keys(c.best))
			}
			// The best move's rank must equal the expected optimal score, and no
			// ranked move may beat it (smaller positive).
			var bestRank int
			found := false
			minPositive := 1 << 30
			for _, r := range ranking {
				if r.Move == best {
					bestRank = r.Rank
					found = true
				}
				if r.Rank > 0 && r.Rank < minPositive {
					minPositive = r.Rank
				}
			}
			if !found {
				t.Fatalf("best move %s not present in ranking", best.String())
			}
			if bestRank != c.bestScore {
				t.Errorf("best move %s rank = %d, want %d", best.String(), bestRank, c.bestScore)
			}
			if minPositive != c.bestScore {
				t.Errorf("smallest positive rank = %d, want %d (best move is not minimal-DTZ)", minPositive, c.bestScore)
			}
			// Every legal move must be ranked.
			if got, want := len(ranking), len(p.MovesLegal().AsSlice()); got != want {
				t.Errorf("ranked %d moves, want %d (all legal moves)", got, want)
			}
		})
	}
}

func keys(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
