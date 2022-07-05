package uci

import (
	"bufio"
	"fmt"
	"io"

	"github.com/ollybritton/StupidChess/engines"
)

// UCI is the bridge between a UCI-compliant UI and an engine. Messages

// Listen starts accpeting input and forwarding all relevant commands to the engine.
// In actual use, this will be called with os.Stdin as the first argument.
func Listen(input io.Reader, eng engines.Engine) {
	scanner := bufio.NewScanner(input)
	session := NewSession(eng)
	for scanner.Scan() {
		commandLine := scanner.Text()
		if commandLine == "quit" {
			return
		}

		err := session.Handle(commandLine)
		if err != nil {
			fmt.Printf("info string error: %s\n", err)
		}
	}
}
