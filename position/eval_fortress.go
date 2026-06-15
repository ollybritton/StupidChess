package position

// The fortress engine plays real, searching chess (it wants to win) but with an ultra-defensive
// style: it likes a closed position with its pawns and pieces kept back and together. That style is a
// small overlay on top of the normal EvalComplex evaluation, applied only to the fortress's own side,
// so the deep search still does the heavy lifting (and never throws material away for tidiness).

const (
	// Penalty, in centipawns, per rank a pawn / piece has advanced from home. These resist opening the
	// position and over-extending; they are deliberately small so the search's material and king-safety
	// judgement always wins out.
	fortressPawnAdvancePenalty  = 4
	fortressPieceAdvancePenalty = 7

	// fortressMaxOverlay caps the whole overlay so it can never outweigh as much as a single pawn, and
	// so a fully developed position still has gradient left rather than pinning at the cap.
	fortressMaxOverlay = 80

	// fortressMateGuard: don't perturb near-mate scores, so forced mates are still seen as mates.
	fortressMateGuard = 29000
)

// EvalFortressUs is the fortress's own evaluation. Because the search only uses the "us" evaluator at
// nodes where it is the engine's turn, the overlay is applied to the side to move (the fortress).
func EvalFortressUs(pos *Position) int16 {
	base := EvalComplex(pos)
	if base > fortressMateGuard || base < -fortressMateGuard {
		return base
	}

	// fortressDefensiveScore is <= 0; it is good for the side to move, so it is added white-positively
	// (subtracted when the fortress is Black) and then handled by ScoreFromPerspective downstream.
	score := fortressDefensiveScore(pos, pos.SideToMove)
	if pos.SideToMove == White {
		return base + int16(score)
	}
	return base - int16(score)
}

// EvalFortressThem assumes the opponent just plays ordinary chess, with no defensive quirk.
func EvalFortressThem(pos *Position) int16 {
	return EvalComplex(pos)
}

// fortressDefensiveScore returns a non-positive penalty (0 = fully closed and at home, more negative =
// more advanced and exposed) measuring how un-fortress-like the given side's position is.
func fortressDefensiveScore(pos *Position, me Color) int {
	penalty := 0
	for sq := 0; sq < 64; sq++ {
		piece := pos.Squares[sq]
		if piece == Empty || piece.Color() != me {
			continue
		}

		rank := sq / 8
		switch piece.Colorless() {
		case Pawn:
			penalty += fortressPawnAdvancePenalty * pawnRanksAdvanced(me, rank)
		case Knight, Bishop, Rook, Queen:
			penalty += fortressPieceAdvancePenalty * pieceRanksAdvanced(me, rank)
		}
	}

	if penalty > fortressMaxOverlay {
		penalty = fortressMaxOverlay
	}
	return -penalty
}

// pawnRanksAdvanced is how many ranks a pawn has moved from its starting rank.
func pawnRanksAdvanced(c Color, rank int) int {
	if c == White {
		return rank - 1 // white pawns start on rank 2 (index 1); pawns are never on rank 1
	}
	return 6 - rank // black pawns start on rank 7 (index 6)
}

// pieceRanksAdvanced is how far a piece has moved off its back rank.
func pieceRanksAdvanced(c Color, rank int) int {
	if c == White {
		return rank // white back rank is index 0
	}
	return 7 - rank // black back rank is index 7
}
