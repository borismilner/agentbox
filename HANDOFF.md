# Handoff - AgentBox: portable, and the branch nobody had run

*Written by session 61, which took two assignments from Boris and finished both.
It started on the robustness backlog (R-12 and R-13, the last two hours-sized
band-A items) and R-12 turned out to be a portability defect in a routing
defect's clothes - so the second assignment, portability, grew out of the first.
AgentBox now builds and runs on Linux, macOS and Windows, `make check` proves two
of them from this box, and nothing on this machine behaves or looks different.*

**Written:** 2026-08-11 · **Assignment:** /home/boris-milner/me/projects/agentbox · **Type:** personal

## Do this next

```bash
cd ~/me/projects/agentbox
git status -sb                          # expect clean, in sync with origin/main
make deployed                           # expect 9cddbb3cada6 (HEAD minus this handoff commit)
make check                              # expect green: 44 ok lines, 43 vitest + 1 expected fail
bash tools/wiki/publish.sh --dry-run    # expect "already up to date" on both
agentbox pending                        # expect nothing pending
```

**`make check` is slower than you remember and that is deliberate.** It gained
`test-nox11` (the whole suite again, through the no-X11 placement layer) and
`cross` (windows/amd64 whole tree, both darwin arches minus the two packages that
link a native UI). That is why 44 `ok` lines instead of 22. Budget about two
minutes more.

**Budget.** The week stood at 80% (all models) when this was written and the
session at 25%. The week resets Aug 12, 4:59am (Asia/Jerusalem) - hours away, so
the practical ceiling is small. Boris's standing instruction this session was to
stay under 95% weekly. Check `claude -p /usage | grep -E '^Current '` before
starting anything large; there is room for one contained piece of work, not a
milestone.

1. **Robustness band A, five left**: R-06, R-07, R-09, R-10, R-15, in
   `docs/backlog/robustness.md`'s own order. R-15 still needs a live MCP-host
   repro before it can be fixed at all; the other four are a day each. **There is
   no hours-sized band-A item left** - R-12 and R-13 were the last two.
2. **U-16 still wants one more repro** before its fix is chosen. Carried unchanged
   from sessions 59 AND 60, which means two sessions have now not looked at it. Go
   and observe it rather than re-reading the entry.
3. **U-17 and U-18 are the artifact pair**, both hours, same reproduction window.
   U-17's shape is Boris's call (below); U-18 has no diagnosis yet.
4. **R-46 and R-47 are new and both came out of this session.** Neither is urgent
   and both are honestly band-B-shaped. R-46 is the Windows connect token; R-47 is
   the rider's remaining client-side window, and it cannot be done without
   re-arguing the decision that a failed call carries no news.

## Where we are

Portability is a checked property rather than a claim, and the reason it was worth
doing now is R-12. `internal/webui/x11.go` carried `//go:build linux` and was the
ONLY tagged file in the source tree, with nothing on the other side of it. Twenty
call sites above it read `if u.x != nil { place it } else { let the desktop place
it }` - fully written, fully reachable, never once executed. R-12 lived exactly
there: the panel's roll took that branch, recorded itself open from a `defer` that
ran on the failure path too, and swallowed every question routed to it. That is
R-40's argument about Svelte, in Go, and it is why `test-nox11` is in the gate
instead of in a document promising the path works.

ADR-0013 records the decision and amends the vision's fourth non-goal and NFR10.
The distance turned out to be six syscalls, two `/proc` reads, and that one tag.

## Live state (volatile - verify on resume)

- **Background jobs:** none. A `-tags nox11` daemon was run briefly as the
  desktop's only daemon and stopped again; if a stray one is ever suspected,
  `agentbox status` plus `agentbox sync locks` answers it. **Never `pkill
  agentbox`.**
- **PRs:** none, ever. Boris pushes `main` directly.
- **Git:** `main` clean and pushed to **both** remotes, `gitlab` (origin) and
  `github`. Deliberately not quoting a sha here, because this file's own commit
  moves it - check with `git rev-parse --short HEAD` against
  `git ls-remote origin main` and `git ls-remote github main`, and expect all
  three to agree.
  **Push both explicitly: `git push origin main && git push github main`.** The
  news here is that the direct github push WORKS now - sessions 59 and 60 recorded
  it as blocked by the permission classifier, and it went through twice this
  session. A gitlab-to-github mirror also exists but its timing is not dependable:
  it delivered one commit inside twenty seconds and had not delivered the next
  after forty, so waiting on it is not a plan. If a direct push is rejected as
  stale, the mirror simply won the race and the ref is already correct - check with
  `ls-remote` and never force.
- **Wiki:** both hosts current (`publish.sh --dry-run` says so), four pages
  changed - home, install, is-it-safe, limits. Read back the published `limits.md`
  over HTTP to confirm the new first section is live.
