package engines

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

	WhiteTimeRemaining uint    // Time remaining for white in milliseconds.
	BlackTimeRemaining uint    // Time remaining for black in milliseconds.
	WhiteIncrement     float64 // Increment for white in seconds.
	BlackIncrement     float64 // Increment for black in seconds.
	MovesToGo          uint    // Number of moves until the next time control.
}

// NewDefaultOptions returns the default search options for an engine.
func NewDeafultOptions() SearchOptions {
	return SearchOptions{
		Infinite:           false,
		SearchMoves:        []position.Move{},
		Depth:              math.MaxUint,
		Nodes:              math.MaxUint,
		Mate:               math.MaxUint,
		MoveTime:           time.Duration(math.MaxInt64),
		WhiteTimeRemaining: math.MaxUint,
		BlackTimeRemaining: math.MaxUint,
		WhiteIncrement:     math.MaxFloat64,
		BlackIncrement:     math.MaxFloat64,
		MovesToGo:          math.MaxUint,
	}
}
