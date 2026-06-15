package lichess

import (
	"sync"

	"github.com/ollybritton/StupidChess/position"
	"github.com/ollybritton/StupidChess/search"
	"github.com/ollybritton/StupidChess/uciclient"
)

// Mover computes a move for a position. The bot depends on this interface (not on a concrete engine)
// so that games can be driven by a real UCI subprocess in production and by a stub in tests.
type Mover interface {
	// Move returns the engine's chosen move in UCI long-algebraic form (e.g. "e2e4", "e7e8q") for the
	// position reached by playing moves from fen.
	Move(fen string, moves []string, opts search.SearchOptions) (string, error)
	Close() error
}

// EngineMover drives a StupidChess engine running as a UCI subprocess.
type EngineMover struct {
	mu      sync.Mutex
	session *uciclient.GUISession
}

// NewEngineMover launches an engine binary (e.g. the stupidchess binary itself with "uci -e fortress")
// and completes the UCI handshake and new-game setup.
func NewEngineMover(path string, args []string) (*EngineMover, error) {
	session, err := uciclient.NewGUISessionFromBinary(path, args...)
	if err != nil {
		return nil, err
	}

	if err := session.Open(); err != nil {
		return nil, err
	}
	if err := session.NewGame(); err != nil {
		return nil, err
	}

	return &EngineMover{session: session}, nil
}

// Move sets the position and searches, returning the engine's best move as a UCI string.
func (m *EngineMover) Move(fen string, moves []string, opts search.SearchOptions) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.session.SetPosition(fen, moves); err != nil {
		return "", err
	}

	move, err := m.session.Search(opts, nil)
	if err != nil {
		return "", err
	}
	if move == position.NoMove {
		return "", nil
	}

	return move.String(), nil
}

// Close shuts the engine subprocess down.
func (m *EngineMover) Close() error {
	return m.session.Close()
}
