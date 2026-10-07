#!/bin/zsh
# Probe the remaining two legs (glm-5.3 via z.ai, deepseek via OpenRouter), 1 task each.
set -u
ENV_FILE="$HOME/.hermes/profiles/coding/.env"
set -a; source "$ENV_FILE"; set +a
cd "$(dirname "$0")"
PY=../../.venv-xcheck/bin

echo "=== probe: anthropic/glm-5.3 via z.ai ==="
ANTHROPIC_BASE_URL="https://api.z.ai/api/anthropic" \
ANTHROPIC_API_KEY="$ZAI_API_KEY" \
$PY/inspect eval crosscheck_task.py --model anthropic/glm-5.3 \
  -T max_tokens=1024 --reasoning-effort none --limit 1 --max-connections 1 \
  --log-dir ./logs-probe 2>&1 | tail -3

echo "=== probe: openrouter/deepseek/deepseek-chat-v3.1 ==="
$PY/inspect eval crosscheck_task.py --model openrouter/deepseek/deepseek-chat-v3.1 \
  -T max_tokens=2048 --limit 1 --max-connections 1 \
  --log-dir ./logs-probe 2>&1 | tail -3
