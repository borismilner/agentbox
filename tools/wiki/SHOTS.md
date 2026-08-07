# Taking the wiki screenshots

`tools/wiki/shots.py` stages and captures the twelve shots specified in
`docs/wiki/DESIGN.md` section 5. This file is how a person runs it.

The procedure lives here rather than in DESIGN.md because the two answer
different questions and change at different speeds. DESIGN.md says what must be
on screen and why, and it is the thing a page author reads. This is an operations
runbook for one script, and it changes whenever the script does.

**As of this writing the script has never been run.** It was written and tested
against fabricated input only. The first person to run it should expect to retake
two or three shots and should read the "what usually goes wrong" section before
starting, not after.

## What it costs you

About **five minutes of desktop** when nothing goes wrong, and up to fifteen if
shots need retaking. Most of that is the script waiting for windows and for four
manufactured history items to resolve. During it:

- The machine's daemon goes down and a throwaway one takes its place. Any other
  agent that tries to ask you something will not get a window. This is the one
  real cost and it is why the script refuses to start while other sessions are
  live.
- Cards, boards, a reader, an artifact and the hands-off strip appear and
  disappear on the monitor you park the pointer on. Do not touch the mouse or
  the keyboard. The hands-off strip goes up for two of the shots and means
  exactly what it says.
- Nothing is written to your real store until the last phase, which only reads.

## Before you start

1. **Be alone on the machine.** `agentbox sync agents` should show nobody but
   you. The script checks this itself and refuses otherwise; `--force` overrides
   it, and the price is that somebody else's question goes unanswered for ten
   minutes.
2. **Commit or stash.** The script writes into `docs/wiki/img/`, and you want
   `git diff` to be the record of which shots are new.
3. **Have a backdrop.** S5, S6 and S11 have to sit over somebody's real work: the
   strip only makes sense over a desktop in use, and the panel rolling down over
   an editor is the feature. Open your editor on a real file and leave it there.
   The default title match is `Code`; pass `--backdrop 'part of the title'` if
   yours is called something else.
4. **Nothing fullscreen.** `import -window root` cannot see a fullscreen window
   at all: mutter unredirects one for direct scanout and the root pixmap holds
   what is underneath, so the capture silently gets the wallpaper. If you must
   have something fullscreen in frame, use `--capture shell`, which goes through
   the shell's own capture.
5. **GNOME banners on.** `gsettings get org.gnome.desktop.notifications
   show-banners` must not be `false`; AgentBox reads that as do-not-disturb and
   would hold every card. The script warns but does not change it.
6. **Look at the plan.** `tools/wiki/shots.py --list` prints all twelve with
   their filenames, crops and phases, and touches nothing. `--dry-run` prints
   every command it would run, and also touches nothing.

## The command

```
tools/wiki/shots.py --yes
```

That is the whole sitting: all twelve, in one pass, in three phases. `--yes` is
required because the run takes the machine's only daemon down.

A retake of one or two:

```
tools/wiki/shots.py --only S2,S4 --yes
```

Retaking still costs the daemon swap, because the surfaces only render for the
machine's one daemon. Retaking three shots costs about as much as retaking one, so
retake in batches.

Two retakes have a catch. **S5 and S6 have to be retaken together** (`--only
S5,S6`): the pair only reads if both frames have the same background, and the
script says so if you take S6 alone. And **S3 on its own stages its own two
pending rows**, since the questions S1 leaves behind are not there in a
single-shot run; the rows are the same questions, so the shot is the same, but the
outcome column below them will be shorter unless S3's history staging has run in
that session too, which it does.

Re-checking files you already have, with no desktop involved at all:

```
tools/wiki/shots.py --verify-only
```

## What happens, in order

**Phase 1, isolated.** `make stop`, then a throwaway daemon on instance
`wikishots` with its own `XDG_STATE_HOME` and `XDG_CONFIG_HOME`. Your real
history, settings and pending queue are not what gets experimented on, and no
call in this phase can reach them: the instance name moves both the socket and
the store. Your config is copied in as a starting point so the theme is the real
one, with `fullscreen_auto_dnd` forced off; your own file is read and never
written.

Then, in this order: the roster and a manufactured history, the toast, the
progress window, the card, the inbox, the agents board, the review board, the
artifact, the reader, and the hands-off strip in both its states. The order is
not arbitrary. The toast goes before the strip because the strip pins itself to
the top of the same top-centre column. The card goes before the inbox because
what the card leaves pending is what the inbox needs to show.

