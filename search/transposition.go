package search

import (
	"sync/atomic"

	"github.com/ollybritton/StupidChess/position"
)

// ttBound describes what a stored score tells us relative to the search window it was found in.
type ttBound uint8

const (
	boundNone  ttBound = iota
	boundExact         // the score is exact
	boundLower         // the score is a lower bound (the node failed high / caused a beta cutoff)
	boundUpper         // the score is an upper bound (the node failed low)
)

// compactMove packs a move's from/to/promotion into 15 bits. The transposition table only needs those
// to recognise the move during ordering (it never makes the stored move directly), so the prior-state
// bits of a full position.Move are dropped. A value of 0 means "no move" (no real move is a1->a1).
type compactMove uint16

func toCompact(m position.Move) compactMove {
	return compactMove(uint16(m.From()) | uint16(m.To())<<6 | uint16(m.Promotion())<<12)
}

// matches reports whether m is the same move (by from/to/promotion) as this stored move.
func (c compactMove) matches(m position.Move) bool {
	return c != 0 &&
		uint8(c&0x3f) == m.From() &&
		uint8((c>>6)&0x3f) == m.To() &&
		position.Piece((c>>12)&0x7) == m.Promotion()
}

// A transposition-table entry is packed into 64 bits so it can be read and written atomically:
//
//	bits  0..14  compact move
//	bits 15..30  score (int16)
//	bits 31..38  depth (uint8)
//	bits 39..40  bound
//
// Any stored entry has a non-zero bound, so a packed value of 0 means "empty slot".
func packEntry(move compactMove, score int16, depth uint8, bound ttBound) uint64 {
	return uint64(move) | uint64(uint16(score))<<15 | uint64(depth)<<31 | uint64(bound)<<39
}

func entryMove(data uint64) compactMove { return compactMove(data & 0x7fff) }
func entryScore(data uint64) int16      { return int16(uint16(data >> 15)) }
func entryDepth(data uint64) uint8      { return uint8(data >> 31) }
func entryBound(data uint64) ttBound    { return ttBound((data >> 39) & 0x3) }

// ttSlot is one bucket. To stay correct under concurrent access from several search threads without
// locking, each write stores `data` and then `lock = hash ^ data`. A reader recomputes `lock ^ data`
// and trusts the entry only if it equals the hash it probed for. A torn read (data from one write, lock
// from another) fails that check and is treated as a miss, so a stale or half-written entry can never
// be acted on. Both words are accessed atomically.
type ttSlot struct {
	lock uint64
	data uint64
}

const (
	ttBits = 21 // 2^21 entries (~32 MB), shared across all search threads
	ttSize = 1 << ttBits
	ttMask = ttSize - 1
)

type transpositionTable struct {
	slots []ttSlot
}

func newTranspositionTable() *transpositionTable {
	return &transpositionTable{slots: make([]ttSlot, ttSize)}
}

// probe returns the stored move, score, depth and bound for hash, if a valid entry is present.
func (t *transpositionTable) probe(hash uint64) (move compactMove, score int16, depth uint8, bound ttBound, ok bool) {
	slot := &t.slots[hash&ttMask]
	data := atomic.LoadUint64(&slot.data)
	lock := atomic.LoadUint64(&slot.lock)

	if data != 0 && lock^data == hash {
		return entryMove(data), entryScore(data), entryDepth(data), entryBound(data), true
	}
	return 0, 0, 0, boundNone, false
}

// store records a result, preferring to keep a deeper entry already at the slot for the same position.
func (t *transpositionTable) store(hash uint64, depth uint, score int16, bound ttBound, move position.Move) {
	slot := &t.slots[hash&ttMask]

	existing := atomic.LoadUint64(&slot.data)
	if existing != 0 && atomic.LoadUint64(&slot.lock)^existing == hash && uint(entryDepth(existing)) > depth {
		return
	}

	data := packEntry(toCompact(move), score, uint8(depth), bound)
	// Write data first, then the lock; a reader that sees one but not the other detects the mismatch.
	atomic.StoreUint64(&slot.data, data)
	atomic.StoreUint64(&slot.lock, hash^data)
}

// mateScoreBound is the threshold above (or below) which a score denotes a forced mate.
const mateScoreBound = position.MaxEval - 1000

// Mate scores encode the distance to mate from the root, so the same position transposed to at a
// different ply would have an inconsistent score. scoreToTT rewrites a mate score to be relative to the
// node it is stored at; scoreFromTT undoes that on the way out. Non-mate scores pass through unchanged.
func scoreToTT(score int16, ply int) int16 {
	if score >= mateScoreBound {
		return score + int16(ply)
	}
	if score <= -mateScoreBound {
		return score - int16(ply)
	}
	return score
}

func scoreFromTT(score int16, ply int) int16 {
	if score >= mateScoreBound {
		return score - int16(ply)
	}
	if score <= -mateScoreBound {
		return score + int16(ply)
	}
	return score
}
