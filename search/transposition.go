package search

import "github.com/ollybritton/StupidChess/position"

// ttBound describes what a stored score tells us relative to the search window it was found in.
type ttBound uint8

const (
	boundNone  ttBound = iota
	boundExact         // the score is exact
	boundLower         // the score is a lower bound (the node failed high / caused a beta cutoff)
	boundUpper         // the score is an upper bound (the node failed low)
)

// ttEntry is one transposition-table slot. The full hash is kept so index collisions can be detected.
type ttEntry struct {
	hash  uint64
	move  position.Move
	score int16
	depth uint8
	bound ttBound
}

const (
	ttBits = 20 // 2^20 entries (~24 MB), plenty for these engines
	ttSize = 1 << ttBits
	ttMask = ttSize - 1
)

// transpositionTable caches search results keyed by Zobrist hash. It is accessed only from the single
// search goroutine, so it needs no synchronisation.
type transpositionTable struct {
	entries []ttEntry
}

func newTranspositionTable() *transpositionTable {
	return &transpositionTable{entries: make([]ttEntry, ttSize)}
}

// probe returns the entry for hash, if one is stored at its slot.
func (t *transpositionTable) probe(hash uint64) (ttEntry, bool) {
	e := t.entries[hash&ttMask]
	if e.bound != boundNone && e.hash == hash {
		return e, true
	}
	return ttEntry{}, false
}

// store records a result, preferring to keep a deeper entry already in the same slot for this position.
func (t *transpositionTable) store(hash uint64, depth uint, score int16, bound ttBound, move position.Move) {
	idx := hash & ttMask
	if existing := t.entries[idx]; existing.bound != boundNone && existing.hash == hash && existing.depth > uint8(depth) {
		return
	}
	t.entries[idx] = ttEntry{hash: hash, move: move, score: score, depth: uint8(depth), bound: bound}
}

// mateScoreBound is the threshold above (or below) which a score denotes a forced mate.
const mateScoreBound = position.MaxEval - 1000

// Mate scores encode the distance to mate from the root, so the same position transposed to at a
// different ply would have an inconsistent score. scoreToTT rewrites a mate score to be relative to the
// node it is stored at; scoreFromTT undoes that on the way out. Non-mate scores pass through unchanged.
// Without this the table simply skipped mate scores, so the engine re-found every mate from scratch on
// each deepening and each move (slow, and it never "remembered" a mate it had already seen).
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
