#!/usr/bin/env python3
"""Tests for the parts of shots.py that do not need a desktop.

    python3 -m unittest discover -s tools/wiki -p 'test_*.py' -v

The staging itself cannot be tested here: it needs the daemon swap, and the swap
takes down the one daemon Boris is reachable through. What IS tested is
everything a wrong answer in would waste a whole sitting on: the geometry
arithmetic, the parsers that read `drive where` and `xrandr`, the check that says
a capture is really a picture of a surface, and the argument handling.
"""

import json
import os
import shutil
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
import shots  # noqa: E402

MON = (0, 0, 1920, 1200)          # one monitor at the origin
MON2 = (1920, 0, 2560, 1440)      # a second one to its right


class TestParseWhere(unittest.TestCase):
    def test_four_numbers(self):
        self.assertEqual(shots.parse_where("745 48 430 78"), (745, 48, 430, 78))

    def test_trailing_newline(self):
        self.assertEqual(shots.parse_where("0 0 1920 1200\n"), (0, 0, 1920, 1200))

    def test_negative_origin(self):
        self.assertEqual(shots.parse_where("-12 -4 100 200"), (-12, -4, 100, 200))

    def test_no_window_matched(self):
        for bad in ("", "\n", "no window on screen matches", "1 2 3", "1 2 3 4 5", "a b c d"):
            self.assertIsNone(shots.parse_where(bad), bad)

    def test_none(self):
        self.assertIsNone(shots.parse_where(None))


class TestParseMonitors(unittest.TestCase):
    ONE = "Monitors: 1\n 0: +*eDP-1 1920/301x1200/188+0+0  eDP-1\n"
    TWO = ("Monitors: 2\n"
           " 0: +*eDP-1 1920/301x1200/188+0+0  eDP-1\n"
           " 1: +HDMI-1 2560/600x1440/340+1920+0  HDMI-1\n")

    def test_one(self):
        self.assertEqual(shots.parse_monitors(self.ONE), [("eDP-1", 1920, 1200, 0, 0)])

    def test_two(self):
        self.assertEqual(shots.parse_monitors(self.TWO),
                         [("eDP-1", 1920, 1200, 0, 0), ("HDMI-1", 2560, 1440, 1920, 0)])

    def test_junk(self):
        self.assertEqual(shots.parse_monitors("Monitors: 0\n"), [])
        self.assertEqual(shots.parse_monitors(""), [])

    def test_widest_wins_by_default(self):
        mons = shots.parse_monitors(self.TWO)
        self.assertEqual(shots.pick_monitor(mons)[0], "HDMI-1")

    def test_named(self):
        mons = shots.parse_monitors(self.TWO)
        self.assertEqual(shots.pick_monitor(mons, "eDP-1")[0], "eDP-1")

    def test_named_missing_is_loud(self):
        mons = shots.parse_monitors(self.TWO)
        with self.assertRaises(SystemExit):
            shots.pick_monitor(mons, "DP-9")


class TestCropWindow(unittest.TestCase):
    def test_pad_all_four_sides(self):
        # S1: the card plus about 24px of dark desktop so the shadow reads.
        self.assertEqual(shots.crop_rect("window", (700, 400, 520, 300), MON, 24),
                         (676, 376, 568, 348))

    def test_no_pad_is_the_window(self):
        self.assertEqual(shots.crop_rect("window", (100, 100, 800, 600), MON, 0),
                         (100, 100, 800, 600))

    def test_pad_clamped_at_the_left_edge(self):
        # A window at x=10 with 24px of pad cannot have 24px of desktop to its
        # left. The crop starts at the monitor edge and loses the width it did
        # not get, rather than sliding right and framing the wrong thing.
        x, y, w, h = shots.crop_rect("window", (10, 400, 520, 300), MON, 24)
        self.assertEqual((x, y), (0, 376))
        self.assertEqual(w, 520 + 24 + 10)

    def test_pad_clamped_at_the_right_edge(self):
        x, y, w, h = shots.crop_rect("window", (1400, 400, 500, 300), MON, 24)
        self.assertEqual(x, 1376)
        self.assertEqual(x + w, 1920)

    def test_pad_clamped_at_the_bottom(self):
        x, y, w, h = shots.crop_rect("window", (100, 1100, 400, 90), MON, 40)
        self.assertEqual(y + h, 1200)

    def test_stays_inside_a_second_monitor(self):
        x, y, w, h = shots.crop_rect("window", (1930, 20, 400, 300), MON2, 40)
        self.assertEqual(x, 1920)
        self.assertEqual(y, 0)
        self.assertLessEqual(x + w, 1920 + 2560)


