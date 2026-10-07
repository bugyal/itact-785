#!/bin/zsh
# P2 baseline sweep v3 — Tax-Model t_7a5c0799 (post-fix rerun)
# Changes vs v2:
#   - runs the FIXED harness (single-writer + integrity gate + grading fixes)
#   - glm-4.6 (OpenRouter) switched to SERIAL workers (-concurrency 1): free
#     tier in-flight cap wiped all 4 OR legs in run1 with 402s
#   - deepseek-r1 smoke gate kept; claude ceilings LAST (Max session limit
#     resets 15:10 IST — run after the reset, ~3h budget)
# Run as:  caffeinate -is ./run_sweep.sh
set -u
REPO="$HOME/Projects/tax-model"
ENV_FILE="$HOME/.hermes/profiles/coding/.env"
cd "$REPO/eval" || exit 1

set -a; source "$ENV_FILE"; set +a

log() { echo "[$(date '+%m-%d %H:%M:%S')] $*" | tee -a sweep.log; }

# ---------- 1. OpenRouter legs (serial for rate-limited routes) ----------
# Per-model max-tokens: reasoning models need headroom for thinking tokens.
or_leg() {
  local m="$1" mt="$2" to="$3" conc="$4"
  log "LEG openrouter/$m start (max_tokens=$mt timeout=$to concurrency=$conc)"
  ./tax-eval -backend openrouter -model "$m" -concurrency "$conc" -timeout "$to" \
    -max-tokens "$mt" -out results >> sweep.log 2>&1
  log "LEG openrouter/$m exit=$?"
}
or_leg "deepseek/deepseek-chat-v3.1" 2048 180 6   # cheap route tolerated 6-wide in run1
or_leg "qwen/qwen3-235b-a22b"       4096 300 1   # P3 fine-tune base family — REQUIRED
or_leg "qwen/qwen3-8b"              4096 300 1   # small-base comparison
or_leg "z-ai/glm-4.6"               2048 180 1   # GLM family, non-plan route (402-prone: serial)
# deepseek-r1: smoke 3 first to verify route + reasoning extraction, then full.
log "LEG openrouter/deepseek/deepseek-r1 smoke(3) start"
./tax-eval -backend openrouter -model deepseek/deepseek-r1 -limit 3 -concurrency 1 \
  -timeout 300 -max-tokens 4096 -out results/_r1_smoke >> sweep.log 2>&1
log "LEG openrouter/deepseek/deepseek-r1 smoke exit=$?"
if grep -q '"raw_response":"[^"]' results/_r1_smoke/*.jsonl 2>/dev/null; then
  or_leg "deepseek/deepseek-r1" 4096 300 1
else
  log "LEG openrouter/deepseek/deepseek-r1 SKIPPED — smoke produced no completions"
fi
or_leg "openai/gpt-oss-120b"       4096 300 1   # open-weights GPT ceiling proxy

# ---------- 2. Claude ceilings via claude CLI ($0, Claude Max) ----------
# Max session limit resets 15:10 IST; if we get here before the reset the leg
# will 429-error out — rerun this block after 15:10.
log "LEG claude/sonnet start"
./tax-eval -backend claude -model claude-sonnet-4-5 -concurrency 4 -timeout 300 \
  -out results >> sweep.log 2>&1
log "LEG claude/sonnet exit=$?"
log "LEG claude/haiku start"
./tax-eval -backend claude -model claude-haiku-4-5 -concurrency 4 -timeout 300 \
  -out results >> sweep.log 2>&1
log "LEG claude/haiku exit=$?"

# ---------- 3. ollama.com cloud retry (session limit may have reset) ----------
for M in deepseek-v4.1-flash gpt-oss:20b; do
  log "LEG ollama/$M start"
  ./tax-eval -backend ollama -model "$M" -concurrency 4 -timeout 240 \
    -out results >> sweep.log 2>&1
  log "LEG ollama/$M exit=$?"
done

# ---------- 4. Local ollama qwen3.5:4b (P3 fine-tune base) — capped sample ----------
if curl -s --max-time 3 http://127.0.0.1:11434/api/tags | grep -q qwen3.5; then
  log "LEG local qwen3.5:4b start (300-task shuffled sample)"
  ./tax-eval -backend custom -base-url http://127.0.0.1:11434/v1 \
    -model qwen3.5:4b -shuffle -limit 300 -concurrency 2 -timeout 300 -max-tokens 2048 \
    -out results >> sweep.log 2>&1
  log "LEG local qwen3.5:4b exit=$?"
else
  log "LEG local qwen3.5:4b SKIPPED — ollama server not reachable/model absent"
fi

# ---------- 5. Aggregate report ----------
log "building report"
./tax-eval -report results/baseline_scores.md -out results >> sweep.log 2>&1
log "ALL LEGS COMPLETE — report at eval/results/baseline_scores.md"
