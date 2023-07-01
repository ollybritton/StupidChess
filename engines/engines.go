package engines

import (
	"github.com/ollybritton/StupidChess/position"
	"github.com/ollybritton/StupidChess/search"
)

type Engine interface {
	Name() string
	Author() string

	NewGame() error
	Prepare() error
	Search(*position.Position, search.SearchOptions) (position.Move, error)
}
