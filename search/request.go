package search

import "github.com/ollybritton/StupidChess/position"

// Request represents a request to a search algorithm to begin searching from a given position.
type Request struct {
	pos     *position.Position
	options SearchOptions
}

func NewRequest(pos *position.Position, searchOptions SearchOptions) Request {
	return Request{
		pos:     pos,
		options: searchOptions,
	}
}
