package search

import "time"

// TimeManager decides how long to spend on a move, given the clock remaining, the increment, and how
// many moves remain until the next time control (0 / unknown is handled with an estimate).
type TimeManager func(timeRemaining, increment time.Duration, movesToGo uint) time.Duration

var DefaultTimeManager = defaultTimeManager

// moveOverhead is held back from EVERY move's budget to cover the time it takes to transmit the move to
// Lichess (read the stream event, run the search, POST the move) plus scheduling jitter, so the engine
// does not flag by overshooting its real deadline. It is paid once per move, so it must be reserved per
// move - an earlier version subtracted it once from the whole clock, reserving ~2ms per move, which let
// the bot overspend a little on every move and lose on time in long or fast games.
const moveOverhead = 150 * time.Millisecond

// defaultTimeManager allocates time for one move. It spends an even share of the clock plus most of the
// increment, reserves the move overhead, and never gambles too much of the remaining time on a single
// move so it survives time scrambles. It is increment-aware and uses the moves-to-go hint, which lets
// the engine search deeper when it can afford to.
func defaultTimeManager(timeRemaining, increment time.Duration, movesToGo uint) time.Duration {
	if timeRemaining <= moveOverhead {
		return 10 * time.Millisecond // emergency: barely any clock left, move almost immediately
	}

	// If the moves-to-go is unknown (or implausibly large), assume a typical middlegame horizon.
	moves := movesToGo
	if moves == 0 || moves > 40 {
		moves = 30
	}

	// An even slice of the remaining time, plus most of the increment (replenished each move, so it can be
	// spent freely), minus the per-move overhead we must hold back.
	target := timeRemaining/time.Duration(moves) + increment*3/4 - moveOverhead

	// Never spend more than a quarter of what's left on one move.
	if limit := timeRemaining / 4; target > limit {
		target = limit
	}
	if target < 10*time.Millisecond {
		target = 10 * time.Millisecond
	}
	return target
}
