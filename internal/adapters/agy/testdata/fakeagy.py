#!/usr/bin/env python3
# Minimal stand-in for `agy -p= --input-format stream-json --output-format stream-json`.
import sys, json, time, os
if sys.argv[1:2] == ["models"]:
    print("Fetching available models..."); print("m-high\tModel (High)"); sys.exit(0)
args = sys.argv[1:]
conv = args[args.index("--conversation")+1] if "--conversation" in args else "conv-" + str(os.getpid())
log = os.environ.get("FAKEAGY_LOG")
if log:
    open(log, "a").write(" ".join(args) + "\n")
def out(o): sys.stdout.write(json.dumps(o)+"\n"); sys.stdout.flush()
out({"event": "init", "conversation_id": conv, "init": {"tools": []}})
for l in sys.stdin:
    m = json.loads(l)
    if m.get("event") != "user": continue
    text = m["message"]["content"]
    if "[slow]" in text: time.sleep(30)
    if "[quota]" in text:
        out({"event": "result", "result": {"conversation_id": conv, "status": "ERROR", "error": "Individual quota reached. RESOURCE_EXHAUSTED (429)"}}); continue
    if "[nodelta]" in text:
        out({"event": "result", "result": {"conversation_id": conv, "status": "OK", "response": "final only", "usage": {"input_tokens": 1, "output_tokens": 2}}}); continue
    out({"event": "step_update", "step_update": {"conversation_id": conv, "step_index": 1, "state": "RUNNING", "text_delta": "hello "}})
    out({"event": "result", "result": {"conversation_id": conv, "status": "OK", "response": "hello", "usage": {"input_tokens": 3, "output_tokens": 4}}})
