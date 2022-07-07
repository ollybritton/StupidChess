package engines

import (
	"time"

	"github.com/ollybritton/StupidChess/position"
)

// SearchOptions represents options that can be passed to engines doing a search.
// Successive options narrow down the search. Therefore if both depth and node options are specified, whichever comes into
// effect first will terminate the search.
type SearchOptions struct {
	Infinite    bool            // Don't stop searching until being told to do so.
	SearchMoves []position.Move // Only explore these moves.
	Depth       int             // Explore the search tree to this many plies only.
	Nodes       int             // Only search this many nodes.
	MoveTime    time.Duration   // Only search for the specified duration.
}
