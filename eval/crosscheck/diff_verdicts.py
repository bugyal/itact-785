"""Diff Go-vs-Python grader verdicts over the archived sweep records.

Run after:
    (cd eval/crosscheck/parity_go && ./parity_go -archive ../../results/_prefix_run1_archive -out verdicts_go.jsonl)
    python3 eval/crosscheck/py_verdicts.py eval/results/_prefix_run1_archive eval/crosscheck/verdicts_py.jsonl
    python3 eval/crosscheck/diff_verdicts.py eval/crosscheck/verdicts_go.jsonl eval/crosscheck/verdicts_py.jsonl

Exit 0 iff zero real verdict mismatches (graded/pass/score_parts), proving the
Python port reproduces the CURRENT Go grader record-for-record.
"""

import json
import sys


def load(path):
    m = {}
    with open(path) as f:
        for line in f:
            line = line.strip()
            if line:
                v = json.loads(line)
                m[v["key"]] = v
    return m


def main():
    go = load(sys.argv[1])
    py = load(sys.argv[2])
    keys = sorted(set(go) | set(py))
    if len(go) != len(py):
        print("NOTE: verdict counts differ: go=%d py=%d" % (len(go), len(py)))
    diffs = []
    n_skip = n_unparse = 0
    for k in keys:
        g = go.get(k)
        p = py.get(k)
        if g is None or p is None:
            diffs.append((k, "verdict missing on one side"))
            continue
        if g.get("unparseable") or p.get("unparseable"):
            n_unparse += 1
            continue
        if bool(g.get("skipped")) != bool(p.get("skipped")):
            diffs.append((k, "skip-flag mismatch go=%s py=%s" % (g.get("skipped"), p.get("skipped"))))
            continue
        if g.get("skipped"):
            n_skip += 1
            continue
        if g.get("graded") != p.get("graded"):
            diffs.append((k, "graded go=%s py=%s (id=%s)" % (g.get("graded"), p.get("graded"), g.get("id"))))
            continue
        if not g.get("graded"):
            continue
        if g.get("pass") != p.get("pass"):
            diffs.append((k, "pass go=%s py=%s (id=%s)" % (g.get("pass"), p.get("pass"), g.get("id"))))
        elif abs(g.get("score_parts", 0) - p.get("score_parts", 0)) > 1e-9:
            diffs.append((k, "score_parts go=%s py=%s (id=%s)"
                          % (g.get("score_parts"), p.get("score_parts"), g.get("id"))))
    print("compared %d keys: %d skipped(api-err), %d unparseable, %d DIFFS"
          % (len(keys), n_skip, n_unparse, len(diffs)))
    for k, msg in diffs[:20]:
        print("  ", k, msg)
    sys.exit(0 if not diffs else 1)


if __name__ == "__main__":
    main()
