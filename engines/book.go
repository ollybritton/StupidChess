package engines

import (
	"embed"
	"math/rand"
	"strings"
	"sync"

	"github.com/ollybritton/StupidChess/position"
)

// The opening book is built from the Lichess chess-openings dataset: a few thousand named openings and
// their variations, written as PGN movetext. Each line is replayed from the start position; every
// position it passes through records the move played there as a candidate book move. Popular moves
// (1.e4, 1.d4, ...) recur across hundreds of lines and so accumulate a high weight, while offbeat
// lines (1.Nh3) carry a tiny weight, so weighted-random selection plays mostly sound, occasionally
// surprising, openings and answers instantly instead of searching.
//
//go:embed openings/*.tsv
var openingsFS embed.FS

// bookMaxPly bounds how deep the book reaches. Beyond the opening the engine should think for itself
// rather than blindly trust theory it cannot evaluate.
const bookMaxPly = 24

// bookEntry is one candidate move from a position, weighted by how many opening lines play it.
type bookEntry struct {
	move   string // UCI long-algebraic
	weight int
}

// openingBook maps a position (by Zobrist hash) to the weighted book moves playable from it.
type openingBook struct {
	moves map[uint64][]bookEntry
}

var (
	bookOnce sync.Once
	bookInst *openingBook
)

// getBook returns the shared opening book, building it on first use. The build parses several thousand
// SAN lines and is deferred so engines that never open a book do not pay for it.
func getBook() *openingBook {
	bookOnce.Do(func() {
		bookInst = buildBookFromOpenings()
	})
	return bookInst
}

// buildBookFromOpenings reads the embedded TSV files and replays every opening line into the book.
func buildBookFromOpenings() *openingBook {
	b := &openingBook{moves: map[uint64][]bookEntry{}}

	entries, err := openingsFS.ReadDir("openings")
	if err != nil {
		return b
	}

	for _, entry := range entries {
		data, err := openingsFS.ReadFile("openings/" + entry.Name())
		if err != nil {
			continue
		}
		b.addFile(string(data))
	}

	return b
}

// addFile parses one TSV (columns: eco, name, pgn) and replays each line's movetext.
func (b *openingBook) addFile(content string) {
	for i, line := range strings.Split(content, "\n") {
		if i == 0 || line == "" {
			continue // header row, or trailing blank line
		}

		cols := strings.Split(line, "\t")
		if len(cols) < 3 {
			continue
		}

		b.addLine(cols[2])
	}
}

// addLine replays a single PGN movetext line, recording the move played from each position.
func (b *openingBook) addLine(pgn string) {
	pos, err := position.NewPositionFromFEN(position.StartingPosition)
	if err != nil {
		return
	}

	for ply, san := range sanMoves(pgn) {
		if ply >= bookMaxPly {
			break
		}

		move, ok := sanToMove(pos, san)
		if !ok {
			break // an unparseable or illegal token: stop replaying this line
		}

		b.record(pos.ZobristHash(), move.String())
		pos.MakeMove(move)
	}
}

// record adds one occurrence of a move to a position, accumulating its weight.
func (b *openingBook) record(hash uint64, uci string) {
	entries := b.moves[hash]
	for i := range entries {
		if entries[i].move == uci {
			entries[i].weight++
			return
		}
	}
	b.moves[hash] = append(entries, bookEntry{move: uci, weight: 1})
}

// lookup returns a weighted-random book move for the position, or ("", false) if it is out of book.
func (b *openingBook) lookup(pos *position.Position) (string, bool) {
	entries := b.moves[pos.ZobristHash()]
	if len(entries) == 0 {
		return "", false
	}

	total := 0
	for _, e := range entries {
		total += e.weight
	}

	pick := rand.Intn(total)
	for _, e := range entries {
		pick -= e.weight
		if pick < 0 {
			return e.move, true
		}
	}

	return entries[len(entries)-1].move, true // unreachable, but keeps the compiler happy
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
