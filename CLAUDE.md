# Mise

Self-hosted, single-user recipe app: Go 1.26 backend (`backend/`, chi + pgx), React 19 + Vite + TypeScript frontend (`frontend/`), Ollama for LLMs; recipe embeddings run in-process (EmbeddingGemma on ONNX Runtime). Pushing to `master` runs CI, which builds the Docker image; the owner deploys it.

## Commands
- `cd backend && go build ./... && go vet ./... && go test ./...` - backend checks
- `cd backend && golangci-lint run ./...` - CI's lint step; needs v2.15 (the `golangci-lint` in `~/go/bin` is v1.26 and can't type-check this code), so install with `GOBIN=<scratch> go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0`
- `cd frontend && npx tsc --noEmit -p . && npm run lint && npx vitest run && npm run build` - frontend checks
- `MISE_OLLAMA_HOST=http://<host>:11434 go test -tags e2e -run E2E ./internal/llm/` - live LLM tests (opt-in)
- `MISE_EMBEDDING_DIR=<dir with model_quantized.onnx(+_data), tokenizer.json> MISE_ONNXRUNTIME_LIB=<onnxruntime.dll|.so> go test ./internal/embedding/` - tokenizer + embedder tests (skipped without the files; the pip `onnxruntime` wheel ships a usable `onnxruntime.dll`)
- `MISE_REC_DATA=<dir> go test -tags e2e -run Data ./internal/recommend/` - run the recommender on exported library JSON
- `MISE_API=http://<server>:8080 npx vite` - dev server proxying to another backend

## Gotchas
- Never run plain `npm install` in `frontend/`: the local npm 6 rewrites `package-lock.json` to v1 (it must stay v3). Install npm 11 into a scratch dir and run `node <dir>/node_modules/npm/bin/npm-cli.js install ...`.
- Many files use CRLF: regex/sed replacements anchored on `\n` silently miss. Use the Edit tool or normalise CRLF first.
- `gofmt -l` flags every CRLF file; check with `tr -d '\r' < f.go > tmp.go && gofmt -l tmp.go`.
- Postgres lives on the NAS: don't run the backend locally against it (background jobs would touch real data). No local DB/Docker, so keep new SQL plain and verify via the deployed API.
- Headless Edge can't render viewports narrower than ~480px: check phone layouts inside a 390px iframe. Pages with SSE (Review) never settle under `--virtual-time-budget`.
- Vite's file watcher sometimes misses rewritten files on Windows: restart the dev server if a page looks stale.

## Product conventions
- Ratings (1–10): 7+ good (Favourites start at 7), 6 fine only if under 15 min, 5 or below disliked.
- Plan names follow the owner's grocery-period convention "DD-MM-YYYY tot DD-MM-YYYY": never reformat them. Plans are two-week grocery cycles, not day-by-day schedules.
- Mise is for inspiration and ingredients, not cooking guidance: no step-by-step cooking mode; always show the full method.
- Keep recipes authentic; don't restrict them to Albert Heijn stock (the owner substitutes or shops elsewhere).
- New-recipe prompts ask for a complete dinner (protein, starch, ≥125 g vegetables per serving); imports, refinements and leftover recipes must not.

## Architecture notes
- LLM providers live in the DB (Settings UI) and are routed by tag: generation, background-generation, chat, search, translation, inventory, deduplication.
- gemma4 follows user-turn instructions more reliably than system-prompt ones (avoid lists, "call db_search first").
- `internal/recommend`: TF-IDF blended 30/70 with embeddinggemma, spherical k-means flavour families, Thompson sampling across families. Embeddings persist in `recipe_embeddings` (keyed by model + text hash); recipe creates/edits/deletes fire `Queries.OnRecipesChanged` → `Service.Notify`, and a background worker embeds stale recipes and rebuilds the families (~3 s) while requests keep using the cached model (only the first build after startup is synchronous; an outdated model skips recipes deleted since). Embeddings use the clustering prompt, are mean-centred, and the same-dish thresholds are percentiles of the library's pairwise similarities. The model is fixed on purpose: the 30/70 blend and the percentiles were measured on EmbeddingGemma (it beat qwen3-embedding, bge-m3, mxbai, nomic and embeddinggemma-2 on flavour structure); re-benchmark before switching.
- `internal/embedding`: EmbeddingGemma int8 ONNX via onnxruntime_go (cgo) with a pure-Go port of its BPE tokenizer (matches HF `tokenizers` exactly; int8 scores the same as fp32/Ollama). The Dockerfile downloads the model and `libonnxruntime.so` by pinned checksum into the `distroless/cc` image; the model loads per sync and is freed afterwards. onnxruntime_go's version pins the ORT C API version, so bump the ORT tarball with it.
- Plan filling, replacing dinners and the home/library picks all go through the recommender (`suggestDinners` in `handlers/meal_plan.go` adds the favourites share).
- Styles live in `frontend/src/styles/*.css`, imported in cascade order from `index.css`; tokens in `tokens.css`; icons from `lucide-react`.
