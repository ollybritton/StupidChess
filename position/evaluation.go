package position

// Evaluator decides the numerical value of a position.
// An int16 is used so that evaluations can be packed into moves compactly.
// TODO: Consider refactoring into seperate package
type Evaluator func(p *Position) int16

var EvaluatorInfo = map[string]Evaluator{
	"simple": EvalSimple,
}

// GetEvaluator looks up an evaluator by name.
func GetEvaluator(name string) Evaluator {
	return EvaluatorInfo[name]
}

// ScoreFromPerspective takes in a score where negative scores represent good positions for black and positive positions
// for white, and takes in a side to move, and returns a positive evaluation from their perspective.
func ScoreFromPerspective(score int16, sideToMove Color) int16 {
	if sideToMove == Black {
		return -score
	}

	return score
}
