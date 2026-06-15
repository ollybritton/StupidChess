package search

import "time"

// TimeManager decides how long to spend on a move, given the clock remaining, the increment, and how
// many moves remain until the next time control (0 / unknown is handled with an estimate).
type TimeManager func(timeRemaining, increment time.Duration, movesToGo uint) time.Duration

var DefaultTimeManager = defaultTimeManager

// moveOverhead is held back on every move to cover the time it takes to transmit the move and any
// scheduling jitter, so the engine doesn't flag by overshooting its own deadline.
const moveOverhead = 60 * time.Millisecond

// defaultTimeManager allocates time for one move. It spends an even share of the clock plus most of the
// increment, keeps a safety reserve, and never gambles too much of the remaining time on a single move
// so it survives time scrambles. Compared with the old "remaining/40" rule this is increment-aware and
// uses the moves-to-go hint, which lets the engine search deeper when it can afford to.
func defaultTimeManager(timeRemaining, increment time.Duration, movesToGo uint) time.Duration {
	if timeRemaining <= 0 {
		return 50 * time.Millisecond // emergency: move almost immediately
	}

	// Keep a little in reserve so we never spend the clock down to zero.
	available := timeRemaining - moveOverhead
	if available < timeRemaining/2 {
		available = timeRemaining / 2
	}

	// If the moves-to-go is unknown (or implausibly large), assume a typical middlegame horizon.
	moves := movesToGo
	if moves == 0 || moves > 40 {
		moves = 30
	}

	// An even slice of the remaining time, plus most of the increment (it is replenished each move, so
	// it can be spent freely).
	target := available/time.Duration(moves) + increment*3/4

	// Never spend more than a quarter of what's left on one move.
	if limit := available / 4; target > limit {
		target = limit
	}
	if target < 10*time.Millisecond {
		target = 10 * time.Millisecond
	}
	return target
}
