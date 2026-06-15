package engines

import (
	"regexp"
	"strings"

	"github.com/ollybritton/StupidChess/position"
)

// san.go resolves Standard Algebraic Notation (SAN) moves against a concrete position. The opening
// book is distributed as PGN-style SAN ("1. e4 e6 2. d4 d5 3. Nc3"), but the engine speaks in fully
// described moves, so each SAN token is matched back to one of the position's legal moves.

var moveNumberPrefix = regexp.MustCompile(`^\d+\.+`)

// sanMoves tokenises a PGN movetext string into bare SAN moves, dropping move numbers ("1.", "3...")
// and game-result markers ("1-0", "1/2-1/2", "*").
func sanMoves(pgn string) []string {
	fields := strings.Fields(pgn)
	moves := make([]string, 0, len(fields))

	for _, f := range fields {
		f = moveNumberPrefix.ReplaceAllString(f, "")
		if f == "" {
			continue
		}
		switch f {
		case "1-0", "0-1", "1/2-1/2", "*":
			continue
		}
		moves = append(moves, f)
	}

	return moves
}

// sanPieceLetter maps a SAN piece letter to its colourless piece. The boolean reports whether the
// letter named a piece at all (pawns carry no letter).
func sanPieceLetter(c byte) (position.Piece, bool) {
	switch c {
	case 'N':
		return position.Knight, true
	case 'B':
		return position.Bishop, true
	case 'R':
		return position.Rook, true
	case 'Q':
		return position.Queen, true
	case 'K':
		return position.King, true
	default:
		return position.Pawn, false
	}
}

// sanToMove resolves a single SAN move (e.g. "exd5", "Nbd2", "O-O", "e8=Q+") to the matching legal
// move in pos, or reports false if no legal move corresponds to it.
func sanToMove(pos *position.Position, san string) (position.Move, bool) {
	// Strip check / mate / annotation decorations: they carry no resolution information.
	san = strings.TrimRight(san, "+#!?")
	if san == "" {
		return position.NoMove, false
	}

	legal := pos.MovesLegal().AsSlice()

	// Castling: identified by the destination file of the king's two-square move (g-file kingside,
	// c-file queenside). Accept both "O-O" and the "0-0" digit variant.
	switch san {
	case "O-O", "0-0":
		return matchKingFile(legal, 6)
	case "O-O-O", "0-0-0":
		return matchKingFile(legal, 2)
	}

	// Promotion: everything after "=" names the promoted piece.
	promotion := position.None
	if i := strings.IndexByte(san, '='); i >= 0 {
		if len(san) > i+1 {
			if p, ok := sanPieceLetter(san[i+1]); ok {
				promotion = p
			}
		}
		san = san[:i]
	}

	// Leading piece letter (absent for pawn moves).
	piece := position.Pawn
	if len(san) > 0 {
		if p, ok := sanPieceLetter(san[0]); ok {
			piece = p
			san = san[1:]
		}
	}

	// Captures are written with "x"; it adds nothing once we know the destination.
	san = strings.Replace(san, "x", "", 1)

	// The destination is the trailing two characters; whatever precedes it disambiguates the origin.
	if len(san) < 2 {
		return position.NoMove, false
	}
	dest := position.StringToSquare(san[len(san)-2:])
	disambig := san[:len(san)-2]

	var fromFile, fromRank int = -1, -1
	for i := 0; i < len(disambig); i++ {
		switch c := disambig[i]; {
		case c >= 'a' && c <= 'h':
			fromFile = int(c - 'a')
		case c >= '1' && c <= '8':
			fromRank = int(c - '1')
		}
	}

	for _, m := range legal {
		if m.Moved().Colorless() != piece {
			continue
		}
		if m.To() != dest {
			continue
		}
		if m.Promotion() != promotion {
			continue
		}
		if fromFile >= 0 && int(m.From()%8) != fromFile {
			continue
		}
		if fromRank >= 0 && int(m.From()/8) != fromRank {
			continue
		}
		return m, true
	}

	return position.NoMove, false
}

// matchKingFile finds the legal king move whose destination is on the given file (used for castling).
func matchKingFile(legal []position.Move, file uint8) (position.Move, bool) {
	for _, m := range legal {
		if m.Moved().Colorless() == position.King && m.To()%8 == file {
			return m, true
		}
	}
	return position.NoMove, false
}