class TestCropTop(unittest.TestCase):
    def test_keeps_the_top_edge_in_frame(self):
        # S9: the strip plus the top edge of the screen, so its position is part
        # of the information, and a little desktop below it.
        x, y, w, h = shots.crop_rect("top", (745, 48, 430, 78), MON, 48)
        self.assertEqual(y, 0, "the monitor's top edge must be in frame")
        self.assertEqual(h, 48 + 78 + 48)
        self.assertEqual(x, 745 - 48)
        self.assertEqual(w, 430 + 96)

    def test_top_on_the_second_monitor_uses_its_own_top(self):
        x, y, w, h = shots.crop_rect("top", (2000, 60, 430, 78), MON2, 48)
        self.assertEqual(y, 0)

    def test_wide_pad_clamps_to_the_monitor_width(self):
        # S5 and S6 want a lot of desktop context around the strip.
        x, y, w, h = shots.crop_rect("top", (745, 48, 430, 78), MON, 260)
        self.assertEqual((x, y), (485, 0))
        self.assertEqual(w, 430 + 520)
        self.assertEqual(h, 48 + 78 + 260)

    def test_pad_wider_than_the_screen(self):
        x, y, w, h = shots.crop_rect("top", (900, 40, 200, 60), MON, 2000)
        self.assertEqual((x, y), (0, 0))
        self.assertEqual(w, 1920)
        self.assertEqual(h, 1200)


class TestCropCorner(unittest.TestCase):
    def test_reaches_the_bottom_right_corner(self):
        # S10: the corner is the point of the shot, so cropping tight would
        # destroy it.
        x, y, w, h = shots.crop_rect("corner", (1500, 900, 380, 260), MON, 140)
        self.assertEqual((x, y), (1360, 760))
        self.assertEqual(x + w, 1920, "the right edge of the monitor must be in frame")
        self.assertEqual(y + h, 1200, "the bottom edge of the monitor must be in frame")

    def test_corner_on_the_second_monitor(self):
        x, y, w, h = shots.crop_rect("corner", (4300, 1200, 160, 200), MON2, 100)
        self.assertEqual(x + w, 1920 + 2560)
        self.assertEqual(y + h, 1440)

    def test_pad_clamped_at_the_left(self):
        x, y, w, h = shots.crop_rect("corner", (40, 900, 380, 260), MON, 140)
        self.assertEqual(x, 0)
        self.assertEqual(x + w, 1920)


class TestCropRefusals(unittest.TestCase):
    def test_unknown_mode(self):
        with self.assertRaises(ValueError):
            shots.crop_rect("centre-ish", (0, 0, 100, 100), MON, 0)

    def test_a_window_hand_would_have_refused(self):
        # `hand` refuses a window under 16 pixels, so a window still animating
        # reads as absent. If one gets this far, the crop is not a picture of
        # anything and must say so rather than write a 4x4 png.
        with self.assertRaises(ValueError):
            shots.crop_rect("window", (100, 100, 4, 4), MON, 0)

    def test_a_window_entirely_off_this_monitor(self):
        with self.assertRaises(ValueError):
            shots.crop_rect("window", (3000, 100, 400, 300), MON, 0)


