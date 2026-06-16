// This file adds the incremental-update path for the HalfKAv2_hm accumulator,
// mirroring the HalfKP Accumulator.Update in nnue.go but accounting for the
// differences of the modern architecture:
//
//   - Kings ARE encoded as features (PS_KING), so a king move contributes an
//     add/remove feature column on the perspective that is NOT refreshed.
//   - Each perspective is king-bucketed AND horizontally mirrored, and the
//     mirror/bucket changes whenever the king crosses files. Therefore a move
//     of a perspective's own king always invalidates that whole perspective and
//     it is rebuilt from scratch (matching HalfKAv2_hm::requires_refresh, which
//     refreshes a perspective iff its own king is the moved piece).
//   - There are two accumulations to carry forward: the int16 FT accumulation
//     (1024 wide) and the int32 PSQT side-channel (8 buckets).
//
// Everything else - captures (including en passant, whose captured pawn is not
// on the destination square), promotions and the castling rook - is the same
// small list of removed and added (piece, square) features applied to both
// perspectives, exactly as in the HalfKP path.
package nnue

import (
	"github.com/ollybritton/StupidChess/position"
)

// kaFeatureChange is a single piece placement (a piece appearing on, or leaving,
// a square) caused by a move. Unlike the HalfKP featureChange, kings ARE recorded
// because HalfKAv2_hm encodes them as features.
type kaFeatureChange struct {
	piece position.ColoredPiece
	sq    uint8
}

// addColumn adds the FT and PSQT weight columns of feature index to a single
// perspective's accumulation.
func (a *KAAccumulator) addColumn(n *KANetwork, p int, index uint32) {
	acc := &a.accumulation[p]
	col := n.ftWeightColumn(index)
	for j := 0; j < KAHalfDims; j++ {
		acc[j] += col[j]
	}
	psqt := &a.psqt[p]
	pcol := n.psqtColumn(index)
	for k := 0; k < KAPSQTBuckets; k++ {
		psqt[k] += pcol[k]
	}
}

// subColumn subtracts the FT and PSQT weight columns of feature index from a
// single perspective's accumulation.
func (a *KAAccumulator) subColumn(n *KANetwork, p int, index uint32) {
	acc := &a.accumulation[p]
	col := n.ftWeightColumn(index)
	for j := 0; j < KAHalfDims; j++ {
		acc[j] -= col[j]
	}
	psqt := &a.psqt[p]
	pcol := n.psqtColumn(index)
	for k := 0; k < KAPSQTBuckets; k++ {
		psqt[k] -= pcol[k]
	}
}

// Update computes this accumulator as the result of applying move m to the
// position that prev describes, leaving pos as the board AFTER the move. It is
// the incremental counterpart of Refresh: instead of rebuilding from scratch it
// copies prev and adds/removes only the handful of feature columns the move
// touched, on both the FT accumulation and the PSQT side-channel.
//
// HalfKAv2_hm keys every feature of a perspective by that perspective's own king
// square (through both the king bucket and the horizontal mirror), so when the
// side to move moves its king - including castling - that whole perspective is
// rebuilt with refreshPerspective; the opponent's perspective is updated
// incrementally, and there the moved king itself appears as a remove/add feature
// pair (kings are encoded in HalfKAv2_hm, unlike HalfKP). All the other cases -
// captures (including en passant, whose captured pawn is not on the destination
// square), promotions, and the castling rook - reduce to a small list of removed
// and added (piece, square) features applied to both perspectives. prev and a
// may not alias.
func (a *KAAccumulator) Update(n *KANetwork, prev *KAAccumulator, pos *position.Position, m position.Move) {
	mover := m.Moved().Color()
	moved := m.Moved()
	from, to := m.From(), m.To()
	captured := m.Captured()
	promo := m.Promotion()
	movedKing := moved.Colorless() == position.King

	// Build the removed/added feature lists. Kings ARE recorded (they are
	// features in HalfKAv2_hm); the mover's own king move is handled on its own
	// perspective by a full refresh below, but the king feature still needs to
	// be carried on the opponent's perspective, so we record it here regardless.
	var removed, added [3]kaFeatureChange
	nr, na := 0, 0
	remove := func(pc position.ColoredPiece, sq uint8) {
		removed[nr] = kaFeatureChange{pc, sq}
		nr++
	}
	add := func(pc position.ColoredPiece, sq uint8) {
		added[na] = kaFeatureChange{pc, sq}
		na++
	}

	// The moving piece leaves its origin square.
	remove(moved, from)

	// The captured piece, if any. En passant captures a pawn that is not on the
	// destination square but one rank behind it (relative to the mover).
	enPassant := moved.Colorless() == position.Pawn &&
		m.PriorEnPassantTarget() != position.NoEnPassant && to == m.PriorEnPassantTarget()
	switch {
	case enPassant:
		capSq := to - 8
		if mover == position.Black {
			capSq = to + 8
		}
		remove(captured, capSq)
	case captured != position.Empty:
		remove(captured, to)
	}

	// The moving piece arrives on its destination, becoming the promoted piece if
	// this is a promotion.
	if promo != position.None {
		add(promo.OfColor(mover), to)
	} else {
		add(moved, to)
	}

	// Castling additionally relocates the rook (the king is handled by refreshing
	// the mover's perspective below, plus the king feature on the opponent's).
	if movedKing && (int(from)-int(to) == 2 || int(to)-int(from) == 2) {
		rook := position.Rook.OfColor(mover)
		var rookFrom, rookTo uint8
		switch to {
		case position.SquareG1:
			rookFrom, rookTo = position.SquareH1, position.SquareF1
		case position.SquareC1:
			rookFrom, rookTo = position.SquareA1, position.SquareD1
		case position.SquareG8:
			rookFrom, rookTo = position.SquareH8, position.SquareF8
		case position.SquareC8:
			rookFrom, rookTo = position.SquareA8, position.SquareD8
		}
		remove(rook, rookFrom)
		add(rook, rookTo)
	}

	for p := 0; p < 2; p++ {
		perspective := position.Color(p)

		// The mover's own king moved: this perspective's king bucket and mirror
		// changed, so nothing can be carried over - rebuild it from the board.
		if movedKing && perspective == mover {
			a.refreshPerspective(n, pos, perspective)
			continue
		}

		// Carry the parent perspective forward, then apply the column deltas. The
		// king square of THIS perspective is unchanged (its own king did not
		// move), so every index is keyed by the same, current king square.
		a.accumulation[p] = prev.accumulation[p]
		a.psqt[p] = prev.psqt[p]
		ksq := pos.KingLocation[p]
		for i := 0; i < nr; i++ {
			index := kaMakeIndex(perspective, removed[i].sq, removed[i].piece, ksq)
			a.subColumn(n, p, index)
		}
		for i := 0; i < na; i++ {
			index := kaMakeIndex(perspective, added[i].sq, added[i].piece, ksq)
			a.addColumn(n, p, index)
		}
	}
	a.computed = true
}
