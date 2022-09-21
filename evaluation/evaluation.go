package evaluation

import "github.com/ollybritton/StupidChess/position"

// Evaluator decides the numerical value of a position.
type Evaluator func(p position.Position) float64