class TestUniformVerdict(unittest.TestCase):
    def test_solid_colour_is_a_missed_window(self):
        ok, why = shots.uniform_verdict(1, 0.0)
        self.assertFalse(ok)
        self.assertIn("solid", why)

    def test_near_uniform_is_suspect(self):
        ok, why = shots.uniform_verdict(4000, 0.004)
        self.assertFalse(ok)
        self.assertIn("near-uniform", why)

    def test_too_few_colours_is_suspect(self):
        ok, _ = shots.uniform_verdict(8, 0.4)
        self.assertFalse(ok)

    def test_a_rendered_surface_passes(self):
        ok, why = shots.uniform_verdict(21000, 0.19)
        self.assertTrue(ok)
        self.assertIn("21000", why)


@unittest.skipUnless(shutil.which("identify") and (shutil.which("convert") or shutil.which("magick")),
                     "imagemagick not installed")
class TestUniformityAgainstRealFiles(unittest.TestCase):
    """The verdict is only worth anything if it agrees with what identify says
    about a real file, so these generate the two failure modes and read them back
    through the same command shots.py uses."""

    @classmethod
    def setUpClass(cls):
        cls.tmp = Path(tempfile.mkdtemp(prefix="wikishots-test-"))
        cls.gen = ([shutil.which("convert")] if shutil.which("convert")
                   else [shutil.which("magick")])

    @classmethod
    def tearDownClass(cls):
        shutil.rmtree(cls.tmp, ignore_errors=True)

    def read(self, path):
        out = subprocess.run(["identify", "-format", "%w %h %k %[fx:standard_deviation]", str(path)],
                             capture_output=True, text=True, check=True).stdout.split()
        return int(out[0]), int(out[1]), int(out[2]), float(out[3])

    def make(self, name, *args):
        p = self.tmp / name
        subprocess.run(self.gen + list(args) + [str(p)], check=True, capture_output=True)
        return p

    def test_a_solid_capture_is_caught(self):
        # This is what a crop of an empty desktop, or of a window that never
        # painted, actually looks like on disk.
        p = self.make("solid.png", "-size", "600x400", "xc:#1b1b1f")
        w, h, colors, sd = self.read(p)
        self.assertEqual((w, h), (600, 400))
        ok, why = shots.uniform_verdict(colors, sd)
        self.assertFalse(ok, f"a solid PNG passed the check: {why}")

    def test_a_plain_wallpaper_gradient_is_caught_or_flagged(self):
        p = self.make("grad.png", "-size", "600x400", "gradient:#101014-#16161c")
        _, _, colors, sd = self.read(p)
        ok, _ = shots.uniform_verdict(colors, sd)
        self.assertFalse(ok, "a near-flat gradient should not read as a surface")

    def test_something_with_content_passes(self):
        p = self.make("noise.png", "-size", "600x400", "xc:#1b1b1f",
                      "+noise", "Random")
        _, _, colors, sd = self.read(p)
        ok, why = shots.uniform_verdict(colors, sd)
        self.assertTrue(ok, f"a busy image was called suspect: {why}")


class TestParseDnd(unittest.TestCase):
    def test_the_real_wording(self):
        # `agentbox dnd status` prints exactly this.
        self.assertEqual(shots.parse_dnd("do not disturb: off\n"), "off")
        self.assertEqual(shots.parse_dnd("do not disturb: on\n"), "on")

    def test_it_reads_the_side_after_the_colon(self):
        # "do not disturb" is full of letters that a substring search for on/off
        # trips over. Only the value counts.
        self.assertEqual(shots.parse_dnd("do-not-disturb: off"), "off")
        self.assertEqual(shots.parse_dnd("notifications: on"), "on")

    def test_unknown_is_empty_so_nothing_is_changed(self):
        for bad in ("", None, "daemon not running", "do not disturb"):
            self.assertEqual(shots.parse_dnd(bad), "", repr(bad))

    def test_the_value_is_a_verb_the_cli_takes(self):
        # It is handed straight back to `agentbox dnd <value>`, which takes only
        # on, off or status.
        for text in ("do not disturb: on", "do not disturb: off"):
            self.assertIn(shots.parse_dnd(text), ("on", "off"))


