import { defineConfig } from "vite";
import { svelte } from "@sveltejs/vite-plugin-svelte";
import { fileURLToPath } from "node:url";

// The component tests (robustness.md R-40). Nothing in this repository executed a
// line of Svelte before this file existed, so every display defect in
// docs/backlog/ux.md was found by reading rather than by a failing test.
//
// Two deliberate boundaries.
//
// These live in test/ and not beside their sources, because `make test-js` runs
// `node --test "frontend/src/**/*.test.js"` over the pure modules and node cannot
// import a .svelte file. Keeping the globs disjoint means the zero-dependency
// tests keep running on a machine with no node_modules, which is the same reason
// frontend/dist is committed. `make test-svelte` skips instead of failing there.
//
// @wailsio/runtime is aliased rather than mocked per test. It is the only import
// in a surface's chain that cannot exist outside a webview; mermaid, katex and the
// artifact runtime are all lazy and never load for a card.
export default defineConfig({
  plugins: [svelte()],
  resolve: {
    alias: {
      "@wailsio/runtime": fileURLToPath(new URL("./test/stubs/wailsio-runtime.js", import.meta.url)),
    },
    // The browser condition is what gives us Svelte's client runtime; without it
    // vite resolves the SSR build and mount() is not what a DOM test needs.
    conditions: ["browser"],
  },
  test: {
    environment: "jsdom",
    include: ["test/**/*.test.js"],
    restoreMocks: true,
  },
});
