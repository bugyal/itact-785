from inspect_ai.log import read_eval_log

import glob
import json

logs = sorted(glob.glob("logs-probe/*.eval"))
log = read_eval_log(logs[-1])
s = log.samples[0]
print("value:", s.scores["tax_entity_check"].value)
for e in s.events:
    if e.event == "model":
        call = e.call
        r = json.loads(json.dumps(call.response, default=str))
        content = r.get("content", [])
        print("blocks:", [b.get("type") for b in content])
        for b in content:
            if b.get("type") == "text":
                print("TEXT:", repr(b.get("text", ""))[:250])
        print("stop:", r.get("stop_reason"), "| out tokens:", r.get("usage", {}).get("output_tokens"))
        # request side: did thinking get disabled?
        req = call.request
        req = json.loads(json.dumps(req, default=str))
        print("request has thinking param:", "thinking" in req, req.get("thinking"))
        break
