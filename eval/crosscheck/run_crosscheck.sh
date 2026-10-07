#!/bin/zsh
# P2.5 cross-check sweep: replicate the locked 785-task sweep in inspect-ai.
# Legs: glm-5.3 + glm-5.3-flash (z.ai coding plan, Rs 0) and local
# qwen3.5:4b (ollama, Rs 0). deepseek-chat-v3.1 via OpenRouter is BLOCKED
# (account has zero credits; buying credits violates the zero-marginal-cost
# constraint) — documented in crosscheck.md.
set -u
ENV_FILE="$HOME/.hermes/profiles/coding/.env"
set -a; source "$ENV_FILE"; set +a
cd "$(dirname "$0")"
PY=../../.venv-xcheck/bin
LOGDIR=./logs
mkdir -p "$LOGDIR"

leg() {
  local name="$1"; shift
  echo "[$(date '+%H:%M:%S')] LEG $name start"
  "$@" 2>&1 | tail -4
  echo "[$(date '+%H:%M:%S')] LEG $name exit=$?"
}

# ---- leg 1: glm-5.3-flash via z.ai Anthropic wire (thinking disabled, 1024) ----
leg glm-5.3-flash env ANTHROPIC_BASE_URL="https://api.z.ai/api/anthropic" \
  ANTHROPIC_API_KEY="$ZAI_API_KEY" \
  $PY/inspect eval crosscheck_task.py --model anthropic/glm-5.3-flash \
  -T max_tokens=1024 --reasoning-effort none --max-connections 4 \
  --log-dir "$LOGDIR"

# ---- leg 2: glm-5.3 via z.ai Anthropic wire ----
leg glm-5.3 env ANTHROPIC_BASE_URL="https://api.z.ai/api/anthropic" \
  ANTHROPIC_API_KEY="$ZAI_API_KEY" \
  $PY/inspect eval crosscheck_task.py --model anthropic/glm-5.3 \
  -T max_tokens=1024 --reasoning-effort none --max-connections 4 \
  --log-dir "$LOGDIR"

# ---- leg 3: local qwen3.5:4b (P3 fine-tune base) via ollama ----
leg qwen3.5-4b $PY/inspect eval crosscheck_task.py \
  --model ollama/qwen3.5:4b \
  -T max_tokens=2048 --max-connections 2 \
  --log-dir "$LOGDIR"

echo "[$(date '+%H:%M:%S')] ALL LEGS DONE"
