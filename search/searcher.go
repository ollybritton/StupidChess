package search

type Searcher interface {
	Requests() chan Request
	Responses() chan string
	Listen() error
	Stop()
}
