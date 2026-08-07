#!/usr/bin/env python3
"""Stage and capture the twelve wiki screenshots (docs/wiki/DESIGN.md section 5).

Each shot is staged, not found: the script writes the copy, raises the surface,
waits until the window is really on screen, and crops a PNG from that window's
own geometry. Nobody clicks anything.

Read tools/wiki/SHOTS.md before running this. What follows is why the script is
shaped the way it is; the runbook is the other file.

WHY IT IS ONE PASS
  Surfaces render only for the machine's ONE daemon. A second agentbox shows no
  windows while another runs, because the first process owns org.wails.agentbox
  on the session bus and a later one becomes a remote GApplication instance whose
  window requests go to the holder. So a sitting costs a daemon swap on a live
  desktop, and the swap is the expensive part, not the shot. All twelve happen in
  one run.

WHY THREE PHASES
  1  isolated   the deployed daemon goes down, a throwaway one comes up on its own
                instance and its own XDG_STATE_HOME/XDG_CONFIG_HOME. Ten shots.
                Boris's history, settings and pending queue are not what we
                experiment on, and no call in this phase can reach them: the
                instance name puts the socket and the store somewhere else.
  2  demo       no daemon at all, `agentbox webui-demo ask panel`. One shot (S11).
                The inline ask panel only renders for a session agentbox itself
                started (inlineRoutable needs sessionShown), and this is the only
                route to that frame that does not mean launching a real session.
  3  real       the deployed daemon comes back and S12 is taken against the real
                store, read-only. That shot's whole point per DESIGN.md is real
                unpruned data including the rehearsal rows, which the owner
                decided on 2026-07-25 to leave honest rather than tidy. Opening a
                tab writes nothing.
  Phase 3 last is deliberate: the run ends with the deployed daemon already back,
  which is the state the machine has to be left in.

TRAPS THIS SCRIPT IS BUILT AROUND, each of which has cost somebody a take
  * Never pkill. `pkill agentbox` matches the `agentbox mcp` child every Claude
    session holds, and `pkill -f` has killed the invoking shell. `make stop` reads
    each candidate's own cmdline and kills only daemons. It is the only route here.
  * An unquoted =agentbox is equals-expanded by zsh to the binary's path, and the
    lookup then searches for a window titled /home/you/.local/bin/agentbox. Every
    call here goes through a list argv with no shell, so it cannot happen; if you
    copy a line into a terminal, quote it.
  * `hand` refuses a window under 16 pixels, so a window still animating reads as
    absent. Never sleep and hope: poll `drive where` and fail loudly with the shot
    it was.
  * Every window opens on the monitor the pointer is on. The pointer is parked
    once, before anything opens, and the log says where.
  * Never click a list row at a coordinate read off an earlier screenshot: any
    queue change reflows the inbox. Nothing here clicks a row. Tab changes close
    the window and reopen it with --tab instead of clicking the rail, because the
    rail has grown and old coordinates are the same class of mistake.
  * `import -window root` cannot see a fullscreen window: mutter unredirects one
    for direct scanout and the root pixmap holds what is underneath. Do not run
    this with anything fullscreen. --capture shell is the escape hatch.
  * A card stays mapped for the grace period after it is answered, so a check for
    "gone" that is too quick calls every hit a miss.

Usage
  tools/wiki/shots.py --list                  print the plan, touch nothing
  tools/wiki/shots.py --dry-run               print every action, touch nothing
  tools/wiki/shots.py --yes                   the whole sitting
  tools/wiki/shots.py --only S1,S3 --yes      a retake
  tools/wiki/shots.py --verify-only           re-check the files already there
"""

import argparse
import atexit
import json
import os
import re
import shutil
import subprocess
import sys
import tempfile
import time
from pathlib import Path

REPO = Path(__file__).resolve().parents[2]
AGENTBOX = os.environ.get("AGENTBOX_BIN", str(Path.home() / ".local" / "bin" / "agentbox"))
OUT_DIR = REPO / "docs" / "wiki" / "img"

# The throwaway instance. This string is the safety belt for the whole isolated
# phase: AGENTBOX_INSTANCE names the socket AND the store (stateDir() prefixes
# "agentbox-"), so a dismiss --all carrying it cannot reach the deployed queue
# however wrong everything else goes.
INSTANCE = "wikishots"

# The fiction, from DESIGN.md section 5. One project, four agents, one afternoon.
PROJECT = "checkout-api"
RELEASE = "release-bot"
TESTS = "test-runner"
DEPS = "dependency-bot"
ONCALL = "oncall-helper"
AREA = f"project:{PROJECT}"

# Session keys for the staged roster. The "asking you" chip is derived from
# Identity.Key (askingKeys in internal/daemon/sync.go), not from --session, which
# is why the asks carry --key and never --session: an item with a session could be
# routed inline instead of onto a card, and S1 needs a card.
KEY_RELEASE = "wikishot-release-bot"
KEY_TESTS = "wikishot-test-runner"
KEY_DEPS = "wikishot-dependency-bot"
KEY_ANON = "wikishot-anonymous"
KEY_GONE = "wikishot-oncall-helper"

LOCK = f"deploy:{PROJECT}"

# ---------------------------------------------------------------------------
# the plan
#
# Order is chosen so no surface is ever reopened and so that what one shot leaves
# behind is what the next one needs:
#
#   S9  toast first, because the hands-off strip pins itself to the top of the
#       same top-centre column and would sit in the toast's frame.
#   S10 progress next, bottom right, nothing else in that corner yet.
#   S1  the card, and it leaves three pending: one current plus two queued, which
#       is what makes the footer read "2 waiting" (View.Waiting is len(queue), so
#       two waiting means three pending).
#   S3  the inbox, after dropping one of the queued two, because DESIGN wants two
#       pending rows. The history behind them was manufactured before S1.
#   S2  the agents board. Its roster was staged before S1 as well, so the release
#       question's key already had a row to light up.
#   S4  the review board, S7 the artifact, S8 the reader: three windows, each
#       closed before the next opens.
#   S5/S6 the hands-off strip last in the phase, because it is state rather than a
#       card and it would otherwise be in every frame above.
# ---------------------------------------------------------------------------

SHOTS = [
    dict(
        key="S9", out="toast.png", phase="isolated", stage="stage_s9",
        title="=agentbox · toast", crop="top", pad=48,
        page="notifications.md",
        what="warning toast from dependency-bot, no countdown, top edge of the screen in frame",
    ),
    dict(
        key="S10", out="progress.png", phase="isolated", stage="stage_s10",
        title="=agentbox · progress", crop="corner", pad=140,
        page="notifications.md",
        what="three progress bars in the bottom-right corner, with enough desktop to prove where it sits",
    ),
    dict(
        key="S1", out="card-restaged.png", phase="isolated", stage="stage_s1",
        title="=agentbox", crop="window", pad=24,
        page="home.md, the-card.md",
        what="the choice card with two questions queued behind it, so the footer earns '2 waiting'",
    ),
    dict(
        key="S3", out="inbox.png", phase="isolated", stage="stage_s3",
        title="=agentbox · app", crop="window", pad=0,
        page="nothing-gets-lost.md",
        what="two pending rows on top, key hints under the selected one, outcomes in the right column",
    ),
    dict(
        key="S2", out="agents-board.png", phase="isolated", stage="stage_s2",
        title="=agentbox · app", crop="window", pad=0,
        page="agents-board.md, taking-turns.md",
        what="four rows: asking you, blocked behind a lock, listening, and one with no purpose",
    ),
    dict(
        key="S4", out="review-board.png", phase="isolated", stage="stage_s4",
        # No leading "=": the board titles itself "agentbox · review board · <the
        # walkthrough's title>", so an exact match would find nothing.
        title="agentbox · review board", crop="window", pad=0,
        page="review-board.md",
        what="a real diff from this repo, one step open, a highlighted range with a note on it",
    ),
    dict(
        key="S7", out="artifact.png", phase="isolated", stage="stage_s7",
        title="=agentbox · Canary rollout", crop="window", pad=0,
        page="documents-and-artifacts.md",
        what="the canary console at 50%, its bar, two buttons, and the interactive badge with the code toggle",
    ),
    dict(
        key="S8", out="viewer.png", phase="isolated", stage="stage_s8",
        title="=agentbox · sample.md", crop="window", pad=0,
        page="documents-and-artifacts.md",
        what="a table, a mermaid diagram and a highlighted code block in one frame, watching badge lit",
    ),
    dict(
        key="S5", out="hands-off.png", phase="isolated", stage="stage_s5",
        title="=agentbox · hands off", crop="top", pad=260,
        page="hands-off.md",
        what="the amber HANDS OFF strip over a real desktop, activity line reading about four seconds old",
    ),
    dict(
        key="S6", out="hands-off-paused.png", phase="isolated", stage="stage_s6",
        title="=agentbox · hands off", crop="top", pad=260,
        page="hands-off.md",
        what="the same strip green, PAUSED - YOURS, the frozen activity line still readable",
    ),
    dict(
        key="S11", out="panel.png", phase="demo", stage="stage_s11",
        title="=agentbox · panel", crop="window", pad=0,
        page="sessions.md",
        what="the console rolled down over the editor, inline ask above the composer",
    ),
    dict(
        key="S12", out="history-stats.png", phase="real", stage="stage_s12",
        title="=agentbox · app", crop="window", pad=0,
        page="nothing-gets-lost.md or settings.md",
        what="real unpruned history and the median answer time, against the real store, read-only",
    ),
]

