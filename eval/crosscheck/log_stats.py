import sys
from inspect_ai.log import read_eval_log
log = read_eval_log(sys.argv[1])
n = len(log.samples)
graded = 0
errs = 0
for s in log.samples:
    ss = s.scores.get("tax_entity_check")
    if ss and ss.metadata and ss.metadata.get("graded"):
        graded += 1
    if s.error is not None:
        errs += 1
print("model:", log.eval.model)
print("n:", n, "graded:", graded, "errors:", errs)
if log.results and log.results.scores:
    sc = log.results.scores[0]
    print("accuracy:", sc.metrics["accuracy"].value, "stderr:", sc.metrics["stderr"].value)
