package position

import "fmt"

// Move represents a chess move.
// TODO: switch to a more compact binary format.
type Move struct {
	From      uint8
	To        uint8
	Promotion ColoredPiece
}

// ParseMove parses a UCI-style long algebraic notation move into a Move.
func ParseMove(str string) (Move, error) {
	var fromString, toString string
	var promotion ColoredPiece = Empty

	switch len(str) {
	case 4:
		fromString = str[0:2]
		toString = str[2:4]
	case 5:
		fromString = str[0:2]
		toString = str[2:4]
		promotionString := string(str[5])

		promotion = strToColored(promotionString)
	default:
		return Move{}, fmt.Errorf("invalid move")
	}

	return Move{
		From:      StringToSquare(fromString),
		To:        StringToSquare(toString),
		Promotion: promotion,
	}, nil

}
