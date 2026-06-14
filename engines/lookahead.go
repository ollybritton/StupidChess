package engines

import (
	"math"

	"github.com/ollybritton/StupidChess/position"
)

// This file gives the "personality" engines (suicide-king, sprinter) a shallow look-ahead so they
// pursue their goal across several moves instead of greedily one move at a time. For example the
// suicide king will shift a pawn out of the way now so that its king can march forward next move,
// rather than shuffling random pieces until the king happens to have somewhere to go.
//
// Both searches use pseudolegal generation with lazy legality (MakeMove reports illegality) and simple
// alpha-beta, and run to a fixed shallow depth so a move is chosen quickly.

func absInt(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// moveDistance is the Chebyshev (king-move) distance a move travels across the board.
func moveDistance(m position.Move) int {
	df := absInt(int(m.From()%8) - int(m.To()%8))
	dr := absInt(int(m.From()/8) - int(m.To()/8))
	return maxInt(df, dr)
}

// kingDistanceSquared is the squared Euclidean distance between the two kings.
func kingDistanceSquared(pos *position.Position) int {
	wk := pos.KingLocation[position.White]
	bk := pos.KingLocation[position.Black]
	df := int(wk%8) - int(bk%8)
	dr := int(wk/8) - int(bk/8)
	return df*df + dr*dr
}

// bestPositionalMove returns the move that, looking `depth` plies ahead, best serves a positional goal
// for the side `me`: it maximises eval(position) while the opponent minimises it. eval is from a fixed
// perspective (higher = better for `me`).
func bestPositionalMove(pos *position.Position, depth int, me position.Color, eval func(*position.Position) int) position.Move {
	best := position.NoMove
	bestScore := math.MinInt

	for _, m := range pos.MovesPseudolegal().AsSlice() {
		if !pos.MakeMove(m) {
			continue
		}
		s := positionalMinimax(pos, depth-1, me, eval, math.MinInt, math.MaxInt)
		pos.UndoMove(m)

		if best == position.NoMove || s > bestScore {
			best, bestScore = m, s
		}
	}
	return best
}

func positionalMinimax(pos *position.Position, depth int, me position.Color, eval func(*position.Position) int, alpha, beta int) int {
	if depth <= 0 {
		return eval(pos)
	}

	maximizing := pos.SideToMove == me
	v := math.MaxInt
	if maximizing {
		v = math.MinInt
	}
	legal := 0

	for _, m := range pos.MovesPseudolegal().AsSlice() {
		if !pos.MakeMove(m) {
			continue
		}
		legal++
		s := positionalMinimax(pos, depth-1, me, eval, alpha, beta)
		pos.UndoMove(m)

		if maximizing {
			if s > v {
				v = s
			}
			if v > alpha {
				alpha = v
			}
		} else {
			if s < v {
				v = s
			}
			if v < beta {
				beta = v
			}
		}
		if alpha >= beta {
			break
		}
	}

	if legal == 0 {
		return eval(pos) // checkmate or stalemate: just score the static position
	}
	return v
}

// bestSprinterMove returns the move that maximises the total distance the sprinter travels over the
// next `depth` plies of its own moves (the opponent plays to minimise it). It avoids repeating the
// previous piece type when any other legal move exists.
func bestSprinterMove(pos *position.Position, depth int, prevPiece position.Piece) position.Move {
	me := pos.SideToMove

	// First try only moves that don't repeat the previous piece type; if that leaves nothing legal,
	// fall back to allowing a repeat.
	for _, allowRepeat := range []bool{false, true} {
		best := position.NoMove
		bestScore := math.MinInt

		for _, m := range pos.MovesPseudolegal().AsSlice() {
			if !allowRepeat && m.Moved().Colorless() == prevPiece {
				continue
			}
			if !pos.MakeMove(m) {
				continue
			}
			s := moveDistance(m) + sprinterMinimax(pos, depth-1, me, math.MinInt, math.MaxInt)
			pos.UndoMove(m)

			if best == position.NoMove || s > bestScore {
				best, bestScore = m, s
			}
		}

		if best != position.NoMove {
			return best
		}
	}
	return position.NoMove
}

func sprinterMinimax(pos *position.Position, depth int, me position.Color, alpha, beta int) int {
	if depth <= 0 {
		return 0
	}

	maximizing := pos.SideToMove == me
	v := math.MaxInt
	if maximizing {
		v = math.MinInt
	}
	legal := 0

	for _, m := range pos.MovesPseudolegal().AsSlice() {
		if !pos.MakeMove(m) {
			continue
		}
		legal++

		s := sprinterMinimax(pos, depth-1, me, alpha, beta)
		if maximizing {
			s += moveDistance(m) // only the sprinter's own moves count toward its distance
		}
		pos.UndoMove(m)

		if maximizing {
			if s > v {
				v = s
			}
			if v > alpha {
				alpha = v
			}
		} else {
			if s < v {
				v = s
			}
			if v < beta {
				beta = v
			}
		}
		if alpha >= beta {
			break
		}
	}

	if legal == 0 {
		return 0
	}
	return v
}
