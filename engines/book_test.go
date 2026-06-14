package engines

import (
	"testing"

	"github.com/ollybritton/StupidChess/position"
	"github.com/stretchr/testify/assert"
)

// TestBookOpensFromStart: the start position is in book, and the move it returns is one of the standard
// first moves and is legal.
func TestBookOpensFromStart(t *testing.T) {
	pos, err := position.NewPositionFromFEN(position.StartingPosition)
	assert.NoError(t, err)

	uci, ok := defaultBook.lookup(pos)
	assert.True(t, ok, "the start position should be in the opening book")
	assert.Contains(t, []string{"e2e4", "d2d4", "c2c4", "g1f3"}, uci)

	_, legal := bookLegalMove(pos, uci)
	assert.True(t, legal, "the book move %s should be legal", uci)
}

// TestBookHasManyPositions sanity-checks that the lines actually built a reasonable book.
func TestBookHasManyPositions(t *testing.T) {
	assert.Greater(t, len(defaultBook.moves), 50, "expected a few dozen book positions")
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
