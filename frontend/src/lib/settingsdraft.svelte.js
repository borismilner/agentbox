// The settings draft: what a person has changed and not yet written (U-10).
//
// It lives in a module rather than in Settings.svelte because the shell swaps
// surfaces by destroying them - App.svelte renders one branch of an {#if} chain
// - so every field of that component's state went away with the tab. Settings
// says three times over that it is holding pending edits (a dot per knob, a
// count in the footer, a Revert button), and then a click on any rail icon threw
// them away without a word. The live preview made it likelier rather than less:
// the theme is already applied to the preview, so the change looks committed.
//
// The backlog entry proposed a confirmation on the way out. Keeping the draft
// here is the other half of that entry, and it removes the need for the first:
// nothing is lost, so there is nothing to ask about, and a modal between a
// person and the tab they clicked is an interruption rather than a rescue.
//
// It also covers what a guard on the rail could not. The rail is not the only
// way out of Settings: `agentbox inbox`, the tray menu and any other caller of
// ShowApp raise the window on a named surface, which pushes agentbox:surface and
// swaps what is in front with no click in this window at all
// (internal/webui/app.go:38). A guard would have covered one of the two doors.
//
// What survives is the whole surface and not only the edited values: the section
// you were reading, and the result of the last Save. Coming back gives you what
// you left. What is deliberately NOT here is anything bound to the DOM - the
// preview element and its debounce belong to whichever mount is on screen.

// One object rather than a field per export: a `let` exported from a module is
// read once at import and never updates, and the state proxy handed out here is
// the same one every mount of the surface writes through.
export const draft = $state({
  // The schema and current file values as Go last described them, or null before
  // the first read. The surface renders nothing until this arrives.
  data: null,
  // knob id -> value as shown. This is the part a tab change used to destroy.
  values: {},
  // knob id -> value in config.toml as of the last read. `values` differing from
  // `base` is the whole definition of a pending edit, so the two travel together
  // and a re-read has to replace both at once (Settings.svelte's load).
  base: {},
  // Which section tab is open. Not an edit, but part of where you were.
  active: "appearance",
  saving: false,
  note: "",
  written: [],
  err: "",
});

// The knobs whose shown value differs from the file, as knob objects: the
// footer counts them, names them and reads `restart` off them, and the rail only
// wants how many. Reading `draft` here means a $derived in either place tracks
// it without the caller knowing how a knob is stored.
export function unsavedKnobs() {
  if (!draft.data) return [];
  return draft.data.sections
    .flatMap((s) => s.groups.flatMap((g) => g.knobs))
    .filter((k) => draft.values[k.id] !== draft.base[k.id]);
}
