# Writing a walkthrough

A agentbox walkthrough is a durable, step-by-step code review the human walks on the
board and hands back in one turn. Read this before authoring one. Every rule
here was earned by getting it wrong first and being told so.

`create_walkthrough` takes the spec; this is how to fill it. The field
reference is in `agentbox docs agent`; this document is about the reading, not the
fields.

## The job

One reader has to sign off on a change. They will read what you write once, on
a screen, with a limited amount of attention, and then say yes or no. Your job
is to spend their attention well: at the end they should be able to rebuild
your reasoning, and to say exactly which parts they do not believe.

That is the whole test. Not "did I cover it", not "does it sound thorough".
**Can they rebuild it, and can they doubt it in the right places.**

If the change needs one yes/no, use `request_review` instead. A walkthrough is
expensive to author and expensive to read; spend it where teaching is the point.

## Write like a person explaining to a colleague

1. **Plain words, short sentences.** Say "runs twice" not "exhibits duplicate
   invocation semantics". If a sentence needs a second read to parse, it is
   costing attention the code deserves.
2. **Never be clever.** No wordplay, no suspense, no withheld punchline, no
   flourish in a heading. The reader is not an audience. Every sentence exists
   to make the next piece of code obvious.
3. **Say the thing, then stop.** Cut any sentence the previous one already
   implied. A short step is not a lazy step; it is a step the reader finishes.
4. **Second person, present tense.** "You are looking at the fetch loop. It
   asks for one page at a time." Not "the reader will observe that...".
5. **No filler and no throat-clearing.** Start at the content. "It is worth
   noting that" is never worth writing.

## The shape of one step

6. **Orient before you explain.** Open by placing the reader: what arrived
   here, what this step does with it, what leaves and which step picks it up. A
   reader who knows the shape of the journey does not have to hold it together
   themselves.
7. **State what changed before showing any code.** When everything in view is
   new, say so outright. Anyone reviewing a branch assumes emphasis means
   "changed"; leaving them to infer it is what made the first field draft
   confusing.
8. **Lead-in, code, takeaway - and the spec has a field for each.** Never show
   code before the reason to look at it. `lead` on the block says what is
   coming and which question it answers; `close` on the step says what to
   remember, after they have seen it. Code first forces the reader to hold
   un-interpreted syntax in their head; a takeaway written before the code is
   a promise rather than a conclusion.
9. **One idea per step.** If a step needs two takeaways it is two steps. If a
   step's code block needs scrolling to take in, it is two blocks.
10. **End by handing off.** The last line of a step points at the next: what is
    still unexplained and where it gets explained. A walk with no handoffs is a
    list, and a reader who cannot see the thread stops following it.
11. **No forward reference without an explicit handoff.** If a function is split
    across steps, say where the rest is, which step owns it, and that nothing is
    being skipped. Unannounced gaps read as omissions.
12. **Finish with a reading order.** After the explanation, tell the reader the
    order to read the real code in, which is often not file order. State that
    the list is complete for that step, and mark explicitly what not to read
    yet. Explaining and reading are different activities; do the first so the
    second is easy.

## The TL;DR: write it for the reader who will not read the step

**The board opens in TL;DR.** Not the step - the TL;DR. So for most readers this
is what your review IS, and the prose beneath it is what a reader chooses when
your summary has convinced them the detail is worth the time. Write it that way
round.

17. **It is not the shortened version.** The reader it is for has a very short
    attention span and must still come away with mastery of the most important
    aspects of this step. Nothing important gets cut; what changes is the
    STRUCTURE. Prose asks to be read from the start and held to the end; a
    TL;DR asks to be glanced at and must survive being stopped anywhere.