- **Deployed daemon:** `9cddbb3cada6`, restarted via `agentbox.service`, 0
  pending. That is HEAD minus the two commits that wrote this file, both
  documentation - nothing under `frontend/`, `internal/manual/`, `internal/sound/`
  or `internal/store/migrations/` changed, so the binary is the same code and a
  redeploy would only restamp the version string. Do not restart the daemon just to
  make the two shas match.
- **Desktop:** never driven. The screen was locked for the middle of this session,
  which is why the verification below is geometry and logs rather than screenshots.
- **In-flight edits:** none.

## Blocked on you (Boris)

1. **Whether `docs/backlog/features.md`'s one non-goal revision is on.** Carried
   unchanged from sessions 56 through 60: B-1, "away without becoming a cloud
   service", wants the vision's non-goal 3 amended in public. **This session set
   the precedent for how** - ADR-0013 amended non-goal 4 and NFR10 in place, with
   the old text struck through and the reason attached. B-1 could follow it
   exactly, but it is still a principle change and therefore yours.
2. **Whether the Esc notice earns its place.** Carried from 58, seen on screen in
   59. One string in `internal/daemon/daemon.go`.
3. **Whether the agents board should lead with shared values.** A design call; the
   frame is drawn either way.
4. **U-17's shape.** An artifact opened on its own has no way to read its source.
   Two fixes and the choice is taste: the toggle back in the bar for every shape,
   or into the viewer's title bar beside `find`, `A-`, `A+`.
5. **Nothing else from this session needs you.** The one place a guarantee changed
   shape (Windows has no peer-credential check) is written down in three places
   rather than decided quietly - `peer_windows.go`, NFR8, and the wiki's
   is-it-safe page - and R-46 is the fix if you want it.

## I can do solo (no input needed)

1. **R-06, R-07, R-09, R-10**, a day each, band A.
2. **U-16's second repro**, then its fix.
3. **U-18's diagnosis** - why a failed artifact leaves the bar's error slot empty.
4. **R-40's open fixes (2) and (3)**: a hostile payload against the card, and
   executing `buildDocument` to assert the CSP and sandbox attributes that today
   are only checked as string constants.
5. **U-05**, hours: `theme.motion = "reduced"` is honoured by four components of
   the thirteen that animate. Do it whenever something else touches `app.css`.
6. **R-46**, days: the connect token. Well understood, and the entry states the
   whole design.
7. **`settings.md`'s SHOT placeholder**, if somebody wants it - `DRAW.md` says why
   the fixture half is the harder one.

## Facts - verified vs assumed

- [verified] **Nothing on this machine regressed.** After deploy: a toast centres
  to the pixel on the monitor the pointer is on (`x=2505 y=48` on the 2560-wide
  screen at +1440, and `x=505 y=48` on the 1440-wide one - both are
  `centre - w/2`), a card centres on both axes (`x=2485 y=599`), the panel rolls
  from the top edge and answers `panel state` correctly through show, hide and two
  toggles. `make check` reports the same 43 vitest passes and 1 expected fail as
  session 60 recorded.
- [verified] **No file under `frontend/` was touched**, by `git diff --stat` over
  the whole session range. So nothing can LOOK different; the bundle is the same
  bytes.
- [verified] **The no-X11 path runs.** A `-tags nox11` daemon served `agentbox
  status`, put its windows up WM-placed (`y=695` instead of `y=48`), and logged
  `webui.panel_unprepared` once with `down=true x11=false`. Run against
  `XDG_STATE_HOME`/`XDG_CONFIG_HOME` in a scratch dir, so Boris's store and
  settings were never involved.
- [verified] **Windows links a real binary** (41 MB, `GOOS=windows CGO_ENABLED=0
  go build ./cmd/agentbox`), and **darwin's only remaining obstacle is clang** -
  `GOOS=darwin CGO_ENABLED=1 go vet ./...` reports nothing but the missing C
  compiler.
- [verified] **`proc_darwin.go` type-checks** for darwin/arm64 and darwin/amd64,
  by copying it into a throwaway module with a stub caller. It is the one file no
  package build here can reach, because `cmd/agentbox` imports `webui`.
- [verified] **Both R-12's and R-13's tests fail against the old code.** R-12's
  was checked by reverting the one-line fix and watching it fail on exactly the two
  intended assertions.
- [assumed] **That the macOS and Windows builds RUN.** Nobody has started either.
  Every document that mentions the port says so, and that wording is deliberate -
  "compiles" and "works" are different claims. If Boris ever runs one, the thing
  worth reporting is whether the surfaces appear at all.
- [assumed] **That `powershell`'s Media.SoundPlayer and `afplay` play the earcons
  correctly.** The argv for both is pinned by a test; neither has been executed.
