package position

// EvalComplex is a centipawn evaluation combining material with piece-square tables, returned from
// White's point of view (positive = good for White). It is what the `tryhard` engine uses. Compared
// with the bare material count of EvalSimple it rewards development, central control and a tucked-away
// king, so the engine plays far more sensibly even at shallow depth.
//
// The piece-square tables are Tomasz Michniewski's "Simplified Evaluation Function" tables, paired
// with the centipawn piece values in simpleEvalTable. They are written from White's view with index 0
// = a8 (rank 8 first); whitePOVIndex maps a board square into them, mirroring for Black so each side
// is judged from its own perspective.
func EvalComplex(pos *Position) int16 {
	var score int16

	for sq := 0; sq < 64; sq++ {
		cp := pos.Squares[sq]
		if cp == Empty {
			continue
		}

		piece := cp.Colorless()
		value := simpleEvalTable[piece] + pieceSquareTables[piece][whitePOVIndex(sq, cp.Color())]

		if cp.Color() == White {
			score += value
		} else {
			score -= value
		}
	}

	return score
}

// whitePOVIndex maps a board square (a1 = 0 .. h8 = 63) into a White-POV, a8-first piece-square table.
// For White, that is the vertical mirror of our indexing (sq ^ 56); for Black we use the square
// directly, which mirrors the table so Black is scored from its own side of the board.
func whitePOVIndex(sq int, c Color) int {
	if c == White {
		return sq ^ 56
	}
	return sq
}

var pieceSquareTables = map[Piece]*[64]int16{
	Pawn:   &pstPawn,
	Knight: &pstKnight,
	Bishop: &pstBishop,
	Rook:   &pstRook,
	Queen:  &pstQueen,
	King:   &pstKing,
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
var pstKing = [64]int16{
	-30, -40, -40, -50, -50, -40, -40, -30,
	-30, -40, -40, -50, -50, -40, -40, -30,
	-30, -40, -40, -50, -50, -40, -40, -30,
	-30, -40, -40, -50, -50, -40, -40, -30,
	-20, -30, -30, -40, -40, -30, -30, -20,
	-10, -20, -20, -20, -20, -20, -20, -10,
	20, 20, 0, 0, 0, 0, 20, 20,
	20, 30, 10, 0, 0, 10, 30, 20,
}
