#!/usr/bin/env python3
"""Draw the wiki's frames instead of photographing them (FR99).

`shots.py` stages real items on a throwaway daemon and photographs real windows
over XTEST. That proves the product looks like that, and it costs a daemon swap
on a live desktop, a script that has to get the desktop into an exact state, and
a frame nobody can review as a diff. Its first run cost eight defects in the
harness before ten frames were fit to publish, and the four still outstanding are
all "the desktop did not happen to be in the state the page wanted".

This draws them instead. A vite server mounts the SHIPPED surfaces with the
daemon door aliased to a fixture (frontend/draw/), and headless chrome
photographs the page. Exact content, exact composition, no desktop, no daemon,
no XTEST, and a mockup that is a file in the repo rather than a photograph.

What it costs is that the pictures stop being evidence. The rule that keeps them
honest, from FR99: a drawn frame is built from the product's own tokens and its
own strings, and may say nothing a real run would contradict. Screenshot-as-proof
stays available in shots.py for the places that need it; SHOTS.md says which.

Usage:
  draw.py                 # every frame in frames.js, into docs/wiki/img/
  draw.py s1 s2           # only these
  draw.py --out /tmp/x    # somewhere else, for a look before committing
  draw.py --keep-serving  # leave the server up and print the URLs, to iterate
"""

import argparse
import json
import os
import re
import shutil
import signal
import subprocess
import sys
import tempfile
import time
from pathlib import Path

REPO = Path(__file__).resolve().parents[2]
FRONTEND = REPO / "frontend"
FRAMES_JS = FRONTEND / "draw" / "frames.js"
DEFAULT_OUT = REPO / "docs" / "wiki" / "img"

CHROME = next(
    (c for c in ("google-chrome", "google-chrome-stable", "chromium", "chromium-browser") if shutil.which(c)),
    None,
)


def frame_specs() -> dict:
    """The frame table, read out of frames.js by executing it in node.

    Parsing it with a regex was the first version and it was wrong within one
    frame: the fixtures hold nested objects, template strings and a comment about
    a brace. node already knows how to read its own module format, and the file
    is a module precisely so it can be imported by both the browser and this.
    """
    src = f"""
    // frames.js derives its identity colours from the product's own hash, and
    // tokens.js reads document.documentElement.dataset.mode to pick the light or
    // dark variant. There is no document in node. Shimming it here rather than
    // teaching tokens.js about a headless caller keeps the harness out of the
    // product: the empty dataset takes the dark branch, which is the mode every
    // wiki frame is drawn in.
    //
    // It is a dynamic import for that reason: a static one is hoisted above this
    // line and frames.js would evaluate its hues before the shim existed.
    globalThis.document ??= {{ documentElement: {{ dataset: {{}} }} }};
    const {{ FRAMES }} = await import({json.dumps(str(FRAMES_JS))});
    const out = {{}};
    for (const [k, v] of Object.entries(FRAMES)) {{
      out[k] = {{ out: v.out ?? (k + ".png"), surface: v.surface, width: v.width,
                  height: v.height, ground: v.ground ?? 24, query: v.query ?? "",
                  desk: v.desk ?? null, ready: v.ready ?? "" }};
    }}
    process.stdout.write(JSON.stringify(out));
    """
    with tempfile.NamedTemporaryFile("w", suffix=".mjs", delete=False) as f:
        f.write(src)
        probe = f.name
    try:
        out = subprocess.run(
            ["node", probe], capture_output=True, text=True, check=True, cwd=FRONTEND
        ).stdout
    finally:
        Path(probe).unlink(missing_ok=True)
    return json.loads(out)


