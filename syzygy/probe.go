package syzygy

import "math/bits"

// This file ports the probe-side logic from tbprobe.c: the bitboard position
// representation (struct pos), the material key (calc_key), probe_wdl_table,
// probe_ab and probe_wdl, plus the minimal move generation needed to resolve
// captures. Only what the WDL probe needs is implemented.

// pos mirrors struct pos in tbprobe.c (a1=bit0 .. h8=bit63).
type pos struct {
	white, black            uint64
	kings, queens, rooks    uint64
	bishops, knights, pawns uint64
	ep                      int  // en-passant target square, 0 if none
	turn                    bool // true = white to move
}

func popcount(b uint64) int  { return bits.OnesCount64(b) }
func lsb(b uint64) int       { return bits.TrailingZeros64(b) }
func poplsb(b uint64) uint64 { return b & (b - 1) }
func board(sq int) uint64    { return uint64(1) << uint(sq) }
func rankOf(sq int) int      { return sq >> 3 }
func fileOf(sq int) int      { return sq & 7 }

// calcKey ports calc_key.
func calcKey(p *pos, mirror bool) uint64 {
	white, black := p.white, p.black
	if mirror {
		white, black = black, white
	}
	return uint64(popcount(white&p.queens))*primeWhiteQueen +
		uint64(popcount(white&p.rooks))*primeWhiteRook +
		uint64(popcount(white&p.bishops))*primeWhiteBishop +
		uint64(popcount(white&p.knights))*primeWhiteKnight +
		uint64(popcount(white&p.pawns))*primeWhitePawn +
		uint64(popcount(black&p.queens))*primeBlackQueen +
		uint64(popcount(black&p.rooks))*primeBlackRook +
		uint64(popcount(black&p.bishops))*primeBlackBishop +
		uint64(popcount(black&p.knights))*primeBlackKnight +
		uint64(popcount(black&p.pawns))*primeBlackPawn
}

// getPieces returns the bitboard for a tbcore piece code (1..6 white, 9..14
// black), matching get_pieces in tbprobe.c.
func getPieces(p *pos, code uint8) uint64 {
	switch code {
	case tbQueen:
		return p.queens & p.white
	case tbRook:
		return p.rooks & p.white
	case tbBishop:
		return p.bishops & p.white
	case tbKnight:
		return p.knights & p.white
	case tbPawn:
		return p.pawns & p.white
	case tbKing:
		return p.kings & p.white
	case tbQueen | 8:
		return p.queens & p.black
	case tbRook | 8:
		return p.rooks & p.black
	case tbBishop | 8:
		return p.bishops & p.black
	case tbKnight | 8:
		return p.knights & p.black
	case tbPawn | 8:
		return p.pawns & p.black
	case tbKing | 8:
		return p.kings & p.black
	}
	return 0
}

