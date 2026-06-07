package api

import "fmt"

// modelRate is a per-1M-tokens rate, in micros (millionths of a dollar).
// Reading: $1.00 / 1M tokens = 1_000_000 micros.
type modelRate struct {
	InputMicros  int64
	OutputMicros int64
}

// vendorRates is a hand-maintained price list for vendor models that Flock
// proxies via egress. Local engines always cost $0, regardless of name.
//
// Rates are listed as $/M tokens × 1_000_000 (i.e., micros per 1M tokens).
// As of: June 2026. Vendor pricing changes infrequently; update here when
// it does. Unknown models silently fall through to $0 — better to under-
// report than to lie.
//
// Lookup is by exact model id. We intentionally do NOT match by prefix
// ("gpt-*") because that would incorrectly charge for local gpt-oss-20b.
var vendorRates = map[string]modelRate{
	// ----- Anthropic -----
	"claude-haiku-4-5":  {InputMicros: 800_000, OutputMicros: 4_000_000},
	"claude-sonnet-4-6": {InputMicros: 3_000_000, OutputMicros: 15_000_000},
	"claude-opus-4-7":   {InputMicros: 15_000_000, OutputMicros: 75_000_000},

	// ----- OpenAI -----
	"gpt-4o":      {InputMicros: 2_500_000, OutputMicros: 10_000_000},
	"gpt-4o-mini": {InputMicros: 150_000, OutputMicros: 600_000},
	"o1":          {InputMicros: 15_000_000, OutputMicros: 60_000_000},
	"o1-mini":     {InputMicros: 3_000_000, OutputMicros: 12_000_000},
	"o3-mini":     {InputMicros: 1_100_000, OutputMicros: 4_400_000},
	"o4-mini":     {InputMicros: 600_000, OutputMicros: 2_400_000},
}

// lookupCostMicros computes the dollar cost of a single completion as an
// integer count of micros. Returns 0 for any model not in vendorRates —
// that covers all local-engine calls and any vendor model we don't have
// rates for. The recordUsage caller does not need to know which case it is.
func lookupCostMicros(model string, promptTokens, completionTokens int) int64 {
	rate, ok := vendorRates[model]
	if !ok {
		return 0
	}
	in := int64(promptTokens) * rate.InputMicros / 1_000_000
	out := int64(completionTokens) * rate.OutputMicros / 1_000_000
	return in + out
}

// FormatCostUSD renders a micros count as a human-friendly dollar string
// suitable for table columns. Examples:
//
//	0       → "$0"
//	1       → "<$0.01"
//	12_345  → "$0.01"
//	340_000 → "$0.34"
//	1_500_000 → "$1.50"
//
// Exported so the CLI usage subcommand can use it for the cost column.
func FormatCostUSD(micros int64) string {
	if micros == 0 {
		return "$0"
	}
	if micros < 10_000 {
		// less than one cent — emphasize non-zero but tiny so it doesn't
		// quietly collapse to "$0.00"
		return "<$0.01"
	}
	cents := micros / 10_000
	dollars := cents / 100
	rem := cents % 100
	return fmt.Sprintf("$%d.%02d", dollars, rem)
}
