package position

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// castlingField returns the castling-rights field of a position's FEN.
func castlingField(p *Position) string {
	return strings.Fields(p.StringFEN())[2]
}

// halfmoveField returns the halfmove-clock field of a position's FEN.
func halfmoveField(p *Position) string {
	return strings.Fields(p.StringFEN())[4]
}

func mustMove(t *testing.T, uci string) Move {
	t.Helper()
	m, err := ParseMove(uci)
	assert.NoError(t, err)
	return m
}

// TestCastlingRevokedWhenRookCapturesRook covers the bug where a rook capturing an enemy rook on its
// home square did not revoke the captured side's castling right, because the moving-piece case in the
// MakeMove switch shadowed the captured-rook case.
func TestCastlingRevokedWhenRookCapturesRook(t *testing.T) {
	// Open a-file: white Ra1 can capture black ra8. Both sides have full castling rights.
	pos, err := NewPositionFromFEN("r3k2r/8/8/8/8/8/8/R3K2R w KQkq - 0 1")
	assert.NoError(t, err)

	pos.MakeMove(mustMove(t, "a1a8")) // Ra1xa8

	rights := castlingField(pos)
	assert.NotContains(t, rights, "q", "black should lose queenside castling after its a8 rook is captured")
	assert.NotContains(t, rights, "Q", "white should lose queenside castling after its a1 rook moves")
	assert.Contains(t, rights, "K", "white keeps kingside castling")
	assert.Contains(t, rights, "k", "black keeps kingside castling")
}

// TestHalfmoveClockResetsOnCapture covers the bug where the capture test in MakeMove read the board
// after the move had been applied, so it never detected a capture and never reset the clock.
func TestHalfmoveClockResetsOnCapture(t *testing.T) {
	pos, err := NewPositionFromFEN("r3k2r/8/8/8/8/8/8/R3K2R w KQkq - 5 1")
	assert.NoError(t, err)

	pos.MakeMove(mustMove(t, "a1a8")) // capture
	assert.Equal(t, "0", halfmoveField(pos), "halfmove clock must reset to 0 on a capture")
}

// TestHalfmoveClockResetsOnPawnMove checks the clock resets on a (non-capturing) pawn push.
func TestHalfmoveClockResetsOnPawnMove(t *testing.T) {
	pos, err := NewPositionFromFEN("4k3/8/8/8/8/8/4P3/4K3 w - - 7 1")
	assert.NoError(t, err)

	pos.MakeMove(mustMove(t, "e2e4"))
	assert.Equal(t, "0", halfmoveField(pos), "halfmove clock must reset to 0 on a pawn move")
}

// TestHalfmoveClockIncrementsOnQuietMove checks a quiet non-pawn move increments the clock.
func TestHalfmoveClockIncrementsOnQuietMove(t *testing.T) {
	pos, err := NewPositionFromFEN("r3k2r/8/8/8/8/8/8/R3K2R w KQkq - 5 1")
	assert.NoError(t, err)

	pos.MakeMove(mustMove(t, "a1c1")) // quiet rook move
	assert.Equal(t, "6", halfmoveField(pos), "halfmove clock must increment on a quiet move")
}

// TestHalfmoveClockRoundTrips checks that MakeMove followed by UndoMove restores the clock, using a
// generated move (which carries the prior castling/en-passant state the search relies on).
func TestHalfmoveClockRoundTrips(t *testing.T) {
	pos, err := NewPositionFromFEN("r3k2r/8/8/8/8/8/8/R3K2R w KQkq - 5 1")
	assert.NoError(t, err)

	before := pos.HalfmoveClock
	for _, m := range pos.MovesLegal().AsSlice() {
		if pos.MakeMove(m) {
			pos.UndoMove(m)
			assert.Equal(t, before, pos.HalfmoveClock, "halfmove clock must be restored after UndoMove for %s", m.String())
		}
	}
}

// TestFullMoveCounterDoesNotDrift covers the bug where generating legal moves drifted the fullmove
// counter: MakeMove incremented it only after the in-check legality test, but UndoMove always
// decremented it, so each illegal pseudolegal move (self-undone by MakeMove) leaked a decrement. The
// position below has Black in check, so it has illegal pseudolegal king moves to exercise the path.
func TestFullMoveCounterDoesNotDrift(t *testing.T) {
	pos, err := NewPositionFromFEN("4k3/8/8/8/8/8/8/4R1K1 b - - 0 10")
	assert.NoError(t, err)

	before := pos.FullMoves
	for i := 0; i < 50; i++ {
		pos.MovesLegal()
	}
	assert.Equal(t, before, pos.FullMoves, "fullmove counter must not drift when generating legal moves")
}

// TestFullMoveCounterRoundTrips checks MakeMove/UndoMove restores the fullmove counter from a few
// positions, including one where the side to move is in check.
func TestFullMoveCounterRoundTrips(t *testing.T) {
	for _, fen := range []string{
		"rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1",
		"rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR b KQkq - 0 1",
		"4k3/8/8/8/8/8/8/4R1K1 b - - 0 10",
	} {
		pos, err := NewPositionFromFEN(fen)
		assert.NoError(t, err)

		before := pos.FullMoves
		for _, m := range pos.MovesLegal().AsSlice() {
			if pos.MakeMove(m) {
				pos.UndoMove(m)
				assert.Equal(t, before, pos.FullMoves, "fullmove counter must be restored after UndoMove for %s in %s", m.String(), fen)
			}
		}
	}
}
