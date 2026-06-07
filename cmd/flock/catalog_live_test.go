package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/hadihonarvar/flock/internal/models"
)

// TestCatalogSourcesReachable does a lightweight HEAD against the upstream
// for every catalog entry to confirm its `source:` actually exists. Catches
// typos (wrong Ollama tag, renamed HF repo) before users hit a 404 at
// `flock model add`.
//
// Gated behind CATALOG_LIVE_CHECK=1 so the default test suite stays fast
// and offline. Run as:
//
//	CATALOG_LIVE_CHECK=1 go test -run TestCatalogSourcesReachable ./cmd/flock/
//
// CI invokes this on a schedule (separate job) rather than every commit so
// upstream flakiness doesn't block merges.
func TestCatalogSourcesReachable(t *testing.T) {
	if os.Getenv("CATALOG_LIVE_CHECK") != "1" {
		t.Skip("set CATALOG_LIVE_CHECK=1 to run upstream HEAD probes")
	}

	repoRoot := findRepoRoot(t)
	cat, err := models.LoadCatalog(filepath.Join(repoRoot, "catalog"))
	if err != nil {
		t.Fatalf("LoadCatalog: %v", err)
	}

	client := &http.Client{Timeout: 15 * time.Second}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	type result struct {
		id      string
		ok      bool
		reason  string
		skipped string
	}
	results := make(chan result, len(cat))

	// Limit concurrency — be a polite citizen against Ollama + HF.
	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup

	for _, e := range cat {
		e := e
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			r := result{id: e.ID}
			switch e.Source.Type {
			case "ollama":
				r.ok, r.reason = headOllama(ctx, client, e.Source.OllamaName)
			case "huggingface":
				r.ok, r.reason = headHuggingFace(ctx, client, e.Source.Repo, e.Source.File)
			case "file":
				r.skipped = "source.type=file (local-only; can't verify without the file)"
			default:
				r.skipped = fmt.Sprintf("unknown source.type=%q", e.Source.Type)
			}
			results <- r
		}()
	}
	go func() { wg.Wait(); close(results) }()

	var failed, skipped int
	for r := range results {
		switch {
		case r.skipped != "":
			t.Logf("SKIP %-30s — %s", r.id, r.skipped)
			skipped++
		case !r.ok:
			t.Errorf("UNREACHABLE %-30s — %s", r.id, r.reason)
			failed++
		default:
			t.Logf("OK    %-30s", r.id)
		}
	}
	t.Logf("catalog live check: %d entries, %d skipped, %d failed", len(cat), skipped, failed)
}

// headOllama checks that an Ollama tag exists in the public registry.
// Tag format is "name:tag" or just "name" (which means name:latest).
func headOllama(ctx context.Context, client *http.Client, fullName string) (bool, string) {
	if fullName == "" {
		return false, "empty ollama_name"
	}
	name, tag, found := strings.Cut(fullName, ":")
	if !found {
		tag = "latest"
	}
	// Library namespace for unscoped names; scoped names look like "owner/name".
	registryPath := name
	if !strings.Contains(name, "/") {
		registryPath = "library/" + name
	}
	url := fmt.Sprintf("https://registry.ollama.ai/v2/%s/manifests/%s", registryPath, tag)
	req, _ := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	// Ollama registry needs an Accept header for the manifest media type.
	req.Header.Set("Accept", "application/vnd.docker.distribution.manifest.v2+json")
	resp, err := client.Do(req)
	if err != nil {
		return false, fmt.Sprintf("HEAD %s: %v", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		return true, ""
	}
	return false, fmt.Sprintf("HEAD %s → %s", url, resp.Status)
}

// headHuggingFace checks that an HF repo (and optional specific file) exists.
func headHuggingFace(ctx context.Context, client *http.Client, repo, file string) (bool, string) {
	if repo == "" {
		return false, "empty source.repo"
	}
	target := file
	if target == "" {
		target = "README.md"
	}
	url := fmt.Sprintf("https://huggingface.co/%s/resolve/main/%s", repo, target)
	req, _ := http.NewRequestWithContext(ctx, http.MethodHead, url, nil)
	resp, err := client.Do(req)
	if err != nil {
		return false, fmt.Sprintf("HEAD %s: %v", url, err)
	}
	defer resp.Body.Close()
	switch resp.StatusCode {
	case http.StatusOK, http.StatusFound:
		return true, ""
	case http.StatusUnauthorized, http.StatusForbidden:
		// Repo exists but is gated (requires accepting a license or HF login).
		// For our "catch typos / removed repos" purpose, this is success.
		return true, ""
	default:
		return false, fmt.Sprintf("HEAD %s → %s", url, resp.Status)
	}
}
