package position

import "math/bits"

// eval_hce.go holds the positional ("hand-crafted evaluation") terms layered on top of material and
// the piece-square tables in eval_complex.go: mobility, pawn structure, king safety, the bishop pair,
// rooks on open files and a tempo bonus. Every term carries a middlegame and an endgame value, blended
// by game phase in EvalComplex. Values are hand-set to sensible, community-standard magnitudes; the
// natural next step is to fit them with Texel tuning against labelled positions.

// Precomputed masks.
var (
	fileMask     [8]Bitboard     // every square on a file
	adjFilesMask [8]Bitboard     // the files immediately left and right of a file
	passedMask   [2][64]Bitboard // squares ahead of a pawn on its own and adjacent files
	kingShield   [2][64]Bitboard // the shield zone in front of a king (its three files, two ranks ahead)
)

func rankMaskBB(rank int) Bitboard { return Bitboard(0xFF) << uint(rank*8) }

func init() {
	for f := 0; f < 8; f++ {
		var fm Bitboard
		for r := 0; r < 8; r++ {
			fm |= Bitboard(1) << uint(r*8+f)
		}
		fileMask[f] = fm
	}
	for f := 0; f < 8; f++ {
		if f > 0 {
			adjFilesMask[f] |= fileMask[f-1]
		}
		if f < 7 {
			adjFilesMask[f] |= fileMask[f+1]
		}
	}
	for sq := 0; sq < 64; sq++ {
		f, r := sq%8, sq/8
		span := fileMask[f] | adjFilesMask[f]

		var white, black Bitboard
		for rr := r + 1; rr < 8; rr++ {
			white |= span & rankMaskBB(rr)
		}
		for rr := r - 1; rr >= 0; rr-- {
			black |= span & rankMaskBB(rr)
		}
		passedMask[White][sq] = white
		passedMask[Black][sq] = black

		var ws, bs Bitboard
		for _, dr := range []int{1, 2} {
			if r+dr < 8 {
				ws |= span & rankMaskBB(r+dr)
			}
			if r-dr >= 0 {
				bs |= span & rankMaskBB(r-dr)
			}
		}
		kingShield[White][sq] = ws
		kingShield[Black][sq] = bs
	}
}

// Positional weights, in centipawns, as (middlegame, endgame) pairs.
const (
	knightMobMG, knightMobEG = 4, 4
	bishopMobMG, bishopMobEG = 4, 4
	rookMobMG, rookMobEG     = 2, 4
	queenMobMG, queenMobEG   = 1, 2

	doubledMG, doubledEG   = 10, 22
	isolatedMG, isolatedEG = 14, 8

	bishopPairMG, bishopPairEG = 25, 45
	rookOpenMG, rookOpenEG     = 22, 12
	rookSemiMG, rookSemiEG     = 10, 6

	shieldBonus         = 9  // middlegame bonus per friendly pawn shielding the king
	kingOpenFilePenalty = 16 // middlegame penalty per (semi-)open file beside the king
	tempoBonus          = 14 // middlegame bonus for the side to move
)

var (
	passedBonusMG = [8]int{0, 5, 10, 20, 35, 60, 100, 0}
	passedBonusEG = [8]int{0, 12, 24, 48, 80, 130, 200, 0}
)

func popcount(b Bitboard) int { return bits.OnesCount64(uint64(b)) }

// evalPositional returns the white-positive middlegame and endgame positional scores.
func evalPositional(pos *Position) (mg, eg int) {
	occupied := pos.Occupied[White] | pos.Occupied[Black]
	whitePawns := pos.Pieces[Pawn] & pos.Occupied[White]
	blackPawns := pos.Pieces[Pawn] & pos.Occupied[Black]

	wmMG, wmEG := pieceMobility(pos, White, occupied)
	bmMG, bmEG := pieceMobility(pos, Black, occupied)
	mg += wmMG - bmMG
	eg += wmEG - bmEG

	pMG, pEG := pawnStructure(whitePawns, blackPawns)
	mg += pMG
	eg += pEG

	// Bishop pair.
	if popcount(pos.Pieces[Bishop]&pos.Occupied[White]) >= 2 {
		mg += bishopPairMG
		eg += bishopPairEG
	}
	if popcount(pos.Pieces[Bishop]&pos.Occupied[Black]) >= 2 {
		mg -= bishopPairMG
		eg -= bishopPairEG
	}

	// Rooks on open / semi-open files.
	rMG, rEG := rookFiles(pos.Pieces[Rook]&pos.Occupied[White], whitePawns, blackPawns)
	mg += rMG
	eg += rEG
	rMG, rEG = rookFiles(pos.Pieces[Rook]&pos.Occupied[Black], blackPawns, whitePawns)
	mg -= rMG
	eg -= rEG

	// King safety (a middlegame concern), as White minus Black.
	mg += kingSafety(pos.KingLocation[White], whitePawns, White)
	mg -= kingSafety(pos.KingLocation[Black], blackPawns, Black)

	// Tempo.
	if pos.SideToMove == White {
		mg += tempoBonus
	} else {
		mg -= tempoBonus
	}

	return mg, eg
}

