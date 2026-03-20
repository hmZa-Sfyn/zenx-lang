package main

import "os"

// Terminal color codes — automatically disabled when stderr is not a TTY.

var (
	colorReset  = "\033[0m"
	colorBold   = "\033[1m"
	colorDim    = "\033[2m"
	colorRed    = "\033[31m"
	colorGreen  = "\033[32m"
	colorYellow = "\033[33m"
	colorBlue   = "\033[34m"
	colorPurple = "\033[35m"
	colorCyan   = "\033[36m"
	colorWhite  = "\033[37m"
	colorOrange = "\033[33m" // bold yellow approximates orange in most terminals
)

func init() {
	// Disable color when not writing to a real terminal (e.g. CI, pipes, files).
	if !isTerminal(os.Stderr) {
		colorReset = ""
		colorBold = ""
		colorDim = ""
		colorRed = ""
		colorGreen = ""
		colorYellow = ""
		colorBlue = ""
		colorPurple = ""
		colorCyan = ""
		colorWhite = ""
		colorOrange = ""
	}
}

// isTerminal reports whether f is connected to a terminal.
// We use a simple heuristic: try to stat the file and check the mode.
func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	// ModeCharDevice is set for terminal devices on Unix and Windows.
	return (fi.Mode() & os.ModeCharDevice) != 0
}
