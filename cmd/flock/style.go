package main

import (
	"os"

	"github.com/mattn/go-isatty"
)

// colorEnabled is the single switch every coloring helper consults.
// Set once at startup based on the standard rules: respect the cross-CLI
// NO_COLOR convention (https://no-color.org), the Flock-specific
// FLOCK_NO_COLOR override, and stdout-TTY detection so redirected /
// piped output stays clean for grep + jq.
var colorEnabled = func() bool {
	if os.Getenv("NO_COLOR") != "" || os.Getenv("FLOCK_NO_COLOR") != "" {
		return false
	}
	return isatty.IsTerminal(os.Stdout.Fd())
}()

// ansi wraps s in an ANSI escape and a reset. No-op when colors are off
// so callers don't have to guard every call site themselves.
func ansi(code, s string) string {
	if !colorEnabled {
		return s
	}
	return "\033[" + code + "m" + s + "\033[0m"
}

func bold(s string) string   { return ansi("1", s) }
func dim(s string) string    { return ansi("2", s) }
func red(s string) string    { return ansi("31", s) }
func green(s string) string  { return ansi("32", s) }
func yellow(s string) string { return ansi("33", s) }
func cyan(s string) string   { return ansi("36", s) }
