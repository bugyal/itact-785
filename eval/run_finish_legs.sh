#!/bin/zsh
# P2 finish: complete the locked 785-task baseline sweep legs that died on
# credits/timeouts, using ONLY the already-paid resources:
#   - gpt-oss:120b + glm-4.6 + deepseek-chat-v3.1 + qwen3-235b/8b all died on
#     OpenRouter 402 (zero credits). gpt-oss:120b is on Ollama Cloud (paid
#     subscription) -> re-run there.
#   - local qwen3.5:4b timed out at 120s per request (4b on M1/M2 CPU+GPU) ->
#     re-run with -timeout 600, -concurrency 2.
#   - sarvam-105b: dead account (402 insufficient_quota). Documented, not re-run.
#   - claude CLI legs: exit status 1 — diagnose separately (auth/plugin issue).
# Writes to eval/results (gitignored) same format as prior legs.
set -u
cd /Users/ashishbhatt/Projects/tax-model/eval
set -a; source "$HOME/.hermes/profiles/coding/.env"; set +a
PY="$(command -v python3)"
mkdir -p results

echo "[$(date '+%H:%M:%S')] leg gpt-oss-120b (ollama cloud) start"
/tmp/tax-eval -split test -backend ollama -model gpt-oss:120b \
  -concurrency 4 -max-tokens 1024 -out results/
echo "[$(date '+%H:%M:%S')] leg gpt-oss-120b exit=$?"

echo "[$(date '+%H:%M:%S')] leg qwen3.5:4b (local) start"
/tmp/tax-eval -split test -backend ollama -base-url http://127.0.0.1:11434/v1 \
  -model qwen3.5:4b -api-key-env OLLAMA_API_KEY_LOCAL -concurrency 2 \
  -max-tokens 2048 -timeout 600 -out results/
echo "[$(date '+%H:%M:%S')] leg qwen3.5:4b exit=$?"

echo "[$(date '+%H:%M:%S')] ALL LEGS DONE"