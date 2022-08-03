package engines

import "github.com/ollybritton/StupidChess/position"

type Engine interface {
	Name() string
	Author() string

	Prepare() error
	Search(*position.Position, SearchOptions) (position.Move, error)
}
