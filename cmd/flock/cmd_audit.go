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
	fs := flag.NewFlagSet("audit", flag.ExitOnError)
	limit := fs.Int("limit", 50, "maximum rows to show")
	actor := fs.String("actor", "", "filter by actor name (client-side)")
	_ = fs.Parse(args)

	cfg := loadConfigOrExit()
	body, err := adminCall(context.Background(), cfg, "GET", "/admin/v1/audit/recent", nil)
	if err != nil {
		die("%v: %s", err, string(body))
	}
	var rows []map[string]any
	_ = json.Unmarshal(body, &rows)
	if len(rows) == 0 {
		fmt.Println("(no audit records yet)")
		return
	}

	fmt.Printf("%-19s %-16s %-40s %s\n", "TIME", "ACTOR", "ACTION", "TARGET")
	count := 0
	for _, r := range rows {
		if *actor != "" && fmt.Sprint(r["Actor"]) != *actor {
			continue
		}
		ts := parseTime(r["TS"])
		fmt.Printf("%-19s %-16s %-40s %s\n",
			ts.Format("2006-01-02 15:04:05"),
			truncStr(fmt.Sprint(r["Actor"]), 16),
			truncStr(fmt.Sprint(r["Action"]), 40),
			fmt.Sprint(r["Target"]))
		count++
		if count >= *limit {
			break
		}
	}
}