class TestOwnPids(unittest.TestCase):
    def test_includes_this_process_and_its_parent(self):
        pids = shots.own_pids()
        self.assertIn(os.getpid(), pids)
        self.assertIn(os.getppid(), pids)

    def test_terminates(self):
        # A walk up /proc that loops would hang the preflight.
        self.assertLess(len(shots.own_pids()), 40)

    def test_a_stranger_is_not_in_it(self):
        self.assertNotIn(999999, shots.own_pids())


class TestParseOnly(unittest.TestCase):
    KEYS = [s["key"] for s in shots.SHOTS]

    def test_empty_is_everything_in_plan_order(self):
        self.assertEqual(shots.parse_only("", self.KEYS), self.KEYS)

    def test_subset_comes_back_in_plan_order_not_argument_order(self):
        self.assertEqual(shots.parse_only("S3,S1", self.KEYS), ["S1", "S3"])

    def test_lowercase_and_spaces(self):
        self.assertEqual(shots.parse_only("s1 s9", self.KEYS), ["S9", "S1"])

    def test_unknown_is_loud(self):
        with self.assertRaises(SystemExit) as e:
            shots.parse_only("S1,S13", self.KEYS)
        self.assertIn("S13", str(e.exception))

    def test_single(self):
        self.assertEqual(shots.parse_only("S12", self.KEYS), ["S12"])


