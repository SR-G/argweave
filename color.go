package argweave

import "os"

// ColorMode controls whether the --help page is decorated with bold
// section headers, group headers, and flag/argument names (ANSI escape
// codes on a terminal), mirroring the Rust "clap" crate's --color option.
type ColorMode int

const (
	// ColorAuto enables color only when stdout is a terminal and the
	// NO_COLOR environment variable is unset. This is the default.
	ColorAuto ColorMode = iota
	// ColorAlways forces color on, regardless of the output destination.
	ColorAlways
	// ColorNever disables color unconditionally.
	ColorNever
)

// shouldColorize resolves mode to a plain yes/no decision.
func shouldColorize(mode ColorMode) bool {
	switch mode {
	case ColorAlways:
		return true
	case ColorNever:
		return false
	default:
		return os.Getenv("NO_COLOR") == "" && isTerminal(os.Stdout)
	}
}

// isTerminal reports whether f is connected to a terminal, as opposed to a
// pipe, file redirection, or non-interactive capture (e.g. under `go
// test`), without requiring a third-party dependency.
func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
