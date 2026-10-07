#!/bin/zsh
# Leg 2 of the P2.5 cross-check sweep: glm-5.3 via z.ai Anthropic wire.
# (Leg 1 glm-5.3-flash already complete in ./logs; leg 3 qwen runs after.)
set -u
ENV_FILE="$HOME/.hermes/profiles/coding/.env"
set -a; source "$ENV_FILE"; set +a
export ANTHROPIC_BASE_URL="https://api.z.ai/api/anthropic"
export ANTHROPIC_AUTH_TOKEN="$ZAI_API_KEY"
cd "$(dirname "$0")"
echo "[$(date '+%H:%M:%S')] LEG glm-5.3 start"
../../.venv-xcheck/bin/inspect eval crosscheck_task.py \
  --model anthropic/glm-5.3 \
  -T max_tokens=1024 --reasoning-effort none --max-connections 4 \
  --log-dir ./logs
echo "[$(date '+%H:%M:%S')] LEG glm-5.3 exit=$?"
