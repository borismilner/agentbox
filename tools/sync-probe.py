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


def structured(res):
    """The tool's own result object, whatever the server called it.

    An MCP result carries both a text rendering and structured content, and the
    lock verbs answer in the structured half - so a probe that read only the text
    would be reading a summary of the thing it is checking.
    """
    r = res.get("result", {}) or {}
    if isinstance(r.get("structuredContent"), dict):
        return r["structuredContent"]
    for t in texts(res):
        try:
            got = json.loads(t)
        except (json.JSONDecodeError, TypeError):
            continue
        if isinstance(got, dict):
            return got
    return {}


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


def scenario_locks():
    """Slice 2's acceptance list, driven by two live mcp children.

    Every check here is a claim the design makes about locks that only two real
    sessions against a real daemon can settle: that the second one waits rather
    than proceeding, that a refusal says enough to act on, that a dead session
    does not free a resource its work still needs, and that a cycle is refused
    rather than suffered.
    """
    bad = []
    a = Session(agent="claude")
    b = Session(agent="codex")
    lock = "probe:deploy"
    other = "probe:repo"

    a.tool("announce", {"purpose": "sync-probe: holder", "activity": "about to deploy"})
    b.tool("announce", {"purpose": "sync-probe: waiter", "activity": "wants the deploy"})

    res = a.tool("acquire_lock", {"name": lock, "note": "make deploy", "timeout_s": 5})
    got = structured(res)
    show("A acquire", res)
    if not got.get("granted"):
        bad.append(f"the first acquire was not granted: {got}")

    # try_lock refuses at once, with everything needed to decide.
    res = b.tool("try_lock", {"name": lock})
    got = structured(res)
    show("B try_lock", res)
    if got.get("granted") or not got.get("refused"):
        bad.append(f"try_lock was granted a held lock: {got}")
    for want in ("holder", "holder_purpose", "holder_activity"):
        if not got.get(want):
            bad.append(f"the refusal does not carry {want}: {got}")

    # An unannounced session is refused with a teaching error.
    c = Session(agent="aider")
    res = c.tool("try_lock", {"name": "probe:unannounced"})
    if "announce" not in " ".join(texts(res)).lower():
        bad.append("an unannounced session was not taught to announce: " + " ".join(texts(res))[:200])
    c.close()

    # A timeout is a result, and it names the holder.
    res = b.tool("acquire_lock", {"name": lock, "timeout_s": 1}, timeout=30)
    got = structured(res)
    show("B acquire (times out)", res)
    if not got.get("timed_out"):
        bad.append(f"expected a timeout result: {got}")
    if not got.get("holder"):
        bad.append("the timeout does not name the holder")

    # The deadlock refusal: B holds `other`, waits on `lock`; A holds `lock` and
    # asks for `other`.
    b.tool("acquire_lock", {"name": other, "timeout_s": 5})
    waiter = threading.Thread(target=lambda: b.tool("acquire_lock", {"name": lock, "timeout_s": 8}, timeout=30))
    waiter.start()
    time.sleep(1.0)
    res = a.tool("acquire_lock", {"name": other, "timeout_s": 5}, timeout=30)
    got = structured(res)
    show("A acquire (would deadlock)", res)
    if not got.get("refused") or not got.get("deadlock"):
        bad.append(f"the cycle was not refused: {got}")
    elif not all(w in got["deadlock"] for w in (lock, other)):
        bad.append(f"the refusal does not name both locks: {got['deadlock']}")

    # Releasing hands it to the waiter that is still there.
    res = a.tool("release_lock", {"name": lock})
    show("A release", res)
    waiter.join(timeout=20)
    b.tool("release_lock", {"name": other})

    # A dead session does not free a lock whose process is still alive: B holds
    # it with this probe's own pid recorded, and this probe is not going anywhere.
    res = b.tool("acquire_lock", {"name": lock, "timeout_s": 5})
    if not structured(res).get("granted"):
        bad.append("B could not retake the lock for the orphan check")
    b.close()
    time.sleep(2.5)
    out = cli("sync", "locks", "--json")
    table = json.loads(out.stdout or "{}").get("locks") or []
    held = [l for l in table if l["name"] == lock]
    print("--- the table after B's child died")
    print("   ", json.dumps(table))
    if not held:
        bad.append("the dead session's lock was freed although its process is alive")
    elif not held[0].get("orphaned"):
        bad.append(f"the hold should read as orphaned: {held[0]}")

    # And a waiter is NOT granted while that pid lives.
    res = a.tool("acquire_lock", {"name": lock, "timeout_s": 2}, timeout=30)
    got = structured(res)
    show("A acquire (behind an orphan)", res)
    if got.get("granted"):
        bad.append("an orphaned lock was handed over while its process was still running")
    if not got.get("orphaned"):
        bad.append(f"the result does not say the hold is orphaned: {got}")

    # Losing a lock has to reach the agent that lost it, on whatever it calls
    # next: breaking reassigns the lock without stopping the ex-holder, so an
    # ex-holder that is never told is one that carries on touching the resource.
    # It has to be a lock A itself holds - the orphan above belongs to B, whose
    # child is dead and has nothing left to read a notice with.
    a.tool("acquire_lock", {"name": "probe:notice", "timeout_s": 3})
    cli("sync", "break", "probe:notice")
    time.sleep(0.5)
    res = a.tool("set_activity", {"activity": "carrying on, unaware"})
    show("A set_activity (after the human broke its lock)", res)
    notice = [t for t in texts(res) if "broke your lock" in t]
    if not notice:
        bad.append("the ex-holder was never told the human broke its lock: " + " ".join(texts(res))[:200])
    elif "NOT stopped" not in notice[0]:
        bad.append(f"the notice is not honest about what breaking does not do: {notice[0]}")
    a.close()
    print("--- leaving no state: every probe lock is released or dies with the probe")
    return bad


