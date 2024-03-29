package engines

var EngineInfo = map[string]Engine{
	"sprinter":    NewEngineSprinter(),
	"random":      NewEngineRandom(),
	"suicideking": NewEngineSuicideKing(),
	"tryhard":     NewEngineTryHard(),
	"pawnstar":    NewEnginePawnStar(),
}
