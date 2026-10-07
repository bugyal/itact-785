from inspect_ai.log import read_eval_log

import glob

for p in sorted(glob.glob("logs-probe/*.eval")):
    log = read_eval_log(p)
    model = log.eval.model
    s = log.samples[0]
    ss = s.scores.get("tax_entity_check")
    print("%-45s value=%s  err=%s" % (model, ss.value if ss else "?", s.error))
