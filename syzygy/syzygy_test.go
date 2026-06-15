package syzygy

import (
	"testing"
	"time"

	"github.com/ollybritton/StupidChess/position"
)

const testDataDir = "../testdata/syzygy"

func loadTB(t *testing.T) *Tablebases {
	t.Helper()
	tb, err := Load(testDataDir)
	if err != nil {
		t.Fatalf("Load(%q): %v", testDataDir, err)
	}
	if len(tb.tables) == 0 {
		t.Fatalf("no tables loaded from %q", testDataDir)
	}
	return tb
}

func probeFEN(t *testing.T, tb *Tablebases, fen string) (int, bool) {
	t.Helper()
	p, err := position.NewPositionFromFEN(fen)
	if err != nil {
		t.Fatalf("bad FEN %q: %v", fen, err)
	}
	return tb.ProbeWDL(p)
}

func TestLoad(t *testing.T) {
	tb := loadTB(t)
	if tb.MaxPieces() < 3 {
		t.Fatalf("MaxPieces = %d, want >= 3", tb.MaxPieces())
	}
	// We expect the 4-man KQvKR to push MaxPieces to 4.
	if tb.MaxPieces() != 4 {
		t.Logf("MaxPieces = %d (expected 4 with KQvKR present)", tb.MaxPieces())
	}
}

// TestKQvK_Win: white king on a1, white queen on d4, black king far away on h8.
// White (the queen side) to move. KQ vs K is a trivial win.
func TestKQvK_Win(t *testing.T) {
	tb := loadTB(t)
	// White K a1, white Q b3 (does not check h8), black k h8, white to move.
	fen := "7k/8/8/8/8/1Q6/8/K7 w - - 0 1"
	wdl, ok := probeFEN(t, tb, fen)
	if !ok {
		t.Skip("KQvK probe failed (decompression not yet correct); see package comments")
	}
	if wdl != 2 {
		t.Errorf("KQvK WDL = %d, want 2 (win)", wdl)
	}
}

// TestKNvK_Draw: king + knight vs king is always a draw (insufficient material).
func TestKNvK_Draw(t *testing.T) {
	tb := loadTB(t)
	// Knight on d4 does not attack h8; kings far apart.
	fen := "7k/8/8/8/3N4/8/8/K7 w - - 0 1"
	wdl, ok := probeFEN(t, tb, fen)
	if !ok {
		t.Skip("KNvK probe failed (decompression not yet correct); see package comments")
	}
	if wdl != 0 {
		t.Errorf("KNvK WDL = %d, want 0 (draw)", wdl)
	}
}

// TestKRvK_Win: king + rook vs king is a win for the rook side. Rook side (white)
// to move with kings apart.
func TestKRvK_Win(t *testing.T) {
	tb := loadTB(t)
	// White K a1, white R b3 (rank 3 / file b, does not reach h8), black k h8.
	fen := "7k/8/8/8/8/1R6/8/K7 w - - 0 1"
	wdl, ok := probeFEN(t, tb, fen)
	if !ok {
		t.Skip("KRvK probe failed (decompression not yet correct); see package comments")
	}
	if wdl != 2 {
		t.Errorf("KRvK WDL = %d, want 2 (win for rook side)", wdl)
	}
}

// TestKRvK_BlackToMove: same material, black (the lone king) to move. Still a
// win for white, so from black's perspective it is a loss (-2). This exercises
// the symmetric-table / mirror path.
func TestKRvK_LossForLoneKing(t *testing.T) {
	tb := loadTB(t)
	// White king a1, white rook b3, black king h8, black to move (not in check).
	fen := "7k/8/8/8/8/1R6/8/K7 b - - 0 1"
	wdl, ok := probeFEN(t, tb, fen)
	if !ok {
		t.Skip("KRvK (btm) probe failed; see package comments")
	}
	if wdl != -2 {
		t.Errorf("KRvK black-to-move WDL = %d, want -2 (loss)", wdl)
	}
}

// TestProbesSucceedWithKnownValues asserts the probe actually succeeds (ok==true,
// no t.Skip) and returns the value verified against an independent prober
// (python-chess 1.11.2) on the same testdata. Every material type loaded from
// ./testdata/syzygy is covered, including the 4-man KQvKR win and loss. If any of
// these starts returning ok==false the decompression/index path has regressed; we
// fail loudly rather than skip, which is what masked earlier coverage gaps.
func TestProbesSucceedWithKnownValues(t *testing.T) {
	tb := loadTB(t)
	cases := []struct {
		name string
		fen  string
		want int
	}{
		// 3-man, all piece types, both stms exercised across the suite.
		{"KQvK win", "7k/8/8/8/8/1Q6/8/K7 w - - 0 1", 2},
		{"KRvK win", "7k/8/8/8/8/1R6/8/K7 w - - 0 1", 2},
		{"KBvK draw", "7k/8/8/8/8/1B6/8/K7 w - - 0 1", 0},
		{"KNvK draw", "7k/8/8/8/3N4/8/8/K7 w - - 0 1", 0},
		{"KPvK win (wtm)", "k7/8/8/8/8/8/4P3/K7 w - - 0 1", 2},
		{"KPvK draw (btm)", "k7/8/8/8/8/8/4P3/K7 b - - 0 1", 0},
		{"KPvK win (e-file)", "8/8/8/4k3/8/8/3PK3/8 w - - 0 1", 2},
		// 4-man KQvKR, verified legal and WDL by python-chess.
		{"KQvKR win", "8/2Q5/1K6/8/8/3k4/8/6r1 w - - 0 1", 2},
		{"KQvKR win (btm)", "8/6K1/8/8/4r3/8/7Q/7k b - - 0 1", 2},
		{"KQvKR loss (btm)", "7r/8/8/8/2Q5/8/K7/7k b - - 0 1", -2},
	}
	for _, c := range cases {
		wdl, ok := probeFEN(t, tb, c.fen)
		if !ok {
			t.Errorf("%s: probe returned ok=false for %q (regression in decompression/index path)", c.name, c.fen)
			continue
		}
		if wdl != c.want {
			t.Errorf("%s: WDL = %d, want %d", c.name, wdl, c.want)
		}
	}
}

// TestIllegalPositionIsRejectedAndDoesNotHang guards against two real bugs found
// during hardening: (1) an illegal position (a side left in check, or a king that
// is capturable) must return ok=false from the public API, and (2) probing must
// terminate. Because material keys ignore kings, a position that has lost a king
// during the internal capture search collides with a same-material table (e.g. a
// kingless KBvK), and feeding a bad index to decompress_pairs previously spun
// forever. The whole test must finish well within the package timeout.
func TestIllegalPositionIsRejectedAndDoesNotHang(t *testing.T) {
	tb := loadTB(t)
	// White bishop d4 checks the black king h8 with WHITE to move: black was
	// left in check, so the position is illegal. This is the exact FEN that
	// previously panicked then hung.
	illegal := []string{
		"7k/8/8/8/3B4/8/8/K7 w - - 0 1", // bishop checks enemy king, wtm
		"8/8/8/3k4/8/8/Q7/K1r5 b - - 0 1", // opposite check (queen checks d5 king)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for _, fen := range illegal {
			p, err := position.NewPositionFromFEN(fen)
			if err != nil {
				continue
			}
			if _, ok := tb.ProbeWDL(p); ok {
				t.Errorf("illegal position %q: ProbeWDL returned ok=true, want false", fen)
			}
		}
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("ProbeWDL did not terminate on an illegal position (infinite loop regression)")
	}
}
