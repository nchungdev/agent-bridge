#!/usr/bin/env python3
# Minimal stand-in for `codex app-server` (JSON-RPC over stdio) for adapter tests.
import sys, json
def out(o): sys.stdout.write(json.dumps(o)+"\n"); sys.stdout.flush()
if sys.argv[1:2] == ["exec"]:
    import os
    log = os.environ.get("FAKECODEX_LOG")
    if log: open(log, "a").write(" ".join(sys.argv[2:-1]) + "\n")
    print(json.dumps({"type": "item.completed", "item": {"type": "agent_message", "text": "codex-summary"}}))
    print(json.dumps({"type": "turn.completed", "usage": {"input_tokens": 9, "output_tokens": 4}})); sys.exit(0)
if sys.argv[1:2] == ["login"]:
    print("Not logged in"); sys.exit(1)
thread = "thr-1"
n = 0
def read():
    return json.loads(sys.stdin.readline())
for line in sys.stdin:
    m = json.loads(line)
    meth = m.get("method"); mid = m.get("id")
    if meth == "initialize":
        out({"id": mid, "result": {"userAgent": "fake"}})
    elif meth in ("thread/start", "thread/resume"):
        tid = m["params"].get("threadId", thread)
        out({"id": mid, "result": {"thread": {"id": tid}}})
    elif meth == "skills/list":
        out({"id": mid, "result": {"data": [{"cwd": "/", "skills": [{"name": "demo-skill", "description": "long text", "shortDescription": "short", "enabled": True}, {"name": "off", "description": "x", "enabled": False}], "errors": []}]}})
    elif meth == "model/list":
        out({"id": mid, "result": {"data": [{"id": "m1", "model": "fake-1", "displayName": "Fake One", "hidden": False}, {"id": "m2", "model": "fake-mini", "displayName": "Fake Mini", "hidden": False}, {"id": "h", "model": "hid", "hidden": True}]}})
    elif meth == "turn/start":
        text = m["params"]["input"][0]["text"]
        out({"id": mid, "result": {"turn": {"id": "turn-1"}}})
        out({"method": "turn/started", "params": {"turn": {"id": "turn-1"}}})
        out({"method": "item/agentMessage/delta", "params": {"delta": "hi "}})
        if "CMD" in text:
            out({"id": 900, "method": "item/commandExecution/requestApproval", "params": {"command": "ls /", "cwd": "/tmp", "reason": "list", "itemId": "i1"}})
            resp = json.loads(sys.stdin.readline())
            ok = resp["result"]["decision"]
            out({"method": "item/started", "params": {"item": {"type": "commandExecution", "id": "i1", "command": "ls /", "cwd": "/tmp"}}})
            out({"method": "item/completed", "params": {"item": {"type": "commandExecution", "id": "i1", "aggregatedOutput": "decision=" + ok}}})
        if "FILE" in text:
            out({"id": 901, "method": "item/fileChange/requestApproval", "params": {"itemId": "i2", "reason": "write"}})
            resp = json.loads(sys.stdin.readline())
            out({"method": "item/completed", "params": {"item": {"type": "fileChange", "id": "i2", "status": resp["result"]["decision"], "changes": [{"path": "a.txt", "diff": "+x", "kind": {}}]}}})
        out({"method": "thread/tokenUsage/updated", "params": {"tokenUsage": {"last": {"inputTokens": 5, "outputTokens": 6}}}})
        if "FAIL" in text:
            out({"method": "turn/completed", "params": {"turn": {"status": "failed", "error": {"message": "unexpected status 401 Unauthorized"}}}})
        else:
            out({"method": "turn/completed", "params": {"turn": {"status": "completed"}}})
    elif meth == "turn/interrupt":
        out({"id": mid, "result": {}})