class Server:
    """The vite dev server, up for as long as the drawing takes.

    Started once for every frame rather than per frame: the cost here is the
    module graph, and paying it thirteen times is the same mistake shots.py's
    cost note names about the daemon swap.
    """

    def __init__(self):
        self.proc = None
        self.url = None

    def __enter__(self):
        # Its own process group, because `npx vite` is a wrapper around a child
        # node process: terminating the wrapper alone orphans the server, which
        # then holds its port and keeps running after the script exits. One was
        # found still up an hour after a run.
        self.proc = subprocess.Popen(
            ["npx", "vite", "--config", "draw.config.js"],
            cwd=FRONTEND,
            stdout=subprocess.PIPE,
            stderr=subprocess.STDOUT,
            text=True,
            start_new_session=True,
        )
        # Read the port off vite's own banner rather than assuming 5173, which is
        # taken the moment anybody runs `npm run dev` beside this.
        deadline = time.monotonic() + 90
        while time.monotonic() < deadline:
            line = self.proc.stdout.readline()
            if not line:
                break
            m = re.search(r"(http://127\.0\.0\.1:\d+|http://localhost:\d+)", line)
            if m:
                self.url = m.group(1).replace("localhost", "127.0.0.1")
                return self
        raise SystemExit("vite did not report a URL within 90s")

    def __exit__(self, *exc):
        if not self.proc:
            return
        # Signal the GROUP, so the node child goes with the npx wrapper.
        for sig in (signal.SIGTERM, signal.SIGKILL):
            try:
                os.killpg(self.proc.pid, sig)
            except (ProcessLookupError, PermissionError):
                return
            try:
                self.proc.wait(timeout=10)
                return
            except subprocess.TimeoutExpired:
                continue


class Browser:
    """One headless chrome, driven over the DevTools protocol.

    It used to be one `chrome --headless --screenshot` per frame, with
    `--virtual-time-budget` standing in for "wait until it has settled". That
    flag fast-forwards the page's CLOCK, not its CPU, so the budget is spent in a
    few real milliseconds and anything still working when it runs out is
    photographed unfinished. Every surface survived that; the artifact did not -
    a sandboxed iframe holding half a megabyte of inline React came out an empty
    stage in one run and a working canary console in the next, from the same
    fixture. tools/wiki/shoot.mjs waits for the page to say it is ready instead.
    """

    def __init__(self):
        self.proc = None
        self.profile = None
        self.endpoint = None

    def __enter__(self):
        self.profile = tempfile.mkdtemp(prefix="agentbox-draw-")
        self.proc = subprocess.Popen(
            [
                CHROME,
                "--headless",
                f"--user-data-dir={self.profile}",
                "--no-sandbox",
                "--disable-gpu",
                "--hide-scrollbars",
                # Port 0 means "pick one", which is what keeps two drawings on
                # one machine from fighting over 9222.
                "--remote-debugging-port=0",
                "about:blank",
            ],
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
            start_new_session=True,
        )
        # Chrome writes the port it chose, and the browser's own websocket path,
        # into the profile as soon as it is listening.
        port_file = Path(self.profile) / "DevToolsActivePort"
        deadline = time.monotonic() + 30
        while time.monotonic() < deadline:
            if port_file.exists():
                parts = port_file.read_text().split("\n")
                if len(parts) >= 2 and parts[0].strip():
                    self.endpoint = f"ws://127.0.0.1:{parts[0].strip()}{parts[1].strip()}"
                    return self
            time.sleep(0.1)
        raise SystemExit("chrome did not report a debugging port within 30s")

    def __exit__(self, *exc):
        if self.proc:
            for sig in (signal.SIGTERM, signal.SIGKILL):
                try:
                    os.killpg(self.proc.pid, sig)
                except (ProcessLookupError, PermissionError):
                    break
                try:
                    self.proc.wait(timeout=10)
                    break
                except subprocess.TimeoutExpired:
                    continue
        if self.profile:
            shutil.rmtree(self.profile, ignore_errors=True)

    def shoot(self, url: str, width: int, height: int, out: str, ready: str = "") -> subprocess.CompletedProcess:
        return subprocess.run(
            ["node", str(REPO / "tools" / "wiki" / "shoot.mjs"), url, str(width), str(height), out, ready],
            capture_output=True,
            text=True,
            env={**os.environ, "AGENTBOX_CDP": self.endpoint},
        )

    def measure(self, url: str, width: int, ready: str = "") -> int | None:
        """The height the surface asked Go for, or None if it never asked.

        A frameless surface measures its own laid-out height and hands it to
        bridge.fit(); runtime.js writes that number onto <html data-h>. Reading
        it back is what makes a drawn frame exactly as tall as the window a real
        run would have opened - the alternative is a guessed viewport, and a card
        in a 2000px one stretches to fill it rather than reporting it is short.

        Measured at the real width, because height depends on it: the title and
        the option chips wrap, and a card measured at the wrong width is the
        wrong height for the right reason.
        """
        r = self.shoot(url, width, 2000, "-", ready)
        if r.returncode != 0:
            # Not the same failure as "the surface never measured itself", and
            # saying so matters: one is a fixture to fix, the other a browser.
            sys.exit(f"measuring failed ({r.returncode})\n{r.stderr[-2000:]}")
        return int(r.stdout.strip()) if r.stdout.strip().isdigit() else None


