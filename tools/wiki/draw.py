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
import re
import shutil
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
                  height: v.height, ground: v.ground ?? 24, query: v.query ?? "" }};
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
        self.proc = subprocess.Popen(
            ["npx", "vite", "--config", "draw.config.js"],
            cwd=FRONTEND,
            stdout=subprocess.PIPE,
            stderr=subprocess.STDOUT,
            text=True,
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
        if self.proc:
            self.proc.terminate()
            try:
                self.proc.wait(timeout=10)
            except subprocess.TimeoutExpired:
                self.proc.kill()


def chrome(url: str, width: int, height: int, *extra: str) -> subprocess.CompletedProcess:
    with tempfile.TemporaryDirectory() as profile:
        return subprocess.run(
            [
                CHROME,
                "--headless",
                f"--user-data-dir={profile}",
                "--no-sandbox",
                "--disable-gpu",
                "--hide-scrollbars",
                # The frames are for a wiki read on ordinary displays; 2x keeps
                # text crisp when the page scales the image down, which is what
                # the published pages do.
                "--force-device-scale-factor=2",
                f"--window-size={width},{height}",
                # The surfaces measure themselves and a ResizeObserver settles a
                # frame or two after mount; runtime.js sets data-drawn when it
                # has. This is the ceiling on that wait, not the wait itself.
                "--virtual-time-budget=8000",
                *extra,
                url,
            ],
            capture_output=True,
            text=True,
        )


def measure(url: str, width: int) -> int | None:
    """The height the surface asked Go for, or None if it never asked.

    A frameless surface measures its own laid-out height and hands it to
    bridge.fit(); runtime.js writes that number onto <html data-h>. Reading it
    back is what makes a drawn frame exactly as tall as the window a real run
    would have opened - the alternative is a guessed viewport, and a card in a
    2000px one stretches to fill it rather than reporting that it is short.

    Measured at the real width, because height depends on it: the title and the
    option chips wrap, and a card measured at the wrong width is the wrong height
    for the right reason.
    """
    r = chrome(url, width, 2000, "--dump-dom")
    if r.returncode != 0:
        # Not the same failure as "the surface never measured itself", and saying
        # so matters: one is a fixture to fix and the other is a browser to fix.
        sys.exit(f"chrome failed while measuring ({r.returncode})\n{r.stderr[-2000:]}")
    m = re.search(r'data-h="(\d+)"', r.stdout)
    return int(m.group(1)) if m else None


def draw(server_url: str, name: str, spec: dict, out_dir: Path) -> tuple[Path, str]:
    """Photograph one frame; return where it landed and how its height was decided."""
    ground = int(spec["ground"])
    width = int(spec["width"]) + 2 * ground
    # A surface can need more of the query string than its own name: the app
    # shell reads ?tab= to decide which of its nine surfaces is in front, and a
    # frame of the agents board is the app window with tab=agents.
    url = f"{server_url}/draw/index.html?surface={spec['surface']}&frame={name}"
    if spec["query"]:
        url += "&" + spec["query"].lstrip("&")

    if spec["height"] == "fit":
        asked = measure(url, width)
        if asked is None:
            sys.exit(
                f"{name}: surface {spec['surface']!r} never called fit(), so its height "
                f"cannot be taken from the product. Give the frame an explicit height."
            )
        viewport_h, how = asked + 2 * ground, f"fit({asked})"
    else:
        viewport_h, how = int(spec["height"]) + 2 * ground, "declared"

    out = out_dir / spec["out"]
    out.parent.mkdir(parents=True, exist_ok=True)
    r = chrome(url, width, viewport_h, f"--screenshot={out}")
    if r.returncode != 0 or not out.exists():
        sys.exit(f"{name}: chrome failed ({r.returncode})\n{r.stderr[-2000:]}")
    return out, how


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

    specs = frame_specs()
    wanted = args.frames or list(specs)
    unknown = [f for f in wanted if f not in specs]
    if unknown:
        sys.exit(f"no such frame: {', '.join(unknown)} (have: {', '.join(specs)})")

    with Server() as server:
        if args.keep_serving:
            for name, spec in specs.items():
                q = ("&" + spec["query"].lstrip("&")) if spec["query"] else ""
                print(f"{name}: {server.url}/draw/index.html?surface={spec['surface']}&frame={name}{q}")
            print("\nserving; Ctrl-C to stop")
            try:
                server.proc.wait()
            except KeyboardInterrupt:
                pass
            return

        for name in wanted:
            out, how = draw(server.url, name, specs[name], args.out)
            size = out.stat().st_size
            print(f"{name:12} {out}  ({size // 1024} kB, height {how})")

    print("\nRead every frame before publishing. A drawn frame cannot be wrong about")
    print("the desktop, but it can be wrong about the product, and only a person")
    print("comparing it against a real surface will notice.")


if __name__ == "__main__":
    main()