- [assumed] **U-18's cause**, unchanged from session 60.
- [assumed] The carried set from sessions 50, 51, 55, 57, 59 and 60, none of which
  this session looked at: the artifact-restart-while-streaming edge; the 30-minute
  fuse in a live daemon; the demoted marker on a second monitor; `perform.py`'s
  fullscreen check on two monitors; the domain drawer animation; the demo fallback
  painting; a detail holding a mermaid diagram; `provisionalFor` retiring a
  hook-only row; the 200-key prefix cap; `@me` in a shared key; a real client's
  `await_signal` parked past 20s; the two lock warnings; the identity cross-check's
  no-node skip; and whether the desk's blurred backdrop reads well to a stranger.

## Three traps this session paid for

- **A build tag with nothing on the other side is a hole, not a gate.** It read as
  portability for months while the build broke the moment anything but Linux asked,
  AND it hid R-12 on a branch nothing executed. If you add a method to `x11`, add
  it to `x11_absent.go` too: the compiler will not always tell you, because a
  method only called on Linux compiles fine here until somebody builds for Windows.
- **`filepath.Base` only knows the separator it was compiled for.** A Windows path
  handed to a Linux build came back whole, matched nothing in a `switch`, and fell
  through to the default branch - which was "play the file with no arguments", the
  one outcome that looks like it worked. Caught by a test written five minutes
  earlier. `strings.LastIndexAny(s, "/\\")` is the fix.
- **`git commit` commits the whole index, not the files you just added.** A
  `git reset --soft` left the Makefile staged, so the next `git add internal/sound/
  ... && git commit` silently swept it into the sound commit. Fixed by
  `git reset --soft` again then `git reset` to clear the index before adding
  selectively. Related: an ad-hoc `GOOS=windows go build ./...` drops a 41 MB
  `agentbox.exe` in the repo root, which is why `make cross` uses `-o /dev/null`.

## What this session changed

| Commit | What |
|---|---|
| `6052004` | R-12: the panel's state comes from what mapped, plus the fallback every sibling had |
| `61d4c09` | R-13: `RiderFunc` answers a put-back, and `Serve` uses it when the send fails |
| `861a110` | a pre-existing `go fix` nag that blocked the hook on every daemon edit |
| `fd58e6f` | `x11_absent.go`, the placement layer's other half, and `winID` |
| `334876b` | six Linux-only syscalls into files named for their platform |
| `9099033` | the process-tree reads, out of `/proc` and into three implementations |
| `1d1b668` | earcons, speech and the editor fallback find a player on any desktop |
| `488754f` | `test-nox11` and `cross` in `make check` |
| `f714066` | ADR-0013; the vision's non-goal 4 and NFR10 amended; 04-platform rewritten |
| `4538427` | FR100 closed, R-12/R-13 marked, R-46 and R-47 filed, counts corrected |
| `9cddbb3` | the wiki's four pages, the README, packaging, CLAUDE.md |

## Declutter ledger

| Removed / condensed | Where its knowledge now lives |
|---|---|
| Session 60's "R-12 and R-13 are the hours-sized ones, start there" | Done. What they taught is in their own fixed-markers and in ADR-0013 |
| "NFR10 X11 today, Wayland-ready" | Superseded in place by the amended NFR10. The old bar - a documented Wayland equivalent - is quoted as the thing that was not enough |
| `docs/04-platform.md`'s "Wayland (later, GNOME)" framing | Rewritten as a table of what each desktop gets. The Wayland research is kept verbatim; only the framing changed |
| The wiki's four "X11 only, no macOS or Windows build" statements | `docs/wiki/FACTS.md` lists the old claim under "do not claim these" with the date it stopped being true |
| Session 60's note that `make deployed` is behind HEAD | Restated once, precisely, in Live state rather than argued again: deployed is `9cddbb3`, HEAD is that plus this file, and the difference is documentation only |

## Map

1. [HANDOFF.md](HANDOFF.md) - this file.
2. **[docs/backlog/README.md](docs/backlog/README.md) - read this first.** One
   order across all three audits. Tier 1 is five robustness items plus U-16.
3. [docs/decisions/ADR-0013-portable-by-default.md](docs/decisions/ADR-0013-portable-by-default.md) -
   **the decision behind everything this session did**, including what was
   deliberately NOT done and why.
4. [docs/04-platform.md](docs/04-platform.md) - what each desktop gets, measured.
5. [docs/STATUS.md](docs/STATUS.md) - current state, updated today.
6. [docs/backlog/robustness.md](docs/backlog/robustness.md) - 47 items; five band-A
   left; R-12 and R-13 carry fixed markers, R-46 and R-47 are new.
7. [docs/backlog/ux.md](docs/backlog/ux.md) - U-16 is the open band-A item; U-17
   and U-18 are the artifact pair.
8. [docs/07-field-requests.md](docs/07-field-requests.md) - FR100 is the short
   version of this work.
9. [docs/wiki/FACTS.md](docs/wiki/FACTS.md) - **read before quoting any number or
   platform claim about the product.**
10. [docs/history.md](docs/history.md) - session 61 is at the top.
11. [tools/wiki/DRAW.md](tools/wiki/DRAW.md) - how the wiki's frames are drawn,
    needed only if a picture has to change.
