package search

type Searcher interface {
	Requests() chan Request
	Responses() chan string
	Root() error
	Stop()
	// PonderHit tells a ponder search that the move it was pondering on was actually played, so the
	// clock should start now.
	PonderHit()
	// SetThreads sets how many search threads to use (Lazy SMP).
	SetThreads(n int)
}