def render_documents() -> None:
    """Re-render the frames' documents with the product's own renderer.

    Three frames show rendered markdown rather than a surface full of fields, and
    in the product that HTML is Go's (internal/webui/mdhtml.go, artifact.go). The
    fixtures hold the markdown; tools/wiki/drawhtml turns it into the HTML a real
    document would carry, so a change to the renderer shows up in the next
    drawing instead of quietly making the frames wrong.

    Its output is committed, so a machine with no Go toolchain still draws - it
    just draws what the last person with one generated.
    """
    if not shutil.which("go"):
        print("no go toolchain; drawing with the committed frontend/draw/rendered.js", file=sys.stderr)
        return
    r = subprocess.run(
        ["go", "run", "./tools/wiki/drawhtml"], cwd=REPO, capture_output=True, text=True
    )
    if r.returncode != 0:
        sys.exit(f"rendering the frames' documents failed ({r.returncode})\n{r.stdout}{r.stderr}")


def surface_url(server_url: str, name: str, spec: dict, *, ground: int, fill: bool, measure: bool = False) -> str:
    """The surface on its own, which is every frame that is cropped to a window.

    A surface can need more of the query string than its own name: the app shell
    reads ?tab= to decide which of its nine surfaces is in front, so a frame of
    the agents board is the app window with tab=agents.
    """
    url = f"{server_url}/draw/index.html?surface={spec['surface']}&frame={name}&ground={ground}"
    if fill:
        url += "&fill=1"
    if measure:
        url += "&measure=1"
    if spec["query"]:
        url += "&" + spec["query"].lstrip("&")
    return url


def desk_url(server_url: str, name: str, spec: dict, box_h: int) -> str:
    """The surface on a desktop, for the frames whose subject is where they sit."""
    desk = spec["desk"]
    url = (
        f"{server_url}/draw/desk.html?surface={spec['surface']}&frame={name}"
        f"&boxw={int(spec['width'])}&boxh={box_h}&place={desk.get('place', 'top')}"
    )
    if spec["query"]:
        url += "&" + spec["query"].lstrip("&")
    return url


