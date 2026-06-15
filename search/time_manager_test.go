package search

import (
	"testing"
	"time"
)

// TestTimeManagerReservesOverheadPerMove checks the fix for the on-time losses: the move overhead must
// be held back from each move's budget, not once from the whole clock. With a known clock and movesToGo,
// the allocation must equal share + most-of-increment - overhead.
func TestTimeManagerReservesOverheadPerMove(t *testing.T) {
	const (
		clock = 60 * time.Second
		inc   = 1 * time.Second
		moves = 30
	)
	got := defaultTimeManager(clock, inc, moves)
	want := clock/moves + inc*3/4 - moveOverhead
	if got != want {
		t.Fatalf("allocation = %v, want %v (share %v + 3/4 inc %v - overhead %v)",
			got, want, clock/moves, inc*3/4, moveOverhead)
	}
	// And it must be strictly less than the naive share+increment (i.e. overhead really was reserved).
	if got >= clock/moves+inc*3/4 {
		t.Fatalf("allocation %v did not reserve the move overhead", got)
	}
}

// TestTimeManagerNeverGamblesTooMuch caps any single move at a quarter of the clock, so a low
// moves-to-go can't blow the whole clock on one move.
func TestTimeManagerNeverGamblesTooMuch(t *testing.T) {
	got := defaultTimeManager(60*time.Second, 0, 1) // movesToGo 1 would otherwise want the whole clock
	if limit := 60 * time.Second / 4; got > limit {
		t.Fatalf("allocation %v exceeds the quarter-clock cap %v", got, limit)
	}
}

// TestTimeManagerEmergency returns a tiny budget when almost no time is left, rather than a negative or
// zero duration, so the engine still returns its fallback move instead of hanging.
func TestTimeManagerEmergency(t *testing.T) {
	for _, clock := range []time.Duration{0, 50 * time.Millisecond, moveOverhead} {
		got := defaultTimeManager(clock, 0, 30)
		if got <= 0 {
			t.Fatalf("clock %v: allocation = %v, want positive", clock, got)
		}
		if got > 100*time.Millisecond {
			t.Fatalf("clock %v: emergency allocation %v too large", clock, got)
		}
	}
}

// TestTimeManagerScalesWithClock sanity-checks that a slow control gets much more per move than bullet.
func TestTimeManagerScalesWithClock(t *testing.T) {
	bullet := defaultTimeManager(60*time.Second, 1*time.Second, 0)
	rapid := defaultTimeManager(600*time.Second, 5*time.Second, 0)
	if rapid <= bullet {
		t.Fatalf("rapid allocation %v should exceed bullet allocation %v", rapid, bullet)
	}
}
