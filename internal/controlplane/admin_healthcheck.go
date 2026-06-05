package controlplane

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/hadihonarvar/flock/internal/engines"
)

// healthcheck serves POST /admin/v1/healthcheck.
// Body: {model?, max_tokens?}.
// Sends a tiny ("ping") completion through the router using the
// currently-loaded default model (or the model in the body) and reports
// whether it succeeded, how long it took, and which engine answered.
// Lets a user prove the gateway works end-to-end without leaving the
// dashboard or wiring up a real client.
func (s *Server) healthcheck(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	var req struct {
		Model     string `json:"model"`
		MaxTokens int    `json:"max_tokens"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req) // body is optional

	model := strings.TrimSpace(req.Model)
	if model == "" {
		model = s.cfg.Router.DefaultModel
	}
	maxTokens := req.MaxTokens
	if maxTokens <= 0 {
		maxTokens = 5
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	start := time.Now()
	mt := maxTokens // engines.ChatRequest expects *int
	stream, err := s.router.Chat(ctx, engines.ChatRequest{
		Model:     model,
		Messages:  []engines.Message{{Role: "user", Content: "ping"}},
		MaxTokens: &mt,
		Stream:    false,
	})
	if err != nil {
		writeJSON(w, http.StatusOK, healthcheckResult{
			OK:        false,
			LatencyMS: time.Since(start).Milliseconds(),
			Model:     model,
			Engine:    s.router.Name(),
			Error:     err.Error(),
		})
		return
	}

	var delta string
	var streamErr error
	for ev := range stream {
		if ev.Err != nil {
			streamErr = ev.Err
			continue
		}
		delta += ev.Delta
	}
	latency := time.Since(start).Milliseconds()

	if streamErr != nil {
		writeJSON(w, http.StatusOK, healthcheckResult{
			OK:        false,
			LatencyMS: latency,
			Model:     model,
			Engine:    s.router.Name(),
			Error:     streamErr.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, healthcheckResult{
		OK:        true,
		LatencyMS: latency,
		Model:     model,
		Engine:    s.router.Name(),
		Reply:     delta,
	})
}

type healthcheckResult struct {
	OK        bool   `json:"ok"`
	LatencyMS int64  `json:"latency_ms"`
	Model     string `json:"model"`
	Engine    string `json:"engine"`
	Reply     string `json:"reply,omitempty"`
	Error     string `json:"error,omitempty"`
}