// probeWDLTable ports probe_wdl_table. It returns the WDL value (-2..2) and
// whether the probe succeeded.
func (tb *Tablebases) probeWDLTable(p *pos) (int, bool) {
	key := calcKey(p, false)

	// KvK is a draw.
	if key == 0 {
		return 0, true
	}

	t := tb.byKey[key]
	if t == nil {
		return 0, false
	}
	// Material keys ignore kings, so a position that has lost a king during the
	// capture search collides with a same-non-king-material table (e.g. a
	// kingless KBvK). Such a position has the wrong total piece count and must
	// not be encoded: a bad index drives decompress_pairs into a
	// non-terminating bitstream. The reference never hits this because it
	// assumes legal input; we verify the count to stay safe.
	if popcount(p.white|p.black) != t.num ||
		popcount(p.kings&p.white) != 1 || popcount(p.kings&p.black) != 1 {
		return 0, false
	}
	if err := tb.ensureReady(t); err != nil {
		return 0, false
	}

	var bside int
	var cmirror, mirror int
	if !t.symmetric {
		if key != t.key {
			cmirror = 8
			mirror = 0x38
			bside = b2i(p.turn)
		} else {
			cmirror, mirror = 0, 0
			bside = b2i(!p.turn)
		}
	} else {
		if p.turn {
			cmirror, mirror = 0, 0
		} else {
			cmirror, mirror = 8, 0x38
		}
		bside = 0
	}

	pArr := make([]int, tbPieces)

	if !t.hasPawns {
		b := t.buckets[0][bside]
		i := 0
		for i < t.num {
			bb := getPieces(p, b.pieces[i]^uint8(cmirror))
			for bb != 0 {
				pArr[i] = lsb(bb)
				i++
				bb = poplsb(bb)
			}
		}
		idx := encodePiece(t.encType, t.num, b.norm[:], pArr, b.factor[:])
		res := decompressPairs(b.precomp, idx)
		return int(res) - 2, true
	}

	// Pawn table.
	k := t.buckets[0][0].pieces[0] ^ uint8(cmirror)
	bb := getPieces(p, k)
	i := 0
	for bb != 0 {
		pArr[i] = lsb(bb) ^ mirror
		i++
		bb = poplsb(bb)
	}
	f := pawnFile(int(t.pawns[0]), pArr)
	b := t.buckets[f][bside]
	for i < t.num {
		bb = getPieces(p, b.pieces[i]^uint8(cmirror))
		for bb != 0 {
			pArr[i] = lsb(bb) ^ mirror
			i++
			bb = poplsb(bb)
		}
	}
	idx := encodePawn(t.num, int(t.pawns[0]), int(t.pawns[1]), b.norm[:], pArr, b.factor[:])
	res := decompressPairs(b.precomp, idx)
	return int(res) - 2, true
}

// --- minimal move generation for capture resolution (probe_ab) ---

// move encodes promote(3)|from(6)|to(6) like tbprobe.c.
type tbMove uint16

func makeMove(promote, from, to int) tbMove {
	return tbMove(((promote & 0x7) << 12) | ((from & 0x3f) << 6) | (to & 0x3f))
}
func (m tbMove) from() int     { return int((m >> 6) & 0x3f) }
func (m tbMove) to() int       { return int(m & 0x3f) }
func (m tbMove) promotes() int { return int((m >> 12) & 0x7) }

const (
	promNone   = 0
	promQueen  = 1
	promRook   = 2
	promBishop = 3
	promKnight = 4
)

// genCapturesOrPromotions ports gen_captures_or_promotions.
func genCapturesOrPromotions(p *pos, moves []tbMove) []tbMove {
	occ := p.white | p.black
	var us, them uint64
	if p.turn {
		us, them = p.white, p.black
	} else {
		us, them = p.black, p.white
	}

	// The reference assumes a legal position (both kings present). A king
	// capture during the alpha-beta search would only arise from an illegal
	// input, which ProbeWDL rejects via probeLegal before reaching here. Guard
	// anyway so the package can never index out of range on a kingless side.
	if p.kings&us == 0 {
		return moves
	}

	from := lsb(p.kings & us)
	for att := kingAttacks(from) & them; att != 0; att = poplsb(att) {
		moves = append(moves, makeMove(promNone, from, lsb(att)))
	}
	for b := us & p.queens; b != 0; b = poplsb(b) {
		from := lsb(b)
		for att := queenAttacks(from, occ) & them; att != 0; att = poplsb(att) {
			moves = append(moves, makeMove(promNone, from, lsb(att)))
		}
	}
	for b := us & p.rooks; b != 0; b = poplsb(b) {
		from := lsb(b)
		for att := rookAttacks(from, occ) & them; att != 0; att = poplsb(att) {
			moves = append(moves, makeMove(promNone, from, lsb(att)))
		}
	}
	for b := us & p.bishops; b != 0; b = poplsb(b) {
		from := lsb(b)
		for att := bishopAttacks(from, occ) & them; att != 0; att = poplsb(att) {
			moves = append(moves, makeMove(promNone, from, lsb(att)))
		}
	}
	for b := us & p.knights; b != 0; b = poplsb(b) {
		from := lsb(b)
		for att := knightAttacks(from) & them; att != 0; att = poplsb(att) {
			moves = append(moves, makeMove(promNone, from, lsb(att)))
		}
	}
	for b := us & p.pawns; b != 0; b = poplsb(b) {
		from := lsb(b)
		att := pawnAttacks(from, p.turn)
		if p.ep != 0 && att&board(p.ep) != 0 {
			moves = append(moves, makeMove(promNone, from, p.ep))
		}
		for a := att & them; a != 0; a = poplsb(a) {
			to := lsb(a)
			moves = addMove(moves, rankOf(to) == 7 || rankOf(to) == 0, from, to)
		}
		if p.turn && rankOf(from) == 6 {
			to := from + 8
			if board(to)&occ == 0 {
				moves = addMove(moves, true, from, to)
			}
		} else if !p.turn && rankOf(from) == 1 {
			to := from - 8
			if board(to)&occ == 0 {
				moves = addMove(moves, true, from, to)
			}
		}
	}
	return moves
}

