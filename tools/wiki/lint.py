#!/usr/bin/env python3
"""Refuse to publish a wiki page that will render wrong on one of the two hosts.

The wiki source in docs/wiki/pages is published to two places that disagree:
GitLab renders more markdown than GitHub does, GitHub flattens every page to its
basename, and each wants a different name for the landing page and the sidebar.
Anything outside the subset both hosts agree on looks fine where it was written
and broken where it was mirrored, which is the failure nobody notices until a
reader reports it. So the checks below run before a push, not after.

Four families of check:

  portability  constructs GitLab renders and GitHub does not
  links        every [[wiki link]] and image reference resolves
  layout       filenames GitHub can serve, and the hurry summary every page owes
  tells        the character-level fingerprints of machine-written prose

Exit 1 on any finding. --list prints the pages it would publish and stops.
"""

from __future__ import annotations

import argparse
import os
import re
import sys
import unicodedata
from pathlib import Path

REPO = Path(__file__).resolve().parents[2]
PAGES = REPO / "docs" / "wiki" / "pages"

# Sidebar and landing page are renamed per host by publish.sh, so they are
# allowed to break the flat-lowercase rule the rest of the pages live under.
SPECIAL = {"_sidebar.md", "Home.md", "home.md"}

# GitLab-only markdown. Each entry is (pattern, what a GitHub reader sees).
PORTABILITY = [
    (re.compile(r"\[\[_TOC_\]\]|^\[TOC\]$", re.M), "renders as a broken link on GitHub; write an anchor list"),
    (re.compile(r"^\[\^[^\]]+\]:", re.M), "footnotes are unsupported in GitHub wikis"),
    (re.compile(r"\[\^[^\]]+\](?!:)"), "footnote reference; unsupported in GitHub wikis"),
    (re.compile(r"^```(plantuml|kroki)\b", re.M), "renders as a plain code block on GitHub; use mermaid"),
    (re.compile(r"\{[+-][^}]*[+-]\}|\[[+-][^\]]*[+-]\]"), "GitLab inline diff marker; shows literal braces on GitHub"),
    (re.compile(r"^- \[~\]", re.M), "GitLab inapplicable task box; shows literal [~] on GitHub"),
    (re.compile(r"^>>>$", re.M), "GitLab multiline blockquote; does nothing on GitHub"),
    (re.compile(r"\[wiki_page:"), "GitLab-only link form; renders as text on GitHub"),
    (re.compile(r"^\s*(#{1,6})?\s*<!-- *(TOC|toc) *-->", re.M), "no TOC generator exists on GitHub"),
]

FRONT_MATTER = re.compile(r"\A(---|\+\+\+|;;;)\s*$", re.M)

# Character fingerprints. Replacement is what belongs there in plain ASCII prose.
TELLS = [
    ("—", "em dash", "a hyphen, parentheses, or two sentences"),
    ("–", "en dash", "a hyphen, or 'to' in a range"),
    ("‘", "curly open quote", "'"),
    ("’", "curly apostrophe", "'"),
    ("“", "curly open double quote", '"'),
    ("”", "curly close double quote", '"'),
    ("…", "ellipsis character", "three periods"),
    (" ", "non-breaking space", "a normal space"),
    (" ", "narrow no-break space", "a normal space"),
    ("​", "zero-width space", "nothing"),
    ("‍", "zero-width joiner", "nothing"),
    ("­", "soft hyphen", "nothing"),
    ("→", "rightwards arrow", "the word 'to', or 'becomes'"),
    ("⇒", "double arrow", "the word 'then'"),
    ("−", "math minus", "a hyphen"),
    ("×", "multiplication sign", "the letter x"),
    ("•", "bullet character", "a markdown list"),
    ("️", "variation selector", "nothing"),
]

EMOJI = re.compile(
    "[" "\U0001f300-\U0001faff" "\U0001f000-\U0001f2ff" "☀-➿" "⬀-⯿" "️" "]"
)

WIKILINK = re.compile(r"\[\[([^\]]+)\]\]")
MDLINK = re.compile(r"(?<!\!)\[[^\]]*\]\(([^)\s]+)")
IMAGE = re.compile(r"!\[[^\]]*\]\(([^)\s]+)")
FENCE = re.compile(r"^(```|~~~)")
HEADING = re.compile(r"^#{1,6} ")

BAD_NAME = re.compile(r"[^a-z0-9-]")


class Findings:
    def __init__(self) -> None:
        self.rows: list[tuple[str, int, str, str]] = []

    def add(self, page: str, line: int, kind: str, message: str) -> None:
        self.rows.append((page, line, kind, message))

    def report(self, quiet: bool) -> int:
        if not self.rows:
            if not quiet:
                print("wiki lint: clean")
            return 0
        by_kind: dict[str, list[tuple[str, int, str, str]]] = {}
        for row in self.rows:
            by_kind.setdefault(row[2], []).append(row)
        for kind in ("layout", "portability", "links", "tells", "voice"):
            rows = by_kind.get(kind)
            if not rows:
                continue
            print(f"\n{kind} ({len(rows)}):")
            for page, line, _, message in rows:
                where = f"{page}:{line}" if line else page
                print(f"  {where}: {message}")
        print(f"\n{len(self.rows)} finding(s). Nothing was pushed.")
        return 1


