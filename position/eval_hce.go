package position

import "math/bits"

// eval_hce.go holds the positional ("hand-crafted evaluation") terms layered on top of material and
// the piece-square tables in eval_complex.go: mobility, pawn structure, king safety, the bishop pair,
// rooks on open files and a tempo bonus. Every term carries a middlegame and an endgame value, blended
// by game phase in EvalWith.
//
// All the weights live in EvalParams, so an engine can build its own evaluator with MakeEvaluator by
// copying DefaultEvalParams and re-weighting terms (e.g. one that loves king safety). See
// EvaluatorInfo / MakeEvaluator.

// Pair is a (middlegame, endgame) weight, blended by game phase.
type Pair struct{ MG, EG int }

// EvalParams holds the weight of every positional evaluation term. Copy DefaultEvalParams and tweak.
type EvalParams struct {
	KnightMobility Pair // per square a knight can reach
	BishopMobility Pair
	RookMobility   Pair
	QueenMobility  Pair

	Doubled  Pair // penalty per extra pawn on a file
	Isolated Pair // penalty per pawn with no neighbours
	Passed   [8]Pair // bonus by rank advanced (index = rank, 0 unused)

	BishopPair Pair
	RookOpen   Pair // rook on a file with no pawns
	RookSemi   Pair // rook on a file with no friendly pawns

	KingShield   int // middlegame bonus per friendly pawn shielding the king
	KingOpenFile int // middlegame penalty per (semi-)open file beside the king
	Tempo        int // middlegame bonus for the side to move
}

// DefaultEvalParams are the standard weights used by EvalComplex (and tryhard / fortress).
var DefaultEvalParams = EvalParams{
	KnightMobility: Pair{4, 4},
	BishopMobility: Pair{4, 4},
	RookMobility:   Pair{2, 4},
	QueenMobility:  Pair{1, 2},

	Doubled:  Pair{10, 22},
	Isolated: Pair{14, 8},
	Passed: [8]Pair{
		{0, 0}, {5, 12}, {10, 24}, {20, 48}, {35, 80}, {60, 130}, {100, 200}, {0, 0},
	},

	BishopPair: Pair{25, 45},
	RookOpen:   Pair{22, 12},
	RookSemi:   Pair{10, 6},

	KingShield:   9,
	KingOpenFile: 16,
	Tempo:        14,
}

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

func popcount(b Bitboard) int { return bits.OnesCount64(uint64(b)) }

// evalPositional returns the white-positive middlegame and endgame positional scores under p.
func evalPositional(pos *Position, p *EvalParams) (mg, eg int) {
	occupied := pos.Occupied[White] | pos.Occupied[Black]
	whitePawns := pos.Pieces[Pawn] & pos.Occupied[White]
	blackPawns := pos.Pieces[Pawn] & pos.Occupied[Black]

	wmMG, wmEG := pieceMobility(pos, White, occupied, p)
	bmMG, bmEG := pieceMobility(pos, Black, occupied, p)
	mg += wmMG - bmMG
	eg += wmEG - bmEG

	pMG, pEG := pawnStructure(whitePawns, blackPawns, p)
	mg += pMG
	eg += pEG

	if popcount(pos.Pieces[Bishop]&pos.Occupied[White]) >= 2 {
		mg += p.BishopPair.MG
		eg += p.BishopPair.EG
	}
	if popcount(pos.Pieces[Bishop]&pos.Occupied[Black]) >= 2 {
		mg -= p.BishopPair.MG
		eg -= p.BishopPair.EG
	}

	rMG, rEG := rookFiles(pos.Pieces[Rook]&pos.Occupied[White], whitePawns, blackPawns, p)
	mg += rMG
	eg += rEG
	rMG, rEG = rookFiles(pos.Pieces[Rook]&pos.Occupied[Black], blackPawns, whitePawns, p)
	mg -= rMG
	eg -= rEG

	mg += kingSafety(pos.KingLocation[White], whitePawns, White, p)
	mg -= kingSafety(pos.KingLocation[Black], blackPawns, Black, p)

	if pos.SideToMove == White {
		mg += p.Tempo
	} else {
		mg -= p.Tempo
	}

	return mg, eg
}

