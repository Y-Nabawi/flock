// Command flock is the single binary that runs Flock in all of its modes:
// leader (flock up), worker (flock join + flock up), and one-shot CLI.
//
// All subcommands live in cmd/flock/cmd_*.go alongside this file.
package main

import (
	"fmt"
	"io"
	"os"
)

// version is overwritten at link time via -ldflags.
var version = "0.21.0-dev"

func main() {
	if len(os.Args) < 2 {
		printUsage(os.Stderr)
		os.Exit(2)
	}
	cmd := os.Args[1]
	args := os.Args[2:]
	switch cmd {
	case "version", "--version", "-v":
		cmdVersion(args)
	case "up":
		cmdUp(args)
	case "down":
		cmdDown(args)
	case "status":
		cmdStatus(args)
	case "join":
		cmdJoin(args)
	case "node":
		cmdNode(args)
	case "model":
		cmdModel(args)
	case "shard":
		cmdShard(args)
	case "token":
		cmdToken(args)
	case "usage":
		cmdUsage(args)
	case "audit":
		cmdAudit(args)
	case "config":
		cmdConfig(args)
	case "doctor":
		cmdDoctor(args)
	case "update", "upgrade":
		cmdUpdate(args)
	case "connect":
		cmdConnect(args)
	case "disconnect":
		cmdDisconnect(args)
	case "invite":
		cmdInvite(args)
	case "completion":
		cmdCompletion(args)
	case "help", "--help", "-h":
		printUsage(os.Stdout)
	default:
		fmt.Fprintf(os.Stderr, "flock: unknown command %q\n", cmd)
		if guess := suggestSubcommand(cmd); guess != "" {
			fmt.Fprintf(os.Stderr, "\nDid you mean %q?\n", guess)
		}
		fmt.Fprintln(os.Stderr)
		printUsage(os.Stderr)
		os.Exit(2)
	}
}

// suggestSubcommand returns the closest registered subcommand by edit
// distance, or "" if nothing is within 2 edits. Cheap typo helper.
func suggestSubcommand(cmd string) string {
	known := []string{
		"version", "up", "down", "status", "join", "node", "model", "shard",
		"token", "usage", "audit", "config", "doctor", "update", "upgrade",
		"connect", "disconnect", "invite", "completion", "help",
	}
	best := ""
	bestD := 3
	for _, k := range known {
		d := levenshtein(cmd, k)
		if d < bestD {
			best, bestD = k, d
		}
	}
	return best
}

// levenshtein computes Damerau-Levenshtein edit distance — a single
// transposition of adjacent characters costs 1 (plain Levenshtein scores
// it 2). That way "modle" → "model" is closer than "modle" → "node".
func levenshtein(a, b string) int {
	if a == b {
		return 0
	}
	la, lb := len(a), len(b)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}
	dp := make([][]int, la+1)
	for i := range dp {
		dp[i] = make([]int, lb+1)
		dp[i][0] = i
	}
	for j := 0; j <= lb; j++ {
		dp[0][j] = j
	}
	for i := 1; i <= la; i++ {
		for j := 1; j <= lb; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			v := dp[i-1][j] + 1
			if dp[i][j-1]+1 < v {
				v = dp[i][j-1] + 1
			}
			if dp[i-1][j-1]+cost < v {
				v = dp[i-1][j-1] + cost
			}
			if i > 1 && j > 1 && a[i-1] == b[j-2] && a[i-2] == b[j-1] {
				if dp[i-2][j-2]+1 < v {
					v = dp[i-2][j-2] + 1
				}
			}
			dp[i][j] = v
		}
	}
	return dp[la][lb]
}

func printUsage(w io.Writer) {
	fmt.Fprintln(w, `flock — orchestrate open-weight LLMs across your machines

Usage:
  flock <command> [options]

Commands:
  up                       Start the local node (leader on first run)
  down                     Stop the local node
  status                   Show local node and cluster status
  join <url>?token=...     Join an existing cluster as a worker
  node ls                  List nodes
  node show <id>           Show one node
  node drain <id>          Mark node as draining
  node remove <id>         Remove a node
  model add <id>           Install a model from the catalog
  model ls                 List installed models
  model search [q]         Search the catalog
  model info <id>          Full details for one catalog model
  model remove <id>        Uninstall a model
  shard create <model> [N] Orchestrate a sharded model across N workers
  shard ls                 List shards
  shard remove <model>     Tear down a sharded model
  token create [name]      Issue an API key (--admin, --node)
  token ls                 List API keys
  token revoke <id>        Revoke an API key
  usage [--limit N]        Show recent inference usage records
  audit [--limit N]        Show recent admin audit log entries
  config show              Show effective runtime config (secrets redacted)
  config path              Print config file path
  config edit              Print the editor command to edit config
  connect <client>         Print copy-paste config for a tool (Claude Code, Cursor, …)
  connect --list           List supported clients
  disconnect <client>      Print reversal steps for a previous 'connect'
  invite <name>            Create a user-scope token + share card for a teammate
  doctor                   Diagnose common problems
  update [--check]         Check / install the latest Flock release
  upgrade                  Alias for 'update'
  completion <shell>       Print bash/zsh/fish completion script
  version                  Print version
  help                     Show this help

Every command supports --help:
  flock up --help, flock shard --help, flock token --help, etc.

Docs: https://github.com/hadihonarvar/flock`)
}
