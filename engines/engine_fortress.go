package engines

import (
	"github.com/ollybritton/StupidChess/position"
	"github.com/ollybritton/StupidChess/search"
)

// fortressDepth is how many plies the fortress looks ahead. Depth 3 lets it see whether a move invites
// a capture on the reply, so it can keep the position shut, while staying fast.
const fortressDepth = 3

// Fortress weights, all in (roughly) centipawn units so material dominates. Own material is valued
// above enemy material so that even an equal trade is unwelcome: the fortress would rather keep all of
// its pieces huddled than swap any of them off.
const (
	fortressOwnMaterial   = 12 // clinging to our own pieces (a piece is worth ~12x its centipawns to us)
	fortressEnemyMaterial = 10 // capturing is fine, but worth less, so equal trades come out negative
	fortressHuddle        = 2  // penalty per unit of squared distance from a piece to our own king
	fortressPawnAdvance   = 8  // penalty per rank a pawn has crept forward (advancing opens the position)
)

// NewEngineFortress builds the ultra-defensive engine: it keeps everything bunched around its king,
// leaves its pawns at home to keep the position closed, and refuses to let its pieces be traded.
func NewEngineFortress() *SimpleEngine {
	return NewSimpleEngine(
		"fortress",
		"Olly Britton",
		"Hunkers down behind a wall of pawns: keeps every piece huddled around its king, leaves its pawns at home, and clings to its material so the position stays shut and nothing gets traded.",
		func(pos *position.Position, _ search.SearchOptions) (position.Move, error) {
			me := pos.SideToMove
			move := bestPositionalMove(pos, fortressDepth, me, func(p *position.Position) int {
				return fortressEval(p, me)
			})
			return move, nil
		},
	)
}

// fortressEval scores a position from the fortress's (me's) fixed point of view: higher is more
// defensible. It rewards keeping material, keeping pieces close to the king, and keeping pawns home.
func fortressEval(pos *position.Position, me position.Color) int {
	enemy := me.Invert()
	kingSquare := pos.KingLocation[me]
	kingFile, kingRank := int(kingSquare%8), int(kingSquare/8)

	score := 0
	for sq := 0; sq < 64; sq++ {
		piece := pos.Squares[sq]
		if piece == position.Empty {
			continue
		}

		value := fortressPieceValue(piece.Colorless())

		switch piece.Color() {
		case me:
			score += fortressOwnMaterial * value

			// Huddle: penalise distance from our own king, so the pieces cluster defensively.
			df := sq%8 - kingFile
			dr := sq/8 - kingRank
			score -= fortressHuddle * (df*df + dr*dr)

			// Closedness: penalise pawns that have advanced, since pushing pawns opens lines.
			if piece.Colorless() == position.Pawn {
				score -= fortressPawnAdvance * pawnAdvancement(me, sq/8)
			}
		case enemy:
			score -= fortressEnemyMaterial * value
		}
	}

	return score
}

// pawnAdvancement reports how many ranks a pawn on the given rank has advanced from its home rank.
func pawnAdvancement(c position.Color, rank int) int {
	if c == position.White {
		return rank - 1 // white pawns start on rank 2 (index 1)
	}
	return 6 - rank // black pawns start on rank 7 (index 6)
}

// fortressPieceValue is a local centipawn table (the position package keeps its own table unexported).
func fortressPieceValue(p position.Piece) int {
	switch p {
	case position.Pawn:
		return 100
	case position.Knight:
		return 320
	case position.Bishop:
		return 330
	case position.Rook:
		return 500
	case position.Queen:
		return 900
	default:
		return 0
	}
}
