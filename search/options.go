package search

import (
	"math"
	"time"

	"github.com/ollybritton/StupidChess/position"
)

// SearchOptions represents options that can be passed to engines doing a search.
// Successive options narrow down the search. Therefore if both depth and node options are specified, whichever comes into
// effect first will terminate the search.
type SearchOptions struct {
	Infinite    bool            // Don't stop searching until being told to do so.
	SearchMoves []position.Move // Only explore these moves.
	Depth       uint            // Explore the search tree to this many plies only.
	Nodes       uint            // Only search this many nodes.
	Mate        uint            // Search for mate in this many moves.
	MoveTime    time.Duration   // Only search for the specified duration.

	WhiteTimeRemaining time.Duration // Time remaining for white.
	BlackTimeRemaining time.Duration // Time remaining for black.
	WhiteIncrement     time.Duration // Increment for white.
	BlackIncrement     time.Duration // Increment for black.
	MovesToGo          uint          // Number of moves until the next time control.

	Stop bool
}

// NewDefaultOptions returns the default search options for an engine.
func NewDeafultOptions() SearchOptions {
	return SearchOptions{
		Infinite:           false,
		SearchMoves:        []position.Move{},
		Depth:              math.MaxUint,
		Nodes:              math.MaxUint,
		Mate:               math.MaxUint,
		MoveTime:           0,
		WhiteTimeRemaining: 1 * time.Hour, // TODO: sensible default?
		BlackTimeRemaining: 1 * time.Hour,
		WhiteIncrement:     0,
		BlackIncrement:     0,
		MovesToGo:          math.MaxUint,
	}
}
