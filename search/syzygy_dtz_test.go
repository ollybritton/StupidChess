package search

import (
	"strings"
	"testing"
	"time"

	"github.com/ollybritton/StupidChess/position"
	"github.com/ollybritton/StupidChess/syzygy"
)

// bestMoveFor runs one search request and returns the engine's chosen move (the
// bestmove from the UCI stream). It is the search-level equivalent of the helper
// in syzygy_convert_test.go but pulled out so both the optimal-move assertion and
// the playout can share it.
func bestMoveFor(t *testing.T, s *AlphaBetaSearch, pos *position.Position, history []uint64) string {
	t.Helper()
	opts := NewDeafultOptions()
	opts.Depth = 24
	opts.Nodes = 150000
	opts.MoveTime = time.Hour
	opts.History = history

	s.Requests() <- NewRequest(pos.Clone(), opts)
	for msg := range s.Responses() {
		if strings.HasPrefix(msg, "bestmove ") {
			return strings.Fields(msg)[1]
		}
	}
	return ""
}

// TestSyzygyRootPicksOptimalMove asserts the wired-in root DTZ probe makes the
// engine play a distance-to-zero-optimal move at the root. The KQ vs KR position
// has a single optimal move (Qf4, DTZ 43) and many suboptimal queen moves (DTZ
// 47); without root DTZ the engine has no way to distinguish them. We assert the
// engine plays exactly the optimal move, which can only come from the tablebase.
func TestSyzygyRootPicksOptimalMove(t *testing.T) {
	tb, err := syzygy.Load("../testdata/syzygy")
	if err != nil || tb.MaxPieces() < 4 {
		t.Skip("no syzygy testdata (need 4-man KQvKR)")
	}

	s := NewAlphaBetaSearch(make(chan Request), make(chan string, 4096), position.EvalComplex, position.EvalComplex)
	s.SetTablebases(tb)
	go s.Root()

	// KQ vs KR, white to move. probe_root: unique optimal move is c7f4 (DTZ 43);
	// every other legal move scores DTZ 47 or worse.
	pos, err := position.NewPositionFromFEN("8/2Q5/1K6/8/8/3k4/8/6r1 w - - 0 1")
	if err != nil {
		t.Fatal(err)
	}

	best := bestMoveFor(t, s, pos, []uint64{pos.ZobristHash()})
	if best != "c7f4" {
		t.Fatalf("engine root move = %q, want the DTZ-optimal %q", best, "c7f4")
	}
}

// TestSyzygyDTZPlayoutMatesFast plays both sides of a won KR vs K from the same
// start as TestSyzygyConvertsKRvK. With root DTZ probing the engine converts
// optimally: the root DTZ is 27, and DTZ-optimal play (both sides) reaches mate
// in exactly those 27 plies, far inside the fifty-move limit and far fewer than
// the WDL-only search needed. We assert the playout mates the lone king within a
// tight ply bound that flat-win shuffling could never meet.
func TestSyzygyDTZPlayoutMatesFast(t *testing.T) {
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

	// DTZ-optimal play from this position mates in 27 plies; allow a small margin
	// for tie-broken move choices while still proving the conversion is driven by
	// DTZ (the WDL-only search took many more plies and risked the fifty-move draw).
	const maxPly = 40
	for ply := 0; ply < maxPly; ply++ {
		if len(pos.MovesLegal().AsSlice()) == 0 {
			if pos.KingInCheck(pos.SideToMove) && pos.SideToMove == position.Black {
				return // lone king checkmated within the bound: conversion succeeded.
			}
			t.Fatalf("game ended without mating the lone king at ply %d (stalemate or wrong side mated)", ply)
		}
		if pos.HalfmoveClock >= 100 {
			t.Fatalf("fifty-move draw at ply %d: DTZ conversion failed", ply)
		}

		best := bestMoveFor(t, s, pos, history)
		if best == "" || best == "0000" {
			t.Fatalf("no move returned at ply %d", ply)
		}
		if !applyUCIMove(pos, best) {
			t.Fatalf("engine returned illegal move %q at ply %d", best, ply)
		}
		history = append(history, pos.ZobristHash())
	}
	t.Fatalf("KR vs K not mated within %d plies: DTZ root conversion is not optimal", maxPly)
}
