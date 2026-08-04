#!/usr/bin/env python3
"""A throwaway MCP server whose only job is to park a tool call.

FR83 needs one number the daemon cannot tell us: how long the MCP *client*
lets a tool call sit before it gives up. That is a fact about Claude Code, not
about agentbox, so it is measured with the real client and a server that does
nothing else - a parked `ask_user` would put a card on Boris's desktop and
answer a different question.

    tools/idlecap-probe.sh          # runs both cases against `claude -p`

Two tools, one difference: `park_quiet` sleeps in silence, `park_progress`
sleeps while sending notifications/progress on a ticker. The log is the
measurement - every inbound line with the seconds since the call arrived, so a
notifications/cancelled from the client dates the cap exactly, and the raw
tools/call shows whether the client sent a _meta.progressToken at all (without
one the spec forbids the progress notification the design leans on).

AGENTBOX_IDLECAP_LOG picks the log file; default /tmp/idlecap.log.
"""
import json
import os
import sys
import threading
import time

LOG = os.environ.get("AGENTBOX_IDLECAP_LOG", "/tmp/idlecap.log")
START = time.monotonic()
call_at = None
lock = threading.Lock()


def log(what):
    since = "" if call_at is None else f" +{time.monotonic() - call_at:7.1f}s"
    with open(LOG, "a") as f:
        f.write(f"[{time.monotonic() - START:8.1f}s{since}] {what}\n")
        f.flush()


def send(msg):
    with lock:
        sys.stdout.write(json.dumps(msg) + "\n")
        sys.stdout.flush()


def reply(rid, result):
    send({"jsonrpc": "2.0", "id": rid, "result": result})


TOOLS = [
    {"name": "park_quiet",
     "description": "Park for `seconds` in complete silence, then return. Used to measure how long this client tolerates a silent tool call.",
     "inputSchema": {"type": "object", "properties": {"seconds": {"type": "integer"}},
                     "required": ["seconds"]}},
    {"name": "park_progress",
     "description": "Park for `seconds`, sending a progress notification every `every` seconds, then return.",
     "inputSchema": {"type": "object",
                     "properties": {"seconds": {"type": "integer"}, "every": {"type": "integer"}},
                     "required": ["seconds"]}},
]


def park(rid, name, args, token):
    global call_at
    seconds = int(args.get("seconds", 60))
    every = int(args.get("every", 120)) or 120
    waited = 0
    while waited < seconds:
        step = min(every, seconds - waited) if name == "park_progress" else seconds - waited
        time.sleep(step)
        waited += step
        if name == "park_progress" and waited < seconds:
            if token is None:
                log("WOULD TICK but the client sent no progressToken")
            else:
                send({"jsonrpc": "2.0", "method": "notifications/progress",
                      "params": {"progressToken": token, "progress": waited,
                                 "total": seconds, "message": f"parked {waited}s"}})
                log(f"sent progress {waited}/{seconds}")
    log(f"returning after {waited}s")
    reply(rid, {"content": [{"type": "text", "text": f"parked {waited}s and returned"}]})


def main():
    global call_at
    log(f"server up, pid {os.getpid()}")
    for line in sys.stdin:
        line = line.strip()
        if not line:
            continue
        try:
            msg = json.loads(line)
        except json.JSONDecodeError:
            log(f"unparsable: {line[:200]}")
            continue
        method, rid = msg.get("method"), msg.get("id")
        if method != "notifications/progress":
            log(f"<- {line[:400]}")
        if method == "initialize":
            reply(rid, {"protocolVersion": "2025-06-18",
                        "capabilities": {"tools": {}},
                        "serverInfo": {"name": "idlecap", "version": "0"}})
        elif method == "tools/list":
            reply(rid, {"tools": TOOLS})
        elif method == "tools/call":
            p = msg.get("params", {})
            token = (p.get("_meta") or {}).get("progressToken")
            log(f"progressToken: {token!r}")
            if call_at is None:
                call_at = time.monotonic()
            threading.Thread(target=park, args=(rid, p.get("name"), p.get("arguments") or {}, token),
                             daemon=True).start()
        elif rid is not None:
            reply(rid, {})
    log("stdin closed, exiting")


if __name__ == "__main__":
    main()
