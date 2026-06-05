// Worker HTTP server. Started by `flock join` so the leader can reach this
// node's local inference engine via the mesh. Bound to the agent's tailnet /
// LAN address — not 0.0.0.0 — so only mesh members can connect.
//
// The exposed surface is OpenAI-compatible passthrough to the local engine,
// authenticated with the worker token (the same secret the leader uses for
// outbound calls to this worker). The leader's RoutingEngine talks to this
// endpoint exactly the way it would talk to a standalone vLLM/MLX server.
package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/hadihonarvar/flock/internal/engines"
)

// Server is the worker's HTTP surface.
type Server struct {
	Engine engines.Engine
	Token  string // shared secret; same value the leader stores in node.worker_token

	http *http.Server
}

// Start runs the server until ctx is done. Returns the listen error or nil
// on graceful shutdown.
func (s *Server) Start(ctx context.Context, listen string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.healthz)
	mux.HandleFunc("/v1/models", s.auth(s.listModels))
	mux.HandleFunc("/v1/chat/completions", s.auth(s.chatCompletions))

	s.http = &http.Server{
		Addr:              listen,
		Handler:           mux,
		ReadHeaderTimeout: 30 * time.Second,
	}
	errCh := make(chan error, 1)
	go func() { errCh <- s.http.ListenAndServe() }()
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		return s.http.Shutdown(shutdownCtx)
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			return fmt.Errorf("worker listen: %w", err)
		}
		return nil
	}
}

func (s *Server) auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		got := r.Header.Get("Authorization")
		if strings.HasPrefix(got, "Bearer ") {
			got = strings.TrimPrefix(got, "Bearer ")
		}
		if s.Token == "" || got != s.Token {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

func (s *Server) healthz(w http.ResponseWriter, r *http.Request) {
	if err := s.Engine.Health(r.Context()); err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = fmt.Fprintf(w, "engine: %v", err)
		return
	}
	w.WriteHeader(http.StatusOK)
	_, _ = io.WriteString(w, "ok")
}

func (s *Server) listModels(w http.ResponseWriter, r *http.Request) {
	models, err := s.Engine.List(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	type obj struct {
		ID     string `json:"id"`
		Object string `json:"object"`
	}
	type list struct {
		Object string `json:"object"`
		Data   []obj  `json:"data"`
	}
	out := list{Object: "list"}
	for _, m := range models {
		out.Data = append(out.Data, obj{ID: m, Object: "model"})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(out)
}

// chatCompletions accepts an OpenAI-format chat request and proxies it to the
// local engine. Streaming and non-streaming both supported (the engine's Chat
// returns a channel either way; we re-emit it as SSE for stream=true).
func (s *Server) chatCompletions(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	var req struct {
		Model       string                   `json:"model"`
		Messages    []map[string]string      `json:"messages"`
		System      string                   `json:"system,omitempty"`
		Stream      bool                     `json:"stream,omitempty"`
		Temperature *float32                 `json:"temperature,omitempty"`
		TopP        *float32                 `json:"top_p,omitempty"`
		MaxTokens   *int                     `json:"max_tokens,omitempty"`
		Stop        []string                 `json:"stop,omitempty"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid body: "+err.Error(), http.StatusBadRequest)
		return
	}
	if len(req.Messages) == 0 {
		http.Error(w, "messages required", http.StatusBadRequest)
		return
	}
	msgs := make([]engines.Message, 0, len(req.Messages))
	for _, m := range req.Messages {
		msgs = append(msgs, engines.Message{Role: m["role"], Content: m["content"]})
	}
	engReq := engines.ChatRequest{
		Model:       req.Model,
		System:      req.System,
		Messages:    msgs,
		Temperature: req.Temperature,
		TopP:        req.TopP,
		MaxTokens:   req.MaxTokens,
		Stop:        req.Stop,
		Stream:      true,
	}
	stream, err := s.Engine.Chat(r.Context(), engReq)
	if err != nil {
		http.Error(w, "engine: "+err.Error(), http.StatusBadGateway)
		return
	}

	if req.Stream {
		writeSSE(w, r, stream, req.Model)
	} else {
		writeAggregate(w, stream, req.Model)
	}
}

func writeSSE(w http.ResponseWriter, r *http.Request, stream <-chan engines.StreamEvent, model string) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	flusher, _ := w.(http.Flusher)
	id := fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano())
	created := time.Now().Unix()

	sendChunk := func(payload map[string]any) {
		b, _ := json.Marshal(payload)
		_, _ = fmt.Fprintf(w, "data: %s\n\n", string(b))
		if flusher != nil {
			flusher.Flush()
		}
	}

	// initial role chunk
	sendChunk(map[string]any{
		"id": id, "object": "chat.completion.chunk", "created": created, "model": model,
		"choices": []map[string]any{{
			"index": 0, "delta": map[string]any{"role": "assistant"}, "finish_reason": nil,
		}},
	})

	defer func() {
		// drain in background so engine producer never blocks
		go func() {
			for range stream {
			}
		}()
	}()

	for ev := range stream {
		if r.Context().Err() != nil {
			return
		}
		if ev.Err != nil {
			sendChunk(map[string]any{"error": map[string]any{"message": ev.Err.Error()}})
			return
		}
		if ev.Done {
			reason := ev.Reason
			if reason == "" {
				reason = "stop"
			}
			final := map[string]any{
				"id": id, "object": "chat.completion.chunk", "created": created, "model": model,
				"choices": []map[string]any{{
					"index": 0, "delta": map[string]any{}, "finish_reason": reason,
				}},
			}
			if ev.Usage != nil {
				final["usage"] = map[string]int{
					"prompt_tokens":     ev.Usage.PromptTokens,
					"completion_tokens": ev.Usage.CompletionTokens,
					"total_tokens":      ev.Usage.TotalTokens,
				}
			}
			sendChunk(final)
			_, _ = io.WriteString(w, "data: [DONE]\n\n")
			if flusher != nil {
				flusher.Flush()
			}
			return
		}
		if ev.Delta != "" {
			sendChunk(map[string]any{
				"id": id, "object": "chat.completion.chunk", "created": created, "model": model,
				"choices": []map[string]any{{
					"index": 0, "delta": map[string]any{"content": ev.Delta}, "finish_reason": nil,
				}},
			})
		}
	}
}

func writeAggregate(w http.ResponseWriter, stream <-chan engines.StreamEvent, model string) {
	defer func() {
		go func() {
			for range stream {
			}
		}()
	}()

	var text strings.Builder
	var usage *engines.Usage
	reason := "stop"
	for ev := range stream {
		if ev.Err != nil {
			http.Error(w, "engine: "+ev.Err.Error(), http.StatusBadGateway)
			return
		}
		if ev.Done {
			usage = ev.Usage
			if ev.Reason != "" {
				reason = ev.Reason
			}
			break
		}
		text.WriteString(ev.Delta)
	}
	resp := map[string]any{
		"id":      fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano()),
		"object":  "chat.completion",
		"created": time.Now().Unix(),
		"model":   model,
		"choices": []map[string]any{{
			"index":         0,
			"message":       map[string]string{"role": "assistant", "content": text.String()},
			"finish_reason": reason,
		}},
	}
	if usage != nil {
		resp["usage"] = map[string]int{
			"prompt_tokens":     usage.PromptTokens,
			"completion_tokens": usage.CompletionTokens,
			"total_tokens":      usage.TotalTokens,
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}
