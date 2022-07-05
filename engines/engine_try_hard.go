package engines

type EngineTryHard struct{}

func NewEngineTryHard() *EngineTryHard {
	return &EngineTryHard{}
}

func (e *EngineTryHard) Name() string {
	return "try-hard"
}

func (e *EngineTryHard) Author() string {
	return "Olly Britton"
}

func (e *EngineTryHard) Prepare() error {
	return nil
}
