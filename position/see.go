package position

// Static Exchange Evaluation. SEE answers "if I make this capture and both sides keep recapturing on
// the square with their least valuable attacker, what is the net material result?" without searching.
// The search uses it to throw away obviously losing captures in quiescence (and could order captures
// with it). See https://www.chessprogramming.org/Static_Exchange_Evaluation.

// seePieceValue is the centipawn value used in exchanges. The king is huge so a swap never profitably
// "captures" it (capturing into a defended square is illegal anyway, which SEE doesn't model).
var seePieceValue = [7]int{
	Pawn: 100, Knight: 320, Bishop: 330, Rook: 500, Queen: 900, King: 10000,
}

// whitePawnAttacks[sq] / blackPawnAttacks[sq] are the squares a pawn of that colour on sq attacks.
var whitePawnAttacks, blackPawnAttacks [64]Bitboard

func init() {
	for sq := 0; sq < 64; sq++ {
		file := sq % 8
		if file > 0 && sq+7 < 64 {
			whitePawnAttacks[sq] |= Bitboard(1) << uint(sq+7)
		}
		if file < 7 && sq+9 < 64 {
			whitePawnAttacks[sq] |= Bitboard(1) << uint(sq+9)
		}
		if file > 0 && sq-9 >= 0 {
			blackPawnAttacks[sq] |= Bitboard(1) << uint(sq-9)
		}
		if file < 7 && sq-7 >= 0 {
			blackPawnAttacks[sq] |= Bitboard(1) << uint(sq-7)
		}
	}
}

func rookAttacks(square uint8, occupied Bitboard) Bitboard {
	blockers := occupied & rookMasks[square]
	key := (uint64(blockers) * rookMagics[square].multiplier) >> rookMagics[square].shift
	return rookMoves[square][key]
}

func bishopAttacks(square uint8, occupied Bitboard) Bitboard {
	blockers := occupied & bishopMasks[square]
	key := (uint64(blockers) * bishopMagics[square].multiplier) >> bishopMagics[square].shift
	return bishopMoves[square][key]
}

// attackersTo returns every piece (either colour) that attacks `square` given the occupancy. Passing a
// reduced occupancy is how the swap reveals x-ray attackers behind a piece that has just been removed.
func (p *Position) attackersTo(square uint8, occupied Bitboard) Bitboard {
	knights := knightMoves[square] & p.Pieces[Knight]
	kings := kingMoves[square] & p.Pieces[King]
	bishopsQueens := bishopAttacks(square, occupied) & (p.Pieces[Bishop] | p.Pieces[Queen])
	rooksQueens := rookAttacks(square, occupied) & (p.Pieces[Rook] | p.Pieces[Queen])
	whitePawnsAttacking := blackPawnAttacks[square] & p.Pieces[Pawn] & p.Occupied[White]
	blackPawnsAttacking := whitePawnAttacks[square] & p.Pieces[Pawn] & p.Occupied[Black]

	return (knights | kings | bishopsQueens | rooksQueens | whitePawnsAttacking | blackPawnsAttacking) & occupied
}

// leastValuableAttacker finds the cheapest piece of `side` in the attacker set, returning its square,
// value, and whether one exists.
func (p *Position) leastValuableAttacker(attackers Bitboard, side Color) (uint8, int, bool) {
	sideAttackers := attackers & p.Occupied[side]
	if sideAttackers == 0 {
		return 0, 0, false
	}
	for _, piece := range []Piece{Pawn, Knight, Bishop, Rook, Queen, King} {
		subset := sideAttackers & p.Pieces[piece]
		if subset != 0 {
			return subset.FirstOn(), seePieceValue[piece], true
		}
	}
	return 0, 0, false
}

func seeMax(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// SEE returns the static exchange evaluation of a capture, in centipawns, from the mover's point of
// view (positive means the capture wins material even after all recaptures). Non-captures return 0.
func (p *Position) SEE(move Move) int {
	captured := move.Captured()
	if captured == Empty {
		return 0 // SEE is defined for captures (en passant is treated as neutral)
	}

	to := move.To()

	var gain [32]int
	gain[0] = seePieceValue[captured.Colorless()]
	onSquareValue := seePieceValue[move.Moved().Colorless()] // value of the piece now standing on `to`

	occupied := (p.Occupied[White] | p.Occupied[Black]) &^ (Bitboard(1) << move.From())
	attackers := p.attackersTo(to, occupied)
	side := move.Moved().Color().Invert()

	d := 0
	for {
		d++
		gain[d] = onSquareValue - gain[d-1]
		if seeMax(-gain[d-1], gain[d]) < 0 {
			break // both sides would rather stop here
		}

		square, value, found := p.leastValuableAttacker(attackers, side)
		if !found {
			break
		}
		onSquareValue = value
		occupied &^= Bitboard(1) << square

		// Reveal any slider that was x-raying through the captured-from square.
		attackers |= rookAttacks(to, occupied) & (p.Pieces[Rook] | p.Pieces[Queen])
		attackers |= bishopAttacks(to, occupied) & (p.Pieces[Bishop] | p.Pieces[Queen])
		attackers &= occupied

		side = side.Invert()
	}

	// Negamax the gains back: at each step the side to move only recaptures if it improves on standing pat.
	for d--; d > 0; d-- {
		gain[d-1] = -seeMax(-gain[d-1], gain[d])
	}
	return gain[0]
}
