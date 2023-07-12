package search

import (
	"fmt"
	"math"
	"strings"
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

	// TODO: implement pondering

	Stop bool
}

// NewDefaultOptions returns the default search options for an engine.
func NewDeafultOptions() SearchOptions {
	return SearchOptions{
		Infinite:           false,
		SearchMoves:        []position.Move{},
		Depth:              math.MaxUint,
		Nodes:              math.MaxUint,
		Mate:               0,
		MoveTime:           0,
		WhiteTimeRemaining: 1 * time.Hour, // TODO: sensible default?
		BlackTimeRemaining: 1 * time.Hour,
		WhiteIncrement:     0,
		BlackIncrement:     0,
		MovesToGo:          math.MaxUint,
	}
}

// AsUCI returns the options in the UCI format as a string.
// TODO: would it be better to have a seperate struct in the UCI package and then a function to convert between them?
func (opt *SearchOptions) AsUCI() string {
	var fields []string

	if len(opt.SearchMoves) != 0 {
		fields = append(fields, "searchmoves")

		for _, move := range opt.SearchMoves {
			fields = append(fields, move.String())
		}
	}

	// TODO: implement ponder

	if opt.WhiteTimeRemaining != 0 {
		fields = append(fields, fmt.Sprintf("wtime %d", opt.WhiteTimeRemaining.Milliseconds()))
	}

	if opt.BlackTimeRemaining != 0 {
		fields = append(fields, fmt.Sprintf("btime %d", opt.BlackTimeRemaining.Milliseconds()))
	}

	if opt.WhiteIncrement != 0 {
		fields = append(fields, fmt.Sprintf("winc %d", opt.WhiteIncrement.Milliseconds()))
	}

	if opt.BlackIncrement != 0 {
		fields = append(fields, fmt.Sprintf("binc %d", opt.BlackIncrement.Milliseconds()))
	}

	if opt.MovesToGo != math.MaxUint {
		fields = append(fields, fmt.Sprintf("movestogo %d", opt.MovesToGo))
	}

	if opt.Depth != math.MaxUint {
		fields = append(fields, fmt.Sprintf("depth %d", opt.Depth))
	}

	if opt.Nodes != math.MaxUint {
		fields = append(fields, fmt.Sprintf("nodes %d", opt.Nodes))
	}

	if opt.Mate != 0 {
		fields = append(fields, fmt.Sprintf("mate %d", opt.Mate))
	}

	if opt.MoveTime != 0 {
		fields = append(fields, fmt.Sprintf("movetime %d", opt.MoveTime.Milliseconds()))
	}

	if opt.Infinite {
		fields = append(fields, "infinite")
	}

	return strings.Join(fields, " ")
}
