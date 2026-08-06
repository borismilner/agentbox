// The parser behind two surfaces (the review card and the inbox detail that
// reads a resolved one back), and until now the only shared module with no test
// of its own. Run with `make test-js`, or `node --test frontend/src`.
//
// Node's own runner, deliberately: adding a test framework to a frontend whose
// dist is committed so machines without npm can still build would be a
// dependency bought for one file.

import { test } from "node:test";
import assert from "node:assert/strict";

import { parseDiff, DIFF_CAP } from "./diff.js";

const git = `diff --git a/internal/daemon/daemon.go b/internal/daemon/daemon.go
index 1111111..2222222 100644
--- a/internal/daemon/daemon.go
+++ b/internal/daemon/daemon.go
@@ -10,3 +10,4 @@ func one() {
 	keep()
+	added()
 	tail()
`;

test("files, counts and classes come out of an ordinary git diff", () => {
  const m = parseDiff(git);
  assert.equal(m.files.length, 1);
  assert.equal(m.files[0].name, "internal/daemon/daemon.go");
  assert.equal(m.add, 1);
  assert.equal(m.del, 0);
  assert.deepEqual(
    m.files[0].lines.map((l) => l.cls),
    ["hunk", "ctx", "add", "ctx"],
  );
  // `index ...` is structure, not content: it must not reach the reader.
  assert.ok(!m.files[0].lines.some((l) => l.text.startsWith("index ")));
});

test("a body line that starts with --- or +++ is content, not a file header", () => {
  // The classic unified-diff ambiguity, and the reason the parser consumes hunk
  // line counts. Getting this wrong splits one file into three and loses lines.
  const m = parseDiff(`diff --git a/doc.md b/doc.md
--- a/doc.md
+++ b/doc.md
@@ -1,4 +1,4 @@
 title
---- old rule
+++ new rule
 tail
`);
  assert.equal(m.files.length, 1, "the body lines opened new files");
  assert.equal(m.files[0].name, "doc.md");
  assert.deepEqual(
    m.files[0].lines.map((l) => l.cls),
    ["hunk", "ctx", "del", "add", "ctx"],
  );
  assert.equal(m.add, 1);
  assert.equal(m.del, 1);
});

test("a hunk header with no counts means one line each way", () => {
  const m = parseDiff(`--- a/x
+++ b/x
@@ -1 +1 @@
-old
+new
`);
  assert.equal(m.add, 1);
  assert.equal(m.del, 1);
  assert.deepEqual(
    m.files[0].lines.map((l) => l.cls),
    ["hunk", "del", "add"],
  );
});

test("badges: new, deleted and renamed", () => {
  const made = parseDiff(`diff --git a/new.go b/new.go
new file mode 100644
--- /dev/null
+++ b/new.go
@@ -0,0 +1,1 @@
+package main
`);
  assert.equal(made.files[0].badge, "new");
  assert.equal(made.files[0].name, "new.go");

  const gone = parseDiff(`diff --git a/old.go b/old.go
deleted file mode 100644
--- a/old.go
+++ /dev/null
@@ -1,1 +0,0 @@
-package main
`);
  assert.equal(gone.files[0].badge, "deleted");

  const moved = parseDiff(`diff --git a/from.go b/to.go
similarity index 98%
rename from from.go
rename to to.go
`);
  assert.equal(moved.files[0].badge, "renamed");
  assert.equal(moved.files[0].name, "to.go");
});

test("plain diff -u output opens files and drops the timestamp", () => {
  const m = parseDiff(`--- one.txt\t2026-08-06 10:00:00.000000000 +0300
+++ one.txt\t2026-08-06 10:01:00.000000000 +0300
@@ -1,2 +1,2 @@
 keep
-old
+new
--- two.txt\t2026-08-06 10:00:00.000000000 +0300
+++ two.txt\t2026-08-06 10:01:00.000000000 +0300
@@ -1 +1 @@
-a
+b
`);
  assert.deepEqual(
    m.files.map((f) => f.name),
    ["one.txt", "two.txt"],
  );
  assert.equal(m.add, 2);
  assert.equal(m.del, 2);
});

