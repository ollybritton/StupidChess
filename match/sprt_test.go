package match

import (
	"math"
	"testing"
)

func TestEloToScore(t *testing.T) {
	if got := eloToScore(0); math.Abs(got-0.5) > 1e-9 {
		t.Fatalf("eloToScore(0) = %v, want 0.5", got)
	}
	if eloToScore(100) <= 0.5 || eloToScore(-100) >= 0.5 {
		t.Fatalf("eloToScore not monotonic around 0")
	}
}

func TestEloWithErrorBalanced(t *testing.T) {
	// An even score (equal wins and losses) must read as ~0 Elo.
	elo, _ := EloWithError(100, 200, 100)
	if math.Abs(elo) > 1 {
		t.Fatalf("balanced match Elo = %.2f, want ~0", elo)
	}
	// A clear edge must read positive.
	elo, _ = EloWithError(200, 100, 100)
	if elo <= 0 {
		t.Fatalf("winning match Elo = %.2f, want > 0", elo)
	}
}

func TestSPRTAcceptsH1WhenStrong(t *testing.T) {
	s := NewSPRT(0, 5, 0.05, 0.05)
	// A dominant result (60% score over many games) should accept H1 (A stronger).
	if v := s.Decide(600, 200, 200); v != AcceptH1 {
		t.Fatalf("dominant A: verdict = %v (LLR %.2f), want AcceptH1", v, s.LLR(600, 200, 200))
	}
}

func TestSPRTAcceptsH0WhenWorse(t *testing.T) {
	s := NewSPRT(0, 5, 0.05, 0.05)
	// A clearly losing record (35% score) is below H0 and should reject the change.
	if v := s.Decide(200, 300, 500); v != AcceptH0 {
		t.Fatalf("losing A: verdict = %v (LLR %.2f), want AcceptH0", v, s.LLR(200, 300, 500))
	}
}

func TestSPRTSlowAtBoundary(t *testing.T) {
	s := NewSPRT(0, 5, 0.05, 0.05)
	// An exactly even result sits on H0, so the test is deliberately slow to decide
	// there: 1000 even games should still be inconclusive, not a false rejection.
	if v := s.Decide(300, 400, 300); v != Continue {
		t.Fatalf("even match at 1000 games: verdict = %v, want Continue (SPRT is slow on the boundary)", v)
	}
}

func TestSPRTBoundsOrientation(t *testing.T) {
	s := NewSPRT(0, 5, 0.05, 0.05)
	if s.Lower() >= 0 || s.Upper() <= 0 {
		t.Fatalf("bounds wrong sign: lower=%.2f upper=%.2f", s.Lower(), s.Upper())
	}
	// LLR must increase as A wins more.
	weak := s.LLR(100, 200, 100)
	strong := s.LLR(200, 200, 100)
	if strong <= weak {
		t.Fatalf("LLR not increasing with A wins: weak=%.2f strong=%.2f", weak, strong)
	}
}