PHASE_TEXT = {
    "isolated": "PHASE 1 of 3  ISOLATED   throwaway daemon, throwaway state. Nothing here touches the real queue.",
    "demo": "PHASE 2 of 3  DEMO       no daemon at all, webui-demo canned data.",
    "real": "PHASE 3 of 3  REAL        the DEPLOYED daemon, the REAL store, READ ONLY. Nothing is written or pruned.",
}

# A capture whose standard deviation is below this, or which holds fewer than this
# many distinct colours, is what a missed window looks like: a crop of the
# wallpaper, or a solid-coloured rectangle where a surface should have been.
MIN_STDDEV = 0.02
MIN_COLORS = 32


# ---------------------------------------------------------------------------
# pure helpers. Everything below this line to the next banner is unit-tested in
# tools/wiki/test_shots.py against fabricated input, because none of it can be
# exercised without the desktop.
# ---------------------------------------------------------------------------

def parse_where(text):
    """The four numbers `agentbox drive where` prints, or None.

    It prints "x y w h" on one line so a shell can read it into four variables.
    Anything else means no window matched.
    """
    parts = (text or "").split()
    if len(parts) != 4:
        return None
    try:
        return tuple(int(p) for p in parts)
    except ValueError:
        return None


def parse_monitors(text):
    """Monitors as (name, w, h, x, y), from `xrandr --listmonitors`.

    xrandr is the right source rather than wmctrl -lG, which reports doubled
    coordinates on a scaled display.
    """
    out = []
    pat = re.compile(
        r"^\s*\d+:\s*\+?\*?(?P<name>[A-Za-z0-9-]+)\s+"
        r"(?P<w>\d+)/\d+x(?P<h>\d+)/\d+\+(?P<x>\d+)\+(?P<y>\d+)"
    )
    for line in (text or "").splitlines():
        m = pat.match(line)
        if m:
            out.append((m["name"], int(m["w"]), int(m["h"]), int(m["x"]), int(m["y"])))
    return out


def pick_monitor(mons, name=""):
    """The monitor to shoot on: the named one, else the widest."""
    if not mons:
        return None
    if name:
        for m in mons:
            if m[0] == name:
                return m
        raise SystemExit(f"no monitor called {name}; xrandr knows {[m[0] for m in mons]}")
    return sorted(mons, key=lambda m: -m[1])[0]


def crop_rect(mode, win, mon, pad):
    """The rectangle to hand `import -crop`, from the window's own geometry.

    win and mon are (x, y, w, h) in root coordinates, which is what both
    `drive where` and `import -window root -crop` speak.

      window  the window plus pad on all four sides, so a shadow reads
      top     the window plus pad, with the top edge of the monitor kept in
              frame, because for a toast and for the hands-off strip the
              position IS part of the information
      corner  the window plus pad, extended to the monitor's bottom-right
              corner, for the same reason

    Always clamped to the monitor: a pad that would run off the screen is
    trimmed rather than handed to import, which would silently shrink the crop
    and move it.
    """
    x, y, w, h = win
    mx, my, mw, mh = mon
    if mode == "window":
        x, y, w, h = x - pad, y - pad, w + 2 * pad, h + 2 * pad
    elif mode == "top":
        bottom = y + h + pad
        x, w = x - pad, w + 2 * pad
        y, h = my, bottom - my
    elif mode == "corner":
        x, y = x - pad, y - pad
        w, h = (mx + mw) - x, (my + mh) - y
    else:
        raise ValueError(f"unknown crop mode {mode!r}")

    if x < mx:
        w -= mx - x
        x = mx
    if y < my:
        h -= my - y
        y = my
    w = min(w, mx + mw - x)
    h = min(h, my + mh - y)
    if w < 16 or h < 16:
        raise ValueError(f"crop {mode} came out {w}x{h}, which is not a picture of anything")
    return x, y, w, h


def uniform_verdict(colors, stddev):
    """Whether a capture looks like a surface or like a missed window."""
    if colors <= 1:
        return False, "solid colour: the window was not there"
    if stddev < MIN_STDDEV:
        return False, f"near-uniform (sd {stddev:.4f}): probably wallpaper or an unpainted window"
    if colors < MIN_COLORS:
        return False, f"only {colors} distinct colours: probably not a rendered surface"
    return True, f"sd {stddev:.4f}, {colors} colours"


def parse_dnd(text):
    """on or off, from `agentbox dnd status`, which prints "do not disturb: off".

    Read the side after the colon and nothing else: "do not disturb" contains
    neither word as a token but a substring search over the whole line is the kind
    of thing that works until the wording changes.
    """
    tail = (text or "").rsplit(":", 1)[-1].strip().lower()
    if tail.startswith("on"):
        return "on"
    if tail.startswith("off"):
        return "off"
    return ""


def own_pids():
    """This process and every ancestor of it.

    The peer check reads the same roster the human's Agents board renders, and a
    run started from inside an agent session finds its own row in it. Counting
    yourself as a peer would mean the script always refused.
    """
    pids, pid = set(), os.getpid()
    for _ in range(40):
        pids.add(pid)
        try:
            stat = Path(f"/proc/{pid}/stat").read_text()
            pid = int(stat.rsplit(")", 1)[1].split()[1])
        except (OSError, IndexError, ValueError):
            break
        if pid <= 1:
            break
    return pids


def parse_only(spec, keys):
    """Shot keys from --only, in plan order, rejecting names that are not shots."""
    if not spec:
        return list(keys)
    want = [s.strip().upper() for s in spec.replace(" ", ",").split(",") if s.strip()]
    unknown = [w for w in want if w not in keys]
    if unknown:
        raise SystemExit(f"--only: no such shot {', '.join(unknown)}. Known: {', '.join(keys)}")
    return [k for k in keys if k in want]


def staged_artifact_source(text):
    """tools/showcase/console.jsx, restaged for the wiki's fiction.

    The showcase console is already the right interface (a slider over live
    traffic, a bar in requests a minute, "Start the rollout" and "Hold it"), so
    it is patched rather than forked: DESIGN wants the slider mid-track at 50%,
    and the wiki's release is 2026.7.30 while the deck's is 2026.7.3.
    """
    out = text.replace("useState(10)", "useState(50)")
    out = out.replace("release 2026.7.3 ·", "release 2026.7.30 ·")
    if "useState(50)" not in out:
        raise SystemExit("console.jsx no longer has the percent useState this patches; fix staged_artifact_source")
    if "2026.7.30" not in out:
        raise SystemExit("console.jsx no longer carries the release string this patches; fix staged_artifact_source")
    return out


