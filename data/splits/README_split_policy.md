# Split + decontamination policy (P1 data)

## Goal

Produce train/val/test splits for the mapping fine-tune such that:
1. The fine-tune can LEARN every mapped fact (the product claim is "knows the new-Act
   mapping"), and
2. The locked test set measures phrasing-level generalisation (can the model answer a
   re-worded question about a fact it has seen phrased differently?), which is what a
   CA-facing product actually needs.
3. No verbatim recycling: identical questions never cross splits.

## Why fact-stratified (not fact-held-out)

Holding out entire FACTS (e.g. all code-1023 pairs) to test would measure only whether
the model memorises specific codes it happened to be trained on — a fine-tune that
never saw code 1023 cannot be expected to answer it, and a base-model comparison would
then measure memorisation capacity, not mapping knowledge. Since the deliverable is
"the model knows the mapping", every fact must be in train, and evaluation must be
within-fact paraphrase generalisation. This is stated explicitly so P2/P3 users of the
test set do not over-claim what a good score means.

## Mechanics

- Split is stratified WITHIN fact_key: each fact's pairs are shuffled (seed 42) and cut
  60/20/20. Facts with <3 pairs land in train (+val where volume allows).
- Generation removed exact-duplicate questions before split; post-split the report
  hard-checks verbatim question overlap across splits (must be 0 — verified 0/0/0).
- 8-gram decontamination (8-gram = 8 normalised tokens) is run val-vs-train and
  test-vs-train and REPORTED per template family. In a templated dataset every question
  of a family shares template prose with train ("Under the Income-tax Act 2025, ...
  Which payment code must be quoted on the TDS challan?") so 8-gram hits are expected
  and are NOT evidence of answer leakage; the check exists to catch accidental reuse of
  full question strings (which the verbatim-duplicate hard check guarantees absent).
- test.locked.json stores sha256 of the sorted test questions. Re-running the pipeline
  with new data/seeds will not silently swap the test set — the script refuses to
  overwrite the lock and writes test.locked.new.json with a warning.

## P2 usage

- Baseline sweep runs the LOCKED test set (data/splits/test.jsonl) at temp 0.
- Exact-match grading keys off answer_type:
  code/section/form/rate/threshold/date/amount/days -> normalised string containment or
  equality (grader lives in P2 harness; answer_type field makes it mechanical).
- multi_part (icai_style), reasoned (notice_style), yes_no_reasoned need part-wise or
  rubric grading — start with part-wise token checks ((i)-(iv) labels are stable).