package search

import (
	"strings"
	"testing"
	"time"

	"github.com/ollybritton/StupidChess/position"
)

// searchBestMove drives a full search (the real Root pipeline, so aspiration / PVS / pruning are all
// exercised) to a fixed depth and returns the best move in UCI. It also returns the node count from the
// final depth, so tests can watch selectivity shrink the tree.
func searchBestMove(t *testing.T, fen string, depth uint) (string, int) {
	t.Helper()

	pos, err := position.NewPositionFromFEN(fen)
	if err != nil {
		t.Fatalf("bad fen %q: %v", fen, err)
	}

	s := NewAlphaBetaSearch(make(chan Request), make(chan string, 4096), position.EvalComplex, position.EvalComplex)
	go s.Root()

	opts := NewDeafultOptions()
	opts.Depth = depth
	opts.MoveTime = time.Hour // depth-bounded, not time-bounded
	s.Requests() <- NewRequest(pos, opts)

	nodes := 0
	for msg := range s.Responses() {
		if fields := strings.Fields(msg); len(fields) >= 4 && fields[0] == "info" && fields[1] == "depth" {
			for i, f := range fields {
				if f == "nodes" && i+1 < len(fields) {
					if n, err := parseInt(fields[i+1]); err == nil {
						nodes = n
					}
				}
			}
		}
		if strings.HasPrefix(msg, "bestmove ") {
			return strings.Fields(msg)[1], nodes
		}
	}
	return "", nodes
}

func parseInt(s string) (int, error) {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, errNotInt
		}
		n = n*10 + int(c-'0')
	}
	return n, nil
}

var errNotInt = &intError{}

type intError struct{}

func (*intError) Error() string { return "not an int" }

// tacticalSuite is a set of positions with a single clearly best move. Pruning and reductions must not
// break these: the engine has to keep finding them as the search gets more selective.
var tacticalSuite = []struct {
	name  string
	fen   string
	depth uint
	best  string
}{
	{"mate-in-1 back rank", "6k1/5ppp/8/8/8/8/8/4R1K1 w - - 0 1", 4, "e1e8"},
	{"win the hanging queen", "4k3/8/8/3q4/3Q4/8/8/4K3 w - - 0 1", 6, "d4d5"},
	{"win a rook with a fork", "4k3/8/8/8/8/4n3/8/R3K2R b - - 0 1", 6, "e3c2"},
	{"promote to win", "8/P6k/8/8/8/8/7K/8 w - - 0 1", 6, "a7a8q"},
}

func TestTacticalSuite(t *testing.T) {
	for _, tc := range tacticalSuite {
		got, nodes := searchBestMove(t, tc.fen, tc.depth)
		if got != tc.best {
			t.Errorf("%s: got %s, want %s (depth %d, %d nodes)", tc.name, got, tc.best, tc.depth, nodes)
		} else {
			t.Logf("%s: %s ok (depth %d, %d nodes)", tc.name, got, tc.depth, nodes)
		}
	}
}
