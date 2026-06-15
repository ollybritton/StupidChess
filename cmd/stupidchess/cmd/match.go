package cmd

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/ollybritton/StupidChess/match"
	"github.com/spf13/cobra"
)

// matchCmd plays two engine configurations against each other and measures the
// difference with an SPRT, so search and evaluation changes can be accepted or
// rejected with controlled error instead of guessed at.
var matchCmd = &cobra.Command{
	Use:   "match",
	Short: "play two engines against each other and measure the Elo difference (SPRT)",
	Long: `Play a self-play match between two engine configurations and run a Sequential
Probability Ratio Test on the result. Results are reported from engine A's
perspective, so test a change by putting the new build/engine on side A and the
baseline on side B: a positive Elo means A is stronger.

Each side is "<binary> uci -e <engine>" with optional UCI options. By default
both sides are this binary running the same engine, which (with a real change in
options) is how you A/B two configurations. Use --bin-a/--bin-b to pit two
different builds against each other.

Examples:
  # Does NNUE beat the hand-crafted eval? (A = NNUE, B = baseline)
  stupidchess match --eval-file-a nnue/testdata/net.nnue --nodes 50000

  # A/B two builds of the same engine:
  stupidchess match --bin-a ./stupidchess.new --bin-b ./stupidchess.old --nodes 50000`,
	Run: runMatch,
}

