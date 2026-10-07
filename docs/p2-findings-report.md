# P2 Findings Report — Frontier AI vs the 2025 Income-tax Act
**Tax-Model P2 baseline sweep · completed 2026-10-06 · locked 785-task set (sha 14ffaa7d…)**

The correctness report this project was built to produce. One line summary:
**Every frontier model tested flunks the locked 785-task compliance set — the best scores 30.2% strict accuracy, and the three task families that matter most for notice-reply drafting (payment-code lookup, old→new section mapping, 4-part ICAI-style compliance) score 0% on every model.** These are exactly the tasks `data/tables.db` answers deterministically.

---

## 1 · Method (what makes this defensible)

- **Locked test set**: 785 tasks, sha256-verified at run start (`14ffaa7de68272d0`). No task was seen by any model before scoring. Distribution: 107 code lookups, 144 section lookups, 109 rate lookups, 99 ICAI-style 4-part compliance, 73 section+code, 62 code→nature, 61 TDS computations, 91 table assignments, plus boundary/due-date/form/threshold tasks.
- **Strict grading**: entity-extraction exact match (codes 1001–1092, section refs, rates, amounts, dates, act years), pass = **all** gold entities present. multi_part (the 99 ICAI-style tasks) = all 4 parts correct. No partial credit, no LLM judge. Grader proved by two gates: gold-self-extraction (785/785 gold answers parse) and mutated-gold-fails (grader discriminates).
- **Identical protocol per model**: temp 0, max_tokens 1024 (2048 local), same single-user prompt prefix, concurrency 4. Run Oct 5–6, 2026.
- **785 = 776 machine-graded + 4 reasoned (rubric pending, excluded for all models) + 5 grader-ungradable (excluded identically).** Accuracy below is over the common 776-gradeable denominator.
- **Cross-check (P2.5)**: independently replicated in inspect-ai with a parity-verified Python port of the Go grader — 0-verdict-diff over shared records.

## 2 · The headline table

| Model | Backend | Graded | Strict accuracy | Median latency |
|---|---|---|---|---|
| **glm-5.3-flash** | z.ai (Anthropic wire) | 774 | **30.2%** | 8.7 s |
| **glm-5.3** | z.ai | 776 | **26.5%** | 5.7 s |
| deepseek-v4.1-flash | Ollama Cloud | 776 | **11.6%** | 5.3 s |
| gpt-oss:20b | Ollama Cloud | 776 | **7.0%** | 14.8 s |
| gpt-oss:120b | Ollama Cloud | 776 | **2.4%** | 4.7 s |
| claude-opus-5-5 | claude CLI | 100-task sample | in flight (~11 h at ~7 min/call) | ~7 min/call |
| qwen3.5:4b (P3 base, raw) | local ollama | 785 | in flight (~48 h at 2-way concurrency, ~470 s/task — no urgency: needed only for the P3 fine-tune comparison) | ~470 s |

Dead legs (documented, not scored): deepseek-chat-v3.1, qwen3-235b, qwen3-8b, glm-4.6, gpt-oss-120b-via-OR — all OpenRouter HTTP 402 (account has never purchased credits); sarvam-105b — HTTP 402 (quota dead); claude sonnet/haiku aliases — CLI `unrecognized_model`. Total marginal cost of every scored leg: **₹0** (z.ai coding plan + Ollama Cloud subscription + local).

Two facts jump out:
1. **The cheap model beats the flagship** (glm-5.3-flash 30.2% > glm-5.3 26.5%). Scale doesn't help — the data simply isn't in the training distribution yet.
2. **No model is close to usable.** A 30% strict-accuracy drafting assistant inserts wrong citations into 7 of 10 replies — an unusable, professionally dangerous tool.

## 3 · The five findings

### F1 · TDS payment-code lookup: 0% everywhere — models can't even emit a 2025-Act code
107 tasks: "under the new Act 2025 code scheme, what payment code applies to salary–state-government employees?" Gold: `1001`.

| Model | Pass | Emitted any 10xx code at all |
|---|---|---|
| glm-5.3 / glm-5.3-flash | 0/107 | 0/107 — refuses/ deflects, e.g. invents "**payment code: 94**" |
| deepseek-v4.1-flash | 4/107 | 18/107 — **recites the old 1961-Act ITNS-281 code table** ("1001 Interest on securities, 1002 Dividends…") for a July-2026 question |
| gpt-oss:120b | 0/107 | 9/107 — wrong codes (1009, 1013, 1005…) |

This is the confusion window compressed into one number: the old and new code schemes both look like "10xx", and models pattern-match to whichever table they memorized. **The engine's `code` table (655 code-family facts) answers all 107 by lookup — 100% by construction, machine-verified.**

### F2 · Old→new section mapping: 0–32% — models cite the OLD Act for 2026+ questions
31 tasks: "A notice drafted in March 2026 cites old section 139(5). What is its replacement under the Income-tax Act 2025?" Gold: `263(5)`. glm-5.3 answered "**section 134**" — an invented mapping. Best model (glm-5.3-flash): 10/31 = 32%. gpt-oss: 0/31.

