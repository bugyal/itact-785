"""inspect-ai cross-check of the Tax-Model locked eval sweep (P2.5).

Replicates the Go harness (eval/cmd/tax-eval) under a standard open-source
eval tool: same locked 785-task test set (sha256-verified), same single-user
prompt (preamble + question), same decode params (temperature 0, per-leg
max_tokens), and the same grader — via the parity-verified Python port of the
Go entity-extraction grader (grader.py; 0-diff verdict proof in
eval/crosscheck/, see card comment).

Providers (stock inspect-ai providers only):
  z.ai legs  -> anthropic/glm-5.3, anthropic/glm-5.3-flash
                env: ANTHROPIC_BASE_URL=https://api.z.ai/api/anthropic
                     ANTHROPIC_AUTH_TOKEN=$ZAI_API_KEY   (coding plan, Rs 0)
                (z.ai exposes an Anthropic-wire endpoint; base URL must NOT
                include /v1 — the SDK appends /v1/messages)
  deepseek   -> openrouter/deepseek/deepseek-chat-v3.1
                env: OPENROUTER_API_KEY=...

Run (see run_crosscheck.sh):
  inspect eval crosscheck_task.py -T max_tokens=1024 \
      --model anthropic/glm-5.3-flash --max-connections 4
"""

from __future__ import annotations

import hashlib
import json
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

from inspect_ai import Task, task
from inspect_ai.dataset import Sample, json_dataset
from inspect_ai.model import GenerateConfig
from inspect_ai.scorer import NOANSWER, Score, Scorer, Target, accuracy, scorer, stderr
from inspect_ai.solver import TaskState, generate

from grader import GradeResult, Task as TaxTask, grade

PROMPT_PREAMBLE = "Answer the following Indian tax compliance question precisely and concisely.\n\n"
_HERE = os.path.dirname(os.path.abspath(__file__))
SPLIT_PATH = os.path.join(_HERE, "..", "..", "data", "splits", "test.jsonl")
LOCK_PATH = os.path.join(_HERE, "..", "..", "data", "splits", "test.locked.json")


def _task_id(question: str) -> str:
    """Same stable id as the Go harness: sha256(question)[:8]."""
    return hashlib.sha256(question.encode("utf-8")).hexdigest()[:8]


def load_dataset():
    """Locked test split; verifies the sha256 lock exactly like the Go harness."""

    def record_to_sample(rec: dict) -> Sample:
        return Sample(
            id=_task_id(rec["question"]),
            input=PROMPT_PREAMBLE + rec["question"],
            target=rec["answer"],
            metadata={
                "task_type": rec.get("task_type", ""),
                "table": rec.get("table", ""),
                "fact_key": rec.get("fact_key", ""),
                "answer_type": rec.get("answer_type", ""),
                "template": rec.get("template", ""),
                "question": rec["question"],
            },
        )

    with open(LOCK_PATH) as f:
        lock = json.load(f)
    ds = json_dataset(SPLIT_PATH, record_to_sample)
    questions = sorted(s.metadata["question"] for s in ds)
    actual = hashlib.sha256("\n".join(questions).encode("utf-8")).hexdigest()
    if actual != lock["sha256_questions"]:
        raise RuntimeError(
            "LOCK MISMATCH: %s != locked %s (test set changed since lock; refusing)"
            % (actual, lock["sha256_questions"])
        )
    return ds


def score_from_result(g: GradeResult, completion: str) -> Score:
    meta = {
        "graded": g.graded,
        "score_parts": g.score_parts,
        "missing": g.missing_gold,
        "extract_gold": g.extract_gold,
        "extract_model": g.extract_model,
    }
    if not g.graded:
        return Score(value=NOANSWER, answer=completion,
                     explanation=g.reason or "rubric_pending", metadata=meta)
    return Score(
        value=1.0 if g.passed else 0.0,
        answer=completion,
        explanation="PASS" if g.passed else "missing: " + "; ".join(g.missing_gold),
        metadata=meta,
    )


@scorer(metrics=[accuracy(), stderr()])
def tax_entity_check() -> Scorer:
    """Entity-extraction grading identical to the Go harness (parity-proven)."""

    async def score(state: TaskState, target: Target) -> Score:
        t = TaxTask(
            task_type=state.metadata.get("task_type", ""),
            table=state.metadata.get("table", ""),
            fact_key=state.metadata.get("fact_key", ""),
            question=state.metadata.get("question", ""),
            answer=target.text,
            answer_type=state.metadata.get("answer_type", ""),
            template=state.metadata.get("template", ""),
        )
        g = grade(t, state.output.completion)
        return score_from_result(g, state.output.completion)

    return score


@task
def taxmodel_crosscheck(max_tokens: int = 1024):
    """Same decode contract as the Go harness: temp 0, top_p 1, fixed max_tokens."""
    return Task(
        dataset=load_dataset(),
        solver=generate(),
        scorer=tax_entity_check(),
        config=GenerateConfig(temperature=0.0, max_tokens=max_tokens, top_p=1.0),
    )
