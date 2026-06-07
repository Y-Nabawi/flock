# Roadmap — multimodal + accessibility (v0.4 → v1.0)

Last updated: 2026-06-07 · Current release: **v0.3.0** · See [TASKS.md](TASKS.md) for the per-task tracker.

This file is the strategic plan. It groups everything into three buckets — **modalities that fit Flock's architecture**, **modalities that stretch it**, and the **eight accessibility bets** that turn open-source AI from "I can run a model" into "my team uses this in production."

Video and real-time voice agents are intentionally out of scope. They belong in sibling projects (`Reel` and `Murmur`) that depend on Flock for the LLM piece — see [§ Out of scope](#out-of-scope).

---

## Buckets

### A. Fits naturally — extend the gateway in place (v0.4)

These reuse the existing Engine interface, router, and store. They add endpoints or capability flags; the operational model stays the same.

| Item | Endpoint | Engines that support it | Compat notes | Status |
| --- | --- | --- | --- | --- |
| **Vision (image input)** | `POST /v1/chat/completions` with `image_url` content blocks | Ollama (`images: []`), vLLM, MLX-LM | `engines.Message.Content` → needs `Images []string`. OpenAI content-array parsing in `internal/api/openai.go`. Anthropic `image` blocks in `internal/api/anthropic.go`. Catalog already has `vision` capability. | **Shipped in v0.4 (this commit)** for Ollama |
| **Embeddings** | `POST /v1/embeddings` | Ollama (`/api/embeddings`), vLLM (`/v1/embeddings`), MLX-LM | New `Engine.Embed(ctx, model, input) []float32` method. Catalog entries get `embedding` capability + `embedding_dim`. Router picks by capability. | Planned |
| **Rerank** | `POST /v1/rerank` (Cohere shape) | BGE / cohere-rerank via vLLM, llama.cpp custom | Sibling to embeddings. `Engine.Rerank(ctx, model, query, docs)`. | Planned |

### B. Stretches the gateway — works but requires new code paths (v0.5–v0.6)

These add endpoints that don't fit the chat-streaming pattern. Worth doing but each is a noticeable code-shape change.

| Item | Endpoint | Engines | Stretch | Verdict |
| --- | --- | --- | --- | --- |
| **ASR (speech → text)** | `POST /v1/audio/transcriptions` | faster-whisper, NVIDIA NeMo (Nemotron 3.5 ASR), vLLM-whisper | Synchronous request, non-streaming response. New engine type `ASREngine`. Audio bytes in, text out. | v0.5 — voice agents are next wave; needed |
| **TTS (text → speech)** | `POST /v1/audio/speech` | Piper, Coqui XTTS, Bark | Output is binary audio (mp3/opus/pcm). Streaming via chunked binary or WebSocket. Different result shape than chat. | v0.5 |
| **Image generation** | `POST /v1/images/generations` | Stable Diffusion via diffusers, ComfyUI, Flux | 5–30 s synchronous. Different VRAM profile (squeezes out chat). Router needs to know about "GPU-locked" jobs vs token-streamed jobs. | v0.6 — only if there's demand |

### C. Out of scope — separate apps that depend on Flock

| Workload | Why separate | Sibling project (proposed) |
| --- | --- | --- |
| Video generation (HunyuanVideo, Wan2.1, LTX, Mochi) | Minutes per inference, multi-GB output, needs real job queue + webhook callbacks. Operational model is render farm, not API gateway. | **`Reel`** |
| Real-time voice agents (full-duplex, < 300 ms loop) | Bidirectional streaming, VAD, interruption handling. Tight ASR + LLM + TTS loop in one socket. | **`Murmur`** — uses Flock as the LLM backend |

---

## The eight accessibility bets

Most open-source AI tools optimize for "I can run a model on my GPU." That's solved. What's *not* solved is making **teams** comfortable using local AI in production. Eight bets, ranked by impact-per-effort:

| # | Bet | Why it matters | Where it lives in Flock | Effort | Target |
| --- | --- | --- | --- | --- | --- |
| 1 | **Cost transparency** | Show "$0 (local) vs $0.02 (Claude)" per call. Converts "this is cool" into "we should switch." Nobody else does this well. | Catalog gains `cost_per_1m_tokens_in/out`; `usage_records` gains `cost_micros`; admin UI Usage tab gets a $$ column | **S** | v0.4 |
| 2 | **Better-than-vendor team controls** | RBAC, SSO, billing-per-user analytics, content policies, retention. The reason teams stay on Claude/OpenAI is *administration*, not capability. | Auth refactor — add `roles` table, OIDC handler, policy engine | **L** | v0.5 |
| 3 | **Hardware abstraction across mixed fleets** | Treat M3 Studio + RTX 4090 + Snapdragon X as one compute pool. Scheduler routes by VRAM/load/network. | Replace router's `pick()` with a planner that uses `nodes.capabilities` (already in store) | **M** | v0.6 |
| 4 | **Privacy-by-default RAG** | Embed → store → retrieve → generate, end-to-end zero-network. Local embedding models are now Apache-licensed and good. | Built on top of (1) embeddings + new vector store adapter (SQLite-VSS or pgvector) | **M** | v0.5 |
| 5 | **Latency-aware fallback** | Silently fall back to smaller model (or vendor) when user's box can't keep up at < 2 s TTFT. 10× addressable user base. | Router records p95 TTFT per (node, model); falls back when over threshold | **M** | v0.5 |
| 6 | **Edge runtime (NAS / Pi)** | Real "average small business" segment. 4 B model on $400 Synology NAS is the democratization moment. | Cross-compile for `linux/arm64-musl`, statically link, package as `.deb` / `.spk` | **S** | v0.7 |
| 7 | **Signed model catalogs** | "apt for AI." Verified provenance, signed entries, community contributions with review. | Add `minisign` signature next to each catalog YAML; `flock model add` verifies before install | **S** | v0.6 |
| 8 | **Embeddable Go library** | Let desktop apps (LM Studio clones, IDE plugins) import `flock/runtime` directly. Biggest distribution channel for OSS AI in 2026 isn't a CLI — it's *embedded in tools developers already use*. | Move CLI-glue out of `internal/`, expose `pkg/runtime`, `pkg/router`, `pkg/store` | **L** | v1.0 |

---

## Compatibility review (per item)

Already-shipped subsystems that each item touches. Bold = breaking change, italic = additive only.

| Item | engines.Engine | internal/api/* | internal/store/* | internal/router | catalog YAML schema |
| --- | --- | --- | --- | --- | --- |
| Vision | *add Images []string* | *parse content array* | none | none | none (already has `vision`) |
| Embeddings | *new method `Embed()`* | *new `/v1/embeddings`* | *new `embedding_calls` table* | *route by capability* | *add `embedding_dim`* |
| Rerank | *new method `Rerank()`* | *new `/v1/rerank`* | piggyback embeddings table | route by capability | *add `rerank` capability* |
| ASR | *new sibling interface `ASREngine`* | *new `/v1/audio/transcriptions`* | *audio_calls table* | extend pick() | *add `asr` capability* |
| TTS | *new `TTSEngine`* | *new `/v1/audio/speech`* | reuse audio_calls | extend pick() | *add `tts` capability* |
| Image gen | *new `ImageEngine`* | *new `/v1/images/generations`* | *image_calls + storage* | needs job-aware scheduler | *add `image_gen` capability* |
| (1) Cost | none | none | **add `cost_micros` column** | none | *add `cost_per_1m_tokens_*`* |
| (2) RBAC | none | *check role on every request* | *new `roles` + `policies` tables* | none | none |
| (3) HW abstraction | none | none | extend `nodes.capabilities` JSON | **rewrite `pick()`** | none |
| (4) RAG | sibling — new package `internal/rag/` | new `/v1/rag/*` endpoints | new `rag_collections` + vector storage | route via embeddings | none |
| (5) Latency fallback | none | none | *add `route_telemetry` table* | extend pick() | *add fallback chain to catalog* |
| (6) Edge runtime | none | none | none | none | none |
| (7) Signed catalogs | none | none | none | none | *add `signature` field* |
| (8) Go library | **reorganize package layout** | none | none | none | none |

The only **breaking changes** are (3) router rewrite and (8) package layout — both planned for after v0.5 so users have time to adopt.

---

## Sequence

```
v0.4 (now)    → Vision (Ollama) · stub for vLLM/MLX · catalog cost field (data only)
v0.4.x        → Embeddings · Rerank · Cost UI in admin dashboard
v0.5          → ASR · TTS · RBAC (bet 2) · Latency fallback (bet 5)
v0.5.x        → RAG package (bet 4) · privacy-by-default
v0.6          → Image generation · HW scheduler (bet 3) · signed catalogs (bet 7)
v0.7          → Edge runtime (bet 6) · arm64 NAS packages
v1.0          → Embeddable Go library (bet 8) · API stability commitment
```

Every release is auto-cut from conventional commits — see `.github/workflows/auto-release.yml`.

---

## Out of scope

- **Video generation.** Sibling repo `Reel`. Job-queue model, not gateway.
- **Real-time voice agents.** Sibling repo `Murmur`. Uses Flock as the LLM backend.
- **Training / fine-tuning.** Out of project scope. Use `axolotl`, `unsloth`, or `torchtune`.
- **Vector store as a service.** Flock will ship an *adapter* for SQLite-VSS / pgvector in (4) but won't run its own vector store.