func addMove(moves []tbMove, promotes bool, from, to int) []tbMove {
	if !promotes {
		return append(moves, makeMove(promNone, from, to))
	}
	return append(moves,
		makeMove(promQueen, from, to),
		makeMove(promKnight, from, to),
		makeMove(promRook, from, to),
		makeMove(promBishop, from, to))
}

// genMoves ports gen_moves (all pseudo-legal moves), used to detect a legal
// non-ep move in the en-passant edge case.
func genMoves(p *pos, moves []tbMove) []tbMove {
	occ := p.white | p.black
	var us, them uint64
	if p.turn {
		us, them = p.white, p.black
	} else {
		us, them = p.black, p.white
	}
	if p.kings&us == 0 {
		return moves
	}
	from := lsb(p.kings & us)
	for att := kingAttacks(from) &^ us; att != 0; att = poplsb(att) {
		moves = append(moves, makeMove(promNone, from, lsb(att)))
	}
	for b := us & p.queens; b != 0; b = poplsb(b) {
		from := lsb(b)
		for att := queenAttacks(from, occ) &^ us; att != 0; att = poplsb(att) {
			moves = append(moves, makeMove(promNone, from, lsb(att)))
		}
	}
	for b := us & p.rooks; b != 0; b = poplsb(b) {
		from := lsb(b)
		for att := rookAttacks(from, occ) &^ us; att != 0; att = poplsb(att) {
			moves = append(moves, makeMove(promNone, from, lsb(att)))
		}
	}
	for b := us & p.bishops; b != 0; b = poplsb(b) {
		from := lsb(b)
		for att := bishopAttacks(from, occ) &^ us; att != 0; att = poplsb(att) {
			moves = append(moves, makeMove(promNone, from, lsb(att)))
		}
	}
	for b := us & p.knights; b != 0; b = poplsb(b) {
		from := lsb(b)
		for att := knightAttacks(from) &^ us; att != 0; att = poplsb(att) {
			moves = append(moves, makeMove(promNone, from, lsb(att)))
		}
	}
	for b := us & p.pawns; b != 0; b = poplsb(b) {
		from := lsb(b)
		var next int
		if p.turn {
			next = from + 8
		} else {
			next = from - 8
		}
		att := pawnAttacks(from, p.turn)
		if p.ep != 0 && att&board(p.ep) != 0 {
			moves = append(moves, makeMove(promNone, from, p.ep))
		}
		att &= them
		if board(next)&occ == 0 {
			att |= board(next)
			var next2 int
			if p.turn {
				next2 = from + 16
			} else {
				next2 = from - 16
			}
			if ((p.turn && rankOf(from) == 1) || (!p.turn && rankOf(from) == 6)) && board(next2)&occ == 0 {
				att |= board(next2)
			}
		}
		for ; att != 0; att = poplsb(att) {
			to := lsb(att)
			moves = addMove(moves, rankOf(to) == 7 || rankOf(to) == 0, from, to)
		}
	}
	return moves
}

