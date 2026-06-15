package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"sort"
	"strings"

	"github.com/ollybritton/StupidChess/match"
	"github.com/spf13/cobra"
)

// spsaCmd tunes the engine's search parameters by self-play SPSA, using the same game machinery as the
// match command. It writes the running parameter vector to a checkpoint file so a long run can be
// stopped and its result applied.
var spsaCmd = &cobra.Command{
	Use:   "spsa",
	Short: "tune search parameters by self-play SPSA",
	Long: `Optimise the engine's tunable search parameters (LMR shape, pruning margins) with Simultaneous
Perturbation Stochastic Approximation. Each iteration perturbs every parameter, plays a colour-swapped
game pair between the perturbed-up and perturbed-down settings, and steps the vector along the gradient.

The current vector is printed periodically and written to --out as JSON. Stop any time with ctrl-c; the
last checkpoint holds the best-so-far vector to fold back into the defaults.`,
	Run: runSPSA,
}

func runSPSA(cmd *cobra.Command, args []string) {
	self, err := os.Executable()
	if err != nil {
		fmt.Println("couldn't find own path:", err)
		os.Exit(1)
	}

	engine, _ := cmd.Flags().GetString("engine")
	threads, _ := cmd.Flags().GetInt("threads")
	options := map[string]string{}
	if threads > 0 {
		options["Threads"] = fmt.Sprint(threads)
	}
	if evalFile, _ := cmd.Flags().GetString("eval-file"); evalFile != "" {
		options["EvalFile"] = evalFile
	}
	if syzygy, _ := cmd.Flags().GetString("syzygy-path"); syzygy != "" {
		options["SyzygyPath"] = syzygy
	}

	nodes, _ := cmd.Flags().GetUint("nodes")
	iterations, _ := cmd.Flags().GetInt("iterations")
	concurrency, _ := cmd.Flags().GetInt("concurrency")
	rate0, _ := cmd.Flags().GetFloat64("rate0")
	out, _ := cmd.Flags().GetString("out")

	cfg := match.SPSAConfig{
		Engine: match.EngineSpec{
			Label:   engine,
			Path:    self,
			Args:    []string{"uci", "-e", engine},
			Options: options,
		},
		TC:          match.TimeControl{Nodes: nodes},
		Iterations:  iterations,
		Concurrency: concurrency,
		Rate0:       rate0,
		OnUpdate: func(iter int, theta map[string]int) {
			fmt.Printf("\riter %d/%d  %s        ", iter, iterations, formatTheta(theta))
			writeCheckpoint(out, theta)
		},
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	fmt.Printf("SPSA: tuning %q at nodes %d, %d iterations, writing %s (ctrl-c to stop)\n", engine, nodes, iterations, out)
	final, err := match.RunSPSA(ctx, cfg)
	fmt.Println()
	if err != nil {
		fmt.Println("spsa failed:", err)
		os.Exit(1)
	}
	writeCheckpoint(out, final)
	fmt.Println("final:", formatTheta(final))
	fmt.Println("written to", out)
}

// formatTheta renders the parameter vector in a stable key order.
func formatTheta(theta map[string]int) string {
	keys := make([]string, 0, len(theta))
	for k := range theta {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%d", k, theta[k]))
	}
	return strings.Join(parts, " ")
}

func writeCheckpoint(path string, theta map[string]int) {
	if path == "" {
		return
	}
	b, err := json.MarshalIndent(theta, "", "  ")
	if err != nil {
		return
	}
	_ = os.WriteFile(path, b, 0o644)
}

func init() {
	spsaCmd.Flags().String("engine", "tryhard", "engine to tune")
	spsaCmd.Flags().Int("threads", 1, "search threads per engine")
	spsaCmd.Flags().String("eval-file", "", "NNUE network to tune against (tune on the bot's real eval)")
	spsaCmd.Flags().String("syzygy-path", "", "Syzygy tablebase directory")
	spsaCmd.Flags().Uint("nodes", 25000, "fixed node budget per move")
	spsaCmd.Flags().Int("iterations", 2000, "number of SPSA iterations (each plays 2 games)")
	spsaCmd.Flags().Int("concurrency", 0, "iterations in flight at once (default: number of CPUs)")
	spsaCmd.Flags().Float64("rate0", 0.3, "initial learning-rate factor")
	spsaCmd.Flags().String("out", "spsa.json", "checkpoint file for the running parameter vector")

	rootCmd.AddCommand(spsaCmd)
}