**Phase 2, demo.** The throwaway daemon quits so that `agentbox webui-demo ask
panel` can own the session bus name, and S11 is taken against canned data. The
inline ask panel only renders for a session AgentBox itself started, and this is
the only route to that frame short of launching a real session.

**Phase 3, real.** `make restart-daemon` puts your deployed daemon back, and S12
is taken against the real store, read-only. That shot is the one DESIGN wants
real unpruned data for, rehearsal rows and all, which was decided on 2026-07-25:
an honest table is the point, and a tidied one would be a nicer lie. Opening a
tab writes nothing, and nothing in this phase dismisses, prunes or answers
anything.

Phase 3 is last on purpose. The run ends with your daemon already back.

## What to check in each image

The script's own verification phase runs at the end and reports every file, its
pixel size, and whether it looks like a rendered surface or like a missed window.
A solid colour or a near-flat crop means the window was not there and the shot
caught the wallpaper. That check cannot read, so the rest is your eyes:

| Shot | File | Look for |
|---|---|---|
| S1 | `card-restaged.png` | The footer. It must read `expires in 1:57` **and** `2 waiting` with two dots. The `2 waiting` is the entire reason this shot was retaken. |
| S2 | `agents-board.png` | Four rows and no more. All four chips in one frame: **asking you**, **blocked**, **listening: tests:green**, and one dim row reading `no purpose given`. The wait line on row 2 must name `deploy:checkout-api` and `release-bot` as the holder. |
| S3 | `inbox.png` | Two pending rows on top, the key hints (`y yes · n no · d dismiss · c copy`) under the selected one, and four different outcomes in the right column including one `unanswered`. The `unanswered` row stays in: a wiki that only shows answered rows is selling a fantasy. |
| S4 | `review-board.png` | The rail of steps, one step open at its TL;DR, and a highlighted line range with a note attached. See the note below about the comment. |
| S5 | `hands-off.png` | Amber, `HANDS OFF`, activity line reading about `· 4s`, and your real window visibly underneath. |
| S6 | `hands-off-paused.png` | Green, `PAUSED - YOURS`, the same background as S5. The pair is the aid; neither frame works alone, so if the backgrounds differ, retake both. |
| S7 | `artifact.png` | The slider mid-track at 50%, the bar underneath in requests a minute, both buttons, and the `interactive` badge with the preview/code toggle. The toggle is the trust half of the shot; without it the shot is wrong. |
| S8 | `viewer.png` | A table, a mermaid diagram and a highlighted code block all in frame at once, watching badge lit. Adjust with `--viewer-scroll N` and retake. Check the footer path does not expose a private directory. |
| S9 | `toast.png` | The severity band on the left, no countdown (a warning waits to be read), and the top edge of the screen in frame so its position is part of the information. |
| S10 | `progress.png` | Three bars, and the screen corner. This is the one shot where cropping tight destroys the point. |
| S11 | `panel.png` | The inline question above the composer, your editor still visible behind. |
| S12 | `history-stats.png` | The median answer time. Then read every row: **if a private project name is in frame, the shot is cut rather than doctored.** |

## Four things that need a human decision

The script says all of these out loud at the end of a run. None is a bug.

1. **S1 writes `card-restaged.png`, not `card.png`.** The existing
   `docs/wiki/img/card.png` is correctly staged apart from its footer, and
   `home.md` and `the-card.md` both reference it. Compare the two, and only when
   the new one is right:
   ```
   mv docs/wiki/img/card-restaged.png docs/wiki/img/card.png
   ```
2. **S2's abandoned shared claim needs a fifth session.** A claim reads as
   abandoned because its owner is gone, and an owner that never existed cannot
   make one. So a fifth roster row may be in frame, and DESIGN wants four. If it
   is, retake with `--no-stage-abandoned` and accept two live claims and no
   abandoned one.
3. **S4's comment comes from the spec, not from a typed one.** DESIGN asks for a
   comment with text typed into it. Typing one means selecting code at a
   coordinate, and this script will not click a coordinate. If the spec's
   anchored note does not carry the frame, type the comment by hand and retake
   with `--only S4`.
4. **S11 carries canned agent names.** `webui-demo` fixtures use `claude-code`
   and friends, not the `checkout-api` fiction the rest of the wiki uses. If a
   name in frame contradicts the other pages, that shot needs the fixture edited
   or a real session, and neither is the script's job.

## What usually goes wrong

- **A shot reports `no window matching ... after 20s`.** The surface never came
  up. Check the phase it failed in: in phase 1 that usually means the throwaway
  daemon is not the one holding the session bus name (`busctl --user list | grep
  agentbox` should show exactly one). Raise `--timeout 40` for a slow first paint.