func isEnPassant(p *pos, m tbMove) bool {
	var us uint64
	if p.turn {
		us = p.white
	} else {
		us = p.black
	}
	if p.ep == 0 || m.to() != p.ep {
		return false
	}
	return board(m.from())&us&p.pawns != 0
}

// isLegal ports is_legal: the side that just moved must not leave its king in
// check (here, the side NOT to move).
func isLegal(p *pos) bool {
	occ := p.white | p.black
	var us, them uint64
	if p.turn {
		us, them = p.black, p.white
	} else {
		us, them = p.white, p.black
	}
	// Reject any move that left a side without a king. The reference assumes a
	// legal input where kings are never capturable, so gen_captures never yields
	// a king capture. We are defensive: if the move captured the opponent's king
	// (them) or somehow removed our own (us), the position is not a real chess
	// position and must not be probed (a kingless side shares KBvK/KNvK material
	// keys and would drive decompress_pairs into a non-terminating bitstream).
	if p.kings&us == 0 || p.kings&them == 0 {
		return false
	}
	sq := lsb(p.kings & us)
	if kingAttacks(sq)&(p.kings&them) != 0 {
		return false
	}
	ratt := rookAttacks(sq, occ)
	batt := bishopAttacks(sq, occ)
	if ratt&(p.rooks&them) != 0 {
		return false
	}
	if batt&(p.bishops&them) != 0 {
		return false
	}
	if (ratt|batt)&(p.queens&them) != 0 {
		return false
	}
	if knightAttacks(sq)&(p.knights&them) != 0 {
		return false
	}
	if pawnAttacks(sq, !p.turn)&(p.pawns&them) != 0 {
		return false
	}
	return true
}

// probeLegal returns true if the position is legal to probe: the king of the
// side that is NOT to move must not be under attack by the side to move (i.e.
// the previous mover did not leave the moved-into-check king en prise). It is a
// guard so that ProbeWDL never panics on an illegal position. It is the same
// test as is_legal in tbprobe.c but expressed directly on a *pos that has not
// yet had its turn flipped.
func probeLegal(p *pos) bool {
	occ := p.white | p.black
	var us, them uint64
	if p.turn {
		us, them = p.black, p.white // side not to move = black
	} else {
		us, them = p.white, p.black
	}
	king := p.kings & us
	if king == 0 {
		return false
	}
	sq := lsb(king)
	if kingAttacks(sq)&(p.kings&them) != 0 {
		return false
	}
	ratt := rookAttacks(sq, occ)
	batt := bishopAttacks(sq, occ)
	if ratt&(p.rooks&them) != 0 {
		return false
	}
	if batt&(p.bishops&them) != 0 {
		return false
	}
	if (ratt|batt)&(p.queens&them) != 0 {
		return false
	}
	if knightAttacks(sq)&(p.knights&them) != 0 {
		return false
	}
	// Matches is_legal in tbprobe.c: pawn_attacks(sq, !pos->turn) & (pawns&them).
	if pawnAttacks(sq, !p.turn)&(p.pawns&them) != 0 {
		return false
	}
	return true
}

func doBBMove(b uint64, from, to int) uint64 {
	return (b & ^board(to) & ^board(from)) | (((b >> uint(from)) & 1) << uint(to))
}

