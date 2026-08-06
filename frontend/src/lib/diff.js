// The unified-diff parser, shared by the card that asks for a review and the
// inbox detail that reads a resolved one back (FR73). One parser, because the
// detail's whole promise is that an item reads the same way after it closed as
// it did on screen - two parsers would eventually disagree about the same diff.
//
// A unified diff already carries the structure a reader wants: files, then
// hunks, then lines. This rebuilds it rather than painting every line the same.
// Hunk headers carry line counts, and consuming them is what keeps a content
// line that happens to start with "---" or "+++" from being read as a file
// header - the classic unified-diff ambiguity.
//
// The output is data, never HTML: the surfaces' rule is that the only escape
// from a text node is an allowlisted field (frontend/policy_test.go), and a
// diff is agent-authored text.

// DIFF_CAP bounds how many LINES render, not how much structure: every file
// keeps its header so a rail can always jump somewhere, and a truncated file
// says so.
export const DIFF_CAP = 400;

// isBody: can this line be part of a hunk's body at all? A unified diff marks
// every body line (' ', '+', '-', or the `\` of "no newline at end of file"),
// and an empty line is a context line whose trailing space was stripped
// somewhere on the way here - common enough that reading it as structure would
// break ordinary patches.
const isBody = (t) => t === "" || " +-\\".includes(t[0]);

export function parseDiff(raw, cap = DIFF_CAP) {
  const model = { files: [], add: 0, del: 0 };
  if (!raw.trim()) return model;
  let cur = null;
  let remOld = 0,
    remNew = 0; // unconsumed body lines of the open hunk
  const open = (name) => {
    cur = { name, badge: "", add: 0, del: 0, lines: [] };
    model.files.push(cur);
    return cur;
  };
  const put = (text, cls) => {
    if (!cur) open("");
    cur.lines.push({ text: text === "" ? " " : text, cls });
  };
  const strip = (p) => p.replace(/^[ab]\//, "").split("\t")[0]; // plain diffs put "\tTIMESTAMP" after the path
  for (const t of raw.replace(/\n$/, "").split("\n")) {
    // A hunk header is a claim about how much body follows, and the diff is
    // agent-authored text. Consuming on trust lets one wrong count eat every
    // file after it, and the reader loses them with nothing on screen to say
    // why. A line that cannot be a body line is the evidence against the claim.
    // internal/change ends a hunk on exactly the same rule.
    if ((remOld > 0 || remNew > 0) && !isBody(t)) remOld = remNew = 0;
    if (remOld > 0 || remNew > 0) {
      if (t.startsWith("+")) {
        remNew--;
        cur.add++;
        model.add++;
        put(t, "add");
      } else if (t.startsWith("-")) {
        remOld--;
        cur.del++;
        model.del++;
        put(t, "del");
      } else if (t.startsWith("\\")) {
        put(t, "meta"); // "\ No newline at end of file" - worth seeing
      } else {
        remOld--;
        remNew--;
        put(t, "ctx");
      }
      continue;
    }
    const hunk = t.match(/^@@+ -\d+(?:,(\d+))? \+\d+(?:,(\d+))? @@/);
    if (hunk) {
      remOld = hunk[1] === undefined ? 1 : Number(hunk[1]);
      remNew = hunk[2] === undefined ? 1 : Number(hunk[2]);
      put(t, "hunk");
      continue;
    }
    if (t.startsWith("diff --git ")) {
      const m = t.match(/^diff --git a\/(.*) b\/(.*)$/);
      open(m ? m[2] : "");
      continue;
    }
    if (t.startsWith("--- ")) {
      // Opens a file in plain `diff -u` output; inside a git section (no
      // body yet) it only refines the name.
      if (!cur || cur.lines.length) open("");
      if (!cur.name && t !== "--- /dev/null") cur.name = strip(t.slice(4));
      continue;
    }
    if (t.startsWith("+++ ")) {
      if (!cur) open("");
      if (t === "+++ /dev/null") cur.badge = "deleted";
      else cur.name = strip(t.slice(4));
      continue;
    }
    if (t.startsWith("new file mode")) {
      if (cur) cur.badge = "new";
      continue;
    }
    if (t.startsWith("deleted file mode")) {
      if (cur) cur.badge = "deleted";
      continue;
    }
    if (t.startsWith("rename to ")) {
      if (cur) {
        cur.badge = "renamed";
        cur.name = t.slice(10);
      }
      continue;
    }
    if (
      /^(index |old mode|new mode|similarity|dissimilarity|rename from|copy )/.test(
        t,
      )
    )
      continue;
    // Malformed or hand-written diffs land here: classify by first character
    // so they still render - and still count - just without the structure.
    if (t.startsWith("+")) {
      if (!cur) open("");
      cur.add++;
      model.add++;
      put(t, "add");
    } else if (t.startsWith("-")) {
      if (!cur) open("");
      cur.del++;
      model.del++;
      put(t, "del");
    } else if (t.startsWith(" ") || t === "") put(t, "ctx");
    else put(t, "meta");
  }
  // The cap is on diff lines, not structure: every file keeps its header so
  // the rail can always jump somewhere, and a truncated file says so.
  let budget = cap;
  for (const f of model.files) {
    f.shown = f.lines.slice(0, Math.max(0, budget));
    f.more = f.lines.length - f.shown.length;
    budget -= f.shown.length;
  }
  return model;
}
