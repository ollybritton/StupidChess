// Package syzygy implements probing of Ronald de Man's Syzygy endgame
// tablebases. Only the WDL (.rtbw, win/draw/loss) tables are supported.
//
// The implementation is a faithful Go port of de Man's tbprobe.c / tbcore.c
// (the basis of the Fathom library). The C source is authoritative for every
// table and constant.
package syzygy

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/ollybritton/StupidChess/position"
)

// Tablebases holds a set of loaded WDL tables, keyed by material signature.
type Tablebases struct {
	tables    []*tbTable
	byKey     map[uint64]*tbTable
	maxPieces int

	mu sync.Mutex // serialises the one-time lazy initTableWDL; the hot path uses tbTable.ready atomically
}

// Load opens every .rtbw file in dir and returns a Tablebases ready for
// probing. Files are memory-loaded eagerly (read into RAM) but their internal
// index structures are parsed lazily on first probe, mirroring de Man's design.
func Load(dir string) (*Tablebases, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("syzygy: reading %s: %w", dir, err)
	}

	tb := &Tablebases{
		byKey: make(map[uint64]*tbTable),
	}

	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		n := e.Name()
		if strings.HasSuffix(n, ".rtbw") {
			names = append(names, n)
		}
	}
	sort.Strings(names)

	for _, n := range names {
		base := strings.TrimSuffix(n, ".rtbw")
		raw, err := os.ReadFile(filepath.Join(dir, n))
		if err != nil {
			return nil, fmt.Errorf("syzygy: reading %s: %w", n, err)
		}

		pcs, key, key2, hasPawns, num, encType, pawns, perr := parseName(base)
		if perr != nil {
			// Skip files whose names we cannot parse rather than failing the
			// whole load.
			continue
		}
		_ = pcs

		t := &tbTable{
			key:       key,
			symmetric: key == key2,
			hasPawns:  hasPawns,
			num:       num,
			encType:   encType,
			pawns:     pawns,
			raw:       raw,
			name:      base,
		}
		tb.tables = append(tb.tables, t)
		tb.byKey[key] = t
		if key2 != key {
			tb.byKey[key2] = t
		}
		if num > tb.maxPieces {
			tb.maxPieces = num
		}
	}

	return tb, nil
}

// MaxPieces returns the largest number of pieces among the loaded tables.
func (tb *Tablebases) MaxPieces() int { return tb.maxPieces }

// ensureReady lazily parses a table's internal structure on first use. The common case (already parsed)
// is a single atomic load with no lock, so the many probes a deep endgame search makes do not serialise
// on the mutex across the search threads; the lock is taken only to do the one-time parse.
func (tb *Tablebases) ensureReady(t *tbTable) error {
	if atomic.LoadInt32(&t.ready) != 0 {
		return nil
	}
	tb.mu.Lock()
	defer tb.mu.Unlock()
	if atomic.LoadInt32(&t.ready) != 0 { // re-check: another thread may have parsed it while we waited
		return nil
	}
	if err := t.initTableWDL(); err != nil {
		return err
	}
	atomic.StoreInt32(&t.ready, 1)
	return nil
}

// ProbeWDL probes the position for its WDL value from the side-to-move
// perspective: 2 = win, 1 = cursed win, 0 = draw, -1 = blessed loss, -2 = loss.
//
// ok is false when the material is not loaded, exceeds MaxPieces, or the
// position has castling rights or otherwise cannot be probed.
func (tb *Tablebases) ProbeWDL(p *position.Position) (wdl int, ok bool) {
	if p.Castling != 0 {
		return 0, false
	}

	pc := positionToPos(p)
	if popcount(pc.white|pc.black) > tb.maxPieces {
		return 0, false
	}

	// Guard against illegal positions (the side that just moved left its king
	// in check, i.e. the king of the side NOT to move is attacked). de Man's
	// probe assumes legality and would otherwise generate a king-capture and
	// dereference an empty king bitboard. We reject such positions.
	if !probeLegal(&pc) {
		return 0, false
	}

	return tb.probeWDL(&pc)
}

// positionToPos converts the engine's Position into the internal bitboard pos.
// The engine already uses a1=bit0..h8=bit63 layout, matching Syzygy.
func positionToPos(p *position.Position) pos {
	var pc pos
	pc.white = uint64(p.Occupied[position.White])
	pc.black = uint64(p.Occupied[position.Black])
	pc.pawns = uint64(p.Pieces[position.Pawn])
	pc.knights = uint64(p.Pieces[position.Knight])
	pc.bishops = uint64(p.Pieces[position.Bishop])
	pc.rooks = uint64(p.Pieces[position.Rook])
	pc.queens = uint64(p.Pieces[position.Queen])
	pc.kings = uint64(p.Pieces[position.King])
	pc.turn = p.SideToMove == position.White
	if p.EnPassant != position.NoEnPassant {
		pc.ep = int(p.EnPassant)
	} else {
		pc.ep = 0
	}
	return pc
}
