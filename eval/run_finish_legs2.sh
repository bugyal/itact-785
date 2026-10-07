#!/bin/zsh
# P2 finish legs, part 2 — complete the z.ai partial runs (resume by id) and
# run the claude CLI sample leg (100 shuffled tasks, opus default).
# Run AFTER run_finish_legs.sh completes (shares the Ollama Cloud quota;
# z.ai legs are a separate quota so they can run in parallel with it).
set -u
cd /Users/ashishbhatt/Projects/tax-model/eval
set -a; source "$HOME/.hermes/profiles/coding/.env"; set +a
mkdir -p results

echo "[$(date '+%H:%M:%S')] leg glm-5.3 resume start (167 missing ids)"
/tmp/tax-eval -split test -backend zai-a -model glm-5.3 \
  -concurrency 4 -max-tokens 1024 -out results/ \
  -resume-from results/_prefix_run1_archive/glm-5.3-20261005-194209.jsonl
echo "[$(date '+%H:%M:%S')] leg glm-5.3 resume exit=$?"

echo "[$(date '+%H:%M:%S')] leg glm-5.3-flash resume start (167 missing ids)"
/tmp/tax-eval -split test -backend zai-a -model glm-5.3-flash \
  -concurrency 4 -max-tokens 1024 -out results/ \
  -resume-from results/_prefix_run1_archive/glm-5.3-flash-20261005-200553.jsonl
echo "[$(date '+%H:%M:%S')] leg glm-5.3-flash resume exit=$?"

echo "[$(date '+%H:%M:%S')] leg claude-opus sample start (100 shuffled tasks)"
/tmp/tax-eval -split test -backend claude -model claude-opus-5-5 \
  -concurrency 1 -max-tokens 1024 -timeout 300 -limit 100 -shuffle \
  -out results/
echo "[$(date '+%H:%M:%S')] leg claude-opus sample exit=$?"
echo "[$(date '+%H:%M:%S')] ALL LEGS PART-2 DONE"