package engines

type Engine interface {
	Name() string
	Author() string

	Prepare() error
}
