package engines

import (
	"github.com/ollybritton/StupidChess/position"
	"github.com/ollybritton/StupidChess/search"
)

type Engine interface {
	Name() string
	Author() string
	Description() string

	// Options lists the engine's UCI options, and SetOption applies one.
	Options() []EngineOption
	SetOption(name, value string) error

	NewGame() error
	Prepare() error
	Go(*position.Position, search.SearchOptions) error
	Stop()
}

// EngineOption describes one UCI option an engine exposes.
type EngineOption struct {
	Name    string
	Type    string // UCI option type, e.g. "check" or "spin"
	Default string
}

// noOptions is embedded by engines that expose no UCI options, so they satisfy the interface for free.
type noOptions struct{}

func (noOptions) Options() []EngineOption     { return nil }
func (noOptions) SetOption(_, _ string) error { return nil }
