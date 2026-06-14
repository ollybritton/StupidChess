package search

import (
	"testing"
	"time"

	"github.com/ollybritton/StupidChess/position"
)

// TestStopHasNoDataRace runs a search and concurrently calls Stop from another goroutine. The stop
// flag used to be a plain bool on the shared options struct, written by Stop and read by the search
// goroutine; this exercises that path so `go test -race` flags any regression.
func TestStopHasNoDataRace(t *testing.T) {
	requests := make(chan Request)
	responses := make(chan string, 4096)

	s := NewAlphaBetaSearch(requests, responses, position.EvalSimple, position.EvalSimple)
	go s.Root()
	go func() {
		for range responses { // drain so Root never blocks sending info
		}
	}()

	pos, err := position.NewPositionFromFEN(position.StartingPosition)
	if err != nil {
		t.Fatal(err)
	}

	opts := NewDeafultOptions()
	opts.MoveTime = time.Second // long enough that Stop races a live search

	requests <- NewRequest(pos, opts)

	// Hammer Stop from this goroutine while the search runs on Root's goroutine.
	for i := 0; i < 1000; i++ {
		s.Stop()
	}

	time.Sleep(200 * time.Millisecond) // let the search observe the stop and finish
}
