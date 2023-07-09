package search

import "time"

// TimeManager is a function that, given the time remaining and the increment, calculates how long we should spend
// on the position.
type TimeManager func(timeRemaining time.Duration, options SearchOptions) time.Duration

var DefaultTimeManager = defaultTimeManager

func defaultTimeManager(timeRemaining time.Duration, increment time.Duration) time.Duration {
	return maxDuration(
		timeRemaining/40,
		increment*4/5,
	)
}

func maxDuration(d1, d2 time.Duration) time.Duration {
	if d1 > d2 {
		return d1
	}

	return d2
}
