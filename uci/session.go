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
	moves     []position.Move
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
	case "ucinewgame":
		handler = s.handleCommandNewGame

	// Special debugging commands not in the UCI protocol
	case "_pp", "_prettyprint":
		handler = s.handleCommandPrettyPrint

	case "_bb", "_bitboards":
		handler = s.handleCommandBitboards

	case "_pmv", "_pseudolegalmoves":
		handler = s.handleCommandPseudolegalMoves

	case "_lmv", "_legalmoves":
		handler = s.handleCommandLegalMoves

	case "_isa", "_isattacked":
		handler = s.handleCommandIsAttacked

	case "_mm", "_makemove":
		handler = s.handleCommandMakeMove

	case "_pft", "_perft":
		handler = s.handleCommandPerft

	case "_div", "_divide":
		handler = s.handleCommandDivide

	case "_fen", "_printfen":
		handler = s.handleCommandFen

	case "_um", "_undomove":
		handler = s.handleCommandUndoMove

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

	seed := time.Now().Unix()
	fmt.Println("info string rng seed", seed)
	rand.Seed(time.Now().Unix())

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
//   position fen <fen string>|startpos moves <long algebraic notation moves>
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
			fen = strings.TrimPrefix(all, "fen ")
		} else {
			fen = all[:movesIndex-1] // Index of 'm', need end position of FEN string.
			fen = strings.TrimPrefix(fen, "fen ")
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
	options := engines.NewDeafultOptions()

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

	position := s.positions[len(s.positions)-1]
	bestMove, err := s.engine.Search(position, options)
	if err != nil {
		return fmt.Errorf("got an error searching for a move, %s", err)
	}

	fmt.Println("bestmove", bestMove.String())

	return nil
}

func (s *Session) handleCommandNewGame(arguments []string) error {
	// TODO: implement special logic around ucinewgame command
	err := s.engine.NewGame()
	if err != nil {
		return err
	}

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

func (s *Session) handleCommandBitboards(arguments []string) error {
	if len(s.positions) == 0 {
		fmt.Println("nothing to pretty print, no positions yet")
	} else {
		curr := s.positions[len(s.positions)-1]
		fmt.Println("")

		fmt.Println("WHITE occupation:")
		fmt.Println(curr.Occupied[position.White].String())
		fmt.Println("")

		fmt.Println("BLACK occupation:")
		fmt.Println(curr.Occupied[position.Black].String())
		fmt.Println("")

		fmt.Println("PAWNS:")
		fmt.Println(curr.Pieces[position.Pawn].String())
		fmt.Println("")

		fmt.Println("KNIGHTS:")
		fmt.Println(curr.Pieces[position.Knight].String())
		fmt.Println("")

		fmt.Println("BISHOPS:")
		fmt.Println(curr.Pieces[position.Bishop].String())
		fmt.Println("")

		fmt.Println("ROOKS:")
		fmt.Println(curr.Pieces[position.Rook].String())
		fmt.Println("")

		fmt.Println("QUEENS:")
		fmt.Println(curr.Pieces[position.Queen].String())
		fmt.Println("")

		fmt.Println("KINGS:")
		fmt.Println(curr.Pieces[position.King].String())
		fmt.Println("")
	}

	return nil
}

func (s *Session) handleCommandPseudolegalMoves(arguments []string) error {
	length := len(s.positions)

	if length == 0 {
		return fmt.Errorf("no positions to analyse")
	}

	full := false
	if len(arguments) == 1 && arguments[0] == "full" {
		full = true
	}

	for i, move := range s.positions[length-1].MovesPseudolegal().AsSlice() {
		if !full {
			fmt.Println(move.String())
		} else {
			fmt.Printf("(%d) %s\n", i+1, move.FullString())
		}
	}

	return nil
}

func (s *Session) handleCommandLegalMoves(arguments []string) error {
	length := len(s.positions)

	if length == 0 {
		return fmt.Errorf("no positions to analyse")
	}

	full := false
	if len(arguments) == 1 && arguments[0] == "full" {
		full = true
	}

	for i, move := range s.positions[length-1].MovesLegal().AsSlice() {
		if !full {
			fmt.Println(move.String())
		} else {
			fmt.Printf("(%d) %s\n", i+1, move.FullString())
		}
	}

	return nil
}

func (s *Session) handleCommandIsAttacked(arguments []string) error {
	length := len(s.positions)

	if length == 0 {
		return fmt.Errorf("no positions to make move on")
	}

	if len(arguments) != 2 {
		return fmt.Errorf("need a square and a color")
	}

	square := position.StringToSquare(arguments[0])

	var side position.Color

	switch arguments[1][0] {
	case 'w':
		side = position.White
	case 'b':
		side = position.Black
	}

	fmt.Println(s.positions[length-1].IsAttacked(square, side))
	return nil
}

func (s *Session) handleCommandMakeMove(arguments []string) error {
	length := len(s.positions)

	if length == 0 {
		return fmt.Errorf("no positions to make move on")
	}

	currPosition := s.positions[length-1]

	for _, moveStr := range arguments {
		// TODO: Defining the move twice like this is a bit iffy, just doing it like this because the parse move
		// function doesn't know anything about the position. Maybe parseMove should take a position.
		incompleteMove, err := position.ParseMove(moveStr)
		if err != nil {
			return err
		}

		move := position.NewMove(
			incompleteMove.From(),
			incompleteMove.To(),
			currPosition.Squares[incompleteMove.From()],
			currPosition.Squares[incompleteMove.To()],
			incompleteMove.Promotion(),
			currPosition.Castling,
			currPosition.EnPassant,
		)

		s.positions[length-1].MakeMove(move)
		s.moves = append(s.moves, move)
	}

	return nil
}

func (s *Session) handleCommandPerft(arguments []string) error {
	if len(s.positions) == 0 {
		return fmt.Errorf("no position to analyse")
	}

	if len(arguments) != 1 {
		return fmt.Errorf("need perft depth as an integer, got nothing")
	}

	num, err := strconv.ParseUint(arguments[0], 10, 0)
	if err != nil {
		return fmt.Errorf("need perft depth as an integer, got arguments %v and error: %w", arguments, err)
	}

	fmt.Println(s.positions[len(s.positions)-1].Perft(uint(num)))

	return nil
}

func (s *Session) handleCommandDivide(arguments []string) error {
	if len(s.positions) == 0 {
		return fmt.Errorf("no position to analyse")
	}

	if len(arguments) != 1 {
		return fmt.Errorf("need divide depth as an integer, got nothing")
	}

	num, err := strconv.ParseUint(arguments[0], 10, 0)
	if err != nil {
		return fmt.Errorf("need divide depth as an integer, got arguments %v and error: %w", arguments, err)
	}

	s.positions[len(s.positions)-1].Divide(uint(num))

	return nil
}

func (s *Session) handleCommandFen(arguments []string) error {
	length := len(s.positions)
	if length == 0 {
		return fmt.Errorf("no position to analyse")
	}

	pos := s.positions[length-1]
	fmt.Println(pos.StringFEN())

	return nil
}

func (s *Session) handleCommandUndoMove(arguments []string) error {
	movesLength := len(s.moves)
	if movesLength == 0 {
		return fmt.Errorf("no moves to undo")
	}

	positionsLength := len(s.positions)
	if positionsLength == 0 {
		return fmt.Errorf("no position to analyse")
	}

	move := s.moves[movesLength-1]
	s.positions[positionsLength-1].UndoMove(move)

	return nil
}
