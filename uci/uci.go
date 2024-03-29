package uci

import (
	"bufio"
	"fmt"
	"io"
	"os"

	"github.com/ollybritton/StupidChess/engines"
)

// uci implements the universal chess interface (UCI) both from the perspective of the engine (in `as_engine.go`) and
// from the persepctive of the GUI (in `as_gui.go`).

func log(msg string) {
	logfile := `/tmp/stupidchess-debug-in`
	f, err := os.OpenFile(logfile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0666)

	if err != nil {
		panic(err)
	}

	_, err = f.WriteString("> " + msg + "\n")

	if err != nil {
		panic(err)
	}

	f.Close()
}

// Listen starts accpeting input and forwarding all relevant commands to the engine.
// In actual use, this will be called with os.Stdin as the first argument.
func Listen(input io.Reader, eng engines.Engine) {
	scanner := bufio.NewScanner(input)
	session := NewEngineSession(eng)
	for scanner.Scan() {
		commandLine := scanner.Text()
		log(commandLine)
		if commandLine == "quit" {
			return
		}

		err := session.Handle(commandLine)
		if err != nil {
			fmt.Printf("info string error: %s\n", err)
		}
	}
}
