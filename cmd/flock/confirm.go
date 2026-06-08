package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/mattn/go-isatty"
)

// confirm prints a yes/no prompt and returns true if the user typed "y" or
// "yes" (case-insensitive). Default on a bare enter is "no" — destructive
// ops should require an explicit acknowledgement.
//
// When stdin is not a TTY (CI, piped input), confirm returns false. Callers
// that need scriptable destructive ops accept `--yes` and skip this entirely.
func confirm(prompt string) bool {
	if !isatty.IsTerminal(os.Stdin.Fd()) {
		return false
	}
	fmt.Fprint(os.Stderr, prompt)
	r := bufio.NewReader(os.Stdin)
	line, err := r.ReadString('\n')
	if err != nil {
		return false
	}
	s := strings.ToLower(strings.TrimSpace(line))
	return s == "y" || s == "yes"
}

// extractYesFlag pulls --yes / -y out of args and returns (remaining, yes).
// Order-independent; supports `--yes` and `-y`.
func extractYesFlag(args []string) ([]string, bool) {
	yes := false
	out := make([]string, 0, len(args))
	for _, a := range args {
		if a == "--yes" || a == "-y" {
			yes = true
			continue
		}
		out = append(out, a)
	}
	return out, yes
}
