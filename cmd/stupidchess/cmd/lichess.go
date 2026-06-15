package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"

	"github.com/ollybritton/StupidChess/engines"
	"github.com/ollybritton/StupidChess/lichess"
	"github.com/spf13/cobra"
)

// lichessCmd groups the Lichess bot subcommands.
var lichessCmd = &cobra.Command{
	Use:   "lichess",
	Short: "play on Lichess via the Bot API",
	Long: `Connect a StupidChess engine to Lichess and play games against anyone who challenges it.

Set up (once):
  1. Create a fresh Lichess account for the bot.
  2. Make a token with the "bot:play" scope at https://lichess.org/account/oauth/token
  3. Upgrade the account to a BOT account (irreversible, only works before it plays any games):
       stupidchess lichess upgrade --token <token>

Then run the bot, picking the engine with -e:
       stupidchess lichess play -e fortress --token <token>

The token may also be supplied via the LICHESS_TOKEN environment variable.`,
}

var lichessPlayCmd = &cobra.Command{
	Use:   "play",
	Short: "connect to Lichess and play games with an engine",
	Run: func(cmd *cobra.Command, args []string) {
		token := resolveToken(cmd)
		if token == "" {
			fmt.Println("no API token: pass --token or set LICHESS_TOKEN")
			os.Exit(1)
		}

		name := getEngine(cmd)
		if _, ok := engines.EngineInfo[name]; !ok {
			fmt.Printf("engine %q not found\n", name)
			os.Exit(1)
		}

		// The engine runs as a UCI subprocess of this same binary (one per game).
		exe, err := os.Executable()
		if err != nil {
			fmt.Println("couldn't determine executable path:", err)
			os.Exit(1)
		}

		client := lichess.NewClient(token)
		bot := lichess.NewBot(client, func(gameID string) (lichess.Mover, error) {
			return lichess.NewEngineMover(exe, []string{"uci", "-e", name})
		})
		bot.Logf = func(format string, a ...interface{}) { fmt.Printf(format+"\n", a...) }
		if greeting, _ := cmd.Flags().GetString("greeting"); greeting != "" {
			bot.Greeting = greeting
		}

		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
		defer cancel()

		fmt.Printf("StupidChess Lichess bot starting with engine %q (ctrl-c to stop)\n", name)
		if err := bot.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
			fmt.Println("bot stopped:", err)
			os.Exit(1)
		}
		fmt.Println("bot stopped")
	},
}

var lichessUpgradeCmd = &cobra.Command{
	Use:   "upgrade",
	Short: "irreversibly upgrade the token's account to a BOT account",
	Run: func(cmd *cobra.Command, args []string) {
		token := resolveToken(cmd)
		if token == "" {
			fmt.Println("no API token: pass --token or set LICHESS_TOKEN")
			os.Exit(1)
		}

		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		if err := lichess.NewClient(token).UpgradeToBot(ctx); err != nil {
			fmt.Println("upgrade failed:", err)
			os.Exit(1)
		}
		fmt.Println("account upgraded to a BOT account")
	},
}

// resolveToken reads the API token from the --token flag, falling back to the LICHESS_TOKEN env var.
func resolveToken(cmd *cobra.Command) string {
	if token, _ := cmd.Flags().GetString("token"); token != "" {
		return token
	}
	return os.Getenv("LICHESS_TOKEN")
}

func init() {
	lichessCmd.PersistentFlags().String("token", "", "Lichess API token (or set LICHESS_TOKEN)")
	lichessPlayCmd.Flags().String("greeting", "", "chat message to send at the start of each game")

	lichessCmd.AddCommand(lichessPlayCmd)
	lichessCmd.AddCommand(lichessUpgradeCmd)
	rootCmd.AddCommand(lichessCmd)
}
