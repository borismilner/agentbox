#!/usr/bin/env python3
"""Two or more live MCP sessions at once, against the deployed daemon.

`mcp-probe.py` proves one tool call: it spawns a server, calls, and exits, so
every invocation is a new session with a new key. Sync (FR83) cannot be tested
that way at all - presence IS the connection staying open, and every question
worth asking is about what session A is told when session B does something. So
this keeps several children alive and speaks to each of them in turn.

    tools/sync-probe.py rider     # the discovery rider, end to end
    tools/sync-probe.py locks     # slice 2's acceptance list
    tools/sync-probe.py signals   # slice 3's acceptance list
    tools/sync-probe.py shared    # slice 4's acceptance list (RESTARTS the daemon)
    tools/sync-probe.py board     # a fixture to look at the surface with

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


def pending_ids():
    """The ids of everything currently waiting for the human."""
    out = cli("pending", "--json")
    if out.returncode != 0:
        return set()
    try:
        got = json.loads(out.stdout or "{}")
    except json.JSONDecodeError:
        return set()
    return {it["id"] for it in (got.get("pending") or []) if it.get("id")}


class OwnToasts:
    """Dismiss the toasts THIS probe caused, and nothing else.

    A refused deadlock warns the human by design - that is one of the two
    coordination events the design says earns an interruption - so an acceptance run
    that constructs a lock cycle on purpose leaves a warning on his screen. A warning
    is pending until it is CLICKED and pending items survive a daemon restart, so
    before FR89 every run left one there forever and they came back after every
    deploy. Boris asked about the same four twice.

    The diff is the whole point: `dismiss --all` would also clear a real item of his
    that happened to be waiting, and the warning is posted by agentbox rather than by
    the probe's sessions, so an ownership-scoped retract cannot reach it either.
    """

    def __enter__(self):
        self.before = pending_ids()
        return self

    def __exit__(self, *_):
        mine = pending_ids() - self.before
        if not mine:
            return False
        for item in sorted(mine):
            cli("dismiss", item)
        print(f"--- dismissed {len(mine)} toast(s) this run caused: {', '.join(sorted(mine))}")
        return False


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
    """Put a holder, a waiter, a listener and an orphan on the live board, then wait.

    The surface is the half of this feature Boris actually looks at, and looking
    is how every defect in slice 1 was found. This is the fixture for that: real
    sessions, real locks, real parked waits, held long enough to open the app and
    read the screen.

        tools/sync-probe.py board        # holds for ~2 minutes, then cleans up
    """
    a = Session(agent="claude")
    b = Session(agent="codex")
    c = Session(agent="aider")
    d = Session(agent="claude")
    a.tool("announce", {"purpose": "deploying the mirror fix",
                        "activity": "make deploy, waiting on the build"})
    b.tool("announce", {"purpose": "FR73: a card body readable after it closes",
                        "activity": "editing internal/webui/card.go"})
    c.tool("announce", {"purpose": "the nightly docs pass", "activity": "rewriting 04-ux.md"})
    d.tool("announce", {"purpose": "the release gate: deploy when the suite is green",
                        "activity": "waiting for the suite"})
    # A parked await, so the board has a listening row beside the blocked one. The
    # two look similar and mean opposite things - blocked is contention, listening
    # is the feature working - so they belong on screen together.
    listener = threading.Thread(
        target=lambda: d.tool("await_signal",
                              {"topics": ["tests:green", "done:*"], "timeout_s": 160},
                              timeout=200))
    listener.start()

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
    # Wake the listener rather than leaving it to time out, so the fixture ends
    # with the board empty instead of with one row still parked.
    a.tool("post_signal", {"topic": "tests:green", "data": {"fixture": True}})
    listener.join(timeout=30)
    a.close()
    b.close()
    d.close()
    return []


def scenario_signals():
    """Slice 3's acceptance list, driven by live mcp children.

    Every claim here is one the design makes about signals that only real
    sessions against a real daemon can settle: that a parked call actually wakes,
    that two waiters on one topic both do, that a signal fired while nobody
    listened is still there afterwards, that a trimmed cursor is confessed rather
    than served, and that a request and its reply are two ordinary posts.
    """
    bad = []
    a = Session(agent="claude")
    b = Session(agent="codex")
    a.tool("announce", {"purpose": "sync-probe: the poster", "activity": "running the suite"})
    b.tool("announce", {"purpose": "sync-probe: the listener", "activity": "waiting for green"})

    # A signal posted with nobody parked is stored, not lost. This is the whole
    # durability claim, and the note has to say so rather than reading as failure.
    res = a.tool("post_signal", {"topic": "probe:tests", "data": {"suite": "race"}})
    first = structured(res)
    show("A post (nobody listening)", res)
    if not first.get("seq"):
        bad.append(f"a post did not come back with a sequence number: {first}")
    if first.get("delivered"):
        bad.append(f"nobody was parked, yet delivered={first.get('delivered')}")
    if "not a failure" not in (first.get("note") or ""):
        bad.append(f"the note does not say a stored signal is still delivered: {first.get('note')}")

    # And it is picked up afterwards by cursor, which is what makes the handoff
    # survive the two halves not overlapping.
    res = b.tool("await_signal", {"topics": ["probe:tests"], "after_seq": first["seq"] - 1,
                                  "timeout_s": 5}, timeout=30)
    got = structured(res)
    show("B await (catching up by cursor)", res)
    sigs = got.get("signals") or []
    if len(sigs) != 1 or sigs[0].get("topic") != "probe:tests":
        bad.append(f"the stored signal was not picked up by cursor: {got}")
    elif (sigs[0].get("data") or {}).get("suite") != "race":
        bad.append(f"the payload did not survive the round trip: {sigs[0].get('data')}")
    if got.get("cursor") != first["seq"]:
        bad.append(f"the cursor should be the last delivered seq: {got}")

    # A parked wait wakes on a live post, and two waiters on one topic BOTH do.
    # Fan-out is the one genuinely new behaviour in this hub.
    woken = {}

    def park(name, session, topics):
        woken[name] = structured(session.tool(
            "await_signal", {"topics": topics, "timeout_s": 20}, timeout=40))

    c = Session(agent="aider")
    c.tool("announce", {"purpose": "sync-probe: the second listener", "activity": "listening"})
    threads = [threading.Thread(target=park, args=("b", b, ["probe:done:*"])),
               threading.Thread(target=park, args=("c", c, ["probe:done:one"]))]
    for t in threads:
        t.start()
    # Long enough for both parks to register with the daemon before the post.
    time.sleep(2.0)
    print("--- both listening, as the board sees it:")
    for row in json.loads(cli("sync", "agents", "--json").stdout or "{}").get("agents", []):
        if row.get("purpose", "").startswith("sync-probe"):
            print(f"    {row['agent']} [{row['state']}] {row.get('detail', '')}")
            if row["purpose"].endswith("listener") and row["state"] != "listening":
                bad.append(f"a parked session reads as {row['state']}, not listening")

    res = a.tool("post_signal", {"topic": "probe:done:one", "data": {"chunk": 1}})
    fan = structured(res)
    show("A post (two parked waiters)", res)
    if fan.get("delivered") != 2:
        bad.append(f"fan-out woke {fan.get('delivered')} of 2 waiters")
    for t in threads:
        t.join(timeout=40)
    for name in ("b", "c"):
        sigs = (woken.get(name) or {}).get("signals") or []
        if len(sigs) != 1 or sigs[0].get("seq") != fan.get("seq"):
            bad.append(f"waiter {name} did not wake with the signal: {woken.get(name)}")

    # A timeout is a result: the cursor comes back unchanged, so re-arming misses
    # nothing that happened in between.
    res = b.tool("await_signal", {"topics": ["probe:never"], "after_seq": fan["seq"],
                                  "timeout_s": 2}, timeout=30)
    got = structured(res)
    show("B await (times out)", res)
    if not got.get("timed_out") or got.get("received"):
        bad.append(f"expected an empty timeout: {got}")
    if got.get("cursor") != fan["seq"]:
        bad.append(f"a timeout must not move the cursor: {got}")

    # A request and its reply, both ordinary posts on private topics. This is the
    # whole "direct messages ride the same rails" claim.
    a_key = structured(a.tool("list_agents", {}))
    a_key = next((r["key"] for r in (a_key.get("agents") or [])
                  if r.get("purpose") == "sync-probe: the poster"), None)
    if not a_key:
        bad.append("could not find A's key on the roster to address it")
    else:
        reply = {}

        def wait_reply():
            reply["got"] = structured(a.tool(
                "await_signal", {"topics": ["to:@me"], "timeout_s": 20}, timeout=40))

        t = threading.Thread(target=wait_reply)
        t.start()
        time.sleep(2.0)
        res = b.tool("post_signal", {"topic": "to:" + a_key,
                                     "data": {"ask": "is the suite green?"}})
        show("B post (addressed to A)", res)
        t.join(timeout=40)
        sigs = (reply.get("got") or {}).get("signals") or []
        if len(sigs) != 1:
            bad.append(f"the addressed signal did not arrive: {reply.get('got')}")
        else:
            if sigs[0].get("topic") != "to:" + a_key:
                bad.append(f"the private topic was not expanded from @me: {sigs[0]}")
            if not sigs[0].get("key"):
                bad.append("a message carries no sender key, so a reply is impossible")

    # A cursor older than retention is confessed. The daemon's own retention is
    # 1000 per topic, so this asks with a cursor from before the table existed.
    res = b.tool("await_signal", {"topics": ["probe:tests"], "after_seq": 1,
                                  "timeout_s": 2}, timeout=30)
    got = structured(res)
    show("B await (a cursor from the distant past)", res)
    if got.get("gap"):
        if not got.get("oldest_seq"):
            bad.append("a gap was reported without the oldest surviving sequence")
        if "cannot be complete" not in (got.get("note") or ""):
            bad.append(f"the gap note does not say the batch is incomplete: {got.get('note')}")
        print("    (gap confessed, as it must be on a trimmed store)")
    else:
        # Not a failure: on this machine the store may be young enough that seq 1
        # is still there. Say which case ran rather than passing silently.
        print("    (no gap: seq 1 is still in the store, so there was nothing to confess)")

    # A departure is a signal too, so an idle agent can park on its area.
    area_seen = {}

    def wait_area():
        area_seen["got"] = structured(a.tool(
            "await_signal", {"topics": ["agents:repo:agentbox"], "timeout_s": 20}, timeout=40))

    t = threading.Thread(target=wait_area)
    t.start()
    time.sleep(2.0)
    c.close()
    t.join(timeout=40)
    print("--- A await (a peer left the area)")
    print("   ", json.dumps(area_seen.get("got") or {})[:300])
    sigs = (area_seen.get("got") or {}).get("signals") or []
    if not sigs:
        bad.append(f"a session leaving posted no agents:<area> signal: {area_seen.get('got')}")
    else:
        events = [(s.get("data") or {}).get("event") for s in sigs]
        if "leave" not in events:
            bad.append(f"the area topic carried {events}, not a leave")

    # A lock changing hands is announced on its own topic, which is how an agent
    # that gave up queueing learns the resource freed.
    lock_seen = {}
    a.tool("acquire_lock", {"name": "probe:signal-lock", "timeout_s": 5})

    def wait_lock():
        lock_seen["got"] = structured(b.tool(
            "await_signal", {"topics": ["lock:probe:signal-lock"], "timeout_s": 20}, timeout=40))

    t = threading.Thread(target=wait_lock)
    t.start()
    time.sleep(2.0)
    a.tool("release_lock", {"name": "probe:signal-lock"})
    t.join(timeout=40)
    print("--- B await (the lock changed hands)")
    print("   ", json.dumps(lock_seen.get("got") or {})[:300])
    sigs = (lock_seen.get("got") or {}).get("signals") or []
    if not sigs:
        bad.append(f"a release posted no lock:NAME signal: {lock_seen.get('got')}")
    else:
        data = sigs[0].get("data") or {}
        if data.get("reason") != "released" or data.get("free") is not True:
            bad.append(f"the lock signal does not describe the release: {data}")

    # The CLI's two verbs, which is how a Makefile or a hook joins the same fabric.
    out = cli("sync", "post", "probe:cli", '{"from":"a shell"}', "--json", key=a_key)
    if out.returncode != 0:
        bad.append(f"`sync post` failed: {out.stderr.strip()}")
    else:
        seq = json.loads(out.stdout or "{}").get("seq")
        out = cli("sync", "await", "probe:cli", "--after", str(seq - 1),
                  "--timeout", "5", key=a_key)
        if out.returncode != 0 or "probe:cli" not in out.stdout:
            bad.append(f"`sync await` did not read back the posted signal: {out.stdout} {out.stderr}")
        else:
            print("--- CLI round trip")
            print("   ", out.stdout.strip().replace("\n", " | "))
        # An await that finds nothing exits 1, per the house grammar.
        out = cli("sync", "await", "probe:nothing", "--timeout", "1", key=a_key)
        if out.returncode != 1:
            bad.append(f"a timed-out `sync await` should exit 1, got {out.returncode}")

    a.close()
    b.close()
    print("--- leaving no state: the signals stay (that is the point), the locks do not")
    return bad



def scenario_shared():
    """Slice 4's acceptance list: three sessions drain a ten-chunk claim table.

    The design's acceptance sentence is the whole scenario - "three sessions drain a
    ten-chunk claim table (one key per chunk) with zero double-claims; restart the
    daemon mid-drain: claims survive, the dead session's claim reads as ownerless,
    and the table still drains" - so this runs it literally, against the deployed
    daemon, with real mcp children.

    It restarts the real daemon on purpose. That is the only way to settle the
    durability half, and it is safe: children redial with backoff and replay their
    announce, pending items live in the store, and nothing here touches Boris's
    config. It does mean the run must be the daemon's only business for a moment,
    which is why it says so on the way past.
    """
    bad = []
    a = Session(agent="claude")
    b = Session(agent="codex")
    c = Session(agent="aider")
    a.tool("announce", {"purpose": "sync-probe: drainer A", "activity": "claiming chunks"})
    b.tool("announce", {"purpose": "sync-probe: drainer B", "activity": "claiming chunks"})
    c.tool("announce", {"purpose": "sync-probe: drainer C, about to die", "activity": "claiming chunks"})

    CHUNKS = 10
    KEYS = [f"probe:claims/{i}" for i in range(CHUNKS)]

    # A run leaves nothing behind, and shared values are deliberately never trimmed -
    # so unlike the signals scenario this one has to clean up after itself.
    for k in KEYS + ["probe:cli-claim"]:
        a.tool("shared", {"op": "delete", "key": k})

    # Three sessions claiming AT THE SAME TIME, which is the only version of this
    # worth running: walking the table one session after another would prove nothing
    # about contention, because every key would be free when the first walker reached
    # it. Two go forward and one goes backward, so the collisions land in the middle
    # and no session can win the whole table by being first.
    winners = {}
    claims_lock = threading.Lock()

    def drain(name, sess, keys):
        for key in keys:
            got = structured(sess.tool("shared", {
                "op": "set", "key": key, "value": {"worker": name}, "if_version": 0,
                "own": True}))
            with claims_lock:
                if got.get("applied"):
                    if key in winners:
                        bad.append(f"{key} was claimed by {winners[key]} and again by {name}")
                    winners[key] = name
                elif not got.get("stale"):
                    bad.append(f"a refused claim on {key} said neither applied nor stale: {got}")
                # The refusal has to carry the winner, or the loser needs a second call
                # to decide anything.
                elif not (got.get("value") or {}).get("version"):
                    bad.append(f"a refusal did not carry the current value: {got}")

    racers = [threading.Thread(target=drain, args=("A", a, KEYS)),
              threading.Thread(target=drain, args=("B", b, KEYS)),
              threading.Thread(target=drain, args=("C", c, list(reversed(KEYS))))]
    for r in racers:
        r.start()
    for r in racers:
        r.join(timeout=60)

    print(f"--- three sessions claimed {len(winners)} of {CHUNKS} chunks: "
          + ", ".join(f"{k.split('/')[-1]}={v}" for k, v in sorted(winners.items())))
    if len(winners) != CHUNKS:
        bad.append(f"{len(winners)} of {CHUNKS} chunks were claimed; every key should have a winner")
    if len(set(winners.values())) < 2:
        bad.append(f"one session won every chunk ({set(winners.values())}); this proves nothing about contention")

    # One session dies mid-drain, holding whatever it claimed. Its claims must read as
    # abandoned once the roster loses it - that is what makes the table drainable
    # instead of stuck forever on a chunk nobody is working on.
    c_chunks = [k for k, v in winners.items() if v == "C"]
    if not c_chunks:
        # Not a product failure, but the rest of this scenario is about what a dead
        # owner's claim looks like, so there is nothing left to check.
        bad.append("session C won no chunk, so the death case cannot be exercised")
        for k in KEYS:
            a.tool("shared", {"op": "delete", "key": k})
        a.close(), b.close(), c.close()
        return bad
    c.close()
    time.sleep(8)  # past holder_gone_grace_s (5s), so the row is really gone

    table = structured(a.tool("shared", {"op": "get", "key": "probe:claims/*"}))
    values = {v["key"]: v for v in (table.get("values") or [])}
    print(f"--- the table after C died ({len(values)} keys):")
    for key in sorted(values):
        v = values[key]
        print(f"    {key} v{v['version']} {json.dumps(v.get('value'))}"
              + (f" OWNER GONE ({v.get('owner_agent')})" if v.get("owner_gone") else ""))
    if len(values) != CHUNKS:
        bad.append(f"a prefix read returned {len(values)} of {CHUNKS} keys")
    for key in c_chunks:
        if not values.get(key, {}).get("owner_gone"):
            bad.append(f"{key} was claimed by the session that died and does not read as ownerless: {values.get(key)}")
        elif values[key].get("owner_agent") != "aider":
            bad.append(f"{key} does not name the agent that abandoned it: {values[key]}")
    for key, name in winners.items():
        if name != "C" and values.get(key, {}).get("owner_gone"):
            bad.append(f"{key} is held by a LIVE session and reads as abandoned: {values.get(key)}")
    if "owner_gone" not in (table.get("note") or ""):
        bad.append(f"the family read does not point at the orphans: {table.get('note')}")

    # A waiter parks on the family and is woken by the next write, which is the design's
    # one wake mechanism doing the blackboard's waiting for it.
    woken = {}

    def park():
        woken["got"] = structured(b.tool(
            "await_signal", {"topics": ["shared:probe:claims/*"], "timeout_s": 20}, timeout=40))
    t = threading.Thread(target=park)
    t.start()
    time.sleep(2.0)
    taken = c_chunks[0]
    over = structured(a.tool("shared", {
        "op": "set", "key": taken, "value": {"worker": "A", "took_over": True},
        "if_version": values[taken]["version"], "own": True}))
    print("--- A takes over an abandoned chunk")
    print("   ", json.dumps(over)[:300])
    if not over.get("applied"):
        bad.append(f"taking over an abandoned claim at its current version failed: {over}")
    t.join(timeout=40)
    sigs = (woken.get("got") or {}).get("signals") or []
    print("--- B was woken by the write:", json.dumps(sigs)[:220])
    if len(sigs) != 1 or sigs[0].get("topic") != f"shared:{taken}":
        bad.append(f"a waiter on the family was not woken by the write: {woken.get('got')}")
    else:
        data = sigs[0].get("data") or {}
        if data.get("key") != taken or not data.get("version"):
            bad.append(f"the shared signal does not describe the change: {data}")
        if "value" in data:
            bad.append(f"the signal is a doorbell and must not carry the value: {data}")

    # The durability half: restart the real daemon and the claims are still there.
    # Presence does not survive and is not meant to; coordination state does.
    print("--- restarting the daemon (the claims must outlive it)")
    out = subprocess.run(["systemctl", "--user", "restart", "agentbox.service"],
                         capture_output=True, text=True)
    if out.returncode != 0:
        bad.append(f"could not restart the daemon: {out.stderr.strip()}")
    # Long enough for the unit to come back AND for the surviving children to redial,
    # which is what puts their rows back on the roster.
    time.sleep(6)

    after = structured(a.tool("shared", {"op": "get", "key": "probe:claims/*"}))
    survived = {v["key"]: v for v in (after.get("values") or [])}
    print(f"--- after the restart, {len(survived)} of {CHUNKS} keys survived")
    if len(survived) != CHUNKS:
        bad.append(f"only {len(survived)} of {CHUNKS} claims survived the restart")
    # Compared as objects, not as JSON text: two dicts with the same contents in a
    # different key order are the same value, and a probe that says otherwise reports
    # a defect that is its own.
    if survived.get(taken, {}).get("value") != {"worker": "A", "took_over": True}:
        bad.append(f"the taken-over claim did not survive intact: {survived.get(taken)}")
    if survived.get(taken, {}).get("version") != 2:
        bad.append(f"the taken-over claim lost its version across the restart: {survived.get(taken)}")
    # The one the pid check exists for: A is alive and reattached, so its claims must
    # NOT read as abandoned just because the roster was empty a moment ago.
    live_after = [k for k, v in winners.items() if v == "A"] + [taken]
    for key in live_after:
        if survived.get(key, {}).get("owner_gone"):
            bad.append(f"{key} is held by a live session and read as abandoned after the restart: {survived.get(key)}")
    # And the dead session's claims still read as abandoned, from a daemon that never
    # saw that session at all - which only the recorded owner can answer.
    for key in c_chunks:
        if key == taken:
            continue
        if not survived.get(key, {}).get("owner_gone"):
            bad.append(f"{key}'s dead owner was forgotten by the restart: {survived.get(key)}")

    # The table still drains: every remaining claim is finished and removed.
    for key in sorted(survived):
        got = structured(a.tool("shared", {"op": "delete", "key": key}))
        if not got.get("applied"):
            bad.append(f"finishing {key} failed: {got}")
    left = structured(a.tool("shared", {"op": "get", "key": "probe:claims/*"}))
    print(f"--- drained: {len(left.get('values') or [])} keys left")
    if left.get("values"):
        bad.append(f"the table did not drain: {left.get('values')}")

    # The CLI's three verbs, which is how a Makefile or a hook joins the same fabric.
    a_key = structured(a.tool("list_agents", {}))
    a_key = next((r["key"] for r in (a_key.get("agents") or [])
                  if r.get("purpose") == "sync-probe: drainer A"), None)
    if not a_key:
        bad.append("could not find A's key on the roster for the CLI half")
    else:
        out = cli("sync", "set", "probe:cli-claim", "mine", "--if-version", "0", "--own", key=a_key)
        if out.returncode != 0:
            bad.append(f"`sync set` claiming a free key should exit 0: {out.returncode} {out.stderr.strip()}")
        # A lost claim is exit 1 with the current value on stdout, which is the shape a
        # claiming loop wants from a shell.
        out = cli("sync", "set", "probe:cli-claim", "theirs", "--if-version", "0", "--own", key=a_key)
        if out.returncode != 1 or "probe:cli-claim" not in out.stdout:
            bad.append(f"a lost `sync set` should exit 1 with the value: {out.returncode} {out.stdout} {out.stderr}")
        out = cli("sync", "get", "probe:cli-claim", key=a_key)
        if out.returncode != 0 or "mine" not in out.stdout:
            bad.append(f"`sync get` did not read the claim back: {out.stdout} {out.stderr}")
        else:
            print("--- CLI round trip")
            print("   ", out.stdout.strip())
        out = cli("sync", "del", "probe:cli-claim", key=a_key)
        if out.returncode != 0:
            bad.append(f"`sync del` failed: {out.stderr.strip()}")
        # And a get of nothing is exit 1, per the house grammar.
        out = cli("sync", "get", "probe:cli-claim", key=a_key)
        if out.returncode != 1:
            bad.append(f"a get of a missing key should exit 1, got {out.returncode}")

    a.close()
    b.close()
    print("--- leaving no state: every probe claim is deleted, unlike the signals it posted")
    return bad


SCENARIOS = {"rider": scenario_rider, "locks": scenario_locks,
             "signals": scenario_signals, "shared": scenario_shared,
             "board": scenario_board}


def main():
    name = sys.argv[1] if len(sys.argv) > 1 else ""
    if name not in SCENARIOS:
        print(__doc__, file=sys.stderr)
        print("scenarios: " + ", ".join(sorted(SCENARIOS)), file=sys.stderr)
        return 2
    # Every scenario runs inside the cleanup, so no scenario has to remember: an
    # acceptance run that toasts the human is the run's mess to clear (FR89).
    with OwnToasts():
        bad = SCENARIOS[name]()
    print()
    for b in bad:
        print("!!", b)
    print("RESULT:", "PASS" if not bad else "FAIL")
    return 0 if not bad else 1


if __name__ == "__main__":
    sys.exit(main())
