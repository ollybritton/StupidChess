package position

// Evaluator decides the numerical value of a position.
// An int16 is used so that evaluations can be packed into moves compactly.
// TODO: Consider refactoring into seperate package
type Evaluator func(p *Position) int16

var EvaluatorInfo = map[string]Evaluator{
	"simple": EvalSimple,
}

func GetEvaluator(name string) Evaluator {
	return EvaluatorInfo[name]
}
