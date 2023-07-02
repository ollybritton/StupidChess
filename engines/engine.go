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
	Go(*position.Position, search.SearchOptions) error
	Stop()
}
