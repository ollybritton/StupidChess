package match

import (
	"fmt"
	"math/bits"
	"time"

	"github.com/ollybritton/StupidChess/position"
	"github.com/ollybritton/StupidChess/search"
	"github.com/ollybritton/StupidChess/uciclient"
)

// EngineSpec describes one side of a match: which binary to run, the arguments
// that select the engine, and any UCI options to set before play.
type EngineSpec struct {
	Label   string            // human-readable name used in reports
	Path    string            // binary to launch
	Args    []string          // e.g. {"uci", "-e", "tryhard"}
	Options map[string]string // UCI setoption name->value pairs
}

// TimeControl fixes how long each move's search runs. A node budget is preferred
// for self-play because it is independent of machine load (many games run at
// once); MoveTime is offered as an alternative.
type TimeControl struct {
	Nodes    uint
	MoveTime time.Duration
}

// Outcome is the result of a game from White's point of view.
type Outcome int

const (
	WhiteWins Outcome = iota
	BlackWins
	Draw
)

// session is one engine subprocess held open across many games.
type session struct {
	spec EngineSpec
	gui  *uciclient.GUISession
}

func newSession(spec EngineSpec) (*session, error) {
	gui, err := uciclient.NewGUISessionFromBinary(spec.Path, spec.Args...)
	if err != nil {
		return nil, err
	}
	if err := gui.Open(); err != nil {
		return nil, err
	}
	for name, value := range spec.Options {
		if err := gui.SetOption(name, value); err != nil {
			return nil, fmt.Errorf("setoption %s=%s: %w", name, value, err)
		}
	}
	return &session{spec: spec, gui: gui}, nil
}

func (s *session) close() {
	if s != nil && s.gui != nil {
		_ = s.gui.Close()
	}
}

// searchOptions builds the per-move search options for a time control. Time
// remaining is zeroed so the engine searches exactly to the node (or movetime)
// budget rather than running its own clock management.
func searchOptions(tc TimeControl) search.SearchOptions {
	opts := search.NewDeafultOptions()
	opts.WhiteTimeRemaining = 0
	opts.BlackTimeRemaining = 0
	if tc.Nodes > 0 {
		opts.Nodes = tc.Nodes
		opts.MoveTime = time.Hour // a long clock so the node budget is what stops the search
	} else {
		opts.MoveTime = tc.MoveTime
	}
	return opts
}

// playGame plays one game between the two sessions (white moves first) from the
// start position followed by the opening line, and returns the outcome. A
// crashed or illegal-moving engine forfeits. maxPlies caps the game length, with
// the cap scored as a draw.
func playGame(white, black *session, opening []string, tc TimeControl, maxPlies int) (Outcome, error) {
	pos, err := position.NewPositionFromFEN(position.StartingPosition)
	if err != nil {
		return Draw, err
	}

	moves := make([]string, 0, maxPlies)
	history := map[uint64]int{pos.ZobristHash(): 1}

	for _, mv := range opening {
		if !applyUCIMove(pos, mv) {
			return Draw, fmt.Errorf("illegal opening move %q", mv)
		}
		moves = append(moves, mv)
		history[pos.ZobristHash()]++
	}

	opts := searchOptions(tc)

	for ply := 0; ply < maxPlies; ply++ {
		if over, outcome := adjudicate(pos, history); over {
			return outcome, nil
		}

		mover := white
		if pos.SideToMove == position.Black {
			mover = black
		}

		if err := mover.gui.SetPosition(position.StartingPosition, moves); err != nil {
			return Draw, fmt.Errorf("%s setposition: %w", mover.spec.Label, err)
		}
		move, err := mover.gui.Search(opts, nil)
		if err != nil {
			return Draw, fmt.Errorf("%s search: %w", mover.spec.Label, err)
		}

		uci := move.String()
		if move == position.NoMove || !applyUCIMove(pos, uci) {
			// The engine produced no move or an illegal one: it forfeits.
			if pos.SideToMove == position.White {
				return BlackWins, nil
			}
			return WhiteWins, nil
		}
		moves = append(moves, uci)
		history[pos.ZobristHash()]++
	}

	return Draw, nil
}

// applyUCIMove plays the move named in UCI long-algebraic notation on pos, by
// matching it against the generated legal moves (so the full move - captured
// piece, castling and en passant bits - is correct). It returns false if the
// move is not legal in pos.
func applyUCIMove(pos *position.Position, uci string) bool {
	parsed, err := position.ParseMove(uci)
	if err != nil {
		return false
	}
	for _, m := range pos.MovesLegal().AsSlice() {
		if m.From() == parsed.From() && m.To() == parsed.To() && m.Promotion() == parsed.Promotion() {
			return pos.MakeMove(m)
		}
	}
	return false
}

// adjudicate reports whether the game is over in pos (whose side to move is about
// to play) and, if so, the outcome. It covers checkmate and stalemate, the
// fifty-move rule, threefold repetition, and insufficient mating material.
func adjudicate(pos *position.Position, history map[uint64]int) (bool, Outcome) {
	if len(pos.MovesLegal().AsSlice()) == 0 {
		if pos.KingInCheck(pos.SideToMove) {
			if pos.SideToMove == position.White {
				return true, BlackWins // White is checkmated
			}
			return true, WhiteWins
		}
		return true, Draw // stalemate
	}
	if pos.HalfmoveClock >= 100 {
		return true, Draw
	}
	if history[pos.ZobristHash()] >= 3 {
		return true, Draw
	}
	if insufficientMaterial(pos) {
		return true, Draw
	}
	return false, Draw
}

// insufficientMaterial recognises the clear dead-draw material configurations:
// no pawns, rooks or queens remain and neither side has more than a single minor
// piece (so KvK, K+minor vs K and K+minor vs K+minor). It deliberately does not
// try to judge the rarer drawn fortresses; the fifty-move rule catches those.
func insufficientMaterial(pos *position.Position) bool {
	if pos.Pieces[position.Pawn] != 0 || pos.Pieces[position.Rook] != 0 || pos.Pieces[position.Queen] != 0 {
		return false
	}
	minors := pos.Pieces[position.Knight] | pos.Pieces[position.Bishop]
	w := bits.OnesCount64(uint64(pos.Occupied[position.White] & minors))
	b := bits.OnesCount64(uint64(pos.Occupied[position.Black] & minors))
	return w <= 1 && b <= 1
}
