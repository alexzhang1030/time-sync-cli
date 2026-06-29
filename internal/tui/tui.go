package tui

import (
	"io"
	"os"
)

// Run starts the interactive configuration flow.
func Run() error {
	return RunWithIO(nil, nil)
}

// RunWithIO runs the TUI with custom input/output (for tests).
func RunWithIO(in io.Reader, out io.Writer) error {
	if in == nil {
		in = os.Stdin
	}
	if out == nil {
		out = os.Stdout
	}
	if isInteractiveTTY(in) {
		return runTea()
	}
	return runFallback(in, out)
}
