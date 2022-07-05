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
	Short: "Start a UCI interface",
	Long:  ``,
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("uci called")

		engineName := getEngine(cmd)
		fmt.Println(engineName)

		uci.Listen(os.Stdin, engines.NewEngineTryHard())
	},
}

func init() {
	rootCmd.AddCommand(uciCmd)
}
