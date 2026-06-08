package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
)

// cmdAudit prints the recent audit log entries.
//
//	flock audit [--limit=N] [--actor=X]
func cmdAudit(args []string) {
	args, asJSON := extractJSONFlag(args)
	fs := flag.NewFlagSet("audit", flag.ExitOnError)
	limit := fs.Int("limit", 50, "maximum rows to show")
	actor := fs.String("actor", "", "filter by actor name (client-side)")
	fs.Usage = func() {
		showHelp(helpSpec{
			name:    "audit",
			summary: "show recent admin audit log entries",
			usage:   "flock audit [--limit N] [--actor X] [--json]",
			flags:   fs,
			examples: []string{
				"flock audit                       # latest 50 entries",
				"flock audit --limit 500",
				"flock audit --actor admin",
				"flock audit --json                # machine-readable",
			},
		})
	}
	if wantsHelp(args) {
		fs.Usage()
	}
	_ = fs.Parse(args)

	cfg := loadConfigOrExit()
	body, err := adminCall(context.Background(), cfg, "GET", "/admin/v1/audit/recent", nil)
	if err != nil {
		die("%v: %s", err, string(body))
	}
	var rows []map[string]any
	_ = json.Unmarshal(body, &rows)

	filtered := make([]map[string]any, 0, len(rows))
	for _, r := range rows {
		if *actor != "" && fmt.Sprint(r["Actor"]) != *actor {
			continue
		}
		filtered = append(filtered, r)
		if len(filtered) >= *limit {
			break
		}
	}

	if asJSON {
		emitJSON(filtered)
		return
	}
	if len(filtered) == 0 {
		fmt.Println("(no audit records yet)")
		return
	}

	fmt.Printf("%s %s %s %s\n",
		bold(fmt.Sprintf("%-19s", "TIME")),
		bold(fmt.Sprintf("%-16s", "ACTOR")),
		bold(fmt.Sprintf("%-40s", "ACTION")),
		bold("TARGET"))
	for _, r := range filtered {
		ts := parseTime(r["TS"])
		fmt.Printf("%s %-16s %-40s %s\n",
			dim(ts.Format("2006-01-02 15:04:05")),
			truncStr(fmt.Sprint(r["Actor"]), 16),
			truncStr(fmt.Sprint(r["Action"]), 40),
			cyan(fmt.Sprint(r["Target"])))
	}
}