18. **`bottom` is the one sentence that has to survive.** If the reader reads
    that line and nothing else on the step, what must they know? Write that.
    Not the topic ("how the backfill decides what to repair") - the finding
    ("the backfill re-reads every row, so a partial run costs the same as a
    full one").
19. **Each point stands alone.** Up to six, each a load-bearing fact that makes
    sense on its own and in any order. A point that only makes sense after the
    one above it is a paragraph that has been chopped up, and the reader who
    stops at point three has been left with half a thought.
20. **Lead with the consequence, not the mechanism.** "This is O(n²) above about
    5k rows" before "the loop calls Lookup inside Walk". A short attention span
    spends its budget on the first clause of every line.
21. **A point that will not fit its cap is two points, or belongs in the prose.**
    The caps exist to keep the shape; if the important thing genuinely needs a
    paragraph, the prose is where it goes and the point says what the paragraph
    concludes.
22. **Write it last, from the finished step.** A TL;DR written first is an
    outline, and outlines summarise what you meant to say rather than what the
    step ended up establishing.

## Order the code for understanding, not for the file

23. **Follow one thing through the system.** The best default order is the path
    the data takes: where it enters, what happens to it, where it lands. File
    order is an accident of how the code was typed and is almost never the
    order that teaches.
24. **Ground before consequence.** Show the piece that stands on its own first
    (the shape of the data, the one function everything calls), then the code
    that depends on it. A reader who meets a call before the thing it calls has
    to hold an unknown in their head while you talk.
25. **Say why this order, once, at the start.** One sentence in the first step:
    "we follow one CVE from the feed to the row it updates". That sentence is
    what lets the reader predict where they are going, and predicting is what
    makes reading cheap.
26. **Cite the region worth reading, not the file.** A block caps at 400 lines,
    but that is a limit, not a target. Cite the ten lines the point lives in.

## Put each kind of text where it belongs

The board has five channels. Using the wrong one is the most common way a good
review reads badly, and the most common mistake is putting the whole
explanation in the paragraphs above the code.

**The weight goes next to the code.** A note sits in the margin beside the
exact lines it explains: the reader's eye moves inches, not screens, and they
never have to remember what a sentence was about. Prose above the code cannot
do that - by the time they reach the lines, the sentence is gone. So the
paragraphs open and close, and the notes carry the substance. If a step's prose
is long and its margin is empty, the step is written backwards.

27. **Prose introduces.** Where we are, what is about to happen, why it matters
    here. One or two short paragraphs. Not the explanation itself.
28. **`lead` hands the reader into one block.** A sentence or two on the block
    itself, rendered directly above it: what this is, and why it comes now. A
    step with more than one block needs one on each - without them, all the
    text stacks above the first block and the second arrives cold.
29. **Notes are the explanation, next to the lines.** `{"at": [from, to],
    "text": "..."}` on a block. The board numbers them, puts a badge in the
    gutter and the text in the margin beside those lines. Reasons, objections,
    consequences, the thing that is easy to miss - all of it goes here. If you
    find yourself writing "on the line that does X" in prose, that sentence is
    a note.
30. **`close` is the takeaway, under the code.** One paragraph after the last
    block: the sentence you want them to keep. Above the code it would be a
    promise; below it, it is a conclusion they have just watched you earn.
31. **The glossary is for definitions** (see below). A definition inlined in
    prose interrupts every reader to help the few who needed it.
32. **Checks are for retrieval, not recall.** Ask what happens given a scenario,
    never what a line says. The answer should be reconstructible from the step,
    and revealing it should confirm reasoning rather than supply a fact.
33. **Keep findings out of the explanation.** "How this works" and "what might
    be wrong with it" are different modes; interleaved, the reader cannot tell
    whether they are learning or judging. Give doubts their own step.
34. **Break prose into paragraphs.** Prose is a list of inline segments, because
    a bound phrase has to sit mid-sentence. Set `"p": true` on the segment that
    begins each paragraph, in `prose` and in `close` alike. Without it the step
    renders as one wall with sentences fused across the seam.

## The glossary

35. **Define what this reader cannot guess.** `glossary: [{term, short, body?,
    also?}]`. The board marks the first occurrence of each term in each step
    with a quiet underline and opens the definition only when the reader asks -
    a click, or `g` for the whole list. Nothing pops on hover.
36. **Judge by the reader, not by the field.** An acronym from the domain the
    change touches (NVD, SSVC, KEV), a house term with a local meaning, a
    library idiom that looks like something else. Not words they use daily. A
    glossary of forty obvious words is the same as no glossary.
37. **`short` is one sentence that stands alone.** It is all most readers will
    open. Expand the "so what" in `body`: where it comes from, why it is in
    this change, what it is often confused with.
38. **Spell it in the prose the way the entry spells it**, or list the other
    spelling in `also`. A term nothing says is effort no reader can reach, and
    agentbox warns about it. Terms inside a bound phrase or an inline code chip are
    never marked, so do not rely on those to introduce one.

## Hunt for the aha

39. **Find the thing a careful reader would still miss.** Every change has one
    or two: the reason two guards are not redundant, the ordering that makes a
    race impossible, the one call that makes an expensive-looking loop cheap,
    the failure this design quietly removes. Finding these is most of the value
    you add over the diff.
40. **Give it its own beat.** Set it up ("this looks like it does the work
    twice"), then land it ("it cannot: the second call only ever sees rows the
    first one skipped"). Do not bury it in a list.
41. **Explain mechanisms, not labels.** "We cannot name the key in advance" is
    an assertion the reader must take on trust. "A struct tag is a string
    literal fixed at compile time, and this key's name carries a version the
    publisher changes on its own schedule" is an explanation they can
    reconstruct. If the text does not let a reader rebuild the reasoning, it
    has not explained anything.
42. **Answer the obvious objection in place.** When code does something
    unusual, the reader is already thinking of the neighbouring code that does
    not. Say it for them and answer it, in a note on those lines.
43. **Show code in any panel that makes a claim about code.** A step describing
    what a test proves, with no test code in it, is a summary standing in for
    evidence. Cite the two or three lines that carry the point.

## Pointing at code

44. **Every code reference is a full repo-relative path.** Never a basename.
    Two files called `client.go` in one review is normal, and an ambiguous
    reference sends the reader to the wrong package.
45. **Never write a line number into a sentence.** Bind the phrase instead:
    `{"t": "the guard", "bind": "guard"}` plus `"binds": {"guard": {"block": 0,
    "lines": [77, 79]}}`. The phrase then lights that region when the reader is
    on it. Every literal number in prose is a claim that expires at the next
    edit, silently. The validator refuses them, with directions.
46. **Give the reader something to run.** A `cmd` with the command, the result
    to expect, and the date that expectation last held. Best of all is a way to
    break it on purpose and watch a specific thing fail - that turns a claim
    about the code into an observation. Only record what you actually ran.

## Coverage, and honesty about it

47. **Give a complete route, not only the interesting parts.** Thematic steps
    explain a change; they do not account for it. Include one traversal that
    covers every changed line, in commit order, say plainly that it is
    complete, and annotate whatever the thematic steps never stood on. A reader
    who cannot tell whether the tour was curated has to assume it was, and then
    cannot sign anything off.
48. **Name what you deliberately left out.** `out_of_scope` takes a glob and a
    reason. An unexplained exclusion is a hole; a stated one is a decision.
    This is also where an absence goes: no tests, no migration, no wiring.
49. **End with a check step.** The last station is the gate: the exact build,
    lint and test commands, so "I reviewed it" resolves to something that
    either passed or did not. Finishing is an observation, not a feeling.
50. **Say what you did not verify.** Next to the gate, state plainly what was
    never exercised - a path you could not run, an environment you did not
    have. Silence reads as approval.

## Before you call create_walkthrough

51. **Verify every citation against the tree, and again after any amend or
    rebase.** A walkthrough citing the wrong lines is worse than none, because
    the reader trusts it and loses the time twice. Keep the check one command,
    and keep each range in exactly one place so the check cannot disagree with
    the page it checks.
52. **Pin the sha the citations are true against**, and pass the change's diff.
    The diff is the only carrier of added/removed knowledge - never state diff
    status on a file-backed block; agentbox derives every marking from the manifest.
53. **Read the warnings.** They are teaching notes, not noise, and they are the
    cheapest review you will get.

## Handing it back

`await_walkthrough` blocks until the human submits and returns the whole
handback: the unclear steps first, each with the note saying what is unclear.
Answer those first. If your session ends before they finish, nothing is lost -
the next session calls `read_walkthrough` with `ack: true` and takes the
submission exactly once. Leave finished reviews in the library; they are the
human's record of what was reviewed and what they said.