class TestPlanIntegrity(unittest.TestCase):
    def test_twelve_shots(self):
        self.assertEqual(len(shots.SHOTS), 12)

    def test_keys_are_s1_to_s12_with_no_duplicates(self):
        keys = [s["key"] for s in shots.SHOTS]
        self.assertEqual(sorted(keys, key=lambda k: int(k[1:])), [f"S{i}" for i in range(1, 13)])

    def test_filenames_are_lowercase_hyphenated_png(self):
        for s in shots.SHOTS:
            self.assertRegex(s["out"], r"^[a-z0-9]+(-[a-z0-9]+)*\.png$", s["key"])

    def test_filenames_are_unique(self):
        names = [s["out"] for s in shots.SHOTS]
        self.assertEqual(len(names), len(set(names)))

    def test_s1_does_not_overwrite_the_existing_card(self):
        # The existing docs/wiki/img/card.png is correctly staged apart from its
        # footer. It stays untouched until a human has looked at the new one.
        s1 = next(s for s in shots.SHOTS if s["key"] == "S1")
        self.assertNotEqual(s1["out"], "card.png")

    def test_every_shot_has_a_stager(self):
        for s in shots.SHOTS:
            self.assertIn(s["stage"], shots.STAGERS, s["key"])

    def test_every_stager_is_used(self):
        used = {s["stage"] for s in shots.SHOTS}
        self.assertEqual(used, set(shots.STAGERS))

    def test_phases_are_known_and_grouped(self):
        seen = []
        for s in shots.SHOTS:
            self.assertIn(s["phase"], shots.PHASE_TEXT)
            if not seen or seen[-1] != s["phase"]:
                seen.append(s["phase"])
        self.assertEqual(seen, ["isolated", "demo", "real"],
                         "phases must run in one block each: the daemon swap is the expensive part")

    def test_crop_modes_are_known(self):
        for s in shots.SHOTS:
            self.assertIn(s["crop"], ("window", "top", "corner"), s["key"])

    def test_the_toast_comes_before_the_hands_off_strip(self):
        # The strip pins itself to the top of the same top-centre column, so it
        # would be in the toast's frame if it went up first.
        order = [s["key"] for s in shots.SHOTS]
        self.assertLess(order.index("S9"), order.index("S5"))

    def test_the_card_comes_before_the_inbox(self):
        # S1 leaves the pending rows S3 needs.
        order = [s["key"] for s in shots.SHOTS]
        self.assertLess(order.index("S1"), order.index("S3"))

    def test_the_real_phase_is_last(self):
        self.assertEqual(shots.SHOTS[-1]["phase"], "real")

    def test_the_review_board_is_matched_as_a_substring(self):
        # The board titles itself "agentbox · review board · <walkthrough title>",
        # so an exact match (a leading =) finds nothing and the shot times out.
        s4 = next(s for s in shots.SHOTS if s["key"] == "S4")
        self.assertFalse(s4["title"].startswith("="))
        self.assertIn("review board", s4["title"])

    def test_the_hands_off_pair_matches_the_strip_and_not_its_marker(self):
        # There is also a window called "agentbox · hands off marker", so these two
        # have to be exact or they can land on the wrong one.
        for key in ("S5", "S6"):
            s = next(x for x in shots.SHOTS if x["key"] == key)
            self.assertEqual(s["title"], "=agentbox · hands off", key)

    def test_the_pair_shares_a_crop_mode_and_pad(self):
        s5, s6 = (next(x for x in shots.SHOTS if x["key"] == k) for k in ("S5", "S6"))
        self.assertEqual((s5["crop"], s5["pad"]), (s6["crop"], s6["pad"]),
                         "the pair is the aid; different framing breaks it")

    def test_shots_that_want_desktop_context_do_not_crop_tight(self):
        for key in ("S5", "S6", "S9", "S10"):
            s = next(x for x in shots.SHOTS if x["key"] == key)
            self.assertIn(s["crop"], ("top", "corner"), key)
            self.assertGreater(s["pad"], 0, key)

    def test_window_only_shots_have_no_pad_except_the_card(self):
        # DESIGN says "window only, no desktop" for these. S1 is the exception: it
        # asks for about 24px so the shadow reads.
        for key in ("S2", "S3", "S4", "S7", "S8", "S11", "S12"):
            s = next(x for x in shots.SHOTS if x["key"] == key)
            self.assertEqual((s["crop"], s["pad"]), ("window", 0), key)
        s1 = next(x for x in shots.SHOTS if x["key"] == "S1")
        self.assertEqual((s1["crop"], s1["pad"]), ("window", 24))


class TestArtifactPatch(unittest.TestCase):
    def test_patches_the_slider_and_the_release(self):
        src = "const [percent, setPercent] = useState(10);\n release 2026.7.3 · canary rollout\n"
        out = shots.staged_artifact_source(src)
        self.assertIn("useState(50)", out)
        self.assertIn("release 2026.7.30 ·", out)
        self.assertNotIn("useState(10)", out)

    def test_refuses_when_the_source_has_moved_on(self):
        # If console.jsx is rewritten, the patch must fail rather than quietly
        # produce an artifact with the slider in the wrong place.
        with self.assertRaises(SystemExit):
            shots.staged_artifact_source("nothing to patch here")

    def test_the_real_console_still_patches(self):
        p = shots.REPO / "tools/showcase/console.jsx"
        if not p.exists():
            self.skipTest("console.jsx is gone")
        out = shots.staged_artifact_source(p.read_text())
        self.assertIn("useState(50)", out)
        self.assertIn("2026.7.30", out)
        self.assertIn("Start the rollout", out)
        self.assertIn("Hold it", out)


