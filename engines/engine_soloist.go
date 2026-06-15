package engines

import (
	"fmt"
	"math/rand"

	"github.com/ollybritton/StupidChess/position"
	"github.com/ollybritton/StupidChess/search"
)

// EngineSoloist does everything with a single piece. It drags the same one around the board move after
// move, and only switches to a different piece when its favourite has nowhere left to go (boxed in, or
// captured). It is the spiritual opposite of the sprinter, which refuses to touch the same piece twice
// in a row.
type EngineSoloist struct {
	noOptions
	active    uint8 // the square its current piece sits on
	hasActive bool  // whether a piece has been chosen yet
}

func NewEngineSoloist() *EngineSoloist {
	return &EngineSoloist{}
}

func (e *EngineSoloist) Name() string   { return "soloist" }
func (e *EngineSoloist) Author() string { return "Olly Britton" }
func (e *EngineSoloist) Description() string {
	return "Fixates on one piece and moves only that piece, turn after turn, until it is captured or has nowhere to go; then it grudgingly adopts a new favourite (the one with the most freedom)."
}

func (e *EngineSoloist) NewGame() error {
	e.hasActive = false
	return nil
}

func (e *EngineSoloist) Prepare() error { return nil }
func (e *EngineSoloist) Stop()          {}

func (e *EngineSoloist) Go(pos *position.Position, _ search.SearchOptions) error {
	move := e.chooseMove(pos)
	if move == position.NoMove {
		fmt.Println("bestmove 0000")
		return nil
	}

	fmt.Println("bestmove", move.String())
	return nil
}

// chooseMove picks the soloist's move and updates which piece it is fixated on. It keeps moving the
// active piece while it can, and otherwise adopts the most mobile piece available.
func (e *EngineSoloist) chooseMove(pos *position.Position) position.Move {
	legal := pos.MovesLegal().AsSlice()
	if len(legal) == 0 {
		return position.NoMove
	}

	move, ok := e.pickFromActive(pos, legal)
	if !ok {
		move = e.pickNewPiece(pos, legal)
	}

	e.active = move.To()
	e.hasActive = true
	return move
}

// pickFromActive returns a move of the currently fixated piece, if it has any legal move.
func (e *EngineSoloist) pickFromActive(pos *position.Position, legal []position.Move) (position.Move, bool) {
	if !e.hasActive {
		return position.NoMove, false
	}

	var moves []position.Move
	for _, m := range legal {
		if m.From() == e.active {
			moves = append(moves, m)
		}
	}
	if len(moves) == 0 {
		return position.NoMove, false
	}

	return soloistChoose(pos, moves), true
}

// pickNewPiece adopts a new piece to fixate on: the one with the most legal moves, so the gimmick can
// last as long as possible before another switch is forced.
func (e *EngineSoloist) pickNewPiece(pos *position.Position, legal []position.Move) position.Move {
	byOrigin := map[uint8][]position.Move{}
	for _, m := range legal {
		byOrigin[m.From()] = append(byOrigin[m.From()], m)
	}

	bestFrom, bestCount := legal[0].From(), 0
	for from, moves := range byOrigin {
		if len(moves) > bestCount {
			bestFrom, bestCount = from, len(moves)
		}
	}

	return soloistChoose(pos, byOrigin[bestFrom])
}

// soloistChoose picks one move from a set, preferring a capture (so the lone piece actually achieves
// something) and otherwise choosing at random for variety.
func soloistChoose(pos *position.Position, moves []position.Move) position.Move {
	var captures []position.Move
	for _, m := range moves {
		if pos.Squares[m.To()] != position.Empty {
			captures = append(captures, m)
		}
	}

	pool := moves
	if len(captures) > 0 {
		pool = captures
	}

	return pool[rand.Intn(len(pool))]
}
