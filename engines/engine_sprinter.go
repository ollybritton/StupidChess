package engines

import (
	"fmt"

	"github.com/ollybritton/StupidChess/position"
	"github.com/ollybritton/StupidChess/search"
)

// sprinterDepth is how many plies ahead the sprinter plans. Looking ahead lets it pick a move now that
// sets up another long move later, rather than grabbing the single furthest move each turn.
const sprinterDepth = 3

type EngineSprinter struct {
	prevPiece position.Piece
}

func NewEngineSprinter() *EngineSprinter {
	return &EngineSprinter{prevPiece: position.None}
}

func (e *EngineSprinter) Name() string {
	return "sprinter"
}

func (e *EngineSprinter) Author() string {
	return "Olly Britton"
}

func (e *EngineSprinter) Description() string {
	return "Covers as much board distance as possible over the game, planning ahead; never moves the same piece twice in a row."
}

func (e *EngineSprinter) NewGame() error {
	e.prevPiece = position.None
	return nil
}

func (e *EngineSprinter) Prepare() error {
	return nil
}

func (e *EngineSprinter) Go(pos *position.Position, _ search.SearchOptions) error {
	move := bestSprinterMove(pos, sprinterDepth, e.prevPiece)
	if move == position.NoMove {
		fmt.Println("bestmove 0000")
		return nil
	}

	e.prevPiece = move.Moved().Colorless()
	fmt.Println("bestmove", move.String())
	return nil
}

func (e *EngineSprinter) Stop() {}