class TestReviewSpec(unittest.TestCase):
    def test_is_valid_json_with_the_shape_the_board_reads(self):
        spec = json.loads(shots.review_spec("cmd/agentbox/main.go", 100, 140, 108, 114))
        self.assertEqual(spec["version"], 1)
        self.assertEqual(spec["repo_root"], ".")
        kinds = [s["kind"] for s in spec["steps"]]
        self.assertEqual(kinds, ["ground", "code", "check"])
        code = spec["steps"][1]["code"][0]
        self.assertEqual(code["path"], "cmd/agentbox/main.go")
        self.assertEqual(code["lines"], [100, 140])
        self.assertEqual(code["notes"][0]["at"], [108, 114])

    def test_the_note_range_is_inside_the_shown_range(self):
        # A note anchored outside the lines on screen is a highlighted range
        # nobody can see, which is the whole subject of S4.
        spec = json.loads(shots.review_spec("x.go", 100, 140, 108, 114))
        code = spec["steps"][1]["code"][0]
        lo, hi = code["lines"]
        a, b = code["notes"][0]["at"]
        self.assertGreaterEqual(a, lo)
        self.assertLessEqual(b, hi)

    def test_the_cited_file_and_lines_exist(self):
        path = shots.REPO / "cmd/agentbox/main.go"
        if not path.exists():
            self.skipTest("main.go is gone")
        self.assertGreater(len(path.read_text().splitlines()), 140,
                           "S4 cites lines 100-140 of main.go; the file is shorter than that now")


class TestCommandLine(unittest.TestCase):
    """The script is run as a subprocess so the argument handling is exercised
    the way a human exercises it, not by calling main() in process."""

    def run_it(self, *args):
        return subprocess.run([sys.executable, str(Path(shots.__file__)), *args],
                              capture_output=True, text=True)

    def test_list_prints_twelve_and_touches_nothing(self):
        r = self.run_it("--list")
        self.assertEqual(r.returncode, 0, r.stderr)
        for s in shots.SHOTS:
            self.assertIn(s["key"], r.stdout)
            self.assertIn(s["out"], r.stdout)
        self.assertIn("card-restaged.png", r.stdout)
        self.assertIn("leaves docs/wiki/img/card.png alone", r.stdout)

    def test_list_names_all_three_phases(self):
        r = self.run_it("--list")
        for phase in ("ISOLATED", "DEMO", "REAL"):
            self.assertIn(phase, r.stdout)

    def test_list_with_only_still_prints_the_whole_plan(self):
        r = self.run_it("--list", "--only", "S1")
        self.assertEqual(r.returncode, 0)
        self.assertIn("S12", r.stdout)

    def test_bad_only_fails_before_anything_happens(self):
        r = self.run_it("--only", "S99", "--list")
        self.assertNotEqual(r.returncode, 0)
        self.assertIn("S99", r.stdout + r.stderr)

    def test_running_without_yes_refuses_and_says_why(self):
        r = self.run_it("--only", "S1")
        self.assertNotEqual(r.returncode, 0)
        self.assertIn("--yes", r.stdout + r.stderr)
        self.assertIn("daemon", r.stdout + r.stderr)

    def test_help_works(self):
        r = self.run_it("--help")
        self.assertEqual(r.returncode, 0)
        self.assertIn("--only", r.stdout)
        self.assertIn("--verify-only", r.stdout)


