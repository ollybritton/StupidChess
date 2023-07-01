/*
Copyright © 2022 NAME HERE <EMAIL ADDRESS>

*/
package cmd

import (
	"fmt"
	"os"

	"github.com/ollybritton/StupidChess/engines"
	"github.com/ollybritton/StupidChess/uci"
	"github.com/spf13/cobra"
)

// uciCmd represents the uci command
var uciCmd = &cobra.Command{
	Use:   "uci",
	Short: "start a UCI interface",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		engineName := getEngine(cmd)
		engine, ok := engines.EngineInfo[engineName]

		if !ok {
			fmt.Println("engine", engineName, "not found")
			os.Exit(1)
		}

		fmt.Println("stupidchess ~", engineName)

		uci.Listen(os.Stdin, engine)
	},
}

func init() {
	rootCmd.AddCommand(uciCmd)
}
