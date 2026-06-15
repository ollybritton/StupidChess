package engines

import "github.com/ollybritton/StupidChess/position"

// NewEngineFortress builds the ultra-defensive engine. Unlike the other personality engines it plays
// real, searching chess and is genuinely trying to win, but through a defensive lens: its evaluation
// (EvalFortressUs) favours closed positions with its pawns and pieces kept back and huddled together,
// so it grinds from behind a wall rather than charging out. The strength comes from the same
// alpha-beta search, quiescence and transposition table as try-hard; only the evaluation differs.
func NewEngineFortress() Engine {
	return newSearchEngine(
		"fortress",
		"Olly Britton",
		"Plays to win but hates open positions: a full alpha-beta search wrapped around a defensive evaluation that keeps its pawns and pieces back and huddled, grinding away from behind a closed wall.",
		position.EvalFortressUs,
		position.EvalFortressThem,
	)
}
