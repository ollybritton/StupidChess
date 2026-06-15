package lichess

import (
	"sync"

	"github.com/ollybritton/StupidChess/position"
	"github.com/ollybritton/StupidChess/search"
	"github.com/ollybritton/StupidChess/uciclient"
)

// Mover computes a move for a position and supports pondering (thinking on the opponent's clock). The
// bot depends on this interface (not on a concrete engine) so that games can be driven by a real UCI
// subprocess in production and by a stub in tests.
//
// Moves are UCI long-algebraic (e.g. "e2e4", "e7e8q"); an empty string means "no move". The ponder
// move a search returns is the reply it expects, used to decide what to think about next.
type Mover interface {
	// MoveWithPonder searches the position reached by playing moves from fen and returns the chosen
	// move plus the expected reply (empty if the engine offers none).
	MoveWithPonder(fen string, moves []string, opts search.SearchOptions) (best, ponder string, err error)
	// StartPonder begins thinking about the position after the given moves (which already include the
	// predicted reply), without a clock, returning immediately.
	StartPonder(fen string, moves []string, opts search.SearchOptions) error
	// PonderHit confirms the pondered move was actually played; the in-progress ponder search becomes a
	// real, timed search and this returns its move (and the next expected reply).
	PonderHit() (best, ponder string, err error)
	// StopPonder abandons a ponder search because the opponent played something unexpected.
	StopPonder() error
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

// MoveWithPonder sets the position and searches, returning the best move and the expected reply.
func (m *EngineMover) MoveWithPonder(fen string, moves []string, opts search.SearchOptions) (string, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.session.SetPosition(fen, moves); err != nil {
		return "", "", err
	}

	result, err := m.session.SearchWithPonder(opts, nil)
	if err != nil {
		return "", "", err
	}
	return uciOf(result.Best), uciOf(result.Ponder), nil
}

// StartPonder sets the predicted position and tells the engine to start pondering it.
func (m *EngineMover) StartPonder(fen string, moves []string, opts search.SearchOptions) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.session.SetPosition(fen, moves); err != nil {
		return err
	}
	return m.session.StartPonder(opts, nil)
}

// PonderHit confirms the ponder move and waits for the (now timed) search to produce a move.
func (m *EngineMover) PonderHit() (string, string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.session.PonderHit(); err != nil {
		return "", "", err
	}
	result, err := m.session.AwaitResult(nil)
	if err != nil {
		return "", "", err
	}
	return uciOf(result.Best), uciOf(result.Ponder), nil
}

// StopPonder aborts the ponder search and consumes the move the engine emits as it stops, so it does
// not leak into the next search.
func (m *EngineMover) StopPonder() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if err := m.session.Stop(); err != nil {
		return err
	}
	_, err := m.session.AwaitResult(nil)
	return err
}

// Close shuts the engine subprocess down.
func (m *EngineMover) Close() error {
	return m.session.Close()
}

// uciOf renders a move as UCI, or "" for NoMove.
func uciOf(move position.Move) string {
	if move == position.NoMove {
		return ""
	}
	return move.String()
}