def review_spec(path, first, last, note_from, note_to):
    """The walkthrough spec for S4, citing a real file in this repo.

    A real diff needs no fiction, which is what DESIGN asks for. The note is in
    the spec rather than typed in by synthetic input: a comment anchored by
    clicking would mean clicking a coordinate, and this file does not do that.
    """
    return json.dumps({
        "version": 1,
        "title": "the exit-code contract",
        "repo_root": ".",
        "steps": [
            {"id": "ground", "kind": "ground", "title": "What this change is",
             "prose": [{"t": "Five numbers a script branches on, and the reason they can never move."}]},
            {"id": "codes", "kind": "code", "title": "Exit codes are a contract",
             "purpose": "Serves: FR41 - scripts branch on these numbers. Decided by: the CLI's first release.",
             "prose": [{"t": "Five numbers, "}, {"t": "stable forever", "bind": "codes"},
                       {"t": ", because agents write scripts against them the day they learn them."}],
             "code": [{"path": path, "lines": [first, last],
                       "notes": [{"at": [note_from, note_to],
                                  "text": "0 through 4: answered, refused, misused, unanswered, broken."}]}],
             "binds": {"codes": {"lines": [note_from, note_to]}},
             "checks": [{"q": "A blocking ask times out. Which code?",
                         "a": "3 - unanswered. 1 is a person saying no; 4 is agentbox itself failing."}]},
            {"id": "gate", "kind": "check", "title": "The gate",
             "purpose": "Serves: finishing is an observation, not a feeling.",
             "prose": [{"t": "Mark a step unclear with no note and submit: the modal jumps back."}],
             "cmds": [{"cmd": "agentbox walkthrough list", "expect": "the library lists this review"}]},
        ],
    })


# ---------------------------------------------------------------------------
# the runner
# ---------------------------------------------------------------------------

class Run:
    """Everything the staging functions need, and everything cleanup must undo."""

    def __init__(self, args):
        self.args = args
        self.dry = args.dry_run
        self.out_dir = Path(args.out_dir)
        self.failures = []
        self.notes = []
        self.bg = []            # background children, newest last
        self.tmp = None
        self.env_iso = None     # set once the throwaway daemon is up
        self.env_real = dict(os.environ)
        self.env_real.pop("AGENTBOX_INSTANCE", None)
        self.env_real.pop("AGENTBOX_SESSION_KEY", None)
        self.dnd_was = None
        self.held_desktop = False
        self.held_desktop_line = False   # whether S5 already set the activity line
        self.iso_started = False
        self.touched_daemon = False
        self.mon = None
        self.mon_name = "?"
        self.phase = None
        self.captured = []
        self.project_dir = None   # a directory named checkout-api, so sync says so

    # -- logging ---------------------------------------------------------
    def say(self, msg):
        print(msg, flush=True)

    def step(self, msg):
        print(f"    {msg}", flush=True)

    def banner(self, msg):
        print("\n" + "=" * 78, flush=True)
        print(msg, flush=True)
        print("=" * 78, flush=True)

    def fail(self, key, msg):
        self.failures.append(f"{key}: {msg}")
        print(f"    FAIL {key}: {msg}", flush=True)

    def note(self, msg):
        self.notes.append(msg)
        print(f"    note: {msg}", flush=True)

    # -- processes -------------------------------------------------------
    def sh(self, argv, env=None, check=False, timeout=None, quiet=False, cwd=None):
        if not quiet:
            self.step("$ " + " ".join(argv[:12]))
        if self.dry:
            return subprocess.CompletedProcess(argv, 0, "", "")
        return subprocess.run(argv, env=env or os.environ, capture_output=True,
                              text=True, check=check, timeout=timeout, cwd=cwd)

    def abx(self, *args, env=None, check=False, timeout=None, quiet=False, cwd=None):
        """One agentbox call, as an argv list.

        A list argv and no shell is not a style choice: it is how "=agentbox" can
        never be equals-expanded to the binary's path on the way in.
        """
        return self.sh([AGENTBOX, *args], env=env, check=check, timeout=timeout,
                       quiet=quiet, cwd=cwd)

    def bg_start(self, argv, env=None, stdin=None, label="", cwd=None):
        """A child that must outlive this call, and must be killed at exit."""
        self.step(f"& {label or ' '.join(argv[:8])}")
        if self.dry:
            return None
        # text=True matters: `agentbox progress` reads percent lines from stdin and
        # the caller writes str into that pipe.
        p = subprocess.Popen(argv, env=env or os.environ, cwd=cwd,
                             stdin=stdin, stdout=subprocess.DEVNULL,
                             stderr=subprocess.DEVNULL, start_new_session=True,
                             text=True)
        self.bg.append(p)
        return p

    def abx_bg(self, *args, env=None, stdin=None, label="", cwd=None):
        return self.bg_start([AGENTBOX, *args], env=env, stdin=stdin,
                             label=label or args[0], cwd=cwd)

    # -- windows ---------------------------------------------------------
    def where(self, title, env=None):
        r = self.abx("drive", "where", title, env=env, quiet=True)
        if r.returncode != 0:
            return None
        return parse_where(r.stdout)

    def wait_window(self, key, title, env=None, timeout=None):
        """Poll until the window is really there, or fail naming the shot.

        `hand` refuses a window under 16 pixels, so a surface still animating
        reads as absent and a fixed sleep is a coin toss.
        """
        timeout = timeout or self.args.timeout
        if self.dry:
            self.step(f"would wait up to {timeout:.0f}s for {title}")
            return (100, 100, 800, 600)
        deadline = time.time() + timeout
        while time.time() < deadline:
            g = self.where(title, env)
            if g:
                # One more poll after it appears: a window that has just mapped is
                # often still growing, and a crop taken now is a crop of half of it.
                time.sleep(self.args.settle)
                return self.where(title, env) or g
            time.sleep(0.25)
        self.fail(key, f"no window matching {title} after {timeout:.0f}s")
        return None

    def wait_gone(self, title, env=None, seconds=5.0):
        if self.dry:
            return True
        deadline = time.time() + seconds
        while time.time() < deadline:
            if self.where(title, env) is None:
                return True
            time.sleep(0.25)
        return False

    def close_window(self, title, env=None):
        """Close by window id. No focus, no coordinates, so it works when a pixel
        offset does not."""
        if self.dry:
            self.step(f"would close {title}")
            return True
        exact = title.startswith("=")
        want = title.lstrip("=")
        ids = subprocess.run(["wmctrl", "-l"], capture_output=True, text=True).stdout
        for line in ids.splitlines():
            parts = line.split(None, 3)
            if len(parts) != 4:
                continue
            got = parts[3].strip()
            # Same matching rule as `drive where`: a leading = means exact, and
            # without it a substring, because a window can title itself
            # "agentbox · review board · the exit-code contract".
            if got == want or (not exact and want in got):
                subprocess.run(["wmctrl", "-i", "-c", parts[0]], capture_output=True)
                break
        return self.wait_gone(title, env)

    def park(self):
        """The pointer decides which monitor every window opens on, so it is
        parked once, before anything opens, and the log says where."""
        x, y, w, h = self.mon
        px, py = (self.args.park.split(",") + [""])[:2] if self.args.park else ("", "")
        if px and py:
            px, py = int(px), int(py)
        else:
            # Left edge, half way down: deliberately not the bottom corner, where
            # a pointer summons desktop furniture into the frame.
            px, py = x + 60, y + h // 2
        self.say(f"    pointer parked at {px},{py} on monitor {self.mon_name} "
                 f"({w}x{h} at +{x}+{y}). Every window will open here.")
        # `screen` first, always: a move with no frame reset is read in whatever
        # frame the last script left behind.
        if not self.dry:
            subprocess.run([AGENTBOX, "drive", "run", "-"], input=f"screen\nmove {px} {py}\n",
                           text=True, capture_output=True)

    def drive(self, script, env=None):
        self.step("drive: " + script.replace("\n", " | ").strip())
        if self.dry:
            return None
        return subprocess.run([AGENTBOX, "drive", "run", "-"], input=script, text=True,
                              capture_output=True, env=env or os.environ)

    # -- capture ---------------------------------------------------------
    def capture(self, shot, geom):
        try:
            rect = crop_rect(shot["crop"], geom, self.mon, shot["pad"])
        except ValueError as e:
            # A window off this monitor, or one hand would have refused. One bad
            # shot must not end the sitting: the daemon swap is the expensive part.
            self.fail(shot["key"], f"{e}. The window was at {geom} and the monitor "
                                   f"is {self.mon}; it probably opened on the other screen.")
            return None
        x, y, w, h = rect
        path = self.out_dir / shot["out"]
        self.step(f"crop {w}x{h}+{x}+{y}  ->  {path.relative_to(REPO)}")
        if self.dry:
            return rect
        self.out_dir.mkdir(parents=True, exist_ok=True)
        if self.args.capture == "import":
            argv = ["import", "-silent", "-window", "root",
                    "-crop", f"{w}x{h}+{x}+{y}", "+repage", str(path)]
            r = subprocess.run(argv, capture_output=True, text=True)
            if r.returncode != 0:
                self.fail(shot["key"], f"import failed: {r.stderr.strip()}")
                return None
        else:
            # The shell's own capture sees the composited output, which
            # `import -window root` does not when anything is fullscreen.
            full = Path(self.tmp) / f"full-{shot['key']}.png"
            r = subprocess.run(["gnome-screenshot", "-f", str(full)], capture_output=True, text=True)
            if r.returncode != 0:
                self.fail(shot["key"], f"gnome-screenshot failed: {r.stderr.strip()}")
                return None
            r = subprocess.run(["convert", str(full), "-crop", f"{w}x{h}+{x}+{y}",
                                "+repage", str(path)], capture_output=True, text=True)
            if r.returncode != 0:
                self.fail(shot["key"], f"convert failed: {r.stderr.strip()}")
                return None
        self.captured.append(shot["key"])
        return rect


