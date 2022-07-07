package uci

import (
	"fmt"
	"strings"

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
	// TODO: parse 'go' command into SearchOptions

	// Special debugging commands not in the UCI protocol
	case "_pp", "_prettyprint":
		handler = s.handleCommandPrettyPrint

	// Handle unknown commands
	default:
		fmt.Printf("info string don't understand %s", commandName)
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

func (s *Session) handleCommandUnknown(arguments []string) error {
	return nil
}

func (s *Session) handleCommandPrettyPrint(arguments []string) error {
	fmt.Println("")
	fmt.Println(s.positions[len(s.positions)-1].PrettyPrint())
	fmt.Println("")

	return nil
}
