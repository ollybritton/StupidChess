package position

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func seeFindMove(t *testing.T, pos *Position, uci string) Move {
	t.Helper()
	for _, m := range pos.MovesPseudolegal().AsSlice() {
		if m.String() == uci {
			return m
		}
	}
	t.Fatalf("move %s not found", uci)
	return NoMove
}

func TestSEE(t *testing.T) {
	cases := []struct {
		name, fen, move string
		want            int
	}{
		{"win a free queen", "4k3/8/8/3q4/3Q4/8/8/4K3 w - - 0 1", "d4d5", 900},
		{"queen takes defended pawn", "4k3/8/4p3/3p4/8/3Q4/8/4K3 w - - 0 1", "d3d5", -800},
		{"pawn takes defended pawn (equal)", "4k3/8/2p5/3p4/4P3/8/8/4K3 w - - 0 1", "e4d5", 0},
		{"pawn takes undefended pawn", "4k3/8/8/3p4/4P3/8/8/4K3 w - - 0 1", "e4d5", 100},
	}
	for _, c := range cases {
		pos, err := NewPositionFromFEN(c.fen)
		require.NoError(t, err, c.name)
		got := pos.SEE(seeFindMove(t, pos, c.move))
		assert.Equal(t, c.want, got, c.name)
	}
}