def scenario_board():
    """Put a holder, a waiter and an orphan on the live board, then wait.

    The surface is the half of this feature Boris actually looks at, and looking
    is how every defect in slice 1 was found. This is the fixture for that: real
    sessions, real locks, held long enough to open the app and read the screen.

        tools/sync-probe.py board        # holds for ~2 minutes, then cleans up
    """
    a = Session(agent="claude")
    b = Session(agent="codex")
    c = Session(agent="aider")
    a.tool("announce", {"purpose": "deploying the mirror fix",
                        "activity": "make deploy, waiting on the build"})
    b.tool("announce", {"purpose": "FR73: a card body readable after it closes",
                        "activity": "editing internal/webui/card.go"})
    c.tool("announce", {"purpose": "the nightly docs pass", "activity": "rewriting 04-ux.md"})

    a.tool("acquire_lock", {"name": "deploy:agentbox", "note": "make deploy", "timeout_s": 5})
    c.tool("acquire_lock", {"name": "repo:agentbox", "note": "a wide rename", "timeout_s": 5})
    # C dies holding it: its row goes, the hold does not, and the board has to
    # show a taken resource with nobody to hang it on.
    c.close()
    waiter = threading.Thread(
        target=lambda: b.tool("acquire_lock", {"name": "deploy:agentbox", "timeout_s": 150}, timeout=200))
    waiter.start()
    time.sleep(1.5)
    print("--- on the board now:")
    print(cli("sync", "agents").stdout.strip())
    print(cli("sync", "locks").stdout.strip())
    print("--- holding for 150s: open `agentbox app --tab agents` and look")
    time.sleep(150)
    a.tool("release_lock", {"name": "deploy:agentbox"})
    waiter.join(timeout=20)
    b.tool("release_lock", {"name": "deploy:agentbox"})
    a.close()
    b.close()
    return []


SCENARIOS = {"rider": scenario_rider, "locks": scenario_locks, "board": scenario_board}


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
