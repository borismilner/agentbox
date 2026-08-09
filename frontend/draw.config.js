import { defineConfig } from "vite";
import { svelte } from "@sveltejs/vite-plugin-svelte";
import tailwindcss from "@tailwindcss/vite";
import { fileURLToPath } from "node:url";

// The vite server behind the wiki's DRAWN frames (FR99). It is vite.config.js
// with one substitution and no build output.
//
// The substitution is the daemon door: @wailsio/runtime becomes draw/runtime.js,
// which answers from a fixture instead of dispatching to Go. Everything above
// that door is the shipped code - the same Svelte components, the same app.css,
// the same token hashing - so a drawn frame is the product's own markup around
// content written for the page. What is invented is exactly the daemon's
// answers, which is the boundary FR99 argued for and the reason a frame may not
// say anything a real run would contradict.
//
// It SERVES rather than builds, on purpose, and it REFUSES to build - see the
// plugin below. `dist/` is committed and embedded with go:embed, so a build run
// against this config would overwrite the shipped bundle with one whose card
// answers from frames.js instead of the daemon, and `make build`'s own output
// would look untouched. That is not a hypothetical: vite's default outDir is
// `dist` relative to root, and root here is frontend/, so the damaging command
// is the obvious one somebody types when serving does not fit their workflow.
//
// The chunking rules in vite.config.js are deliberately not copied: they keep the
// committed bundle small and the card cheap to load, and nothing here is
// committed or fetched over a wire.
// The guard that makes the paragraph above true rather than merely intended.
// A config comment cannot stop `vite build --config draw.config.js`; this can.
function serveOnly() {
  return {
    name: "draw-serve-only",
    apply: "build",
    config() {
      throw new Error(
        "draw.config.js serves the wiki's frames and must never build.\n" +
          "vite's default outDir is dist/, which is committed and embedded with\n" +
          "go:embed - a build here would ship a product whose card answers from\n" +
          "frames.js instead of the daemon. Use tools/wiki/draw.py, and\n" +
          "`make build` (vite.config.js) for the real bundle.",
      );
    },
  };
}

export default defineConfig({
  root: fileURLToPath(new URL(".", import.meta.url)),
  plugins: [svelte(), tailwindcss(), serveOnly()],
  resolve: {
    alias: {
      "@wailsio/runtime": fileURLToPath(new URL("./draw/runtime.js", import.meta.url)),
    },
    conditions: ["browser"],
  },
  server: { host: "127.0.0.1", strictPort: false },
});
