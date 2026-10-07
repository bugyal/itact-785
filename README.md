# itact-785

**Can LLMs cite India's new Income-tax Act? We locked 785 compliance tasks before running any model. Best frontier model: 30.2% strict accuracy. On the four-part task a notice-reply draft actually needs — code + section + rate + due-date, all correct — every model scored 0%.**

itact-785 is a locked, sha256-verified evaluation set and grading harness for applied Indian tax-law compliance under the **Income-tax Act, 2025** (in force 1 April 2026, replacing the 1961 Act). Unlike US-bar-style legal QA, every task requires emitting exact machine-checkable entities: TDS payment codes (1001–1092), section references, rates, thresholds, due dates, and the governing Act on the 1-Apr-2026 boundary.

## Headline results (strict entity-match, no partial credit, no LLM judge)

| Model | Backend | Graded | Strict accuracy |
|---|---|---|---|
| glm-5.3-flash | z.ai | 774 | **30.2%** |
| glm-5.3 | z.ai | 776 | 26.5% |
| claude-haiku-4-5 | claude CLI | 548 | 32.5% |
| claude-sonnet-4-5 | claude CLI | 87 | 96.6%* |
| deepseek-v4.1-flash | Ollama Cloud | 776 | 11.6% |
| qwen3.5:4b (raw) | local | 776 | 11.6% |
| gpt-oss:20b | Ollama Cloud | 776 | 7.0% |
| gpt-oss:120b | Ollama Cloud | 776 | 2.4% |
| karsastra-4b (fine-tuned 4B) | local | 776 | **45.7%** |
| claude-opus | claude CLI | in flight | — |

\* sonnet-4-5 and haiku-4-5 legs are partial runs in progress; numbers will move. claude-opus leg currently failing on CLI errors — re-running.

Three observations that matter:

1. **Cheap beats flagship.** glm-5.3-flash (30.2%) outperforms its own flagship glm-5.3 (26.5%). Scale doesn't help — the 2025 Act simply isn't in any training distribution yet.
2. **Three product-defining zeros.** TDS payment-code lookup: 0/107 on glm legs (a model *invented "payment code: 94"*); deepseek recited the **repealed 1961-Act ITNS-281 code table** for a July-2026 question. Old→new section mapping: 0–32%. Four-part ICAI-style compliance: **0/99 on every model.**
3. **A fine-tuned 4B beats every frontier model.** karsastra-4b (Qwen3.5-4B fine-tuned on mapping tables) scores 45.7% — training data, not scale, is the lever. Its remaining failures are lookup near-misses, exactly what a verified-table layer exists to catch.

## What's here

```
data/splits/test.jsonl        785 locked tasks (questions + gold answers + provenance)
data/splits/test.locked.json  sha256 manifest (14ffaa7de68272d0…) — tamper check
eval/                         Go harness: runner, strict grader, tests, run scripts
eval/results/                 per-model answer JSONLs (raw, unedited) + score tables
docs/p2-findings-report.md    full findings report
```

Task families: boundary rules (which Act governs across 1 Apr 2026), code lookup, old→new section mapping, code→nature, due dates, form crosswalks, rate/threshold applications, TDS computations, and 99 four-part ICAI-style compliance bundles.

## Method (the part that makes this defensible)

- **Locked set**: 785 tasks, sha256-verified at every run start. No model saw a task before scoring.
- **Strict grading**: entity-extraction exact match; a task passes only with **all** gold entities present. No partial credit, no LLM-as-judge.
- **Grader validated two ways**: gold self-extraction (785/785 parse) and mutated-gold rejection (grader discriminates).
- **Identical protocol per model**: temperature 0, same prompt prefix, same token limits.
- **Independent cross-check**: grader logic ported and re-run in inspect-ai, zero verdict diffs on shared records. see eval/crosscheck/

## Reproduce

```bash
cd eval
go build ./...
./run_sweep.sh   # or run_finish_legs.sh — writes per-model JSONLs to eval/results/
```

The grader refuses to run unless the test-set sha256 matches `test.locked.json` (override with `-allow-unlocked` for your own variants — but then say so).

**Run your own model?** Add a backend in `eval/`, run the same 785, and open a PR with your JSONL. Numbers must be grader-generated, not hand-tallied — that's the whole point.

## Limitations (read before citing)

- Measures **factual mapping correctness** (codes, sections, rates, boundaries), not legal judgement or prose quality.
- Strict match means a semantically-fine answer with one wrong digit fails. We consider that a feature: in tax compliance, one wrong digit is a wrong challan.
- Scored only models we could run under existing subscriptions; some commercial APIs are absent (documented in findings report). Invite: run your keys, PR your numbers.
- Indian central law only (income tax/ITNS codes); GST citations out of scope for v1.

## Provenance

Gold answers are derived from Gazette-notified sources: the Income-tax Act 2025 (as amended by Finance Act 2026), notified form crosswalks, and CBDT transition FAQs. Each task carries a `source_row` with its source URL. Curated by [Prasthana Lab](https://github.com/bugyal).

## License

Dataset, results, and docs: **CC BY-4.0**. Harness code: Apache-2.0 — free for research and evaluation; if your commercial product embeds the drafting engine built on this data, we'd like to hear from you.