# ---------------------------------------------------------------------------
# staging, one function per shot
# ---------------------------------------------------------------------------

def ident(agent, key=""):
    """Identity flags for the item commands (ask, notify, review, progress, veto).

    `sync` does NOT take these: it builds its identity from agentName() and
    projectName(), so a roster row is named through the AGENTBOX_AGENT env var and
    projected through the working directory. sync_env below is that route.
    """
    a = ["--agent", agent, "--project", PROJECT]
    if key:
        a += ["--key", key]
    return a


def sync_env(c, key, agent):
    """The environment a `sync` call needs to appear as one of the four agents.

    agentName() takes AGENTBOX_AGENT first, because it is the only thing that
    survives setsid cutting the process tree. projectName() derives from the
    working directory, and deriveArea walks up looking for a .git it will not find
    under the staging directory, so the project ends up the basename: which is why
    the staging directory is called checkout-api.
    """
    return dict(c.env_iso, AGENTBOX_SESSION_KEY=key, AGENTBOX_AGENT=agent)


def wait_for_row(c, key, cwd, timeout=10.0):
    """True once `key` is on the throwaway roster.

    Only needed for the rows staged by a backgrounded command: `announce` returns
    once the row is written and can be trusted, `attach` never returns at all.
    """
    deadline = time.time() + timeout
    while time.time() < deadline:
        r = c.abx("sync", "agents", "--json", env=c.env_iso, cwd=cwd, quiet=True)
        if r.returncode == 0:
            try:
                if any(a.get("key") == key for a in json.loads(r.stdout).get("agents", [])):
                    return True
            except (ValueError, AttributeError):
                pass
        time.sleep(0.3)
    return False


def stage_roster(c):
    """The four rows S2 needs, staged before S1 so the release question's key
    already has a row to light up.

    Row 1 asking you        release-bot, which will hold the pending question
    Row 2 blocked           test-runner, waiting on a lock release-bot holds
    Row 3 listening         dependency-bot, parked on a signal
    Row 4 no purpose given  a session that attached and never announced
    """
    c.step("staging the roster")
    cwd = str(c.project_dir)
    for key, agent, purpose, activity in [
        (KEY_RELEASE, RELEASE, "cutting release 2026.7.30", "waiting on the region choice"),
        (KEY_TESTS, TESTS, "running the pre-release suite", "waiting for the deploy lock"),
        (KEY_DEPS, DEPS, "auditing transitive dependencies", "parked on tests:green"),
    ]:
        r = c.abx("sync", "announce", purpose, "--area", AREA, "--activity", activity,
                  env=sync_env(c, key, agent), cwd=cwd)
        if r.returncode != 0:
            c.fail("S2", f"sync announce for {agent} exited {r.returncode}: {r.stderr.strip()[:200]}")

    # The dim row. A board that only shows well-behaved agents is not believable,
    # and `attach` is how a session gets onto the roster without a purpose.
    #
    # It MUST be backgrounded. `sync attach` holds presence open for as long as its
    # process runs and never returns on its own (cmd/agentbox/sync.go:299-301 says
    # so: "No timeout: the whole point is to stay"). Running it in the foreground
    # hung the first real sitting here forever, at the fourth line of the first
    # phase, with the machine's daemon already down. bg_start tracks it and kills it
    # at exit, which is also what ends the row.
    c.abx_bg("sync", "attach", "--area", AREA,
             env=sync_env(c, KEY_ANON, ONCALL), cwd=cwd,
             label="a session that attached and never announced")
    # Backgrounding costs the return code, so the row is confirmed instead. Without
    # this a failed attach is a three-row board and S2 is quietly wrong.
    if not c.dry and not wait_for_row(c, KEY_ANON, cwd):
        c.fail("S2", "sync attach never put its row on the roster, so the board would "
                     "show three rows where DESIGN wants four")

    # release-bot holds the deploy lock detached; test-runner waits on it, which
    # is what puts the holder's name on row 2's wait line.
    c.abx("sync", "lock", LOCK, "--ttl", "900",
          env=sync_env(c, KEY_RELEASE, RELEASE), cwd=cwd)
    c.abx_bg("sync", "lock", LOCK, "--timeout", "900", "--", "sleep", "900",
             env=sync_env(c, KEY_TESTS, TESTS), cwd=cwd,
             label="test-runner waits on the deploy lock")
    c.abx_bg("sync", "await", "tests:green", "--timeout", "900",
             env=sync_env(c, KEY_DEPS, DEPS), cwd=cwd,
             label="dependency-bot listens for tests:green")

    # The shared block: two live claims and one whose owner is gone.
    c.abx("sync", "set", "canary:region", "eu-west", "--own",
          env=sync_env(c, KEY_RELEASE, RELEASE), cwd=cwd)
    c.abx("sync", "set", "suite:shard-3", "running", "--own",
          env=sync_env(c, KEY_TESTS, TESTS), cwd=cwd)
    if c.args.stage_abandoned:
        gone = sync_env(c, KEY_GONE, ONCALL)
        c.abx("sync", "announce", "retrying the flaky payment test", "--area", AREA,
              env=gone, cwd=cwd)
        c.abx("sync", "set", "suite:shard-7", "running", "--own", env=gone, cwd=cwd)
        c.note("S2: the abandoned claim needs an owner that announced and then went away, "
               "so a fifth row may be on the roster. DESIGN wants four. If a fifth row is in "
               "frame, retake S2 with --no-stage-abandoned and accept two live claims.")