test("the no-newline marker is shown rather than counted", () => {
  const m = parseDiff(`--- a/x
+++ b/x
@@ -1,1 +1,1 @@
-old
+new
\\ No newline at end of file
`);
  assert.equal(m.add, 1);
  assert.equal(m.del, 1);
  assert.equal(m.files[0].lines.at(-1).cls, "meta");
});

test("a hand-written diff still counts, without the structure", () => {
  const m = parseDiff(`+added on its own
-removed on its own
 context
`);
  assert.equal(m.files.length, 1);
  assert.equal(m.add, 1);
  assert.equal(m.del, 1);
});

test("empty input is an empty model, not a crash", () => {
  for (const raw of ["", "   ", "\n\n"]) {
    const m = parseDiff(raw);
    assert.deepEqual(m, { files: [], add: 0, del: 0 });
  }
});

test("the cap bounds lines, never structure", () => {
  // Every file keeps its header so the rail can always jump somewhere, and a
  // truncated file says how much it is hiding.
  const body = Array.from({ length: 30 }, (_, i) => `+line ${i}`).join("\n");
  const raw = `diff --git a/a.go b/a.go
--- a/a.go
+++ b/a.go
@@ -1,0 +1,30 @@
${body}
diff --git a/b.go b/b.go
--- a/b.go
+++ b/b.go
@@ -1,0 +1,30 @@
${body}
`;
  const m = parseDiff(raw, 10);
  assert.equal(m.files.length, 2, "the second file lost its header to the cap");
  assert.equal(m.files[0].shown.length, 10);
  assert.equal(m.files[0].more, 21); // 30 added lines plus the hunk header
  assert.equal(m.files[1].shown.length, 0);
  assert.equal(m.files[1].more, 31);
  // Counts are of the whole diff, not of what fits on screen.
  assert.equal(m.add, 60);
});

test("the default cap is the exported one", () => {
  const body = Array.from({ length: DIFF_CAP + 20 }, (_, i) => `+l${i}`).join(
    "\n",
  );
  const m = parseDiff(`--- a/a.go
+++ b/a.go
@@ -1,0 +1,${DIFF_CAP + 20} @@
${body}
`);
  assert.equal(m.files[0].shown.length, DIFF_CAP);
  assert.equal(m.files[0].more, 21); // the header plus the overflow
});

test("every line is data, never markup", () => {
  // The surfaces' rule: the only escape from a text node is an allowlisted
  // field (frontend/policy_test.go). A diff is agent-authored text, so the
  // parser must hand back strings and classes and nothing that renders.
  const m = parseDiff(`--- a/x
+++ b/x
@@ -1,1 +1,1 @@
-<script>alert(1)</script>
+<img src=x onerror=alert(1)>
`);
  for (const l of m.files[0].lines) {
    assert.equal(typeof l.text, "string");
    assert.deepEqual(Object.keys(l).sort(), ["cls", "text"]);
  }
  assert.equal(m.files[0].lines[1].text, "-<script>alert(1)</script>");
});


test("a lying hunk header cannot swallow the files after it", () => {
  // The header is a claim about how much body follows, and the diff is
  // agent-authored text. Consuming on trust lets one wrong count eat every file
  // after it, and the reader loses them with nothing on screen to say why.
  const m = parseDiff(`diff --git a/a.go b/a.go
--- a/a.go
+++ b/a.go
@@ -1,9000 +1,9000 @@
+one
diff --git a/b.go b/b.go
--- a/b.go
+++ b/b.go
@@ -1,1 +1,1 @@
+two
`);
  assert.deepEqual(
    m.files.map((f) => f.name),
    ["a.go", "b.go"],
  );
  assert.equal(m.add, 2);
});

test("an empty line is still context", () => {
  // A context line whose trailing space was stripped on the way here. Reading it
  // as structure would end a hunk in the middle of an ordinary patch, which is a
  // far more common shape than a lying header.
  const m = parseDiff("--- a/x\n+++ b/x\n@@ -1,3 +1,3 @@\n one\n\n-two\n+three\n");
  assert.equal(m.files.length, 1);
  assert.deepEqual(
    m.files[0].lines.map((l) => l.cls),
    ["hunk", "ctx", "ctx", "del", "add"],
  );
});
