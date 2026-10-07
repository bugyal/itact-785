from inspect_ai.log import read_eval_log

import sys

log = read_eval_log(sys.argv[1])
print("model:", log.eval.model)
print("dataset:", log.eval.dataset)
print("n_samples:", len(log.samples))
if log.results and log.results.scores:
    sc = log.results.scores[0]
    print("scorer:", sc.name, "metrics:", {k: v.value for k, v in sc.metrics.items()})
s = log.samples[0]
ss = s.scores.get("tax_entity_check")
print("sample id:", s.id)
print("value:", ss.value, "| answer:", (ss.answer or "")[:80])
print("metadata:", {k: (v if not isinstance(v, list) else v[:2]) for k, v in (ss.metadata or {}).items()})
print("model events:", len(s.model_events))
me = s.model_events[0]
print("gen config temp:", me.call.request.temperature if me.call.request else None)
print("request input:", [m.role for m in me.call.request.input] if me.call.request else None)
print("request content head:", (me.call.request.input[0].content[:60] if me.call.request else None))
