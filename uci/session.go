package uci

import (
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"github.com/ollybritton/StupidChess/engines"
	"github.com/ollybritton/StupidChess/position"
)

type Session struct {
	engine    engines.Engine
	positions []*position.Position
}

func NewSession(eng engines.Engine) *Session {
	return &Session{
		engine:    eng,
		positions: []*position.Position{},
	}
}

func (s *Session) Handle(commandLine string) error {
	fields := strings.Fields(commandLine)

	if len(fields) == 0 {
		return nil
	}

	commandName := fields[0]
	arguments := fields[1:]

	var handler func(arguments []string) error

	switch commandName {
	//
	case "uci":
		handler = s.handleCommandUci
	case "isready":
		handler = s.handleCommandIsReady
	case "position":
		handler = s.handleCommandPosition
	case "go":
		handler = s.handleCommandGo

	// Special debugging commands not in the UCI protocol
	case "_pp", "_prettyprint":
		handler = s.handleCommandPrettyPrint

	// Handle unknown commands
	default:
		fmt.Printf("info string don't understand %s\n", commandName)
		handler = s.handleCommandUnknown
	}

	return handler(arguments)
}

func (s *Session) handleCommandUci(arguments []string) error {
	fmt.Printf("id name %s\n", s.engine.Name())
	fmt.Printf("id author %s\n", s.engine.Author())

	// TODO: implement options being printed out

	fmt.Println("uciok")
	return nil
}

func (s *Session) handleCommandIsReady(arguments []string) error {
	err := s.engine.Prepare()
	if err != nil {
		return err
	}

	fmt.Println("readyok")
	return nil
}

// handleCommandPosition is called when the GUI gives the "position" command.
// The position command in the UCI protocol has the following format:
//   position <fen string>|startpos moves <long algebraic notation moves>
// Long algebraic notation moves look like so:
//   e2e4, e7e5, e1g1 (white short castling), e7e8q (for promotion)
func (s *Session) handleCommandPosition(arguments []string) error {
	fen := ""
	var moves []string

	if len(arguments) == 0 {
		return fmt.Errorf("invalid position command sent: %v", strings.Join(arguments, " "))
	}

	if arguments[0] == "startpos" {
		fen = position.StartingPosition

		if len(arguments) == 1 {
			moves = []string{} // i.e. no moves were specified, it was just "position startpos"
		} else {
			moves = arguments[2:]
		}

	} else {
		all := strings.Join(arguments, " ")
		movesIndex := strings.Index(all, "moves")

		if movesIndex == -1 {
			fen = all
		} else {
			fen = all[:movesIndex-1] // Index of 'm', need end position of FEN string.
			moves = strings.Fields(all[movesIndex+6:])
		}

	}

	pos, err := position.NewPositionFromFEN(fen)
	if err != nil {
		return fmt.Errorf("invalid position command sent %q, can't parse FEN: %w", strings.Join(arguments, " "), err)
	}

	for _, move := range moves {
		parsed, err := position.ParseMove(move)
		if err != nil {
			return fmt.Errorf("invalid position command sent %q, can't understand move %q: %w", strings.Join(arguments, " "), move, err)
		}

		pos.MakeMove(parsed)
	}

	s.positions = append(s.positions, pos)

	return nil
}

