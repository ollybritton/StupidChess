package position

// Evaluator decides the numerical value of a position.
// An int16 is used so that evaluations can be packed into moves compactly.
type Evaluator func(p *Position) int16
