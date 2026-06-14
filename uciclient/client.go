// Package uciclient drives an external UCI engine from the perspective of the controller (what the
// UCI documentation calls the "GUI"). It lives in its own package — depending only on position and
// search, not engines — so that it can be used both by the web server and by engines that consult an
// oracle (e.g. worstfish driving Stockfish) without an import cycle.
package uciclient

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync/atomic"
	"time"

	"github.com/ollybritton/StupidChess/position"
	"github.com/ollybritton/StupidChess/search"
)

// GUISession drives a UCI engine.
//
//	sess, _ := NewGUISessionFromBinary("stockfish")
//	sess.Open()                       // handshake: uci -> uciok
//	sess.SetPosition(fen, moves)      // position fen ... moves ...
//	move, _ := sess.Search(opts, onInfo)
//	sess.Close()
//
// The reader runs on its own goroutine and never blocks: info lines are delivered on a buffered
// channel (dropped if nobody is listening) and best moves on a small buffered channel, so a slow or
// absent consumer can't wedge the engine.
type GUISession struct {
	in  io.Writer
	out io.Reader

	command *exec.Cmd

	moves chan position.Move
	infos chan string
	ready chan struct{}
	errs  chan error

	closed int32

	// Trace, if set, is invoked for every line sent to ("send") and received from ("recv") the engine.
	// It is used by the web UI to surface raw UCI traffic in its debug console.
	Trace func(dir, line string)
}

// readyTimeout bounds how long we wait for uciok / readyok before giving up on the engine.
const readyTimeout = 5 * time.Second

func newGUISession(in io.Writer, out io.Reader, command *exec.Cmd) *GUISession {
	return &GUISession{
		in:      in,
		out:     out,
		command: command,
		moves:   make(chan position.Move, 1),
		infos:   make(chan string, 256),
		ready:   make(chan struct{}, 1),
		errs:    make(chan error, 1),
	}
}

// NewGUISession builds a session around an arbitrary reader/writer pair (mainly useful for tests).
func NewGUISession(in io.Writer, out io.Reader) *GUISession {
	return newGUISession(in, out, nil)
}

// NewGUISessionFromBinary launches an engine binary and wires up its stdin/stdout.
func NewGUISessionFromBinary(path string, args ...string) (*GUISession, error) {
	expandedPath, err := exec.LookPath(path)
	if errors.Is(err, exec.ErrDot) {
		err = nil
		expandedPath = path
	}
	if err != nil {
		return nil, fmt.Errorf("couldn't find binary on path: %w", err)
	}

	command := exec.Command(expandedPath, args...)

	stdin, err := command.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("couldn't get stdin pipe from binary %s: %w", path, err)
	}

	stdout, err := command.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("couldn't get stdout pipe from binary %s: %w", path, err)
	}

	if err := command.Start(); err != nil {
		return nil, fmt.Errorf("couldn't start chess engine binary: %w", err)
	}

	return newGUISession(stdin, stdout, command), nil
}

func (s *GUISession) trace(dir, line string) {
	if s.Trace != nil {
		s.Trace(dir, line)
	}
}

func (s *GUISession) sendCommand(format string, a ...interface{}) error {
	line := fmt.Sprintf(format, a...)
	s.trace("send", line)

	if _, err := io.WriteString(s.in, line+"\n"); err != nil {
		return fmt.Errorf("couldn't send command %q: %w", line, err)
	}

	return nil
}

// Open starts the reader goroutine and performs the uci -> uciok handshake.
func (s *GUISession) Open() error {
	go s.readLoop()

	if err := s.sendCommand("uci"); err != nil {
		return err
	}

	return s.waitReady()
}

func (s *GUISession) readLoop() {
	scanner := bufio.NewScanner(s.out)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Text()
		s.trace("recv", line)
		s.handleCommand(line)
	}

	// The engine's output closed (it exited or errored). Surface that so any in-flight Search unblocks
	// instead of hanging forever on the moves channel.
	if atomic.LoadInt32(&s.closed) == 0 {
		select {
		case s.errs <- fmt.Errorf("engine exited unexpectedly"):
		default:
		}
	}
}

func (s *GUISession) handleCommand(line string) {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return
	}

	switch fields[0] {
	case "uciok", "readyok":
		select {
		case s.ready <- struct{}{}:
		default:
		}

	case "info":
		select {
		case s.infos <- line:
		default: // drop info if nobody is currently consuming it
		}

	case "bestmove":
		if len(fields) < 2 {
			return
		}
		move, err := position.ParseMove(fields[1])
		if err != nil {
			select {
			case s.errs <- fmt.Errorf("invalid bestmove %q: %w", line, err):
			default:
			}
			return
		}
		select {
		case s.moves <- move:
		default:
		}

	default:
		// Ignore anything else (id lines, option lines, banner text, "info string" is handled above).
	}
}

func (s *GUISession) waitReady() error {
	select {
	case <-s.ready:
		return nil
	case err := <-s.errs:
		return err
	case <-time.After(readyTimeout):
		return fmt.Errorf("timed out waiting for engine handshake")
	}
}

// IsReady sends "isready" and blocks until the engine answers "readyok".
func (s *GUISession) IsReady() error {
	if err := s.sendCommand("isready"); err != nil {
		return err
	}
	return s.waitReady()
}

// NewGame tells the engine a new game is starting and waits for it to be ready.
func (s *GUISession) NewGame() error {
	if err := s.sendCommand("ucinewgame"); err != nil {
		return err
	}
	return s.IsReady()
}

// SetOption sets a UCI option, e.g. SetOption("MultiPV", "20").
func (s *GUISession) SetOption(name, value string) error {
	return s.sendCommand("setoption name %s value %s", name, value)
}

// SetPosition sends the position as a FEN plus a list of UCI moves played from it.
func (s *GUISession) SetPosition(fen string, moves []string) error {
	cmd := "position fen " + fen
	if len(moves) > 0 {
		cmd += " moves " + strings.Join(moves, " ")
	}
	return s.sendCommand(cmd)
}

// Search sends "go" with the given options and returns the engine's best move. Every info line that
// arrives while searching is passed to onInfo (which may be nil).
func (s *GUISession) Search(options search.SearchOptions, onInfo func(line string)) (position.Move, error) {
	// Drain any stale info left over from a previous search so it isn't attributed to this one.
	for {
		select {
		case <-s.infos:
			continue
		default:
		}
		break
	}

	if err := s.sendCommand("go " + options.AsUCI()); err != nil {
		return position.NoMove, err
	}

	for {
		select {
		case line := <-s.infos:
			if onInfo != nil {
				onInfo(line)
			}
		case move := <-s.moves:
			return move, nil
		case err := <-s.errs:
			return position.NoMove, err
		}
	}
}

// Stop asks the engine to stop searching and report a best move.
func (s *GUISession) Stop() error {
	return s.sendCommand("stop")
}

// Close shuts the engine down and frees its resources.
func (s *GUISession) Close() error {
	atomic.StoreInt32(&s.closed, 1)

	if err := s.sendCommand("quit"); err != nil {
		return fmt.Errorf("couldn't close uci session: %w", err)
	}

	if s.command != nil {
		if err := s.command.Wait(); err != nil {
			return fmt.Errorf("couldn't free resources after uci session: %w", err)
		}
	}

	return nil
}
