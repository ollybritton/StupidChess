package engines

import (
	"math/rand"
	"strings"

	"github.com/ollybritton/StupidChess/position"
)

// openingLines are common, sound openings written as sequences of UCI moves from the start position.
// The book is built by replaying each line: the move played from each position along the way becomes a
// book move for that position. So while the game follows known theory, the engine answers instantly
// with a book move instead of searching, and gets a sensible, varied opening.
var openingLines = []string{
	// 1.e4 e5
	"e2e4 e7e5 g1f3 b8c6 f1b5 a7a6 b5a4 g8f6 e1g1", // Ruy Lopez
	"e2e4 e7e5 g1f3 b8c6 f1c4 f8c5 c2c3 g8f6",      // Italian
	"e2e4 e7e5 g1f3 b8c6 f1c4 g8f6 d2d3",           // Two Knights
	"e2e4 e7e5 g1f3 b8c6 d2d4 e5d4 f3d4 g8f6",      // Scotch
	"e2e4 e7e5 g1f3 g8f6 f3e5 d7d6 e5f3 f6e4",      // Petrov
	// 1.e4 c5 (Sicilian)
	"e2e4 c7c5 g1f3 d7d6 d2d4 c5d4 f3d4 g8f6 b1c3 a7a6", // Najdorf
	"e2e4 c7c5 g1f3 b8c6 d2d4 c5d4 f3d4 g8f6 b1c3",      // Classical
	"e2e4 c7c5 b1c3 b8c6 g2g3 g7g6 f1g2",                // Closed Sicilian
	// 1.e4 e6 / c6 / d5 / Nf6 / d6
	"e2e4 e7e6 d2d4 d7d5 b1c3 g8f6",      // French
	"e2e4 e7e6 d2d4 d7d5 e4e5 c7c5 c2c3", // French Advance
	"e2e4 c7c6 d2d4 d7d5 b1c3 d5e4 c3e4", // Caro-Kann
	"e2e4 d7d5 e4d5 d8d5 b1c3 d5a5",      // Scandinavian
	"e2e4 g8f6 e4e5 f6d5 d2d4 d7d6",      // Alekhine
	"e2e4 d7d6 d2d4 g8f6 b1c3 g7g6",      // Pirc
	// 1.d4 d5
	"d2d4 d7d5 c2c4 e7e6 b1c3 g8f6 c1g5", // Queen's Gambit Declined
	"d2d4 d7d5 c2c4 c7c6 g1f3 g8f6 b1c3", // Slav
	"d2d4 d7d5 c2c4 d5c4 g1f3 g8f6 e2e3", // Queen's Gambit Accepted
	// 1.d4 Nf6
	"d2d4 g8f6 c2c4 e7e6 b1c3 f8b4",           // Nimzo-Indian
	"d2d4 g8f6 c2c4 g7g6 b1c3 f8g7 e2e4 d7d6", // King's Indian
	"d2d4 g8f6 c2c4 g7g6 b1c3 d7d5",           // Grünfeld
	"d2d4 g8f6 c2c4 e7e6 g1f3 b7b6",           // Queen's Indian
	"d2d4 f7f5 g2g3 g8f6 f1g2",                // Dutch
	// flank openings
	"c2c4 e7e5 b1c3 g8f6 g1f3 b8c6", // English
	"c2c4 g8f6 b1c3 e7e5 g1f3",      // English
	"g1f3 d7d5 d2d4 g8f6 c2c4 e7e6", // Réti into QGD
	"g1f3 g8f6 c2c4 g7g6 b1c3 f8g7", // Réti/KID
}

// openingBook maps a position (by Zobrist hash) to the book moves playable from it.
type openingBook struct {
	moves map[uint64][]string
}

func newOpeningBook(lines []string) *openingBook {
	b := &openingBook{moves: map[uint64][]string{}}

	for _, line := range lines {
		pos, err := position.NewPositionFromFEN(position.StartingPosition)
		if err != nil {
			continue
		}

		for _, uci := range strings.Fields(line) {
			move, ok := bookLegalMove(pos, uci)
			if !ok {
				break // a typo or illegal continuation: stop replaying this line
			}

			hash := pos.ZobristHash()
			if !bookContains(b.moves[hash], uci) {
				b.moves[hash] = append(b.moves[hash], uci)
			}

			pos.MakeMove(move)
		}
	}

	return b
}

// lookup returns a random book move for the position, or ("", false) if it is out of book.
func (b *openingBook) lookup(pos *position.Position) (string, bool) {
	moves := b.moves[pos.ZobristHash()]
	if len(moves) == 0 {
		return "", false
	}
	return moves[rand.Intn(len(moves))], true
}

// bookLegalMove finds the fully-described legal move matching a UCI string (and validates it).
func bookLegalMove(pos *position.Position, uci string) (position.Move, bool) {
	for _, m := range pos.MovesLegal().AsSlice() {
		if m.String() == uci {
			return m, true
		}
	}
	return position.NoMove, false
}

func bookContains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

// defaultBook is built once from the opening lines above.
var defaultBook = newOpeningBook(openingLines)