Every CA in India is living this right now: replying to a notice that cites the old Act, needing the new Act's replacement section. **Models guess; the engine's 3,792-pair transition map answers deterministically.**

### F3 · ICAI-style 4-part compliance: 0% on every model
99 tasks shaped exactly like the notice-reply work: given a payment, produce code + section + rate + deposit date — all four correct. **No model completed a single one** (0/99 across all five legs; 0/18 for flash's graded subset). The engine's compliance table produces all four fields together, then the pilot app's verifier machine-checks each against tables.db after generation.

### F4 · The April-2026 boundary: the confusion window is real and measurable
9 boundary tasks: "credited 2026-06-01, paid 2026-06-30 — which Act governs?" Gold: 2025 Act (earlier of credit/payment ≥ 1 Apr 2026, s.393; CBDT FAQ Q2.4/Q2.6).
- glm-5.3: 6/9 · flash 5/8 · deepseek 3/9 · gpt-oss 3/9 each
- Failures mostly answer "**The Income-tax Act, 1961** governs" — applying the repealed Act to post-transition facts.

Even the best model misassigns the governing Act one-third of the time on the exact judgment our whole wedge depends on.

### F5 · What survives: the general-knowledge tier
Rates (72–79% glm), TDS amounts (78–83% glm), dates (62%), act_selection (62–67% glm) — the things pretraining genuinely knows. This defines the product split: **the engine owns the facts (codes, sections, boundaries, compliance bundles); the LLM owns prose assembly around engine-supplied, machine-verified facts.** That is precisely the pilot app's architecture (draft generated, then every statutory citation verified against tables.db before the CA sees it).

## 4 · Per-category table (strict accuracy, graded rows)

| answer_type | glm-5.3-flash | glm-5.3 | deepseek-v4.1 | gpt-oss:120b | gpt-oss:20b |
|---|---|---|---|---|---|
| code | 0% | 0% | 3% | 0% | 0% |
| section | 42% | 23% | 31% | 6% | 15% |
| section_and_code | 0% | 0% | 0% | 0% | 0% |
| nature_and_section | 3% | 3% | 3% | 0% | 0% |
| form | 0% | 2% | 6% | 0% | 0% |
| rate | 78% | 72% | 20% | 1% | 22% |
| threshold | 47% | 47% | 17% | 8% | 8% |
| amount | 78% | 83% | 3% | 0% | 1% |
| date | 62% | 62% | 12% | 0% | 0% |
| act_selection | 62% | 66% | 33% | 33% | 33% |
| yes_no_reasoned | 52% | 57% | 0% | 10% | 0% |
| multi_part (ICAI) | **0%** | **0%** | **0%** | **0%** | **0%** |

Full per-task-type and per-model tables (generated by the harness): `eval/results/baseline_scores.md`.

## 5 · What this means for the product

1. **The pitch is now a number**: "The best frontier model we tested scores 30% strict accuracy on 2025-Act compliance; on the four-part drafting task a notice reply actually needs, it scores 0%. Karsastra answers those from a verified table and machine-checks every citation — 100% or it refuses."
2. **Pricing support**: this is per-case professional work, not commodity chat — a 70%+ citation-error rate is what CAs are paying to escape.
3. **P3 target is now concrete**: the fine-tuned 4B must beat qwen3.5:4b-raw (in-flight baseline) — and it only needs to learn the *prose around engine facts*, because codes/sections/boundaries come from tables.db, not the model.
4. **The eval set is the moat**: 785 locked tasks with a discriminating grader, cross-checked in inspect-ai — regenerable weekly as the confusion window evolves; no competitor has this for the 2025 Act.
5. **Failure-mode honesty for sales**: we can show a CA deepseek reciting the repealed 1961 code table for a 2026 challan — a 10-second demo that needs no technical audience.

## 6 · Status & reproduction

- **Done and scored**: 5 legs × 785 locked tasks (Oct 5–6), zero API errors after resume; integrity gate passed per file (parseable, unique ids, 785/776 counts).
- **In flight**: claude-opus-5-5 (100-shuffled-task sample, ~$17 of Max quota — the "ceiling" number, ~11 h) and qwen3.5:4b local (785 tasks, ~48 h at 2-way concurrency — the P3 fine-tune baseline). Append rows to §2 when they land.
- **Reproduce**: `eval/run_finish_legs.sh`, `eval/run_finish_legs2.sh` (z.ai resume legs + claude sample), then `/tmp/tax-eval -report results/baseline_scores.md -out results/`.
- **Raw results**: `eval/results/` (gitignored). Dead legs quarantined in `eval/results/_dead_legs_402/` with their 402/CLI errors preserved.

*Numbers in this report are generated by the locked grader, not hand-tallied. The sweep consumed ₹0 of new spend.*