def stage_history(c):
    """The outcome column S3 needs, made out of real resolutions.

    eu-west, approved and proceeded are not strings written into a store: each one
    is an item that was really answered, so the column is honest. One is left to
    elapse, because a wiki that only shows answered rows is selling a fantasy.
    """
    e = c.env_iso
    c.step("manufacturing history: four items, resolved four ways")

    # answered with option 1 -> outcome "eu-west"
    c.abx_bg("ask", "--title", "Which region takes 2026.7.22 first?",
             "--body", "Same question as every release. The canary starts at 10%.",
             "--option", "eu-west::closest to the traffic peak",
             "--option", "us-east::quietest region right now",
             "--timeout", "90", *ident(RELEASE, KEY_RELEASE), env=e, label="history: region")
    answer_card(c, "1", e)

    # approved -> outcome "approved"
    diff = Path(c.tmp) / "history.diff"
    if not c.dry:
        d = subprocess.run(["git", "-C", str(REPO), "diff", "HEAD~1", "HEAD"],
                           capture_output=True, text=True).stdout
        diff.write_text(d or "diff --git a/x b/x\n--- a/x\n+++ b/x\n@@ -1 +1 @@\n-old\n+new\n")
    c.abx_bg("review", "--title", "Approve the changelog for 2026.7.22?",
             "--body", "Wording only. No code in this one.",
             "--diff-file", str(diff), "--timeout", "90",
             *ident(RELEASE, KEY_RELEASE), env=e, label="history: review")
    answer_card(c, "y", e)

    # left to elapse -> outcome "proceeded"
    # No --json here on purpose: an elapsed veto reads {"answered":false} on the
    # wire, which looks like a failure and is not.
    c.abx("veto", "--in", "4", "--level", "warning",
          "--title", "Pinning the yanked transitive dependency",
          "--body", "Nothing stops this in four seconds.",
          *ident(DEPS, KEY_DEPS), env=e)

    # left to elapse -> outcome "unanswered"
    c.abx("ask", "--title", "Rerun the flaky payment test?",
          "--body", "It failed once and passed twice.",
          "--option", "Rerun it", "--option", "Leave it",
          "--timeout", "6", *ident(TESTS, KEY_TESTS), env=e)
    if not c.dry:
        time.sleep(2.0)


def answer_card(c, key, env, title="=agentbox"):
    """Answer whatever card is up, from the keyboard, and check it went.

    An answered card stays mapped for the grace period before the answer ships,
    so a check that is too quick calls every hit a miss and sends a second
    keystroke into a card that is already answered.
    """
    g = c.wait_window("history", title, env)
    if not g and not c.dry:
        return False
    c.abx("summon", env=env, quiet=True)
    if not c.dry:
        time.sleep(0.4)
    c.drive(f"key {key}\n")
    if c.wait_gone(title, env, seconds=6.0):
        return True
    c.drive("key Return\n")
    return c.wait_gone(title, env, seconds=6.0)


def stage_s9(c, shot):
    """The warning toast. Warning is right: an urgent notify is not a toast at
    all, it becomes a card (toast.go excludes LevelUrgent)."""
    c.abx("notify", "--level", "warning",
          "--title", "Two transitive dependencies moved to a yanked version",
          "--body", "Both are in the release candidate. Nothing has shipped yet.",
          *ident(DEPS, KEY_DEPS), env=c.env_iso)
    return c.wait_window(shot["key"], shot["title"], c.env_iso)


def stage_s10(c, shot):
    """Three bars at once. The window grows downward from the corner, so three
    reports make one window with three bars in it, not three windows."""
    for title, pct, label in [
        ("Reindexing the search catalogue", 64, "rewriting the term index"),
        ("Backfill events.region", 12, "reading rows from events"),
        ("Warming the CDN", 0, "starting"),
    ]:
        p = c.abx_bg("progress", "--title", title, *ident(RELEASE, KEY_RELEASE),
                     env=c.env_iso, stdin=subprocess.PIPE, label=f"progress: {title}")
        if p is not None:
            p.stdin.write(f"{pct} {label}\n")
            p.stdin.flush()
        if not c.dry:
            time.sleep(0.6)
    return c.wait_window(shot["key"], shot["title"], c.env_iso)


def stage_s1(c, shot):
    """The card, restaged so the footer earns its '2 waiting'.

    View.Waiting is len(queue) and the queue excludes the item on screen, so two
    waiting means three pending. An arriving item appends to the queue and the
    current one does not move (advanceLocked only runs when current is nil), so
    the release question is fired FIRST and stays on screen while the other two
    queue behind it. The two behind come from different agents, because the
    footer's dots take one hue per identity.

    The timeout is 120 and the shot is taken three seconds in, which is what
    makes the footer read `expires in 1:57`, exactly as the caption on
    the-card.md says it does.
    """
    fired = time.time()
    c.abx_bg("ask", "--title", "Where should 2026.7.30 go first?",
             "--body", "Tests are green and the changelog is written. The canary starts at "
                       "10% of live traffic wherever we begin.",
             "--option", "eu-west::closest to the traffic peak",
             "--option", "us-east::quietest region right now",
             "--option", "Hold::stay on 2026.7.22",
             "--timeout", "120", *ident(RELEASE, KEY_RELEASE), env=c.env_iso,
             label="S1: the release question")
    g = c.wait_window(shot["key"], shot["title"], c.env_iso)
    if not g and not c.dry:
        return None

    for agent, key, title, body in [
        (TESTS, KEY_TESTS, "The payment suite failed once and passed twice",
         "Rerun it, or take the failure as flaky?"),
        (DEPS, KEY_DEPS, "Pin the yanked dependency or wait for upstream?",
         "Two transitive dependencies moved to a yanked version."),
    ]:
        c.abx_bg("ask", "--title", title, "--body", body,
                 "--option", "Yes", "--option", "No", "--timeout", "600",
                 *ident(agent, key), env=c.env_iso, label=f"S1: queued behind ({agent})")
        if not c.dry:
            time.sleep(0.8)

    # Assert what the footer will say rather than trust it. Three pending, and the
    # release question still the one on screen.
    if not c.dry:
        r = c.abx("pending", "--json", env=c.env_iso, quiet=True)
        try:
            items = json.loads(r.stdout or "{}").get("pending") or []
        except json.JSONDecodeError:
            items = []
        if len(items) != 3:
            c.fail(shot["key"], f"{len(items)} pending, want 3 (one on screen plus two queued, "
                                f"which is what makes the footer read '2 waiting')")
        elif "2026.7.30" not in (items[0].get("title") or ""):
            c.fail(shot["key"], "the release question is not the front item, so the card on "
                                "screen is the wrong one. Check the queue order before retaking.")
        # Take it at 1:57 remaining.
        left = 3.0 - (time.time() - fired)
        if left > 0:
            time.sleep(left)
    return c.where(shot["title"], c.env_iso) or g


def pending_items(c):
    r = c.abx("pending", "--json", env=c.env_iso, quiet=True)
    try:
        return json.loads(r.stdout or "{}").get("pending") or []
    except json.JSONDecodeError:
        return []


def stage_s3(c, shot):
    """The inbox. S1 left three pending and DESIGN wants two rows, so one of the
    queued questions goes. The card on screen goes too: the inbox is the subject
    here and a card over it is not in the spec."""
    if not c.dry:
        items = pending_items(c)
        # A retake of S3 on its own has no S1 in front of it, so the two rows it
        # is a picture of have to be made here. Without this the shot is an inbox
        # with an empty Pending section, which is an argument against the product.
        while len(items) < 2:
            n = len(items)
            agent, key = (RELEASE, KEY_RELEASE) if n == 0 else (TESTS, KEY_TESTS)
            title = ("Where should 2026.7.30 go first?" if n == 0
                     else "The payment suite failed once and passed twice")
            c.abx_bg("ask", "--title", title,
                     "--body", "Tests are green and the changelog is written.",
                     "--option", "eu-west::closest to the traffic peak",
                     "--option", "us-east::quietest region right now",
                     "--timeout", "600", *ident(agent, key), env=c.env_iso,
                     label=f"S3: pending row {n + 1}")
            time.sleep(1.2)
            if len(pending_items(c)) == n:
                c.fail(shot["key"], "a question was fired and nothing became pending")
                break
            items = pending_items(c)
        # Drop the last queued one by id. Never by row position: the queue is
        # shared and a position read a moment ago is not a position now.
        if len(items) > 2:
            victim = items[-1].get("id")
            if victim:
                assert_isolated(c.env_iso)
                c.abx("dismiss", victim, env=c.env_iso)
    # Take the card off the screen without resolving it: shift+Escape dismisses,
    # plain Escape defers and brings it back five minutes later, in the middle of
    # a later shot.
    c.abx("summon", env=c.env_iso, quiet=True)
    c.drive("key shift+Escape\n")
    c.wait_gone("=agentbox", c.env_iso, seconds=4.0)
    c.abx("app", "--tab", "inbox", env=c.env_iso)
    return c.wait_window(shot["key"], shot["title"], c.env_iso)


