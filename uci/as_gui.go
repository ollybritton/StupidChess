package uci

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"

	"github.com/ollybritton/StupidChess/position"
	"github.com/ollybritton/StupidChess/search"
)

// GUISession represents a UCI session from the perspective of the GUI.
// This code might not actually be used by a GUI application and instead be used to e.g. automate
// tournaments between various engines, but is called a GUI session to match the UCI documentation.
//
// s := NewGUISession()
// go s.Listen()
// bestmove := s.Go(opts)
type GUISession struct {
	in  io.Writer
	out io.Reader

	command *exec.Cmd
	stopped bool

	moves  chan position.Move
	errors chan error
}

func NewGUISession(in io.Writer, out io.Reader) *GUISession {
	return &GUISession{
		in:     in,
		out:    out,
		moves:  make(chan position.Move),
		errors: make(chan error),
	}
}

func NewGUISessionFromBinary(path string, args ...string) (*GUISession, error) {
	expandedPath, err := exec.LookPath(path)
	if errors.Is(err, exec.ErrDot) {
		err = nil
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

	err = command.Start()
	if err != nil {
		return nil, fmt.Errorf("couldn't start chess engine binary: %w", err)
	}

	return &GUISession{
		in:      stdin,
		out:     stdout,
		command: command,
		moves:   make(chan position.Move),
		errors:  make(chan error),
	}, nil
}

func (s *GUISession) sendCommand(command string, a ...interface{}) error {
	_, err := io.WriteString(s.in, fmt.Sprintf(command, a...)+"\n")
	if err != nil {
		return fmt.Errorf("couldn't send command '%s': %w", command, err)
	}

	return nil
}

func (s GUISession) handleCommand(line string) error {
	fields := strings.Fields(line)

	if len(fields) == 0 {
		return nil
	}

	command := fields[0]
	args := fields[1:]

	switch command {
	case "info":
		fmt.Println(line)

	case "bestmove":
		if len(args) == 0 {
			return fmt.Errorf("bestmove command '%s' invalid", line)
		}

		move, err := position.ParseMove(args[0])
		if err != nil {
			return fmt.Errorf("bestmove command '%s' invalid: %w", line, err)
		}

		s.moves <- move

	default:
		fmt.Println("got other", line)
	}

	return nil
}

func (s *GUISession) initialise() error {
	err := s.sendCommand("uci")
	if err != nil {
		return fmt.Errorf("couldn't initialise engine: %w", err)
	}

	return nil
}

func (s *GUISession) Open() error {
	go func() {
		scanner := bufio.NewScanner(s.out)

		s.initialise()

		for !s.stopped && scanner.Scan() {
			line := scanner.Text()
			err := s.handleCommand(line)

			if err != nil {
				s.errors <- err
			}
		}

	}()

	return nil
}

func (s *GUISession) Close() error {
	err := s.sendCommand("quit")
	if err != nil {
		return fmt.Errorf("couldn't close uci session: %w", err)
	}

	s.stopped = true

	if s.command != nil {
		err = s.command.Wait()
		if err != nil {
			return fmt.Errorf("couldn't free resources after uci session: %w", err)
		}
	}

	return nil
}

func (s *GUISession) LoadPosition(pos *position.Position) {
	s.sendCommand("position %s", pos.StringFEN())
}

func (s *GUISession) Go(options search.SearchOptions) position.Move {
	s.sendCommand("go " + options.AsUCI())
	move := <-s.moves
	return move
}