- **A capture is flagged suspect.** The crop landed on wallpaper. Most often the
  window opened on the other monitor, which means the pointer was on the other
  monitor: pass `--monitor NAME` or `--park X,Y`. Failing that, `--capture shell`.
- **A card shot times out and the log looks fine.** Something is holding cards.
  GNOME banners off, do-not-disturb on, or an agent muted.
- **The frame has somebody else's card in it.** Another session queued something.
  Retake that one shot.

## Restoring

The script restores everything itself, on a clean run and on an abort: it
releases the desktop, dismisses everything it queued on the throwaway instance,
quits the throwaway daemon, puts do-not-disturb back to what it was, and brings
the deployed daemon back.

If it died hard and left the machine without a daemon, this is the one command:

```
make restart-daemon
```

Then confirm with `make deployed`, which asks the running daemon its build rather
than trusting the file. `agentbox dnd status` is worth a look too, in case the
script died between turning it off and putting it back.

## Which pages actually want these

Re-reconciled 2026-08-07, after all eighteen pages were written and published. The
paragraph below this table was written when four pages existed; the table is the
current answer.

**Matched: a page asks for it and the plan captures it**

| Shot | File | The page that wants it |
|---|---|---|
| S1 | `card.png` (via `card-restaged.png`) | `home.md`, `the-card.md`, both already referencing `img/card.png` |
| S2 | `agents-board.png` | `agents-board.md:12` |
| S3 | `inbox.png` | `nothing-gets-lost.md:19` |
| S4 | `review-board.png` | `review-board.md:32` |
| S5 and S6 | `hands-off.png`, `hands-off-paused.png` | `hands-off.md:12` asks for both in one frame pair, which is the page allowed two |
| S7 | `artifact.png` | `documents-and-artifacts.md:92` |
| S9 | `toast.png` | `notifications.md:20` |
| S11 | `panel.png` | `sessions.md:11` |

**Two pages ask for a shot the plan does not have.** Add these before or after
the first sitting, not instead of it:

| Wanted by | What it must show |
|---|---|
| `is-it-safe.md:18` | the secret card mid-entry: the field masked with a few dots in it, and the line naming where the value is going |
| `settings.md:51` | the Appearance page with an unsaved change pending, so the live preview and the accent are both visible |

**The third is taken.** `install.md` now carries `img/install-doctor.png`, made on
2026-08-07. Two things about it are worth knowing before anyone retakes it.

It is not a photograph of a terminal. This machine has no `Xvfb` and no terminal
emulator installed, so an off-screen capture was impossible and the only alternative
was capturing a window on Boris's live display, which the shot spec ruled out
("terminal only, no desktop"). The image is therefore `make doctor` and `agentbox
status` run for real, their output captured verbatim, and that text typeset by
ImageMagick. No colour was invented: `doctor` emits no ANSI, so the only distinction
drawn is the `%-22s` label column its own `printf` creates. Retaking it means
re-running both commands and re-rendering, not restaging anything.

The spec asked for one row honestly reading `missing (only needed for showcase
screenshots)`, so the frame would read as a real machine rather than a staged one.
That row is ImageMagick's `import`, and `import` is present here, so the row reads
`present` and the requirement could not be met without lying. It was dropped rather
than faked. What carries the same weight instead is that every path, build hash and
timestamp in the frame is this machine's own, and the build shown is the one
deployed the same afternoon.

**Three planned shots no page asks for yet.** S8 (`viewer.png`), S10
(`progress.png`) and S12 (`history-stats.png`). They are still worth taking in the
sitting, because the sitting is the expensive part, but nothing is currently broken
without them. S8 and S10 are the two overlaps the note below predicted:
`documents-and-artifacts.md` chose the artifact over the reader, and
`notifications.md` chose the toast over the progress window.

**One tension worth resolving before the pages are written.** DESIGN assigns
**both S9 and S10 to `notifications.md`**, and the rhythm rules in section 5 allow
one image per page, with `hands-off.md` named as the only deliberate exception. So
one of the two either moves to another page or does not get used. Both are
captured; the choice is a page author's, not this script's. The same overlap
between S7 and S8 on `documents-and-artifacts.md` is already acknowledged in
DESIGN, which says S8 is for that page only if S7 moves to `is-it-safe.md` and is
otherwise cut.

## Publishing

`tools/wiki/publish.sh` copies `docs/wiki/img/*` into both wiki repos. Nothing
mirrors wiki repos on its own, so a new image is not live until that script runs.
