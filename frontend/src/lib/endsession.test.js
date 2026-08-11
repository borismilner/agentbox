// The wording two surfaces share before they kill a child (U-09). Node's own
// runner, like diff.test.js: this module has no DOM in it, and the zero-dependency
// suite has to keep running on a machine with no node_modules.

import { test } from "node:test";
import assert from "node:assert/strict";

import { endQuestion, ARM_MS } from "./endsession.js";

// The two losses are different and the question has to name the right one.
test("a working session is asked about its work", () => {
  const q = endQuestion({ state: "working" });
  assert.match(q, /working here/);
  assert.match(q, /\?$/);
});

test("any other state is asked about the conversation", () => {
  for (const state of ["idle", "ended", "error", "", undefined]) {
    const q = endQuestion({ state });
    assert.match(q, /unsaved conversation/, `state ${JSON.stringify(state)}`);
  }
});

// U-09 itself: the panel used to treat idle as not worth a question. If idle ever
// stops producing a question again, it will be because someone reintroduced a
// branch here, and this is the line that says no.
test("idle is not exempt - it gets a real question, not an empty one", () => {
  const q = endQuestion({ state: "idle" });
  assert.ok(q.length > 20);
  assert.notEqual(q, endQuestion({ state: "working" }));
});

// A missing session must not throw: the panel's armed tooltip renders from
// whatever is in the row, and a row can be swapped out from under a push.
test("no session at all still reads as a question", () => {
  assert.match(endQuestion(undefined), /unsaved conversation/);
  assert.match(endQuestion(null), /unsaved conversation/);
});

test("the arm window is long enough to mean it and short enough to forget", () => {
  assert.ok(ARM_MS >= 1500 && ARM_MS <= 5000);
});
