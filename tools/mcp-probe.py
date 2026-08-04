#!/usr/bin/env python3
"""Call an AgentBox MCP tool over a fresh stdio server, from the shell.

Why this exists: an MCP host reads the tool list once, at handshake. A Claude
session running when you add a tool cannot see it - `mcp__agentbox__*` stays at
whatever `agentbox mcp` advertised when the session started - so a new tool cannot be
exercised from the session that just wrote it without a restart. This spawns its
own server against the deployed daemon, which is the only way to prove a new
tool end to end in the session that added it.

Two things it gets right that a here-doc into `agentbox mcp` does not: it keeps stdin
open until the answer arrives (closing it makes the server exit before writing),
and it sends the `notifications/initialized` the SDK waits for.

    tools/mcp-probe.py list                       # every tool, with its schema keys
    tools/mcp-probe.py list request_control       # just these
    tools/mcp-probe.py call notify_user '{"title":"probe"}'
    tools/mcp-probe.py call request_control '{"reason":"...","window_s":30}' 150

The last argument to `call` is the seconds to wait, since a blocking tool waits
for a human. Identity: the daemon sees `agent` as this script's own process name
(the MCP server stamps its parent), which is what `held_by` will show.

Each invocation is a whole new session, which makes this the wrong tool for
anything about two agents seeing each other: use `tools/sync-probe.py`, which
keeps several children open at once and reads the text blocks a structured-output
reader misses.
"""
import json
import subprocess
import sys
import threading

REPLY = 2  # the id we wait for; 1 is the handshake


def main():
    if len(sys.argv) < 2 or sys.argv[1] not in ("list", "call"):
        print(__doc__, file=sys.stderr)
        return 2
    mode = sys.argv[1]
    proc = subprocess.Popen(
        ["agentbox", "mcp"], stdin=subprocess.PIPE, stdout=subprocess.PIPE,
        stderr=subprocess.PIPE, text=True, bufsize=1)
    answers, done = {}, threading.Event()

    def send(msg):
        proc.stdin.write(json.dumps(msg) + "\n")
        proc.stdin.flush()

    def read():
        for line in proc.stdout:
            try:
                msg = json.loads(line)
            except ValueError:
                continue  # a log line, not a frame
            if "id" in msg:
                answers[msg["id"]] = msg
                if msg["id"] == REPLY:
                    done.set()

    threading.Thread(target=read, daemon=True).start()
    send({"jsonrpc": "2.0", "id": 1, "method": "initialize",
          "params": {"protocolVersion": "2025-06-18", "capabilities": {},
                     "clientInfo": {"name": "mcp-probe", "version": "1"}}})
    send({"jsonrpc": "2.0", "method": "notifications/initialized"})

    if mode == "list":
        send({"jsonrpc": "2.0", "id": REPLY, "method": "tools/list", "params": {}})
        wait = 15.0
    else:
        if len(sys.argv) < 4:
            print("call wants a tool name and a JSON object of arguments", file=sys.stderr)
            return 2
        wait = float(sys.argv[4]) if len(sys.argv) > 4 else 120.0
        send({"jsonrpc": "2.0", "id": REPLY, "method": "tools/call",
              "params": {"name": sys.argv[2], "arguments": json.loads(sys.argv[3])}})

    heard = done.wait(wait)
    proc.stdin.close()
    proc.terminate()
    if not heard:
        print("no answer in %ss; stderr: %s" % (wait, proc.stderr.read()[:400]), file=sys.stderr)
        return 3
    res = answers[REPLY]
    if mode == "list":
        tools = {t["name"]: t for t in res["result"]["tools"]}
        print("tools:", len(tools))
        for name in sys.argv[2:] or sorted(tools):
            t = tools.get(name)
            print("  %-22s %s" % (name, "MISSING" if not t
                                  else sorted(t["inputSchema"].get("properties", {})) or "no args"))
    else:
        print(json.dumps(res.get("result", res), indent=2)[:2000])
    return 0


sys.exit(main())