def stage_s2(c, shot):
    """The agents board.

    The app window is closed and reopened on --tab agents rather than clicking
    the rail. The rail has grown since anybody measured it, and a coordinate read
    off an old screenshot is exactly the mistake that opens somebody else's item.
    """
    c.close_window("=agentbox · app", c.env_iso)
    c.abx("app", "--tab", "agents", env=c.env_iso)
    g = c.wait_window(shot["key"], shot["title"], c.env_iso)
    if not c.dry:
        # The roster paints on mount and the lock queue line arrives with it.
        time.sleep(1.5)
    return g


def stage_s4(c, shot):
    """The review board, on a real change in this repo."""
    # The exit-code constants really are at these lines, which is what makes the
    # step's prose and its highlighted range agree.
    path = "cmd/agentbox/main.go"
    spec = Path(c.tmp) / "review.json"
    if not c.dry:
        spec.write_text(review_spec(path, 30, 45, 36, 40))
    c.close_window("=agentbox · app", c.env_iso)
    c.abx("walkthrough", "create", "--spec", str(spec), env=c.env_iso)
    g = c.wait_window(shot["key"], shot["title"], c.env_iso)
    if not c.dry:
        time.sleep(2.0)   # WebKit, and the board paints its rail after the shell
    c.note("S4: the anchored comment comes from the spec's note, not from a typed "
           "one. DESIGN asks for a comment with text typed into it; typing one means "
           "selecting code at a coordinate, which this script will not do. Check the "
           "frame and type the comment by hand if the note does not carry the shot.")
    return g


def stage_s7(c, shot):
    """The artifact: the canary console, patched to sit mid-track at 50%."""
    src = Path(c.tmp) / "canary-console.jsx"
    if not c.dry:
        src.write_text(staged_artifact_source((REPO / "tools/showcase/console.jsx").read_text()))
    c.close_window("=agentbox · review board", c.env_iso)
    c.abx_bg("show", "--artifact", "--title", "Canary rollout", str(src),
             env=c.env_iso, label="S7: the canary console")
    g = c.wait_window(shot["key"], shot["title"], c.env_iso)
    if not c.dry:
        time.sleep(4.0)   # the window is WebKit and takes a moment to paint
    return g


def stage_s8(c, shot):
    """The reading window, with the watching badge lit, scrolled to where a
    table, a diagram and a highlighted code block are in one frame."""
    c.close_window("=agentbox · Canary rollout", c.env_iso)
    c.abx_bg("show", "--watch", str(REPO / "docs/sample.md"), env=c.env_iso,
             label="S8: the reader")
    g = c.wait_window(shot["key"], shot["title"], c.env_iso)
    if not c.dry:
        time.sleep(3.0)
    # A wheel notch goes wherever the pointer is standing, so the pointer is moved
    # into the window first. `window T` sets the frame and moves nothing. Positive
    # notches scroll down (internal/hand/script.go: "+down").
    c.drive(f"window {shot['title']}\nmove center 45%\nscroll {c.args.viewer_scroll}\n")
    if not c.dry:
        time.sleep(1.0)
    return c.where(shot["title"], c.env_iso) or g


def hold_desktop(c, key):
    """Take the desktop, and wait until the strip is really up.

    `control request` blocks until it is allowed, and --window N is the number of
    seconds after which silence counts as consent, which is what lets this run
    unattended. Exit 0 is granted, 3 is denied.
    """
    if c.held_desktop:
        return True
    if not require_backdrop(c, key):
        return False
    c.abx_bg("control", "request", "staging the wiki screenshots", "--window", "3",
             env=c.env_iso, label="taking the desktop")
    c.held_desktop = True
    return c.wait_window(key, "=agentbox · hands off", c.env_iso) is not None or c.dry


def stage_s5(c, shot):
    """The hands-off strip, amber.

    The activity line's age is rendered by the strip itself, so the shot is taken
    about four seconds after the line is set and reads "· 4s" without anybody
    writing that into the copy.
    """
    c.close_window("=agentbox · sample.md", c.env_iso)
    if not hold_desktop(c, shot["key"]):
        return None
    c.abx("control", "activity", "renaming the staging secret in the console", env=c.env_iso)
    c.held_desktop_line = True
    if not c.dry:
        time.sleep(4.2)
    return c.where(shot["title"], c.env_iso)


def stage_s6(c, shot):
    """The same strip, paused. Green, PAUSED - YOURS, the frozen activity line
    still readable. Same background as S5, because the pair is the aid.

    A retake of S6 alone has to take the desktop first, and then the two frames
    have different backgrounds unless the same windows are on screen. Retake both.
    """
    if not hold_desktop(c, shot["key"]):
        return None
    if not c.held_desktop_line:
        c.abx("control", "activity", "renaming the staging secret in the console",
              env=c.env_iso)
        c.note("S6 was taken without S5 in the same run. The pair only reads if both "
               "frames have the same background, so check them side by side.")
    r = c.abx("control", "pause", env=c.env_iso)
    if r.returncode != 0:
        c.fail(shot["key"], f"control pause exited {r.returncode}: {r.stderr.strip()[:200]}")
        return None
    if not c.dry:
        time.sleep(1.2)
    return c.where(shot["title"], c.env_iso)


def stage_s11(c, shot):
    """The drop-down panel with an inline ask in it.

    This one cannot be staged for real without launching a session: the routing
    gate is inlineRoutable, which needs the item's session to be one the panel is
    showing, and only a session agentbox itself started is. `webui-demo ask panel`
    is the route to that frame, and it needs no daemon, which is why it has a
    phase of its own.
    """
    if not require_backdrop(c, shot["key"]):
        return None
    c.bg_start([AGENTBOX, "webui-demo", "ask", "panel"], label="S11: webui-demo ask panel")
    g = c.wait_window(shot["key"], shot["title"], timeout=max(c.args.timeout, 25))
    if not c.dry:
        time.sleep(3.0)
    c.note("S11: webui-demo carries canned agent names, not the checkout-api fiction. "
           "If a name in frame contradicts the rest of the wiki, this shot needs the "
           "fixture edited or a real session, and neither is this script's job.")
    return c.where(shot["title"]) or g


def stage_s12(c, shot):
    """History and stats, against the REAL store. Read only.

    This is the one shot the design wants real unpruned data for, rehearsal rows
    and all, which the owner decided on 2026-07-25 to leave honest rather than
    tidy. Opening a tab writes nothing. Nothing in this phase dismisses, prunes or
    answers anything.
    """
    c.abx("app", "--tab", "history", env=c.env_real)
    g = c.wait_window(shot["key"], shot["title"], c.env_real)
    if not c.dry:
        time.sleep(2.0)
    c.note("S12: check the frame for a private project name before this goes near the "
           "wiki. DESIGN says if a real project name has to go, the shot is cut rather "
           "than doctored.")
    return g


def require_backdrop(c, key):
    """S5, S6 and S11 need somebody's real work underneath. The strip only makes
    sense over a desktop that is in use, and the panel rolling down over an editor
    is the feature."""
    if c.dry:
        c.step("would check for a backdrop window")
        return True
    g = c.where(c.args.backdrop)
    if g:
        c.step(f"backdrop {c.args.backdrop} at {g}")
        return True
    c.fail(key, f"no window matching {c.args.backdrop!r} to sit over. Open the editor or "
                f"file you want in the background and retake with --only {key}, or pass "
                f"--backdrop with a title that is on screen.")
    return False


STAGERS = {
    "stage_s1": stage_s1, "stage_s2": stage_s2, "stage_s3": stage_s3, "stage_s4": stage_s4,
    "stage_s5": stage_s5, "stage_s6": stage_s6, "stage_s7": stage_s7, "stage_s8": stage_s8,
    "stage_s9": stage_s9, "stage_s10": stage_s10, "stage_s11": stage_s11, "stage_s12": stage_s12,
}


