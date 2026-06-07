package api

import "testing"

func TestLookupCostMicros_LocalModelIsFree(t *testing.T) {
	if got := lookupCostMicros("qwen3.6-27b", 1000, 1000); got != 0 {
		t.Fatalf("local model cost = %d, want 0", got)
	}
	// gpt-oss-20b is local — must NOT match the OpenAI prefix-shaped rate.
	if got := lookupCostMicros("gpt-oss-20b", 1000, 1000); got != 0 {
		t.Fatalf("gpt-oss-20b (local) cost = %d, want 0 (no prefix matching)", got)
	}
}

func TestLookupCostMicros_VendorModelCharged(t *testing.T) {
	// claude-sonnet-4-6: $3/M input, $15/M output.
	// 1000 input + 500 output =
	//   1000 * 3_000_000 / 1_000_000 = 3000 micros ($0.003)
	// + 500 * 15_000_000 / 1_000_000 = 7500 micros ($0.0075)
	// = 10500 micros ($0.0105)
	if got := lookupCostMicros("claude-sonnet-4-6", 1000, 500); got != 10_500 {
		t.Fatalf("claude-sonnet-4-6 cost = %d, want 10500", got)
	}
}

func TestLookupCostMicros_AllVendorRatesPositive(t *testing.T) {
	for model, rate := range vendorRates {
		if rate.InputMicros <= 0 || rate.OutputMicros <= 0 {
			t.Errorf("%s has non-positive rate: %+v", model, rate)
		}
	}
}

func TestFormatCostUSD(t *testing.T) {
	cases := []struct {
		micros int64
		want   string
	}{
		{0, "$0"},
		{1, "<$0.01"},
		{9_999, "<$0.01"},
		{10_000, "$0.01"},
		{12_345, "$0.01"},
		{340_000, "$0.34"},
		{1_500_000, "$1.50"},
		{99_990_000, "$99.99"},
		{9_999_000_000, "$9999.00"},
	}
	for _, c := range cases {
		if got := FormatCostUSD(c.micros); got != c.want {
			t.Errorf("FormatCostUSD(%d) = %q, want %q", c.micros, got, c.want)
		}
	}
}