// pieceMobility scores a colour's piece mobility (the number of squares each piece can move to or
// capture on), which rewards active, well-placed pieces.
func pieceMobility(pos *Position, color Color, occupied Bitboard) (mg, eg int) {
	own := pos.Occupied[color]

	for bb := pos.Pieces[Knight] & own; bb != 0; bb &= bb - 1 {
		sq := bits.TrailingZeros64(uint64(bb))
		m := popcount(knightMoves[sq] &^ own)
		mg += m * knightMobMG
		eg += m * knightMobEG
	}
	for bb := pos.Pieces[Bishop] & own; bb != 0; bb &= bb - 1 {
		sq := uint8(bits.TrailingZeros64(uint64(bb)))
		m := popcount(bishopAttacks(sq, occupied) &^ own)
		mg += m * bishopMobMG
		eg += m * bishopMobEG
	}
	for bb := pos.Pieces[Rook] & own; bb != 0; bb &= bb - 1 {
		sq := uint8(bits.TrailingZeros64(uint64(bb)))
		m := popcount(rookAttacks(sq, occupied) &^ own)
		mg += m * rookMobMG
		eg += m * rookMobEG
	}
	for bb := pos.Pieces[Queen] & own; bb != 0; bb &= bb - 1 {
		sq := uint8(bits.TrailingZeros64(uint64(bb)))
		m := popcount((rookAttacks(sq, occupied)|bishopAttacks(sq, occupied)) &^ own)
		mg += m * queenMobMG
		eg += m * queenMobEG
	}
	return mg, eg
}

// pawnStructure scores doubled, isolated and passed pawns, white-positive.
func pawnStructure(whitePawns, blackPawns Bitboard) (mg, eg int) {
	for f := 0; f < 8; f++ {
		wc := popcount(whitePawns & fileMask[f])
		bc := popcount(blackPawns & fileMask[f])

		if wc > 1 {
			mg -= (wc - 1) * doubledMG
			eg -= (wc - 1) * doubledEG
		}
		if bc > 1 {
			mg += (bc - 1) * doubledMG
			eg += (bc - 1) * doubledEG
		}
		if wc > 0 && whitePawns&adjFilesMask[f] == 0 {
			mg -= wc * isolatedMG
			eg -= wc * isolatedEG
		}
		if bc > 0 && blackPawns&adjFilesMask[f] == 0 {
			mg += bc * isolatedMG
			eg += bc * isolatedEG
		}
	}

	for bb := whitePawns; bb != 0; bb &= bb - 1 {
		sq := bits.TrailingZeros64(uint64(bb))
		if passedMask[White][sq]&blackPawns == 0 {
			r := sq / 8
			mg += passedBonusMG[r]
			eg += passedBonusEG[r]
		}
	}
	for bb := blackPawns; bb != 0; bb &= bb - 1 {
		sq := bits.TrailingZeros64(uint64(bb))
		if passedMask[Black][sq]&whitePawns == 0 {
			r := 7 - sq/8
			mg -= passedBonusMG[r]
			eg -= passedBonusEG[r]
		}
	}
	return mg, eg
}

// rookFiles rewards a colour's rooks for standing on open files (no pawns) and semi-open files (no
// friendly pawns).
func rookFiles(rooks, ownPawns, enemyPawns Bitboard) (mg, eg int) {
	for bb := rooks; bb != 0; bb &= bb - 1 {
		f := bits.TrailingZeros64(uint64(bb)) % 8
		if ownPawns&fileMask[f] != 0 {
			continue
		}
		if enemyPawns&fileMask[f] == 0 {
			mg += rookOpenMG
			eg += rookOpenEG
		} else {
			mg += rookSemiMG
			eg += rookSemiEG
		}
	}
	return mg, eg
}

// kingSafety scores the pawn cover in front of a king (middlegame only): a bonus per shield pawn, and a
// penalty for each (semi-)open file beside the king.
func kingSafety(kingSquare uint8, ownPawns Bitboard, color Color) int {
	mg := popcount(ownPawns&kingShield[color][kingSquare]) * shieldBonus

	kingFile := int(kingSquare % 8)
	for df := -1; df <= 1; df++ {
		f := kingFile + df
		if f < 0 || f > 7 {
			continue
		}
		if ownPawns&fileMask[f] == 0 {
			mg -= kingOpenFilePenalty
		}
	}
	return mg
}
