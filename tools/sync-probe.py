#!/usr/bin/env python3
"""Two or more live MCP sessions at once, against the deployed daemon.

`mcp-probe.py` proves one tool call: it spawns a server, calls, and exits, so
every invocation is a new session with a new key. Sync (FR83) cannot be tested
that way at all - presence IS the connection staying open, and every question
worth asking is about what session A is told when session B does something. So
this keeps several children alive and speaks to each of them in turn.

    tools/sync-probe.py rider     # the discovery rider, end to end

Add a scenario as a function and a line in SCENARIOS. What it gives you:

    a = Session(agent="claude")        # a live child, handshaken
    a.tool("announce", {"purpose": "..."})
    texts(res)                         # every text block on a result, in order
    riders(res)                         # just the `sync:` lines

The last one is the point for sync work: a rider is not in the structured result,
it is an extra text block the child appends (internal/mcp/rider.go), so a probe
that only reads structured output sees nothing and concludes wrongly.

AGENTBOX_BIN picks the binary, so a working copy can be exercised against the
deployed daemon without deploying: `go build -o agentbox ./cmd/agentbox` first,
because `go build ./...` does NOT rewrite that file and you will otherwise probe
the last binary you happened to build.
"""
import json
import os
import queue
import subprocess
import sys
import threading
import time

REPO = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
BIN = os.environ.get("AGENTBOX_BIN", "agentbox")


class Session:
    """One live `agentbox mcp` child: its own session key, kept open."""

    def __init__(self, agent=None, cwd=REPO, env_extra=None):
        env = dict(os.environ)
        # The child mints its own key. An inherited one would make two sessions
        # share a row, which is the one thing this is here to tell apart.
        env.pop("AGENTBOX_SESSION_KEY", None)
        if agent:
            env["AGENTBOX_AGENT"] = agent
        env.update(env_extra or {})
        self.p = subprocess.Popen(
            [BIN, "mcp"], cwd=cwd, env=env, text=True, bufsize=1,
            stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
        self.q = queue.Queue()
        self.next_id = 0
        threading.Thread(target=self._read, daemon=True).start()
        self._handshake()

    def _read(self):
        for line in self.p.stdout:
            line = line.strip()
            if line:
                try:
                    self.q.put(json.loads(line))
                except json.JSONDecodeError:
                    pass

    def _send(self, method, params=None, notify=False, timeout=30):
        msg = {"jsonrpc": "2.0", "method": method}
        if params is not None:
            msg["params"] = params
        if notify:
            self.p.stdin.write(json.dumps(msg) + "\n")
            self.p.stdin.flush()
            return None
        self.next_id += 1
        want = self.next_id
        msg["id"] = want
        self.p.stdin.write(json.dumps(msg) + "\n")
        self.p.stdin.flush()
        while True:
            got = self.q.get(timeout=timeout)
            if got.get("id") == want:
                return got

    def _handshake(self):
        self._send("initialize", {
            "protocolVersion": "2025-06-18", "capabilities": {},
            "clientInfo": {"name": "sync-probe", "version": "0"}})
        # The SDK waits for this before serving anything.
        self._send("notifications/initialized", {}, notify=True)

    def tool(self, name, args=None, timeout=30):
        return self._send("tools/call", {"name": name, "arguments": args or {}},
                          timeout=timeout)

    def close(self):
        try:
            self.p.stdin.close()
        except Exception:
            pass
        self.p.terminate()


def texts(res):
    out = []
    for c in (res.get("result", {}) or {}).get("content", []) or []:
        if c.get("type") == "text":
            out.append(c.get("text", ""))
    return out


def riders(res):
    return [t for t in texts(res) if t.startswith("sync:")]


def cli(*args, key=None):
    env = dict(os.environ)
    if key:
        env["AGENTBOX_SESSION_KEY"] = key
    return subprocess.run([BIN, *args], cwd=REPO, env=env,
                          capture_output=True, text=True)


def show(label, res):
    print(f"--- {label}")
    for t in texts(res):
        print("   ", t[:300].replace("\n", " "))


def scenario_rider():
    """The discovery rider: an agent that is not asking must still be told."""
    bad = []
    a = Session(agent="claude")
    res = a.tool("announce", {"purpose": "sync-probe: session A",
                              "activity": "reading internal/daemon/sync.go"})
    show("A announce", res)
    if riders(res):
        bad.append("announce carried a rider as well as the roster it returns")

    res = a.tool("set_activity", {"activity": "editing internal/daemon/sync.go"})
    show("A set_activity (nothing new)", res)
    if riders(res):
        bad.append("a rider arrived with no change in the area")

    b = Session(agent="codex")
    show("B announce", b.tool("announce", {"purpose": "sync-probe: session B",
                                           "activity": "editing the inbox"}))
    time.sleep(0.5)

    res = a.tool("set_activity", {"activity": "still editing sync.go"})
    show("A set_activity (after B joined)", res)
    got = riders(res)
    if not got:
        bad.append("no rider: A was never told B joined")
    else:
        for want in ("codex", "session B", "repo:agentbox", "Coordinate"):
            if want not in got[0]:
                bad.append(f"the rider does not mention {want!r}")

    res = a.tool("set_activity", {"activity": "writing a test"})
    show("A set_activity (again)", res)
    if riders(res):
        bad.append("the same arrival was reported twice")

    # A hook's shell call must not eat the news: it has nowhere to show it.
    b.close()
    time.sleep(1.2)
    key = None
    out = cli("sync", "agents", "--json")
    for row in json.loads(out.stdout or "{}").get("agents", []):
        if row.get("purpose") == "sync-probe: session A":
            key = row["key"]
    if not key:
        bad.append("could not find A's row to act on behalf of")
    else:
        cli("sync", "activity", "a hook fired here", key=key)
        res = a.tool("set_activity", {"activity": "after the hook"})
        show("A set_activity (hook fired in between)", res)
        got = riders(res)
        if not got:
            bad.append("the hook's CLI call ate the departure news")
        elif "left" not in got[0]:
            bad.append(f"expected a departure, got: {got[0]}")

    a.close()
    return bad


SCENARIOS = {"rider": scenario_rider}


def main():
    name = sys.argv[1] if len(sys.argv) > 1 else ""
    if name not in SCENARIOS:
        print(__doc__, file=sys.stderr)
        print("scenarios: " + ", ".join(sorted(SCENARIOS)), file=sys.stderr)
        return 2
    bad = SCENARIOS[name]()
    print()
    for b in bad:
        print("!!", b)
    print("RESULT:", "PASS" if not bad else "FAIL")
    return 0 if not bad else 1


if __name__ == "__main__":
    sys.exit(main())