func runMatch(cmd *cobra.Command, args []string) {
	self, err := os.Executable()
	if err != nil {
		fmt.Println("couldn't find own path:", err)
		os.Exit(1)
	}

	specA, err := buildSpec(cmd, "a", self)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
	specB, err := buildSpec(cmd, "b", self)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	nodes, _ := cmd.Flags().GetUint("nodes")
	moveTimeMs, _ := cmd.Flags().GetInt("movetime")
	tc := match.TimeControl{Nodes: nodes}
	if moveTimeMs > 0 {
		tc = match.TimeControl{MoveTime: time.Duration(moveTimeMs) * time.Millisecond}
	}

	maxGames, _ := cmd.Flags().GetInt("games")
	concurrency, _ := cmd.Flags().GetInt("concurrency")

	var sprt *match.SPRT
	if noSPRT, _ := cmd.Flags().GetBool("no-sprt"); !noSPRT {
		elo0, _ := cmd.Flags().GetFloat64("elo0")
		elo1, _ := cmd.Flags().GetFloat64("elo1")
		alpha, _ := cmd.Flags().GetFloat64("alpha")
		beta, _ := cmd.Flags().GetFloat64("beta")
		sprt = match.NewSPRT(elo0, elo1, alpha, beta)
	}

	cfg := match.Config{
		A:           specA,
		B:           specB,
		TC:          tc,
		MaxGames:    maxGames,
		Concurrency: concurrency,
		SPRT:        sprt,
		OnProgress:  progressPrinter(specA.Label, specB.Label, sprt),
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	budget := "nodes " + fmt.Sprint(nodes)
	if moveTimeMs > 0 {
		budget = fmt.Sprintf("movetime %dms", moveTimeMs)
	}
	fmt.Printf("match: A=%q vs B=%q at %s (ctrl-c to stop)\n", specA.Label, specB.Label, budget)

	stats, err := match.Run(ctx, cfg)
	fmt.Println()
	if err != nil {
		fmt.Println("match failed:", err)
		os.Exit(1)
	}
	printFinal(specA.Label, specB.Label, stats, sprt)
}

// buildSpec assembles an EngineSpec for side "a" or "b" from the flags.
func buildSpec(cmd *cobra.Command, side, self string) (match.EngineSpec, error) {
	bin, _ := cmd.Flags().GetString("bin-" + side)
	if bin == "" {
		bin = self
	}
	engine, _ := cmd.Flags().GetString("engine-" + side)
	threads, _ := cmd.Flags().GetInt("threads")

	options := map[string]string{}
	if threads > 0 {
		options["Threads"] = fmt.Sprint(threads)
	}
	if evalFile, _ := cmd.Flags().GetString("eval-file-" + side); evalFile != "" {
		options["EvalFile"] = evalFile
	}
	if syzygy, _ := cmd.Flags().GetString("syzygy-path"); syzygy != "" {
		options["SyzygyPath"] = syzygy
	}
	raw, _ := cmd.Flags().GetStringArray("option-" + side)
	for _, kv := range raw {
		k, v, ok := strings.Cut(kv, "=")
		if !ok {
			return match.EngineSpec{}, fmt.Errorf("bad --option-%s %q, want name=value", side, kv)
		}
		options[k] = v
	}

	label, _ := cmd.Flags().GetString("name-" + side)
	if label == "" {
		label = strings.ToUpper(side) + ":" + engine
	}

	return match.EngineSpec{
		Label:   label,
		Path:    bin,
		Args:    []string{"uci", "-e", engine},
		Options: options,
	}, nil
}

// progressPrinter returns a callback that rewrites a single status line as games complete.
func progressPrinter(a, b string, sprt *match.SPRT) func(match.Stats) {
	return func(s match.Stats) {
		line := fmt.Sprintf("\r%d games  %s +%d =%d -%d  Elo %+.1f +/- %.1f",
			s.Games(), a, s.Wins, s.Draws, s.Losses, s.Elo, s.EloMargin)
		if sprt != nil {
			line += fmt.Sprintf("  LLR %.2f [%.2f,%.2f]", s.LLR, s.Lower, s.Upper)
		}
		if s.Errors > 0 {
			line += fmt.Sprintf("  errors %d", s.Errors)
		}
		fmt.Print(line + "    ")
	}
}

func printFinal(a, b string, s match.Stats, sprt *match.SPRT) {
	fmt.Printf("Result (%s vs %s): +%d =%d -%d  in %d games\n", a, b, s.Wins, s.Draws, s.Losses, s.Games())
	fmt.Printf("Elo (A - B): %+.1f +/- %.1f (95%%)\n", s.Elo, s.EloMargin)
	if sprt != nil {
		fmt.Printf("LLR: %.2f  bounds [%.2f, %.2f]  -> %s\n", s.LLR, s.Lower, s.Upper, s.Verdict)
	}
	if s.Errors > 0 {
		fmt.Printf("(%d games had engine errors and were discarded)\n", s.Errors)
	}
}

func init() {
	matchCmd.Flags().String("bin-a", "", "binary for engine A (default: this binary)")
	matchCmd.Flags().String("bin-b", "", "binary for engine B (default: this binary)")
	matchCmd.Flags().String("engine-a", "tryhard", "engine name for side A")
	matchCmd.Flags().String("engine-b", "tryhard", "engine name for side B")
	matchCmd.Flags().String("name-a", "", "display label for side A")
	matchCmd.Flags().String("name-b", "", "display label for side B")
	matchCmd.Flags().StringArray("option-a", nil, "UCI option for A as name=value (repeatable)")
	matchCmd.Flags().StringArray("option-b", nil, "UCI option for B as name=value (repeatable)")
	matchCmd.Flags().String("eval-file-a", "", "NNUE network for side A (shortcut for --option-a EvalFile=...)")
	matchCmd.Flags().String("eval-file-b", "", "NNUE network for side B")
	matchCmd.Flags().String("syzygy-path", "", "Syzygy tablebase dir for both engines")
	matchCmd.Flags().Int("threads", 1, "search threads per engine")

	matchCmd.Flags().Uint("nodes", 40000, "fixed node budget per move (hardware-independent)")
	matchCmd.Flags().Int("movetime", 0, "fixed milliseconds per move (overrides --nodes when > 0)")
	matchCmd.Flags().Int("games", 2000, "maximum games to play")
	matchCmd.Flags().Int("concurrency", 0, "games in flight at once (default: number of CPUs)")

	matchCmd.Flags().Bool("no-sprt", false, "play a fixed number of games without early SPRT stopping")
	matchCmd.Flags().Float64("elo0", 0, "SPRT H0 Elo bound")
	matchCmd.Flags().Float64("elo1", 5, "SPRT H1 Elo bound")
	matchCmd.Flags().Float64("alpha", 0.05, "SPRT type-I error rate")
	matchCmd.Flags().Float64("beta", 0.05, "SPRT type-II error rate")

	rootCmd.AddCommand(matchCmd)
}
