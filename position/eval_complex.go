package position

// EvalComplex is a centipawn evaluation combining material with piece-square tables, returned from
// White's point of view (positive = good for White). It is what the `tryhard` engine uses. Compared
// with the bare material count of EvalSimple it rewards development, central control and a sensibly
// placed king, so the engine plays far more sensibly even at shallow depth.
//
// The piece-square tables are Tomasz Michniewski's "Simplified Evaluation Function" tables, paired
// with the centipawn piece values in simpleEvalTable. They are written from White's view with index 0
// = a8 (rank 8 first); whitePOVIndex maps a board square into them, mirroring for Black so each side
// is judged from its own perspective. The king's table is phase-dependent: a "tapered" blend of a
// middlegame table (hide behind the pawns) and an endgame table (march into the centre), interpolated
// by how much material is left.
func EvalComplex(pos *Position) int16 {
	var score, kingMG, kingEG int32
	phase := 0

	for sq := 0; sq < 64; sq++ {
		cp := pos.Squares[sq]
		if cp == Empty {
			continue
		}

		piece := cp.Colorless()
		idx := whitePOVIndex(sq, cp.Color())

		sign := int32(1)
		if cp.Color() == Black {
			sign = -1
		}

		if piece == King {
			kingMG += sign * int32(pstKingMiddlegame[idx])
			kingEG += sign * int32(pstKingEndgame[idx])
			continue
		}

		score += sign * (int32(simpleEvalTable[piece]) + int32(pieceSquareTables[piece][idx]))
		phase += phaseWeight[piece]
	}

	if phase > maxPhase {
		phase = maxPhase // promotions can push it past the starting total
	}

	// Positional terms (mobility, pawn structure, king safety, ...) each carry their own middlegame and
	// endgame value; material and the non-king tables are phase-independent and so contribute to both.
	posMG, posEG := evalPositional(pos)
	mg := score + kingMG + int32(posMG)
	eg := score + kingEG + int32(posEG)

	final := (mg*int32(phase) + eg*int32(maxPhase-phase)) / int32(maxPhase)

	// Keep clear of the mate-score band so a huge material imbalance is never misread as a forced mate.
	if final > evalLimit {
		final = evalLimit
	} else if final < -evalLimit {
		final = -evalLimit
	}
	return int16(final)
}

// evalLimit bounds the static evaluation well below the mate-score band.
const evalLimit = 20000

// gamePhase weights and their starting total. The king and pawns contribute nothing (absent from the
// map yields 0), so a full board of minor pieces, rooks and queens sums to maxPhase.
const maxPhase = 24

var phaseWeight = map[Piece]int{Knight: 1, Bishop: 1, Rook: 2, Queen: 4}

// whitePOVIndex maps a board square (a1 = 0 .. h8 = 63) into a White-POV, a8-first piece-square table.
// For White, that is the vertical mirror of our indexing (sq ^ 56); for Black we use the square
// directly, which mirrors the table so Black is scored from its own side of the board.
func whitePOVIndex(sq int, c Color) int {
	if c == White {
		return sq ^ 56
	}
	return sq
}

// pieceSquareTables holds the (phase-independent) tables for every piece except the king, which is
// tapered between two tables in EvalComplex.
var pieceSquareTables = map[Piece]*[64]int16{
	Pawn:   &pstPawn,
	Knight: &pstKnight,
	Bishop: &pstBishop,
	Rook:   &pstRook,
	Queen:  &pstQueen,
}

var pstPawn = [64]int16{
	0, 0, 0, 0, 0, 0, 0, 0,
	50, 50, 50, 50, 50, 50, 50, 50,
	10, 10, 20, 30, 30, 20, 10, 10,
	5, 5, 10, 25, 25, 10, 5, 5,
	0, 0, 0, 20, 20, 0, 0, 0,
	5, -5, -10, 0, 0, -10, -5, 5,
	5, 10, 10, -20, -20, 10, 10, 5,
	0, 0, 0, 0, 0, 0, 0, 0,
}

var pstKnight = [64]int16{
	-50, -40, -30, -30, -30, -30, -40, -50,
	-40, -20, 0, 0, 0, 0, -20, -40,
	-30, 0, 10, 15, 15, 10, 0, -30,
	-30, 5, 15, 20, 20, 15, 5, -30,
	-30, 0, 15, 20, 20, 15, 0, -30,
	-30, 5, 10, 15, 15, 10, 5, -30,
	-40, -20, 0, 5, 5, 0, -20, -40,
	-50, -40, -30, -30, -30, -30, -40, -50,
}

var pstBishop = [64]int16{
	-20, -10, -10, -10, -10, -10, -10, -20,
	-10, 0, 0, 0, 0, 0, 0, -10,
	-10, 0, 5, 10, 10, 5, 0, -10,
	-10, 5, 5, 10, 10, 5, 5, -10,
	-10, 0, 10, 10, 10, 10, 0, -10,
	-10, 10, 10, 10, 10, 10, 10, -10,
	-10, 5, 0, 0, 0, 0, 5, -10,
	-20, -10, -10, -10, -10, -10, -10, -20,
}

var pstRook = [64]int16{
	0, 0, 0, 0, 0, 0, 0, 0,
	5, 10, 10, 10, 10, 10, 10, 5,
	-5, 0, 0, 0, 0, 0, 0, -5,
	-5, 0, 0, 0, 0, 0, 0, -5,
	-5, 0, 0, 0, 0, 0, 0, -5,
	-5, 0, 0, 0, 0, 0, 0, -5,
	-5, 0, 0, 0, 0, 0, 0, -5,
	0, 0, 0, 5, 5, 0, 0, 0,
}

var pstQueen = [64]int16{
	-20, -10, -10, -5, -5, -10, -10, -20,
	-10, 0, 0, 0, 0, 0, 0, -10,
	-10, 0, 5, 5, 5, 5, 0, -10,
	-5, 0, 5, 5, 5, 5, 0, -5,
	0, 0, 5, 5, 5, 5, 0, -5,
	-10, 5, 5, 5, 5, 5, 0, -10,
	-10, 0, 5, 0, 0, 0, 0, -10,
	-20, -10, -10, -5, -5, -10, -10, -20,
}

// King table for the middlegame: stay home and castled, off the centre.
var pstKingMiddlegame = [64]int16{
	-30, -40, -40, -50, -50, -40, -40, -30,
	-30, -40, -40, -50, -50, -40, -40, -30,
	-30, -40, -40, -50, -50, -40, -40, -30,
	-30, -40, -40, -50, -50, -40, -40, -30,
	-20, -30, -30, -40, -40, -30, -30, -20,
	-10, -20, -20, -20, -20, -20, -20, -10,
	20, 20, 0, 0, 0, 0, 20, 20,
	20, 30, 10, 0, 0, 10, 30, 20,
}

// King table for the endgame: the king becomes a strong piece and wants to be active and central.
var pstKingEndgame = [64]int16{
	-50, -40, -30, -20, -20, -30, -40, -50,
	-30, -20, -10, 0, 0, -10, -20, -30,
	-30, -10, 20, 30, 30, 20, -10, -30,
	-30, -10, 30, 40, 40, 30, -10, -30,
	-30, -10, 30, 40, 40, 30, -10, -30,
	-30, -10, 20, 30, 30, 20, -10, -30,
	-30, -30, 0, 0, 0, 0, -30, -30,
	-50, -30, -30, -30, -30, -30, -30, -50,
}