def strip_code(text: str) -> list[tuple[int, str]]:
    """Lines outside fenced code blocks, as (1-based line number, text)."""
    out: list[tuple[int, str]] = []
    fenced = False
    for n, line in enumerate(text.splitlines(), 1):
        if FENCE.match(line.strip()):
            fenced = not fenced
            continue
        if not fenced:
            out.append((n, line))
    return out


def slugify(target: str) -> str:
    """The filename a [[link]] target resolves to, on either host."""
    if "|" in target:
        target = target.split("|", 1)[1]
    target = target.strip().strip("/")
    return target.replace(" ", "-").lower()


def check_layout(path: Path, text: str, pages: set[str], f: Findings) -> None:
    name = path.name
    if name not in SPECIAL:
        stem = path.stem
        if BAD_NAME.search(stem):
            f.add(name, 0, "layout", "filename must be lowercase letters, digits and hyphens only (GitHub serves pages by basename)")
    lines = text.splitlines()
    if not lines:
        f.add(name, 0, "layout", "page is empty")
        return
    if not lines[0].startswith("# "):
        f.add(name, 1, "layout", "page must open with a single '# Title' heading")
    if name in ("_sidebar.md", "_Sidebar.md"):
        return
    # The hurry summary: a blockquote inside the first six lines, before any
    # second-level heading. Every page owes the reader one.
    head = lines[:8]
    if not any(l.startswith(">") for l in head):
        f.add(name, 1, "layout", "no hurry summary; every page opens with a blockquote summary before the first section")
    for n, line in enumerate(lines, 1):
        if HEADING.match(line):
            words = line.lstrip("# ").split()
            capped = [w for w in words[1:] if w[:1].isupper() and not w.isupper() and w.lower() not in ("agentbox", "gitlab", "github", "linux", "claude", "mcp", "wayland", "x11")]
            if len(capped) >= 2:
                f.add(name, n, "voice", f"heading looks title-cased: {line.strip()!r}; use sentence case")


def check_portability(path: Path, text: str, f: Findings) -> None:
    if FRONT_MATTER.match(text):
        f.add(path.name, 1, "portability", "front matter renders as literal content on GitHub")
    for pattern, why in PORTABILITY:
        for m in pattern.finditer(text):
            line = text[: m.start()].count("\n") + 1
            f.add(path.name, line, "portability", f"{m.group(0)[:40]!r}: {why}")


def check_links(path: Path, text: str, pages: set[str], f: Findings) -> None:
    for n, line in strip_code(text):
        for m in WIKILINK.finditer(line):
            slug = slugify(m.group(1))
            if slug.startswith("_"):
                continue  # [[_TOC_]] and friends: the portability check owns those
            if slug and f"{slug}.md" not in pages and slug not in ("home",):
                f.add(path.name, n, "links", f"[[{m.group(1)}]] points at no page (looked for {slug}.md)")
        for m in IMAGE.finditer(line):
            url = m.group(1)
            if not url.startswith(("http://", "https://")):
                f.add(path.name, n, "links", f"image {url!r} is relative; GitHub does not rewrite image paths, use an absolute raw URL")
        for m in MDLINK.finditer(line):
            url = m.group(1)
            if url.startswith(("http://", "https://", "#", "mailto:")):
                continue
            f.add(path.name, n, "links", f"internal link {url!r} is a relative path; use [[Text|slug]] so it resolves on both hosts")


def check_tells(path: Path, text: str, f: Findings) -> None:
    for n, line in enumerate(text.splitlines(), 1):
        for ch, label, instead in TELLS:
            col = line.find(ch)
            if col >= 0:
                f.add(path.name, n, "tells", f"col {col + 1}: {label} (U+{ord(ch):04X}); use {instead}")
        for m in EMOJI.finditer(line):
            name = unicodedata.name(m.group(0), "emoji")
            f.add(path.name, n, "tells", f"col {m.start() + 1}: {name.lower()}; no emoji garnish")


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--list", action="store_true", help="print the pages that would publish and stop")
    ap.add_argument("--quiet", action="store_true", help="say nothing when clean")
    ap.add_argument("paths", nargs="*", help="pages to check (default: all of docs/wiki/pages)")
    args = ap.parse_args()

    if args.paths:
        files = [Path(p) for p in args.paths]
    else:
        if not PAGES.is_dir():
            print(f"no wiki source at {PAGES}", file=sys.stderr)
            return 2
        files = sorted(p for p in PAGES.rglob("*.md"))
    if not files:
        print(f"no pages in {PAGES}", file=sys.stderr)
        return 2

    if args.list:
        for p in files:
            print(os.path.relpath(p, REPO))
        return 0

    pages = {p.name for p in files}
    f = Findings()

    for p in files:
        rel = p.relative_to(PAGES) if PAGES in p.parents else Path(p.name)
        if len(rel.parts) > 1:
            f.add(str(rel), 0, "layout", "pages must be flat; GitHub serves every page by basename and subdirectories collide")
        text = p.read_text(encoding="utf-8")
        check_layout(p, text, pages, f)
        check_portability(p, text, f)
        check_links(p, text, pages, f)
        check_tells(p, text, f)

    return f.report(args.quiet)


if __name__ == "__main__":
    sys.exit(main())
