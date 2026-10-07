#!/bin/zsh
# Live 1-task probe of each cross-check provider route.
set -u
ENV_FILE="$HOME/.hermes/profiles/coding/.env"
set -a; source "$ENV_FILE"; set +a
cd "$(dirname "$0")"
PY=../../.venv-xcheck/bin

echo "=== probe: z.ai anthropic-wire (glm-5.3-flash), ANTHROPIC_API_KEY ==="
ANTHROPIC_BASE_URL="https://api.z.ai/api/anthropic" \
ANTHROPIC_API_KEY="$ZAI_API_KEY" \
$PY/inspect eval crosscheck_task.py --model anthropic/glm-5.3-flash \
  -T max_tokens=1024 --limit 1 --max-connections 1 --log-dir ./logs-probe 2>&1 | tail -6