// pieceMobility scores a colour's piece mobility (squares each piece can move to or capture on).
func pieceMobility(pos *Position, color Color, occupied Bitboard, p *EvalParams) (mg, eg int) {
	own := pos.Occupied[color]

	for bb := pos.Pieces[Knight] & own; bb != 0; bb &= bb - 1 {
		sq := bits.TrailingZeros64(uint64(bb))
		m := popcount(knightMoves[sq] &^ own)
		mg += m * p.KnightMobility.MG
		eg += m * p.KnightMobility.EG
	}
	for bb := pos.Pieces[Bishop] & own; bb != 0; bb &= bb - 1 {
		sq := uint8(bits.TrailingZeros64(uint64(bb)))
		m := popcount(bishopAttacks(sq, occupied) &^ own)
		mg += m * p.BishopMobility.MG
		eg += m * p.BishopMobility.EG
	}
	for bb := pos.Pieces[Rook] & own; bb != 0; bb &= bb - 1 {
		sq := uint8(bits.TrailingZeros64(uint64(bb)))
		m := popcount(rookAttacks(sq, occupied) &^ own)
		mg += m * p.RookMobility.MG
		eg += m * p.RookMobility.EG
	}
	for bb := pos.Pieces[Queen] & own; bb != 0; bb &= bb - 1 {
		sq := uint8(bits.TrailingZeros64(uint64(bb)))
		m := popcount((rookAttacks(sq, occupied)|bishopAttacks(sq, occupied)) &^ own)
		mg += m * p.QueenMobility.MG
		eg += m * p.QueenMobility.EG
	}
	return mg, eg
}

// pawnStructure scores doubled, isolated and passed pawns, white-positive.
func pawnStructure(whitePawns, blackPawns Bitboard, p *EvalParams) (mg, eg int) {
	for f := 0; f < 8; f++ {
		wc := popcount(whitePawns & fileMask[f])
		bc := popcount(blackPawns & fileMask[f])

		if wc > 1 {
			mg -= (wc - 1) * p.Doubled.MG
			eg -= (wc - 1) * p.Doubled.EG
		}
		if bc > 1 {
			mg += (bc - 1) * p.Doubled.MG
			eg += (bc - 1) * p.Doubled.EG
		}
		if wc > 0 && whitePawns&adjFilesMask[f] == 0 {
			mg -= wc * p.Isolated.MG
			eg -= wc * p.Isolated.EG
		}
		if bc > 0 && blackPawns&adjFilesMask[f] == 0 {
			mg += bc * p.Isolated.MG
			eg += bc * p.Isolated.EG
		}
	}

	for bb := whitePawns; bb != 0; bb &= bb - 1 {
		sq := bits.TrailingZeros64(uint64(bb))
		if passedMask[White][sq]&blackPawns == 0 {
			mg += p.Passed[sq/8].MG
			eg += p.Passed[sq/8].EG
		}
	}
	for bb := blackPawns; bb != 0; bb &= bb - 1 {
		sq := bits.TrailingZeros64(uint64(bb))
		if passedMask[Black][sq]&whitePawns == 0 {
			mg -= p.Passed[7-sq/8].MG
			eg -= p.Passed[7-sq/8].EG
		}
	}
	return mg, eg
}

// rookFiles rewards rooks on open and semi-open files.
func rookFiles(rooks, ownPawns, enemyPawns Bitboard, p *EvalParams) (mg, eg int) {
	for bb := rooks; bb != 0; bb &= bb - 1 {
		f := bits.TrailingZeros64(uint64(bb)) % 8
		if ownPawns&fileMask[f] != 0 {
			continue
		}
		if enemyPawns&fileMask[f] == 0 {
			mg += p.RookOpen.MG
			eg += p.RookOpen.EG
		} else {
			mg += p.RookSemi.MG
			eg += p.RookSemi.EG
		}
	}
	return mg, eg
}

// kingSafety scores the pawn cover in front of a king (middlegame only).
func kingSafety(kingSquare uint8, ownPawns Bitboard, color Color, p *EvalParams) int {
	mg := popcount(ownPawns&kingShield[color][kingSquare]) * p.KingShield

	kingFile := int(kingSquare % 8)
	for df := -1; df <= 1; df++ {
		f := kingFile + df
		if f < 0 || f > 7 {
			continue
		}
		if ownPawns&fileMask[f] == 0 {
			mg -= p.KingOpenFile
		}
	}
	return mg
}
