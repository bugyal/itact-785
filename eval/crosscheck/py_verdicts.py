"""Emit per-record grader verdicts from the Python port, keyed file:lineno —
the mirror of parity_go/main.go output.

Usage:
    python3 eval/crosscheck/py_verdicts.py <archive_dir> <out_jsonl>
"""

import json
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from grader import Task, grade  # noqa: E402


def main():
    archive, out_path = sys.argv[1], sys.argv[2]
    files = sorted(f for f in os.listdir(archive) if f.endswith(".jsonl"))
    n = skip = unparse = 0
    with open(out_path, "w") as w:
        for base in files:
            with open(os.path.join(archive, base)) as f:
                for ln, line in enumerate(f, 1):
                    line = line.strip()
                    if not line:
                        continue
                    key = "%s:%d" % (base, ln)
                    try:
                        rec = json.loads(line)
                    except json.JSONDecodeError:
                        unparse += 1
                        w.write(json.dumps({"key": key, "unparseable": True}) + "\n")
                        continue
                    if rec.get("error"):
                        skip += 1
                        w.write(json.dumps({"key": key, "id": rec.get("id"), "skipped": True}) + "\n")
                        continue
                    t = Task(
                        task_type=rec.get("task_type", ""),
                        table="",
                        fact_key=rec.get("fact_key", ""),
                        question=rec.get("question", ""),
                        answer=rec.get("gold", ""),
                        answer_type=rec.get("answer_type", ""),
                        template=rec.get("template", ""),
                    )
                    g = grade(t, rec.get("raw_response") or "")
                    n += 1
                    w.write(json.dumps({
                        "key": key,
                        "id": rec.get("id"),
                        "graded": g.graded,
                        "pass": g.passed,
                        "score_parts": g.score_parts,
                    }) + "\n")
    print("py verdicts: %d graded records, %d api-error skipped, %d unparseable -> %s"
          % (n, skip, unparse, out_path))


if __name__ == "__main__":
    main()
