#!/usr/bin/env python3
# Minimal stand-in for `claude -p --input-format stream-json ...` used by adapter tests.
import sys, json
args = sys.argv[1:]
sid = args[args.index("--session-id")+1] if "--session-id" in args else (args[args.index("--resume")+1] if "--resume" in args else "none")
def out(o): sys.stdout.write(json.dumps(o)+"\n"); sys.stdout.flush()
out({"type":"system","subtype":"init","session_id":sid})
for line in sys.stdin:
    m = json.loads(line)
    if m.get("type") == "user":
        text = m["message"]["content"]
        if "NOTLOGGED" in text:
            out({"type":"result","subtype":"success","is_error":True,"result":"Not logged in · Please run /login"}); continue
        out({"type":"stream_event","event":{"type":"content_block_delta","delta":{"type":"text_delta","text":"hello "}}})
        if "TOOL" in text:
            out({"type":"control_request","request_id":"req1","request":{"subtype":"can_use_tool","tool_name":"Bash","input":{"command":"ls"},"tool_use_id":"tu1"}})
            resp = json.loads(sys.stdin.readline())
            r = resp["response"]["response"]
            out({"type":"user","message":{"role":"user","content":[{"type":"tool_result","tool_use_id":"tu1","content":("ran" if r["behavior"]=="allow" else "denied: "+r.get("message",""))}]}})
        out({"type":"assistant","message":{"content":[{"type":"text","text":"hello world"}]}})
        out({"type":"result","subtype":"success","is_error":False,"result":"hello world","usage":{"input_tokens":3,"output_tokens":4}})
    elif m.get("type") == "control_request":
        if m["request"].get("subtype") == "initialize":
            out({"type":"control_response","response":{"subtype":"success","request_id":m["request_id"],"response":{"commands":[{"name":"compact","description":"Compact context"},{"name":"review","description":"Review","argumentHint":"<file>"}]}}})
        else:
            out({"type":"control_response","response":{"subtype":"success","request_id":m["request_id"]}})
