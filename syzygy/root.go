package syzygy

import "github.com/ollybritton/StupidChess/position"

// This file ports the DTZ part of probe_root / tb_probe_root_impl from
// tbprobe.c: root move selection that converts a won (or holds a drawn, or
// maximally resists a lost) tablebase position optimally under the fifty-move
// rule. Unlike the interior WDL probe it works on the engine's own
// position.Position and legal-move generation so the chosen move slots straight
// into the search, and so move legality / mate detection reuse the engine.

// RootMove is one ranked candidate from ProbeRoot. Move is the engine move;
// Rank is its DTZ-derived score from the side-to-move perspective, on the same
// scale the C reference uses for probe_root's per-move scores (see ProbeRoot):
//
//	> 0  : winning, smaller is faster to convert (1 = mate / fastest)
//	== 0 : drawing
//	< 0  : losing, more negative resists longer (less negative loses sooner)
//
// SCORE_ILLEGAL moves are not included.
type RootMove struct {
	Move position.Move
	Rank int
}

// rootScoreIllegal mirrors SCORE_ILLEGAL in tbprobe.c: a sentinel for a move
// that does not pass legality. We never emit it in the ranking; it is only used
// transiently to skip such moves.

// ProbeRoot ports probe_root's DTZ logic. For a position within tablebase range
// it returns the move that preserves the best achievable result with the best
// DTZ that respects the fifty-move counter, an optional full per-move ranking,
// and ok=true. ok is false when the root cannot be probed (material not loaded,
// too many pieces, castling rights, illegal, or any child probe fails), in which
// case the caller should fall back to the normal search.
//
// The returned best move is, for a win, the one with the smallest positive DTZ
// score (fastest forced conversion); a zeroing move (capture or pawn move) that
// keeps the win is scored 1 or 101 via wdlToDtz, so it is naturally preferred
// when the win is "close" - which is exactly how the fifty-move counter is
// respected. For a draw it is any move that holds the draw; for a loss it is the
// move that drags the loss out the longest.
func (tb *Tablebases) ProbeRoot(p *position.Position) (best position.Move, ranking []RootMove, ok bool) {
	// The DTZ value of the root itself decides which class of move we are
	// selecting (win / draw / loss). If the root cannot be probed there is
	// nothing to do.
	rootDTZ, rok := tb.ProbeDTZ(p)
	if !rok {
		return position.NoMove, nil, false
	}

	legal := p.MovesLegal().AsSlice()
	if len(legal) == 0 {
		// No legal move: stalemate or checkmate. There is nothing for the root
		// prober to choose; let the caller handle the terminal position.
		return position.NoMove, nil, false
	}

	ranking = make([]RootMove, 0, len(legal))

	for _, m := range legal {
		child := p.Clone()
		if !child.MakeMove(m) {
			// MovesLegal already filtered illegal moves, but MakeMove also
			// reports legality; skip anything it rejects (SCORE_ILLEGAL).
			continue
		}

		v, vok := tb.rootMoveScore(child, rootDTZ)
		if !vok {
			// A child probe failed: rather than pick a move on incomplete
			// information, abandon root probing entirely and let the search run.
			return position.NoMove, nil, false
		}

		ranking = append(ranking, RootMove{Move: m, Rank: v})
	}

	if len(ranking) == 0 {
		return position.NoMove, nil, false
	}

	best = selectRootMove(rootDTZ, ranking)
	if best == position.NoMove {
		return position.NoMove, nil, false
	}
	return best, ranking, true
}

// rootMoveScore computes the per-move score v in probe_root for the child
// position (the position after our candidate move, with the opponent to move).
// rootDTZ is the DTZ of the parent (our) position, used to decide whether a
// mate-delivering child collapses to the fastest score. The returned score is
// from our (the parent side-to-move) perspective.
func (tb *Tablebases) rootMoveScore(child *position.Position, rootDTZ int) (int, bool) {
	// If we are winning and this move delivers checkmate, it is the fastest
	// possible conversion: score 1. is_mate in tbprobe.c is "in check with no
	// legal reply"; the engine expresses that as the side to move (the opponent)
	// having no legal move while in check.
	if rootDTZ > 0 && isCheckmate(child) {
		return 1, true
	}

	if child.HalfmoveClock != 0 {
		// Non-zeroing move: the child keeps the same fifty-move count, so its DTZ
		// is directly comparable. Negate to our perspective and bump the
		// magnitude by one ply for the move we just made.
		cdtz, ok := tb.ProbeDTZ(child)
		if !ok {
			return 0, false
		}
		v := -cdtz
		if v > 0 {
			v++
		} else if v < 0 {
			v--
		}
		return v, true
	}

	// Zeroing move (capture or pawn move): the fifty-move counter resets, so DTZ
	// is irrelevant and only the WDL of the resulting position matters. Map the
	// child WDL (negated to our perspective) to its canonical DTZ via wdlToDtz.
	cwdl, ok := tb.ProbeWDL(child)
	if !ok {
		return 0, false
	}
	return wdlToDtz[-cwdl+2], true
}

// selectRootMove ports the move-filtering tail of probe_root: given the parent
// DTZ class and the per-move scores, pick the best move. The first move with the
// best score wins ties, matching the C loop order (which is the move-generation
// order; here that is the engine's legal-move order).
func selectRootMove(rootDTZ int, ranking []RootMove) position.Move {
	switch {
	case rootDTZ > 0: // winning (or fifty-move-rule draw): smallest positive score
		best := bestNone
		var bestMove position.Move = position.NoMove
		for _, r := range ranking {
			v := r.Rank
			if v > 0 && v < best {
				best = v
				bestMove = r.Move
			}
		}
		return bestMove
	case rootDTZ < 0: // losing (or fifty-move-rule draw): most negative score resists longest
		best := 0
		var bestMove position.Move = position.NoMove
		for _, r := range ranking {
			v := r.Rank
			if v < best {
				best = v
				bestMove = r.Move
			}
		}
		// best == 0 in the C reference means checkmate is unavoidable; any move is
		// as good as another, so fall back to the first legal move.
		if bestMove == position.NoMove && len(ranking) > 0 {
			return ranking[0].Move
		}
		return bestMove
	default: // drawing: any move that holds the draw
		for _, r := range ranking {
			if r.Rank == 0 {
				return r.Move
			}
		}
		return position.NoMove
	}
}

// isCheckmate reports whether the side to move in p is checkmated (in check with
// no legal move). It mirrors is_mate in tbprobe.c and is used to recognise the
// fastest conversion (a mating move scores 1).
func isCheckmate(p *position.Position) bool {
	if !p.KingInCheck(p.SideToMove) {
		return false
	}
	return len(p.MovesLegal().AsSlice()) == 0
}
