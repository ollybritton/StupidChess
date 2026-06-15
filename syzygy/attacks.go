package syzygy

// Attack generators, ported from the *_attacks functions in tbprobe.c. These
// are computed once at init. Squares are a1=0 .. h8=63.

var kingAttacksTable [64]uint64
var knightAttacksTable [64]uint64
var pawnAttacksTable [2][64]uint64

func init() {
	for s := 0; s < 64; s++ {
		r, f := s>>3, s&7
		var b uint64
		add := func(rr, ff int) {
			if rr >= 0 && rr <= 7 && ff >= 0 && ff <= 7 {
				b |= board(rr*8 + ff)
			}
		}
		add(r-1, f-1)
		add(r-1, f)
		add(r-1, f+1)
		add(r, f-1)
		add(r, f+1)
		add(r+1, f-1)
		add(r+1, f)
		add(r+1, f+1)
		kingAttacksTable[s] = b

		b = 0
		add(r-1, f-2)
		add(r-1, f+2)
		add(r-2, f-1)
		add(r-2, f+1)
		add(r+1, f-2)
		add(r+1, f+2)
		add(r+2, f-1)
		add(r+2, f+1)
		knightAttacksTable[s] = b

		// White pawn (color index 1 == white-to-move in tbprobe's pawn_attacks
		// table indexing where index [1] is the "up" direction).
		var wb uint64
		if r != 7 {
			if f != 0 {
				wb |= board((r+1)*8 + f - 1)
			}
			if f != 7 {
				wb |= board((r+1)*8 + f + 1)
			}
		}
		pawnAttacksTable[1][s] = wb

		var bb uint64
		if r != 0 {
			if f != 0 {
				bb |= board((r-1)*8 + f - 1)
			}
			if f != 7 {
				bb |= board((r-1)*8 + f + 1)
			}
		}
		pawnAttacksTable[0][s] = bb
	}
}

func kingAttacks(s int) uint64   { return kingAttacksTable[s] }
func knightAttacks(s int) uint64 { return knightAttacksTable[s] }

// pawnAttacks returns the pawn attack set; color true = white.
func pawnAttacks(s int, white bool) uint64 {
	if white {
		return pawnAttacksTable[1][s]
	}
	return pawnAttacksTable[0][s]
}

// Sliding attacks computed by ray-casting (simple and correct; the tables are
// tiny so performance is irrelevant for probing).

func slideAttacks(sq int, occ uint64, dirs [][2]int) uint64 {
	r, f := sq>>3, sq&7
	var att uint64
	for _, d := range dirs {
		rr, ff := r+d[0], f+d[1]
		for rr >= 0 && rr <= 7 && ff >= 0 && ff <= 7 {
			t := rr*8 + ff
			att |= board(t)
			if occ&board(t) != 0 {
				break
			}
			rr += d[0]
			ff += d[1]
		}
	}
	return att
}

var rookDirs = [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}}
var bishopDirs = [][2]int{{1, 1}, {1, -1}, {-1, 1}, {-1, -1}}

func rookAttacks(sq int, occ uint64) uint64   { return slideAttacks(sq, occ, rookDirs) }
func bishopAttacks(sq int, occ uint64) uint64 { return slideAttacks(sq, occ, bishopDirs) }
func queenAttacks(sq int, occ uint64) uint64 {
	return rookAttacks(sq, occ) | bishopAttacks(sq, occ)
}
