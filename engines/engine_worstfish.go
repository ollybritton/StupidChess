package engines

import (
	"fmt"
	"math/rand"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/ollybritton/StupidChess/position"
	"github.com/ollybritton/StupidChess/search"
	"github.com/ollybritton/StupidChess/uciclient"
)

// EngineWorstfish consults Stockfish and plays the move Stockfish rates WORST. It is the canonical
// Elo-World "Worstfish": let a strong engine rank every legal move, then pick the bottom of the list.
// It works by setting Stockfish's MultiPV to the number of legal moves so a single search ranks them
// all, then taking the move at the lowest rank. If Stockfish isn't installed it falls back to random.
type EngineWorstfish struct {
	oracle *uciclient.GUISession
}

func NewEngineWorstfish() *EngineWorstfish {
	return &EngineWorstfish{}
}

func (e *EngineWorstfish) Name() string   { return "worstfish" }
func (e *EngineWorstfish) Author() string { return "Olly Britton" }
func (e *EngineWorstfish) Description() string {
	return "Asks Stockfish to rank every move, then plays the one it rates worst."
}

// Prepare launches the Stockfish oracle once. If Stockfish isn't on PATH, the oracle stays nil and
// the engine falls back to random moves (rather than failing).
func (e *EngineWorstfish) Prepare() error {
	if e.oracle != nil {
		return nil
	}

	path, err := exec.LookPath("stockfish")
	if err != nil {
		return nil
	}

	sess, err := uciclient.NewGUISessionFromBinary(path)
	if err != nil {
		return nil
	}
	if err := sess.Open(); err != nil {
		sess.Close()
		return nil
	}

	e.oracle = sess
	return nil
}

func (e *EngineWorstfish) NewGame() error {
	if e.oracle != nil {
		return e.oracle.NewGame()
	}
	return nil
}

func (e *EngineWorstfish) Stop() {
	if e.oracle != nil {
		e.oracle.Stop()
	}
}

func (e *EngineWorstfish) Go(pos *position.Position, options search.SearchOptions) error {
	legal := pos.MovesLegal().AsSlice()
	if len(legal) == 0 {
		fmt.Println("bestmove 0000")
		return nil
	}

	fmt.Println("bestmove", e.chooseWorst(pos, legal, options))
	return nil
}

// chooseWorst returns the UCI string of the move to play. It always returns a legal move; any oracle
// problem falls back to a random move so a game never stalls.
func (e *EngineWorstfish) chooseWorst(pos *position.Position, legal []position.Move, options search.SearchOptions) string {
	randomMove := func() string { return legal[rand.Intn(len(legal))].String() }

	if e.oracle == nil {
		fmt.Println("info string worstfish: stockfish not found, playing randomly")
		return randomMove()
	}

	if err := e.oracle.SetOption("MultiPV", strconv.Itoa(len(legal))); err != nil {
		return randomMove()
	}
	if err := e.oracle.SetPosition(pos.StringFEN(), nil); err != nil {
		return randomMove()
	}

	oracleOpts := search.NewDeafultOptions()
	oracleOpts.WhiteTimeRemaining = 0
	oracleOpts.BlackTimeRemaining = 0
	oracleOpts.MoveTime = oracleMoveTime(options)

	// Stockfish ranks the MultiPV lines best-to-worst, re-sending all of them each depth. We keep the
	// latest move for each rank; the highest rank is the worst move.
	latest := map[int]string{}
	if _, err := e.oracle.Search(oracleOpts, func(line string) {
		if move, rank, ok := parseMultiPV(line); ok {
			latest[rank] = move
		}
	}); err != nil {
		fmt.Println("info string worstfish: oracle error:", err)
		return randomMove()
	}

	if worst := worstFrom(latest); worst != "" {
		return worst
	}
	return randomMove()
}

// oracleMoveTime decides how long Stockfish may think, bounded so the UI stays responsive.
func oracleMoveTime(options search.SearchOptions) time.Duration {
	t := options.MoveTime
	if t <= 0 {
		t = 500 * time.Millisecond
	}
	if t > 2*time.Second {
		t = 2 * time.Second
	}
	return t
}

// parseMultiPV extracts the MultiPV rank and the first move of the principal variation from a
// Stockfish "info" line.
func parseMultiPV(line string) (move string, rank int, ok bool) {
	fields := strings.Fields(line)
	if len(fields) < 2 || fields[0] != "info" {
		return "", 0, false
	}

	rank = -1
	for i := 0; i+1 < len(fields); i++ {
		switch fields[i] {
		case "multipv":
			rank, _ = strconv.Atoi(fields[i+1])
		case "pv":
			move = fields[i+1]
		}
	}

	if rank > 0 && move != "" {
		return move, rank, true
	}
	return "", 0, false
}

// worstFrom returns the move at the highest MultiPV rank seen (Stockfish's worst-ranked move).
func worstFrom(latest map[int]string) string {
	maxRank := 0
	for rank := range latest {
		if rank > maxRank {
			maxRank = rank
		}
	}
	return latest[maxRank]
}