def draw(browser: "Browser", server_url: str, name: str, spec: dict, out_dir: Path) -> tuple[Path, str]:
    """Photograph one frame; return where it landed and how its height was decided."""
    fit = spec["height"] == "fit"
    ready = spec["ready"]
    if fit:
        # Measured with no ground, no fill and the page's height constraint
        # lifted, because all three change the answer: a surface free to fill a
        # 2000px measuring viewport reports 2000 rather than what it needs.
        asked = browser.measure(
            surface_url(server_url, name, spec, ground=0, fill=False, measure=True),
            int(spec["width"]),
            ready,
        )
        if asked is None:
            sys.exit(
                f"{name}: surface {spec['surface']!r} never called fit(), so its height "
                f"cannot be taken from the product. Give the frame an explicit height."
            )
        box_h, how = asked, f"fit({asked})"
    else:
        box_h, how = int(spec["height"]), "declared"

    if spec["desk"]:
        # The picture is the piece of screen, and the surface inside it is the
        # window Go opens - so the viewport is the desk, not the surface.
        width = int(spec["desk"]["width"])
        viewport_h = int(spec["desk"]["height"])
        url = desk_url(server_url, name, spec, box_h)
        how += f", on {width}x{viewport_h} of desktop"
    else:
        ground = int(spec["ground"])
        width = int(spec["width"]) + 2 * ground
        viewport_h = box_h + 2 * ground
        url = surface_url(server_url, name, spec, ground=ground, fill=not fit)

    out = out_dir / spec["out"]
    out.parent.mkdir(parents=True, exist_ok=True)
    # A desk frame's readiness lives in the iframe, which this cannot see from
    # the wrapper page, so the wrapper only ever waits for its own paint.
    r = browser.shoot(url, width, viewport_h, str(out), "" if spec["desk"] else ready)
    if r.returncode != 0 or not out.exists():
        sys.exit(f"{name}: capture failed ({r.returncode})\n{r.stderr[-2000:]}")
    squeeze(out)
    return out, how


def squeeze(path: Path) -> None:
    """Losslessly re-compress a frame, if the machine can.

    The three desk frames are mostly wallpaper, and chrome dithers a gradient, so
    they land two to five times the size of a frame cropped to a window. These
    are read over a wiki, and optipng takes about a third off without touching a
    pixel - it is a re-encode, so a redraw is still byte-identical to itself.
    Skipped silently when it is not installed: a slightly heavier frame is not
    worth failing a drawing over.
    """
    if shutil.which("optipng"):
        subprocess.run(["optipng", "-o2", "-quiet", str(path)], capture_output=True)


def main() -> None:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("frames", nargs="*", help="frame ids; default is all of them")
    ap.add_argument("--out", type=Path, default=DEFAULT_OUT)
    ap.add_argument("--keep-serving", action="store_true", help="print URLs and wait, to iterate by hand")
    args = ap.parse_args()

    if CHROME is None:
        sys.exit("no chrome found; draw.py needs google-chrome or chromium on PATH")
    if not (FRONTEND / "node_modules").is_dir():
        sys.exit("frontend/node_modules is missing; run `npm install` in frontend/ first")
    render_documents()

    specs = frame_specs()
    wanted = args.frames or list(specs)
    unknown = [f for f in wanted if f not in specs]
    if unknown:
        sys.exit(f"no such frame: {', '.join(unknown)} (have: {', '.join(specs)})")

    with Server() as server:
        if args.keep_serving:
            for name, spec in specs.items():
                if spec["desk"]:
                    h = spec["height"] if spec["height"] != "fit" else 200
                    print(f"{name}: {desk_url(server.url, name, spec, int(h))}")
                else:
                    fill = spec["height"] != "fit"
                    print(f"{name}: {surface_url(server.url, name, spec, ground=int(spec['ground']), fill=fill)}")
            print("\nserving; Ctrl-C to stop")
            try:
                server.proc.wait()
            except KeyboardInterrupt:
                pass
            return

        with Browser() as browser:
            for name in wanted:
                out, how = draw(browser, server.url, name, specs[name], args.out)
                size = out.stat().st_size
                print(f"{name:12} {out}  ({size // 1024} kB, height {how})")

    print("\nRead every frame before publishing. A drawn frame cannot be wrong about")
    print("the desktop, but it can be wrong about the product, and only a person")
    print("comparing it against a real surface will notice.")


if __name__ == "__main__":
    main()
