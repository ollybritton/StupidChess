package engines

import (
	"testing"

	"github.com/ollybritton/StupidChess/position"
	"github.com/stretchr/testify/assert"
)

// TestBookOpensFromStart: the start position is in book, every candidate move is legal, and the
// popular mainline first moves are among the candidates.
func TestBookOpensFromStart(t *testing.T) {
	pos, err := position.NewPositionFromFEN(position.StartingPosition)
	assert.NoError(t, err)

	book := getBook()

	entries := book.moves[pos.ZobristHash()]
	assert.NotEmpty(t, entries, "the start position should be in the opening book")

	moves := map[string]bool{}
	for _, e := range entries {
		_, legal := bookLegalMove(pos, e.move)
		assert.True(t, legal, "book move %s should be legal", e.move)
		moves[e.move] = true
	}

	assert.True(t, moves["e2e4"], "1.e4 should be a book move")
	assert.True(t, moves["d2d4"], "1.d4 should be a book move")

	uci, ok := book.lookup(pos)
	assert.True(t, ok, "lookup should return a move from the start position")
	_, legal := bookLegalMove(pos, uci)
	assert.True(t, legal, "the chosen book move %s should be legal", uci)
}

// TestBookPopularMovesDominate: 1.e4 and 1.d4 each recur across hundreds of lines, so they should far
// outweigh any offbeat first move (1.Nh3 and friends).
func TestBookPopularMovesDominate(t *testing.T) {
	pos, err := position.NewPositionFromFEN(position.StartingPosition)
	assert.NoError(t, err)

	weights := map[string]int{}
	for _, e := range getBook().moves[pos.ZobristHash()] {
		weights[e.move] = e.weight
	}

	assert.Greater(t, weights["e2e4"], 100, "1.e4 should be heavily weighted")
	assert.Greater(t, weights["d2d4"], 100, "1.d4 should be heavily weighted")
	assert.Greater(t, weights["e2e4"], weights["g1h3"], "1.e4 should outweigh 1.Nh3")
}

// TestBookHasManyPositions sanity-checks that the dataset actually built a large book.
func TestBookHasManyPositions(t *testing.T) {
	assert.Greater(t, len(getBook().moves), 1000, "expected thousands of book positions")
}

// TestSanToMove checks the SAN resolver on the cases that trip naive parsers: captures, piece-file
// disambiguation, castling, and promotion.
func TestSanToMove(t *testing.T) {
	cases := []struct {
		fen string
		san string
		uci string
	}{
		{position.StartingPosition, "e4", "e2e4"},
		{position.StartingPosition, "Nf3", "g1f3"},
		// Pawn capture.
		{"rnbqkbnr/ppp1pppp/8/3p4/4P3/8/PPPP1PPP/RNBQKBNR w KQkq d6 0 2", "exd5", "e4d5"},
		// Knight disambiguation by file: both knights reach d2, "Nbd2" picks the b1 knight.
		{"rnbqkbnr/pppp1ppp/8/4p3/3P4/5N2/PPP1PPPP/RNBQKB1R w KQkq e6 0 2", "Nbd2", "b1d2"},
		// Kingside castling.
		{"rnbqk2r/pppp1ppp/5n2/2b1p3/2B1P3/5N2/PPPP1PPP/RNBQK2R w KQkq - 0 1", "O-O", "e1g1"},
		// Queenside castling.
		{"r3kbnr/pppqpppp/2np4/8/3P4/2N1B3/PPPQPPPP/R3KBNR w KQkq - 0 1", "O-O-O", "e1c1"},
		// Promotion to queen.
		{"8/P7/8/8/8/8/8/k6K w - - 0 1", "a8=Q", "a7a8q"},
	}

	for _, c := range cases {
		pos, err := position.NewPositionFromFEN(c.fen)
		assert.NoError(t, err, c.san)

		move, ok := sanToMove(pos, c.san)
		assert.True(t, ok, "should resolve SAN %q in %q", c.san, c.fen)
		assert.Equal(t, c.uci, move.String(), "SAN %q", c.san)
	}
}

// TestTryHardOwnBookOption: the OwnBook UCI option is advertised, defaults on, and toggles the flag.
func TestTryHardOwnBookOption(t *testing.T) {
	e := NewEngineTryHard()
	assert.True(t, e.useBook, "the opening book should be on by default")

	opts := e.Options()
	assert.Len(t, opts, 1)
	assert.Equal(t, "OwnBook", opts[0].Name)

	assert.NoError(t, e.SetOption("OwnBook", "false"))
	assert.False(t, e.useBook)

	assert.NoError(t, e.SetOption("ownbook", "true")) // case-insensitive
	assert.True(t, e.useBook)
}
