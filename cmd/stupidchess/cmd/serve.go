package cmd

import (
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"strings"

	"github.com/ollybritton/StupidChess/engines"
	"github.com/ollybritton/StupidChess/web"
	"github.com/spf13/cobra"
)

// externalUCIEngines lists external UCI binaries to offer if they are found on PATH.
var externalUCIEngines = []string{"stockfish", "lc0"}

// buildEngineSpecs assembles the selectable engines: the built-in engines (which run this binary as a
// UCI subprocess), any known external engines found on PATH, and any registered via --uci-engine.
func buildEngineSpecs(exe string, custom []string) []web.EngineSpec {
	specs := make([]web.EngineSpec, 0)

	for name, eng := range engines.EngineInfo {
		specs = append(specs, web.EngineSpec{
			Name:        name,
			DisplayName: displayName(eng.Name()),
			Description: eng.Description(),
			Path:        exe,
			Args:        []string{"uci", "-e", name},
		})
	}

	for _, name := range externalUCIEngines {
		if path, err := exec.LookPath(name); err == nil {
			specs = append(specs, web.EngineSpec{
				Name:        name,
				DisplayName: displayName(name),
				Description: "Full-strength external engine.",
				Path:        path,
			})
		}
	}

	// --uci-engine name=/path/to/engine (repeatable) registers an arbitrary external UCI engine.
	for _, entry := range custom {
		name, path, ok := strings.Cut(entry, "=")
		if !ok || name == "" || path == "" {
			fmt.Printf("ignoring --uci-engine %q (expected name=path)\n", entry)
			continue
		}
		resolved, err := exec.LookPath(path)
		if err != nil {
			resolved = path // let it fail at launch with a clearer message
		}
		specs = append(specs, web.EngineSpec{
			Name:        name,
			DisplayName: displayName(name),
			Description: "External UCI engine.",
			Path:        resolved,
		})
	}

	return specs
}

// displayName turns an engine id like "try-hard" or "worstfish" into a label like "Try Hard".
func displayName(name string) string {
	parts := strings.FieldsFunc(name, func(r rune) bool { return r == '-' || r == '_' || r == ' ' })
	for i, p := range parts {
		if p != "" {
			parts[i] = strings.ToUpper(p[:1]) + p[1:]
		}
	}
	return strings.Join(parts, " ")
}

// serveCmd starts the web UI for playing against and debugging the engines.
var serveCmd = &cobra.Command{
	Use:   "serve",
	Short: "start the web UI for playing and debugging engines",
	Long:  `Serve a Lichess-style web interface for playing the engines, watching them play each other, and inspecting their search.`,
	Run: func(cmd *cobra.Command, args []string) {
		port, _ := cmd.Flags().GetString("port")
		custom, _ := cmd.Flags().GetStringArray("uci-engine")

		// Built-in engines are spawned as UCI subprocesses of this same binary (stupidchess uci -e <name>).
		exe, err := os.Executable()
		if err != nil {
			fmt.Println("couldn't determine executable path:", err)
			os.Exit(1)
		}

		specs := buildEngineSpecs(exe, custom)

		external := make([]string, 0)
		for _, sp := range specs {
			if sp.Path != exe {
				external = append(external, sp.Name)
			}
		}

		server := web.NewServer(specs)

		fmt.Printf("StupidChess web UI on http://localhost:%s\n", port)
		if len(external) > 0 {
			fmt.Printf("external UCI engines available: %s\n", strings.Join(external, ", "))
		} else {
			fmt.Println("no external UCI engines found (install stockfish, or use --uci-engine name=path)")
		}
		if err := http.ListenAndServe(":"+port, server.Handler()); err != nil {
			fmt.Println("server error:", err)
			os.Exit(1)
		}
	},
}

func init() {
	serveCmd.Flags().StringP("port", "p", "8080", "port to listen on")
	serveCmd.Flags().StringArray("uci-engine", nil, "register an external UCI engine as name=path (repeatable)")
	rootCmd.AddCommand(serveCmd)
}
