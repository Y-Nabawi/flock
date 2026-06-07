package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/hadihonarvar/flock/internal/engines"
)

// ---- /v1/embeddings ----
//
// OpenAI-compatible embedding endpoint. Request shape:
//
//	{
//	  "model": "nomic-embed-text",
//	  "input": "the cat sat on the mat"          // OR ["sentence 1", "sentence 2"]
//	}
//
// Response shape:
//
//	{
//	  "object": "list",
//	  "data": [{"object": "embedding", "embedding": [floats…], "index": 0}, …],
//	  "model": "nomic-embed-text",
//	  "usage": {"prompt_tokens": 12, "total_tokens": 12}
//	}

type embeddingRequest struct {
	Model string          `json:"model"`
	Input json.RawMessage `json:"input"` // string or array of strings
	User  string          `json:"user,omitempty"`
}

type embeddingObject struct {
	Object    string    `json:"object"`
	Embedding []float32 `json:"embedding"`
	Index     int       `json:"index"`
}

type embeddingResponse struct {
	Object string            `json:"object"`
	Data   []embeddingObject `json:"data"`
	Model  string            `json:"model"`
	Usage  usage             `json:"usage"`
}

// Embeddings handles POST /v1/embeddings.
func (h *Handler) Embeddings(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()

	var req embeddingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_request", "Invalid JSON body: "+err.Error())
		return
	}
	if len(req.Input) == 0 {
		writeJSONError(w, http.StatusBadRequest, "invalid_request", "input is required")
		return
	}

	inputs, err := parseEmbeddingInput(req.Input)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid_request", err.Error())
		return
	}
	if len(inputs) == 0 {
		writeJSONError(w, http.StatusBadRequest, "invalid_request", "input must contain at least one non-empty string")
		return
	}

	// The engine must implement EmbedEngine. The Router does this by
	// asserting on the picked backend; other engines that lack embedding
	// support surface as 501 here.
	ee, ok := h.Engine.(engines.EmbedEngine)
	if !ok {
		writeJSONError(w, http.StatusNotImplemented, "embeddings_not_supported",
			"the configured engine does not support embeddings")
		return
	}

	requested := req.Model
	if requested == "" {
		writeJSONError(w, http.StatusBadRequest, "invalid_request", "model is required")
		return
	}
	resolved, err := h.ResolveModel(requested)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "model_not_found", err.Error())
		return
	}

	start := time.Now()
	res, err := ee.Embed(r.Context(), engines.EmbedRequest{
		Model:  resolved,
		Inputs: inputs,
	})
	if err != nil {
		writeJSONError(w, http.StatusBadGateway, "upstream_error", err.Error())
		recordUsage(r.Context(), h.Store, "openai", requested, nil, time.Since(start), "error")
		return
	}

	out := embeddingResponse{
		Object: "list",
		Model:  requested,
		Data:   make([]embeddingObject, 0, len(res.Vectors)),
	}
	for i, v := range res.Vectors {
		out.Data = append(out.Data, embeddingObject{
			Object:    "embedding",
			Embedding: v,
			Index:     i,
		})
	}
	if res.Usage != nil {
		out.Usage = usage{
			PromptTokens: res.Usage.PromptTokens,
			TotalTokens:  res.Usage.TotalTokens,
		}
	}

	// Record the call so quota + audit + cost analytics all see it.
	var u *engines.Usage
	if res.Usage != nil {
		u = &engines.Usage{PromptTokens: res.Usage.PromptTokens, TotalTokens: res.Usage.TotalTokens}
	}
	recordUsage(r.Context(), h.Store, "openai", requested, u, time.Since(start), "ok")

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

// parseEmbeddingInput accepts the OpenAI `input` field — either a single
// string or an array of strings — and returns a normalized list. Skips
// empty strings; returns an error for invalid JSON shape.
func parseEmbeddingInput(raw json.RawMessage) ([]string, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	// Try single string.
	var single string
	if err := json.Unmarshal(raw, &single); err == nil {
		if single == "" {
			return nil, nil
		}
		return []string{single}, nil
	}
	// Try array of strings.
	var multi []string
	if err := json.Unmarshal(raw, &multi); err == nil {
		out := multi[:0]
		for _, s := range multi {
			if s != "" {
				out = append(out, s)
			}
		}
		return out, nil
	}
	return nil, fmt.Errorf("input must be a string or array of strings")
}
