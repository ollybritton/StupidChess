package position

// Zobrist hashing: each position gets a 64-bit key by XOR-ing in a random number for every
// (piece, square), the side to move, the castling rights and the en-passant file. Equal positions
// get equal keys, which is what lets the search recognise transpositions. The keys are filled once,
// deterministically, at package load.
var (
	zobristPieces    [12][64]uint64 // indexed by ColoredPiece (0..11; Empty is skipped)
	zobristSide      uint64         // mixed in when Black is to move
	zobristCastling  [16]uint64     // indexed by the 4-bit CastlingAvailability mask
	zobristEnPassant [8]uint64      // indexed by the en-passant file
)

func init() {
	// splitmix64, seeded with a fixed constant so keys are stable across runs.
	state := uint64(0x9E3779B97F4A7C15)
	next := func() uint64 {
		state += 0x9E3779B97F4A7C15
		z := state
		z = (z ^ (z >> 30)) * 0xBF58476D1CE4E5B9
		z = (z ^ (z >> 27)) * 0x94D049BB133111EB
		return z ^ (z >> 31)
	}

	for cp := 0; cp < 12; cp++ {
		for sq := 0; sq < 64; sq++ {
			zobristPieces[cp][sq] = next()
		}
	}
	zobristSide = next()
	for i := range zobristCastling {
		zobristCastling[i] = next()
	}
	for i := range zobristEnPassant {
		zobristEnPassant[i] = next()
	}
}

// ZobristHash returns the position's Zobrist key, identifying it for the transposition table. The
// halfmove/fullmove clocks are deliberately excluded: they don't change the legal continuations, so
// two positions that differ only in those clocks should share a key.
func (p *Position) ZobristHash() uint64 {
	var h uint64

	for sq := 0; sq < 64; sq++ {
		cp := p.Squares[sq]
		if cp == Empty {
			continue
		}
		h ^= zobristPieces[cp][sq]
	}

	if p.SideToMove == Black {
		h ^= zobristSide
	}

	h ^= zobristCastling[p.Castling]

	if p.EnPassant != NoEnPassant {
		h ^= zobristEnPassant[p.EnPassant%8]
	}

	return h
}