# ---------------------------------------------------------------------------
# phases
# ---------------------------------------------------------------------------

def preflight(c):
    missing = [t for t in ("import", "identify", "convert", "xrandr", "wmctrl") if not shutil.which(t)]
    if missing:
        raise SystemExit(f"missing tools: {', '.join(missing)}. imagemagick, x11-utils and wmctrl.")
    if not Path(AGENTBOX).exists():
        raise SystemExit(f"no agentbox at {AGENTBOX}. Set AGENTBOX_BIN.")

    mons = parse_monitors(subprocess.run(["xrandr", "--listmonitors"],
                                         capture_output=True, text=True).stdout)
    c.mon = pick_monitor(mons, c.args.monitor)
    if not c.mon:
        raise SystemExit("xrandr listed no monitors")
    name, w, h, x, y = c.mon
    c.mon = (x, y, w, h)
    c.mon_name = name

    # Anybody else on this machine loses their cards for the length of the run:
    # the deployed daemon goes down, and a card fired at whatever auto-spawns in
    # its place lands on a daemon that cannot show a window.
    r = c.abx("sync", "agents", "--json", env=c.env_real, quiet=True)
    peers = []
    try:
        data = json.loads(r.stdout or "{}")
        mine = own_pids()
        peers = [a for a in (data.get("agents") or [])
                 if a.get("purpose") and a.get("pid") not in mine]
    except json.JSONDecodeError:
        pass
    if peers:
        c.say(f"    {len(peers)} other session(s) are live on this machine:")
        for p in peers[:8]:
            c.say(f"      {p.get('agent', '?')}  {str(p.get('purpose'))[:60]}")
        if not c.args.force:
            raise SystemExit(
                "refusing to take the daemon down while other sessions are using it.\n"
                "Their cards would land on a daemon with no window for the length of the\n"
                "run. Wait for them, or pass --force if you know they are idle.")

    # Do-not-disturb, on the REAL store, recorded so it can be put back.
    r = c.abx("dnd", "status", env=c.env_real, quiet=True)
    c.dnd_was = parse_dnd(r.stdout)
    c.say(f"    do-not-disturb was: {c.dnd_was or 'unknown, so it will be left alone'}")

    try:
        banners = subprocess.run(["gsettings", "get", "org.gnome.desktop.notifications",
                                  "show-banners"], capture_output=True, text=True).stdout.strip()
    except OSError:
        banners = ""
    if banners == "false":
        c.note("gnome banners are off, which agentbox reads as do-not-disturb: cards will "
               "be held and every card shot will time out. Turn them on first.")


def start_isolated(c):
    """Take the machine's daemon down and put a throwaway one up in its place."""
    c.banner(PHASE_TEXT["isolated"])
    c.phase = "isolated"
    c.tmp = c.tmp or tempfile.mkdtemp(prefix="agentbox-wikishots-")
    state = Path(c.tmp) / "state"
    cfg = Path(c.tmp) / "config" / "agentbox"
    # sync derives an agent's project from its working directory, so the staged
    # sessions are run from a directory with the right name. There is no .git
    # anywhere above it, which is what makes deriveArea fall back to the basename.
    c.project_dir = Path(c.tmp) / PROJECT
    c.say(f"    throwaway state:  {state}")
    c.say(f"    throwaway config: {cfg}")

    if not c.dry:
        state.mkdir(parents=True, exist_ok=True)
        cfg.mkdir(parents=True, exist_ok=True)
        c.project_dir.mkdir(parents=True, exist_ok=True)
        # Start from the owner's config so the theme matches what the product
        # actually looks like, then turn off the one knob that would hold every
        # card. His file is read and never written.
        real_cfg = Path(os.environ.get("XDG_CONFIG_HOME", Path.home() / ".config")) / "agentbox" / "config.toml"
        text = real_cfg.read_text() if real_cfg.exists() else ""
        if re.search(r"^\s*fullscreen_auto_dnd", text, re.M):
            text = re.sub(r"^\s*fullscreen_auto_dnd.*$", "fullscreen_auto_dnd = false", text, flags=re.M)
        else:
            text += "\n[presence]\nfullscreen_auto_dnd = false\n"
        (cfg / "config.toml").write_text(text)

    env = dict(os.environ)
    env["AGENTBOX_INSTANCE"] = INSTANCE
    env["XDG_STATE_HOME"] = str(state)
    env["XDG_CONFIG_HOME"] = str(Path(c.tmp) / "config")
    env.pop("AGENTBOX_SESSION_KEY", None)

    # `make stop` and nothing else. It asks for processes named agentbox, reads
    # each one's own cmdline and kills only daemons, so no session loses the
    # `agentbox mcp` child its tools live in.
    c.sh(["make", "-C", str(REPO), "stop"])
    c.touched_daemon = True
    if not c.dry:
        time.sleep(1.5)
    c.bg_start([AGENTBOX, "daemon"], env=env, label="throwaway daemon")
    c.iso_started = True
    c.env_iso = env

    if not c.dry:
        for _ in range(40):
            if c.abx("status", env=env, quiet=True).returncode == 0:
                break
            time.sleep(0.25)
        else:
            raise SystemExit("the throwaway daemon never answered `status`. Run "
                             "`make restart-daemon` to put the deployed one back.")
    c.abx("dnd", "off", env=env)
    c.say("    throwaway daemon up. It owns the session bus name, so it is the one "
          "showing windows.")


def enter_demo(c):
    """No daemon at all, which is what webui-demo needs.

    `webui-demo` renders nothing while any daemon holds org.wails.agentbox: it
    becomes a remote GApplication instance, logs walkthrough.opened and exits 0
    with no window. So whichever daemon is up has to go, including the deployed one
    when S11 is being retaken on its own.
    """
    c.banner(PHASE_TEXT["demo"])
    c.phase = "demo"
    if c.iso_started:
        stop_isolated(c)
        return
    c.say("    no throwaway daemon in this run, so the deployed one has to stand down "
          "for S11. It comes back in phase 3.")
    c.sh(["make", "-C", str(REPO), "stop"])
    c.touched_daemon = True
    if not c.dry:
        time.sleep(1.5)


def stop_isolated(c):
    """Give up the throwaway daemon so webui-demo can own the bus name."""
    if not c.iso_started:
        return
    assert_isolated(c.env_iso)
    c.abx("dismiss", "--all", env=c.env_iso)
    c.abx("quit", env=c.env_iso)
    kill_bg(c)
    if not c.dry:
        time.sleep(1.5)
    c.iso_started = False


def start_real(c):
    """Put the deployed daemon back. This is also what restores the machine."""
    c.banner(PHASE_TEXT["real"])
    c.phase = "real"
    # webui-demo, or anything else still holding the session bus name, has to go
    # before the deployed daemon can own it.
    kill_bg(c)
    if not c.touched_daemon and c.abx("status", env=c.env_real, quiet=True).returncode == 0:
        # A run of S12 alone never took the daemon down, so there is nothing to
        # restart, and restarting Boris's daemon for a read-only shot would be rude.
        c.say("    the deployed daemon was never taken down; using it as it is.")
        return True
    c.sh(["make", "-C", str(REPO), "restart-daemon"])
    if not c.dry:
        for _ in range(40):
            if c.abx("status", env=c.env_real, quiet=True).returncode == 0:
                break
            time.sleep(0.5)
        else:
            c.fail("S12", "the deployed daemon did not come back; run `make restart-daemon`")
            return False
    c.say("    deployed daemon back. The real store is READ ONLY from here on.")
    return True


def assert_isolated(env):
    """The one guard that matters: a destructive call must be carrying the
    throwaway instance, or it is about to clear the owner's queue."""
    if not env or env.get("AGENTBOX_INSTANCE") != INSTANCE:
        raise SystemExit("refusing a destructive call that is not on the throwaway instance")


