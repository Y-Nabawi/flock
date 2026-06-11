package store

import (
	"context"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

// TestAPIKeyAllowedModelsRoundtrip verifies the three states of the
// allowed_models column round-trip correctly: nil ("any model"), empty
// ([]string{}, "deny all"), and an explicit list. Caught two latent
// JSON-decode bugs in early drafts; keep the explicit cases.
func TestAPIKeyAllowedModelsRoundtrip(t *testing.T) {
	dir := t.TempDir()
	st, err := OpenSQLite(filepath.Join(dir, "x.db"))
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	defer st.Close()
	ctx := context.Background()

	cases := []struct {
		name string
		list []string
	}{
		{"nil → unrestricted", nil},
		{"empty → deny all", []string{}},
		{"single literal", []string{"qwen3-14b"}},
		{"multiple + wildcards", []string{"qwen-coder-7b", "claude-*", "gpt-4o-mini"}},
	}
	for i, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			id := "k_test_" + c.name
			rec := APIKey{
				ID:            id,
				Hash:          "hash_" + c.name,
				Name:          c.name,
				Scope:         "user",
				UserID:        "alice",
				AllowedModels: c.list,
				CreatedAt:     time.Unix(int64(1_700_000_000+i), 0),
			}
			if err := st.APIKeys().Create(ctx, rec); err != nil {
				t.Fatalf("Create: %v", err)
			}
			got, err := st.APIKeys().GetByID(ctx, id)
			if err != nil {
				t.Fatalf("GetByID: %v", err)
			}
			if got == nil {
				t.Fatalf("GetByID: got nil")
			}
			if !sameSlice(got.AllowedModels, c.list) {
				t.Errorf("AllowedModels round-trip: got %#v (nil=%v) want %#v (nil=%v)",
					got.AllowedModels, got.AllowedModels == nil, c.list, c.list == nil)
			}

			// And via UpdateAllowedModels.
			if err := st.APIKeys().UpdateAllowedModels(ctx, id, []string{"updated"}); err != nil {
				t.Fatalf("UpdateAllowedModels: %v", err)
			}
			got, _ = st.APIKeys().GetByID(ctx, id)
			if !reflect.DeepEqual(got.AllowedModels, []string{"updated"}) {
				t.Errorf("after Update: %#v", got.AllowedModels)
			}
			// Clear back to nil.
			if err := st.APIKeys().UpdateAllowedModels(ctx, id, nil); err != nil {
				t.Fatalf("UpdateAllowedModels nil: %v", err)
			}
			got, _ = st.APIKeys().GetByID(ctx, id)
			if got.AllowedModels != nil {
				t.Errorf("after clear: got %#v want nil", got.AllowedModels)
			}
		})
	}
}

// TestUsageBreakdown_ByDayAndModel writes a few synthetic usage rows
// then verifies the bucketed query rolls them up correctly.
func TestUsageBreakdown_ByDayAndModel(t *testing.T) {
	dir := t.TempDir()
	st, err := OpenSQLite(filepath.Join(dir, "u.db"))
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	defer st.Close()
	ctx := context.Background()

	// Two rows on 2026-06-08 for alice/qwen, two for bob/claude on
	// 2026-06-09, and one alice/qwen on 2026-06-09.
	rows := []Usage{
		{TS: mustDay(t, "2026-06-08T10:00:00Z"), UserID: "alice", Model: "qwen3-14b", PromptTokens: 100, CompletionTokens: 50, Protocol: "openai", Outcome: "ok"},
		{TS: mustDay(t, "2026-06-08T11:00:00Z"), UserID: "alice", Model: "qwen3-14b", PromptTokens: 200, CompletionTokens: 100, Protocol: "openai", Outcome: "ok"},
		{TS: mustDay(t, "2026-06-09T09:00:00Z"), UserID: "bob", Model: "claude-3-5-sonnet", PromptTokens: 80, CompletionTokens: 40, Protocol: "anthropic", Outcome: "ok"},
		{TS: mustDay(t, "2026-06-09T10:00:00Z"), UserID: "bob", Model: "claude-3-5-sonnet", PromptTokens: 80, CompletionTokens: 40, Protocol: "anthropic", Outcome: "error"},
		{TS: mustDay(t, "2026-06-09T11:00:00Z"), UserID: "alice", Model: "qwen3-14b", PromptTokens: 50, CompletionTokens: 25, Protocol: "openai", Outcome: "ok"},
	}
	for _, r := range rows {
		if err := st.Usage().Record(ctx, r); err != nil {
			t.Fatalf("Record: %v", err)
		}
	}

	got, totals, err := st.Usage().Breakdown(ctx, BreakdownOpts{
		Bucket:  "day",
		Since:   mustDay(t, "2026-06-08T00:00:00Z"),
		Until:   mustDay(t, "2026-06-10T00:00:00Z"),
		GroupBy: []string{"user", "model"},
	})
	if err != nil {
		t.Fatalf("Breakdown: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3 groups (alice+qwen on 08, bob+claude on 09, alice+qwen on 09), got %d: %+v", len(got), got)
	}
	if totals.Requests != 5 {
		t.Errorf("totals.Requests = %d, want 5", totals.Requests)
	}
	if totals.PromptTokens != 510 {
		t.Errorf("totals.PromptTokens = %d, want 510", totals.PromptTokens)
	}

	// totals mode rolls everything into one bucket.
	tot, _, err := st.Usage().Breakdown(ctx, BreakdownOpts{
		Bucket:  "total",
		Since:   mustDay(t, "2026-06-08T00:00:00Z"),
		Until:   mustDay(t, "2026-06-10T00:00:00Z"),
		GroupBy: []string{"model"},
	})
	if err != nil {
		t.Fatalf("Breakdown total: %v", err)
	}
	if len(tot) != 2 {
		t.Fatalf("expected 2 models in total mode, got %d: %+v", len(tot), tot)
	}
}

func TestUsageBreakdown_RejectsUnknownGroupBy(t *testing.T) {
	dir := t.TempDir()
	st, err := OpenSQLite(filepath.Join(dir, "u.db"))
	if err != nil {
		t.Fatalf("OpenSQLite: %v", err)
	}
	defer st.Close()
	_, _, err = st.Usage().Breakdown(context.Background(), BreakdownOpts{
		GroupBy: []string{"made_up_field"},
	})
	if err == nil {
		t.Fatal("expected error for unknown group_by token")
	}
}

func mustDay(t *testing.T, iso string) time.Time {
	t.Helper()
	tt, err := time.Parse(time.RFC3339, iso)
	if err != nil {
		t.Fatalf("parse %s: %v", iso, err)
	}
	return tt
}

// sameSlice treats nil and []string{} as distinct (the allowlist
// semantics depend on the distinction), but slice equality is by value.
func sameSlice(a, b []string) bool {
	if (a == nil) != (b == nil) {
		return false
	}
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
