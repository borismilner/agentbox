#!/usr/bin/env python3
"""Drive agentbox's windows from the outside: send keys and clicks over XTEST.

Why this exists: agentbox's surfaces are only fully checkable on a real desktop
(focus behaviour, WM placement, live input), and this machine has no xdotool.
python3-xlib is installed, which is enough. The inbox keyboard map (FR34) and
the settings panel were both verified with this.

Two traps worth knowing, both learned the hard way:

  * `wmctrl -lG` reports DOUBLED coordinates on a HiDPI display. Take the window
    origin from `xwininfo` instead - that is what `where` below does. Clicks
    computed from wmctrl land in the wrong place and look like "the webview
    ignores the mouse".
  * Window coordinates are relative to the window; XTEST wants root
    coordinates. Add the origin (`--in-window`does it for you).

Usage:
  uidrive.py where                          # print the agentbox window's origin + size
  uidrive.py keys j j 2 Escape              # X keysym names, in order
  uidrive.py click 970 377                  # root coordinates
  uidrive.py click --in-window 600 207      # window coordinates (origin added)
  uidrive.py scroll 6                        # wheel notches down, pointer where it is
  uidrive.py shot /tmp/agentbox.png            # screenshot the window (needs `import`)

  --title NAME   match a different window title (default: exactly "agentbox")
"""

import argparse
import subprocess
import sys
import time

from Xlib import X, XK, display
from Xlib.ext import xtest

DISP = display.Display()


def window_id(title: str) -> str:
    """The X id of the window whose title is exactly `title`."""
    out = subprocess.run(["wmctrl", "-l"], capture_output=True, text=True, check=True).stdout
    ids = [line.split()[0] for line in out.splitlines() if line.split(None, 3)[3:] == [title]]
    if not ids:
        sys.exit(f"no window titled {title!r} (is agentbox running with DISPLAY set?)")
    return ids[-1]


def geometry(title: str) -> tuple[int, int, int, int]:
    """(x, y, w, h) in real root coordinates, via xwininfo - see the docstring."""
    wid = window_id(title)
    out = subprocess.run(["xwininfo", "-id", wid], capture_output=True, text=True, check=True).stdout
    got = {}
    for line in out.splitlines():
        for key, name in (
            ("Absolute upper-left X", "x"),
            ("Absolute upper-left Y", "y"),
            ("Width", "w"),
            ("Height", "h"),
        ):
            if line.strip().startswith(key):
                got[name] = int(line.split(":")[1])
    return got["x"], got["y"], got["w"], got["h"]


def send_key(name: str, settle: float) -> None:
    keysym = XK.string_to_keysym(name)
    if keysym == 0:
        sys.exit(f"unknown keysym {name!r} (try Return, Escape, Down, slash, a…z, 1…9)")
    code = DISP.keysym_to_keycode(keysym)
    if code == 0:
        sys.exit(f"no keycode for {name!r} on this layout")
    xtest.fake_input(DISP, X.KeyPress, code)
    DISP.sync()
    time.sleep(0.03)
    xtest.fake_input(DISP, X.KeyRelease, code)
    DISP.sync()
    time.sleep(settle)


def click(x: int, y: int, settle: float) -> None:
    # Move first and let the pointer settle: the crossing events that follow are
    # what make the webview treat the press as a real click on a real element.
    xtest.fake_input(DISP, X.MotionNotify, x=x, y=y)
    DISP.sync()
    time.sleep(0.2)
    xtest.fake_input(DISP, X.ButtonPress, 1)
    DISP.sync()
    time.sleep(0.08)
    xtest.fake_input(DISP, X.ButtonRelease, 1)
    DISP.sync()
    time.sleep(settle)


def scroll(notches: int, settle: float) -> None:
    button = 5 if notches > 0 else 4
    for _ in range(abs(notches)):
        xtest.fake_input(DISP, X.ButtonPress, button)
        DISP.sync()
        time.sleep(0.05)
        xtest.fake_input(DISP, X.ButtonRelease, button)
        DISP.sync()
        time.sleep(0.15)
    time.sleep(settle)


def main() -> None:
    p = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    p.add_argument("--title", default="agentbox", help="exact window title to target")
    p.add_argument("--settle", type=float, default=0.28, help="seconds to wait after each event")
    sub = p.add_subparsers(dest="cmd", required=True)

    sub.add_parser("where")
    k = sub.add_parser("keys")
    k.add_argument("names", nargs="+")
    c = sub.add_parser("click")
    c.add_argument("--in-window", action="store_true", help="treat x,y as window coordinates")
    c.add_argument("x", type=int)
    c.add_argument("y", type=int)
    s = sub.add_parser("scroll")
    s.add_argument("notches", type=int, help="positive scrolls down")
    sh = sub.add_parser("shot")
    sh.add_argument("path")

    args = p.parse_args()

    if args.cmd == "where":
        x, y, w, h = geometry(args.title)
        print(f"origin {x},{y}  size {w}x{h}")
        return

    if args.cmd == "keys":
        for name in args.names:
            send_key(name, args.settle)
            print("sent", name)
        return

    if args.cmd == "click":
        x, y = args.x, args.y
        if args.in_window:
            ox, oy, _, _ = geometry(args.title)
            x, y = ox + x, oy + y
        click(x, y, args.settle)
        print(f"clicked {x},{y}")
        return

    if args.cmd == "scroll":
        scroll(args.notches, args.settle)
        print(f"scrolled {args.notches}")
        return

    if args.cmd == "shot":
        subprocess.run(["import", "-window", window_id(args.title), args.path], check=True)
        print("wrote", args.path)


if __name__ == "__main__":
    main()