// doMove ports do_move; returns the new position and whether it is legal.
func doMove(p0 *pos, m tbMove) (pos, bool) {
	from, to, promotes := m.from(), m.to(), m.promotes()
	var p pos
	p.turn = !p0.turn
	p.white = doBBMove(p0.white, from, to)
	p.black = doBBMove(p0.black, from, to)
	p.kings = doBBMove(p0.kings, from, to)
	p.queens = doBBMove(p0.queens, from, to)
	p.rooks = doBBMove(p0.rooks, from, to)
	p.bishops = doBBMove(p0.bishops, from, to)
	p.knights = doBBMove(p0.knights, from, to)
	p.pawns = doBBMove(p0.pawns, from, to)
	p.ep = 0
	if promotes != promNone {
		p.pawns &^= board(to)
		switch promotes {
		case promQueen:
			p.queens |= board(to)
		case promRook:
			p.rooks |= board(to)
		case promBishop:
			p.bishops |= board(to)
		case promKnight:
			p.knights |= board(to)
		}
	} else if board(from)&p0.pawns != 0 {
		if rankOf(from) == 1 && rankOf(to) == 3 &&
			pawnAttacks(from+8, true)&p0.pawns&p0.black != 0 {
			p.ep = from + 8
		} else if rankOf(from) == 6 && rankOf(to) == 4 &&
			pawnAttacks(from-8, false)&p0.pawns&p0.white != 0 {
			p.ep = from - 8
		} else if to == p0.ep {
			var epTo int
			if p0.turn {
				epTo = to - 8
			} else {
				epTo = to + 8
			}
			mask := ^board(epTo)
			p.white &= mask
			p.black &= mask
			p.pawns &= mask
		}
	}
	if !isLegal(&p) {
		return p, false
	}
	return p, true
}

// probeAB ports probe_ab.
func (tb *Tablebases) probeAB(p *pos, alpha, beta int, success *int) int {
	var moves [64]tbMove
	ms := genCapturesOrPromotions(p, moves[:0])
	for _, mv := range ms {
		if isEnPassant(p, mv) {
			continue
		}
		p1, ok := doMove(p, mv)
		if !ok {
			continue
		}
		v := -tb.probeAB(&p1, -beta, -alpha, success)
		if *success == 0 {
			return 0
		}
		if v > alpha {
			if v >= beta {
				*success = 2
				return v
			}
			alpha = v
		}
	}

	v, ok := tb.probeWDLTable(p)
	if !ok {
		*success = 0
		return 0
	}
	if alpha >= v {
		if alpha > 0 {
			*success = 2
		} else {
			*success = 1
		}
		return alpha
	}
	*success = 1
	return v
}

// probeWDL ports probe_wdl. Returns the WDL value and success flag.
func (tb *Tablebases) probeWDL(p *pos) (int, bool) {
	success := 1
	v := tb.probeAB(p, -2, 2, &success)
	if success == 0 {
		return 0, false
	}
	if p.ep == 0 {
		return v, true
	}

	v1 := -3
	var moves [2]tbMove
	ms := genPawnEPCaptures(p, moves[:0])
	for _, mv := range ms {
		p1, ok := doMove(p, mv)
		if !ok {
			continue
		}
		v0 := -tb.probeAB(&p1, -2, 2, &success)
		if success == 0 {
			return 0, false
		}
		if v0 > v1 {
			v1 = v0
		}
	}
	if v1 > -3 {
		if v1 >= v {
			v = v1
		} else if v == 0 {
			var mm [256]tbMove
			all := genMoves(p, mm[:0])
			found := false
			for _, mv := range all {
				if isEnPassant(p, mv) {
					continue
				}
				if _, ok := doMove(p, mv); ok {
					found = true
					break
				}
			}
			if !found {
				v = v1
			}
		}
	}
	return v, true
}

func genPawnEPCaptures(p *pos, moves []tbMove) []tbMove {
	if p.ep == 0 {
		return moves
	}
	ep := board(p.ep)
	to := p.ep
	var us uint64
	if p.turn {
		us = p.white
	} else {
		us = p.black
	}
	for b := us & p.pawns; b != 0; b = poplsb(b) {
		from := lsb(b)
		if pawnAttacks(from, p.turn)&ep != 0 {
			moves = append(moves, makeMove(promNone, from, to))
		}
	}
	return moves
}