func (s *Session) handleCommandGo(arguments []string) error {
	options := engines.SearchOptions{}

	var i int

	for i < len(arguments) {
		curr := arguments[i]

		switch curr {
		case "infinite":
			options.Infinite = true

		case "wtime", "btime":
			if i == len(arguments)-1 {
				return fmt.Errorf("expecting number after 'wtime/btime' option in 'go' command 'go %s'", strings.Join(arguments, " "))
			}

			i++
			millisecondsStr := arguments[i]
			milliseconds, err := strconv.Atoi(millisecondsStr)
			if err != nil {
				return fmt.Errorf("expecting number after 'wtime/btime' option in 'go' command 'go %s', got error: %w", strings.Join(arguments, " "), err)
			}

			if curr == "wtime" {
				options.WhiteTimeRemaining = uint(milliseconds)
			} else {
				options.BlackTimeRemaining = uint(milliseconds)
			}

		case "winc", "binc":
			if i == len(arguments)-1 {
				return fmt.Errorf("expecting number after 'winc/binc' option in 'go' command 'go %s'", strings.Join(arguments, " "))
			}

			i++
			secondsStr := arguments[i]
			seconds, err := strconv.ParseFloat(secondsStr, 64)
			if err != nil {
				return fmt.Errorf("expecting number after 'winc/binc' option in 'go' command 'go %s', got error: %w", strings.Join(arguments, " "), err)
			}

			if curr == "winc" {
				options.WhiteIncrement = seconds
			} else {
				options.BlackIncrement = seconds
			}

		case "movestogo":
			if i == len(arguments)-1 {
				return fmt.Errorf("expecting number after 'movestogo' option in 'go' command 'go %s'", strings.Join(arguments, " "))
			}

			i++
			movesStr := arguments[i]
			moves, err := strconv.Atoi(movesStr)
			if err != nil {
				return fmt.Errorf("expecting number after 'movestogo' option in 'go' command 'go %s', got error: %w", strings.Join(arguments, " "), err)
			}

			options.MovesToGo = uint(moves)

		case "depth":
			if i == len(arguments)-1 {
				return fmt.Errorf("expecting number after 'depth' option in 'go' command 'go %s'", strings.Join(arguments, " "))
			}

			i++
			depthStr := arguments[i]
			depth, err := strconv.Atoi(depthStr)
			if err != nil {
				return fmt.Errorf("expecting number after 'depth' option in 'go' command 'go %s', got error: %w", strings.Join(arguments, " "), err)
			}

			options.MovesToGo = uint(depth)

		case "nodes":
			if i == len(arguments)-1 {
				return fmt.Errorf("expecting number after 'nodes' option in 'go' command 'go %s'", strings.Join(arguments, " "))
			}

			i++
			nodesStr := arguments[i]
			nodes, err := strconv.Atoi(nodesStr)
			if err != nil {
				return fmt.Errorf("expecting number after 'nodes' option in 'go' command 'go %s', got error: %w", strings.Join(arguments, " "), err)
			}

			options.Nodes = uint(nodes)

		case "mate":
			if i == len(arguments)-1 {
				return fmt.Errorf("expecting number after 'mate' option in 'go' command 'go %s'", strings.Join(arguments, " "))
			}

			i++
			mateStr := arguments[i]
			mate, err := strconv.Atoi(mateStr)
			if err != nil {
				return fmt.Errorf("expecting number after 'nodes' option in 'go' command 'go %s', got error: %w", strings.Join(arguments, " "), err)
			}

			options.Mate = uint(mate)

		case "movetime":
			if i == len(arguments)-1 {
				return fmt.Errorf("expecting number after 'movetime' option in 'go' command 'go %s'", strings.Join(arguments, " "))
			}

			i++
			millisecondsStr := arguments[i]
			milliseconds, err := strconv.Atoi(millisecondsStr)
			if err != nil {
				return fmt.Errorf("expecting number after 'movetime' option in 'go' command 'go %s', got error: %w", strings.Join(arguments, " "), err)
			}

			options.MoveTime = time.Millisecond * time.Duration(milliseconds)

		case "ponder", "ponderhit":
			// TODO: implement ponder command
			return fmt.Errorf("ponder command currently not supported")
		}

		i++
	}

	// TODO: remove me
	if len(s.positions) == 0 {
		panic("uh-oh")
	}

	position := s.positions[len(s.positions)-1]
	moves := position.MovesPseudolegal()
	move := moves[rand.Intn(len(moves))]

	fmt.Println("bestmove", move)

	return nil
}

func (s *Session) handleCommandUnknown(arguments []string) error {
	return nil
}

func (s *Session) handleCommandPrettyPrint(arguments []string) error {
	if len(s.positions) == 0 {
		fmt.Println("nothing to pretty print, no positions yet")
	} else {
		fmt.Println("")
		fmt.Println(s.positions[len(s.positions)-1].PrettyPrint())
		fmt.Println("")
	}

	return nil
}