def kill_bg(c):
    for p in reversed(c.bg):
        if p is None or p.poll() is not None:
            continue
        try:
            p.terminate()
        except OSError:
            pass
    if not c.dry:
        time.sleep(0.5)
    for p in reversed(c.bg):
        if p is None or p.poll() is not None:
            continue
        try:
            p.kill()
        except OSError:
            pass
    c.bg = []


# ---------------------------------------------------------------------------
# verification
# ---------------------------------------------------------------------------

def verify(c, keys):
    c.banner("VERIFY  every expected file, its size, and whether it looks like a surface")
    bad = 0
    for shot in [s for s in SHOTS if s["key"] in keys]:
        path = c.out_dir / shot["out"]
        if not path.exists():
            print(f"  {shot['key']:<4} {shot['out']:<24} MISSING")
            bad += 1
            continue
        r = subprocess.run(["identify", "-format", "%w %h %k %[fx:standard_deviation]", str(path)],
                           capture_output=True, text=True)
        parts = r.stdout.split()
        if len(parts) != 4:
            print(f"  {shot['key']:<4} {shot['out']:<24} UNREADABLE: {r.stderr.strip()[:80]}")
            bad += 1
            continue
        w, h, colors, sd = int(parts[0]), int(parts[1]), int(parts[2]), float(parts[3])
        ok, why = uniform_verdict(colors, sd)
        print(f"  {shot['key']:<4} {shot['out']:<24} {w}x{h:<6} "
              f"{'ok  ' if ok else 'SUSPECT'} {why}")
        if not ok:
            bad += 1
    if bad:
        print(f"\n  {bad} file(s) missing or suspect. A near-uniform capture is what a missed\n"
              f"  window looks like: the crop landed on the wallpaper. Retake with\n"
              f"  --only <key>, and if it repeats try --capture shell.")
    return bad


# ---------------------------------------------------------------------------
# main
# ---------------------------------------------------------------------------

def print_plan(args):
    print("The twelve wiki shots, in capture order. docs/wiki/DESIGN.md section 5.\n")
    phase = None
    for s in SHOTS:
        if s["phase"] != phase:
            phase = s["phase"]
            print(PHASE_TEXT[phase])
        print(f"  {s['key']:<4} {s['out']:<24} {s['crop']:<7} pad {s['pad']:<4} "
              f"{s['title']}")
        print(f"       {s['what']}")
        print(f"       page: {s['page']}")
    print(f"\nOutput goes to {Path(args.out_dir)}")
    print("S1 writes card-restaged.png and leaves docs/wiki/img/card.png alone. Compare the\n"
          "two, and if the new one has its '2 waiting' footer, move it over card.png by hand.")


def cleanup(c):
    """Safe to abort. Everything this touched, put back, in the order that
    matters, and never with a name-matching kill."""
    print("\n--- cleanup ---", flush=True)
    if c.held_desktop:
        for verb in ("resume", "release", "loud"):
            c.abx("control", verb, env=c.env_iso or c.env_real, quiet=True)
        c.held_desktop = False
    if c.iso_started and c.env_iso:
        try:
            assert_isolated(c.env_iso)
            c.abx("dismiss", "--all", env=c.env_iso, quiet=True)
            c.abx("quit", env=c.env_iso, quiet=True)
        except SystemExit:
            pass
        c.iso_started = False
    kill_bg(c)

    if c.dnd_was and not c.dry:
        r = c.abx("dnd", c.dnd_was, env=c.env_real, quiet=True)
        if r.returncode == 0:
            print(f"  do-not-disturb put back to {c.dnd_was}", flush=True)
        else:
            print(f"  could NOT restore do-not-disturb. Run: agentbox dnd {c.dnd_was}", flush=True)

    if c.tmp and not c.args.keep_tmp:
        shutil.rmtree(c.tmp, ignore_errors=True)
    elif c.tmp:
        print(f"  staging kept at {c.tmp}", flush=True)

    print("\n  To put the machine's daemon back, run this one command:\n\n"
          "      make -C %s restart-daemon\n" % REPO, flush=True)


def main():
    ap = argparse.ArgumentParser(
        description=__doc__.splitlines()[0],
        formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--list", action="store_true", help="print the plan and stop")
    ap.add_argument("--dry-run", action="store_true", help="print every action, touch nothing")
    ap.add_argument("--only", default="", help="shot keys, e.g. S1,S3")
    ap.add_argument("--verify-only", action="store_true", help="re-check the files already there")
    ap.add_argument("--yes", action="store_true",
                    help="required to run: this takes the machine's daemon down")
    ap.add_argument("--force", action="store_true",
                    help="run even though other sessions are live (their cards go dark)")
    ap.add_argument("--out-dir", default=str(OUT_DIR))
    ap.add_argument("--monitor", default="", help="monitor to shoot on (default: the widest)")
    ap.add_argument("--park", default="", help="x,y for the pointer (default: left edge, mid height)")
    ap.add_argument("--backdrop", default="Code",
                    help="title substring of the window S5, S6 and S11 sit over")
    ap.add_argument("--timeout", type=float, default=20.0, help="seconds to wait for a window")
    ap.add_argument("--settle", type=float, default=0.6,
                    help="seconds between a window appearing and its geometry being trusted")
    ap.add_argument("--viewer-scroll", type=int, default=9,
                    help="wheel notches for S8; positive scrolls down, negative up")
    ap.add_argument("--capture", choices=("import", "shell"), default="import",
                    help="shell uses gnome-screenshot, which sees a fullscreen window")
    ap.add_argument("--no-stage-abandoned", dest="stage_abandoned", action="store_false",
                    help="skip S2's abandoned shared claim (avoids a fifth roster row)")
    ap.add_argument("--keep-tmp", action="store_true")
    args = ap.parse_args()

    keys = parse_only(args.only, [s["key"] for s in SHOTS])

    if args.list:
        print_plan(args)
        return 0

    c = Run(args)
    if args.verify_only:
        return 1 if verify(c, keys) else 0

    if not args.yes and not args.dry_run:
        raise SystemExit(
            "This takes the machine's only daemon down for the length of the run, which is\n"
            "how Boris is reachable while he is away from the terminal. Read\n"
            "tools/wiki/SHOTS.md, then pass --yes. --dry-run and --list touch nothing.")

    atexit.register(cleanup, c)
    c.tmp = tempfile.mkdtemp(prefix="agentbox-wikishots-")

    c.banner("PREFLIGHT")
    preflight(c)
    c.park()

    todo = [s for s in SHOTS if s["key"] in keys]
    started = time.time()

    if any(s["phase"] == "isolated" for s in todo):
        start_isolated(c)
        # These two build what S1, S2 and S3 read. Both are cheap and both are
        # skipped when neither shot is in this run.
        if any(s["key"] in ("S1", "S2", "S3") for s in todo):
            stage_roster(c)
        if "S3" in keys:
            stage_history(c)

    for shot in todo:
        if shot["phase"] == "demo" and c.phase != "demo":
            enter_demo(c)
        if shot["phase"] == "real" and c.phase != "real":
            if not start_real(c):
                break
        print(f"\n  {shot['key']}  {shot['what']}", flush=True)
        geom = STAGERS[shot["stage"]](c, shot)
        if geom is None:
            if not c.dry:
                continue
            geom = (100, 100, 800, 600)
        c.capture(shot, geom)

    # Always end with the deployed daemon back, whether or not S12 was in the run.
    if c.phase != "real":
        stop_isolated(c)
        start_real(c)

    if c.dry:
        print("\n  (verify skipped: a dry run writes no files)", flush=True)
        bad = 0
    else:
        bad = verify(c, keys)

    c.banner(f"DONE in {time.time() - started:.0f}s")
    if c.notes:
        print("Notes to read before these go near the wiki:")
        for n in c.notes:
            print(f"  - {n}")
    if c.failures:
        print(f"\n{len(c.failures)} problem(s):")
        for f in c.failures:
            print(f"  {f}")
    return 1 if (c.failures or bad) else 0


if __name__ == "__main__":
    sys.exit(main())