class TestSafetyGuard(unittest.TestCase):
    def test_a_destructive_call_on_the_default_instance_is_refused(self):
        # dismiss --all on the deployed instance would clear Boris's queue. The
        # instance name is the belt: it moves both the socket and the store.
        for env in ({}, {"AGENTBOX_INSTANCE": ""}, {"AGENTBOX_INSTANCE": "dev"}, None):
            with self.assertRaises(SystemExit):
                shots.assert_isolated(env)

    def test_the_throwaway_instance_is_allowed(self):
        shots.assert_isolated({"AGENTBOX_INSTANCE": shots.INSTANCE})

    def test_the_instance_name_is_not_the_default(self):
        self.assertNotIn(shots.INSTANCE, ("", "dev"))

    def test_no_pkill_anywhere_in_the_script(self):
        # pkill agentbox kills the `agentbox mcp` child every Claude session
        # holds, and the -f form has killed the invoking shell. Comments and
        # docstrings are allowed to name it, since that is where the reason lives;
        # code and string literals that could reach a shell are not, so the check
        # reads tokens rather than lines.
        import io
        import token as T
        import tokenize
        src = Path(shots.__file__).read_text()
        for tok in tokenize.generate_tokens(io.StringIO(src).readline):
            if tok.type in (T.COMMENT, T.NL, T.NEWLINE, T.INDENT, T.DEDENT):
                continue
            if tok.type == T.STRING and tok.line.lstrip().startswith(('"""', "'''")):
                continue   # a docstring, which is prose
            for banned in ("pkill", "killall"):
                self.assertNotIn(banned, tok.string,
                                 f"{banned} must never appear in code or in a string")

    def test_the_daemon_is_only_ever_stopped_through_make(self):
        text = Path(shots.__file__).read_text()
        self.assertIn('"make", "-C", str(REPO), "stop"', text)
        self.assertIn('"make", "-C", str(REPO), "restart-daemon"', text)

    def test_no_shell_true_anywhere(self):
        # A list argv with no shell is why "=agentbox" cannot be equals-expanded
        # to the binary's path on the way in.
        self.assertNotIn("shell=True", Path(shots.__file__).read_text())

    def test_the_real_phase_never_dismisses(self):
        import inspect
        src = inspect.getsource(shots.stage_s12)
        self.assertNotIn("dismiss", src.split('"""')[2] if src.count('"""') > 1 else src)


class TestLongLivedSyncVerbs(unittest.TestCase):
    """The bug that cost the first real sitting.

    `agentbox sync attach` holds presence open for as long as its process runs and
    never returns on its own - cmd/agentbox/sync.go says "No timeout: the whole
    point is to stay". Staging it with the foreground helper hung the run at the
    fourth line of phase 1, with the machine's daemon already down and no output
    for five minutes. Anything that does not return has to go through bg_start.
    """

    LONG_LIVED = {"attach"}

    def _sync_calls(self):
        """Every c.abx*/self.abx* call in the script, as (helper, string args)."""
        import ast
        tree = ast.parse(Path(shots.__file__).read_text())
        for node in ast.walk(tree):
            if not isinstance(node, ast.Call) or not isinstance(node.func, ast.Attribute):
                continue
            if node.func.attr not in ("abx", "abx_bg"):
                continue
            args = [a.value for a in node.args
                    if isinstance(a, ast.Constant) and isinstance(a.value, str)]
            yield node.func.attr, args, node.lineno

    def test_attach_is_never_run_in_the_foreground(self):
        for helper, args, line in self._sync_calls():
            if "sync" in args and self.LONG_LIVED.intersection(args):
                verb = self.LONG_LIVED.intersection(args).pop()
                self.assertEqual(
                    "abx_bg", helper,
                    f"line {line}: `sync {verb}` never returns, so it must be "
                    f"backgrounded with abx_bg, not run through {helper}")

    def test_attach_is_actually_staged_somewhere(self):
        # Guards the test above from passing because the call was deleted.
        self.assertTrue(
            any("sync" in args and "attach" in args for _, args, _ in self._sync_calls()),
            "no `sync attach` call left: the roster's fourth row is what it stages")

    def test_the_backgrounded_row_is_confirmed_rather_than_assumed(self):
        # Backgrounding costs the return code, so a failed attach would otherwise
        # be a silent three-row board.
        import inspect
        self.assertIn("wait_for_row", inspect.getsource(shots.stage_roster))

    def test_wait_for_row_gives_up_rather_than_hanging(self):
        import inspect
        sig = inspect.signature(shots.wait_for_row)
        self.assertIn("timeout", sig.parameters)
        self.assertIsNotNone(sig.parameters["timeout"].default)


if __name__ == "__main__":
    unittest.main(verbosity=2)
