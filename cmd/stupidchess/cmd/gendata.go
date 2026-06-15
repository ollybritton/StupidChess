package cmd

import (
	"bufio"
	"fmt"
	"math/rand"
	"os"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/ollybritton/StupidChess/position"
	"github.com/ollybritton/StupidChess/search"
	"github.com/spf13/cobra"
)

// gendataCmd self-plays games and records (position, game-result) pairs as training data for the NNUE
// evaluation. Many short games are played in parallel from randomised openings; every quiet position is
// labelled with the eventual result of the game it came from.
var gendataCmd = &cobra.Command{
	Use:   "gendata",
	Short: "self-play games to generate NNUE training data",
	Long: `Self-play games and append "<fen>;<result>" lines to a file, where result is 1.0 (white won),
0.5 (draw) or 0.0 (black won). This is the training set for cmd nnuetrain. It runs until the game target
is reached (0 = forever), so it can be left running in the background to accumulate data.`,
	Run: func(cmd *cobra.Command, args []string) {
		out, _ := cmd.Flags().GetString("out")
		target, _ := cmd.Flags().GetInt("games")
		nodes, _ := cmd.Flags().GetInt("nodes")
		workers, _ := cmd.Flags().GetInt("workers")
		randomPlies, _ := cmd.Flags().GetInt("random")

		if workers <= 0 {
			workers = runtime.NumCPU()
		}

		f, err := os.OpenFile(out, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			fmt.Println("could not open output:", err)
			os.Exit(1)
		}
		defer f.Close()

		lines := make(chan string, 4096)
		var games, positions int64

		// Writer goroutine: serialise all output through one buffered writer.
		var writerWG sync.WaitGroup
		writerWG.Add(1)
		go func() {
			defer writerWG.Done()
			w := bufio.NewWriter(f)
			defer w.Flush()
			for line := range lines {
				w.WriteString(line)
			}
		}()

		// Progress reporter.
		done := make(chan struct{})
		go func() {
			ticker := time.NewTicker(10 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-done:
					return
				case <-ticker.C:
					fmt.Printf("gendata: %d games, %d positions\n",
						atomic.LoadInt64(&games), atomic.LoadInt64(&positions))
				}
			}
		}()

		var wg sync.WaitGroup
		for i := 0; i < workers; i++ {
			wg.Add(1)
			go func(seed int64) {
				defer wg.Done()
				rng := rand.New(rand.NewSource(seed))
				eng := newGendataSearcher()
				for target == 0 || atomic.LoadInt64(&games) < int64(target) {
					n := playSelfGame(eng, rng, nodes, randomPlies, lines)
					atomic.AddInt64(&games, 1)
					atomic.AddInt64(&positions, int64(n))
				}
			}(time.Now().UnixNano() + int64(i))
		}

		wg.Wait()
		close(lines)
		writerWG.Wait()
		close(done)
		fmt.Printf("gendata: done — %d games, %d positions\n", games, positions)
	},
}

// gendataSearcher is a minimal in-process driver of the alpha-beta search for self-play.
type gendataSearcher struct {
	searcher *search.AlphaBetaSearch
}

func newGendataSearcher() *gendataSearcher {
	requests := make(chan search.Request)
	responses := make(chan string, 256)
	s := search.NewAlphaBetaSearch(requests, responses, position.EvalComplex, position.EvalComplex)
	go s.Root()
	return &gendataSearcher{searcher: s}
}

// bestMove searches the position to a node budget and returns the engine's move.
func (g *gendataSearcher) bestMove(pos *position.Position, nodes int) (position.Move, bool) {
	opts := search.NewDeafultOptions()
	opts.Nodes = uint(nodes)
	opts.MoveTime = time.Hour // bounded by the node budget, not the clock

	g.searcher.Requests() <- search.NewRequest(pos.Clone(), opts)
	for msg := range g.searcher.Responses() {
		if strings.HasPrefix(msg, "bestmove ") {
			uci := strings.Fields(msg)[1]
			for _, m := range pos.MovesLegal().AsSlice() {
				if m.String() == uci {
					return m, true
				}
			}
			return position.NoMove, false
		}
	}
	return position.NoMove, false
}

// playSelfGame plays one self-play game and emits a labelled line per quiet position. It returns how
// many positions it recorded.
func playSelfGame(eng *gendataSearcher, rng *rand.Rand, nodes, randomPlies int, lines chan<- string) int {
	pos, _ := position.NewPositionFromFEN(position.StartingPosition)

	// Random opening for diversity.
	for i := 0; i < randomPlies; i++ {
		legal := pos.MovesLegal().AsSlice()
		if len(legal) == 0 {
			return 0
		}
		pos.MakeMove(legal[rng.Intn(len(legal))])
	}

	type record struct{ fen string }
	var records []record
	seen := map[uint64]int{}

	result := 0.5
	for ply := 0; ply < 300; ply++ {
		legal := pos.MovesLegal().AsSlice()
		if len(legal) == 0 {
			if pos.KingInCheck(pos.SideToMove) {
				if pos.SideToMove == position.White {
					result = 0.0 // white is mated
				} else {
					result = 1.0
				}
			} // else stalemate -> draw (0.5)
			break
		}
		if pos.HalfmoveClock >= 100 {
			break // fifty-move draw
		}
		h := pos.ZobristHash()
		if seen[h]++; seen[h] >= 3 {
			break // threefold draw
		}

		// Record quiet positions (not in check) for training.
		if !pos.KingInCheck(pos.SideToMove) {
			records = append(records, record{fen: pos.StringFEN()})
		}

		move, ok := eng.bestMove(pos, nodes)
		if !ok {
			move = legal[rng.Intn(len(legal))]
		}
		pos.MakeMove(move)
	}

	// If the game ran to the cap, adjudicate by the static evaluation.
	if result == 0.5 {
		switch e := position.EvalComplex(pos); {
		case e > 600:
			result = 1.0
		case e < -600:
			result = 0.0
		}
	}

	var b strings.Builder
	for _, r := range records {
		fmt.Fprintf(&b, "%s;%.1f\n", r.fen, result)
	}
	lines <- b.String()
	return len(records)
}

func init() {
	gendataCmd.Flags().StringP("out", "o", "nnue-data.txt", "output data file (appended to)")
	gendataCmd.Flags().Int("games", 0, "number of games to play (0 = run until stopped)")
	gendataCmd.Flags().Int("nodes", 5000, "search node budget per move")
	gendataCmd.Flags().Int("workers", 0, "parallel games (0 = number of CPUs)")
	gendataCmd.Flags().Int("random", 8, "random opening plies for diversity")
	rootCmd.AddCommand(gendataCmd)
}
