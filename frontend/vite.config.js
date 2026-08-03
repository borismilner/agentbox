import { defineConfig } from "vite";
import { svelte } from "@sveltejs/vite-plugin-svelte";
import tailwindcss from "@tailwindcss/vite";

// Every agentbox window loads the same bundle and picks its surface from the
// query string (?surface=card|toast|app|viewer|progress). One build, one asset
// server, no per-window entry points to keep in sync.
//
// dist/ is committed and embedded with go:embed, so the shape of the output is a
// repository concern and not only a build detail. Mermaid lazily imports a chunk
// per diagram type, which left ~100 minified files in git for one feature; they
// are merged into a single diagrams chunk instead. It still loads on demand - a
// card showing a one-line question never fetches it - but a diagram costs one
// request and the repository carries one file.
// KaTeX ships each of its fonts three times - woff2, woff and truetype - and
// names all three in every @font-face. A browser stops at the first format it can
// read, and WebKitGTK reads woff2, so the other two are ~800 kB of binaries that
// would sit in git and inside the go:embed binary without ever being requested.
// dist is committed (see above), so that is a repository concern and not a build
// detail: drop them from the bundle, and from the src lists that name them.
//
// This is safe even if the rewrite ever stops matching: woff2 comes first in
// every list, so a leftover reference to a file that is gone is never reached.
function katexWoff2Only() {
  const dead = /KaTeX_[^/]*\.(?:woff|ttf)$/;
  return {
    name: "katex-woff2-only",
    generateBundle(_options, bundle) {
      let dropped = 0;
      for (const name of Object.keys(bundle)) {
        if (dead.test(name)) {
          delete bundle[name];
          dropped++;
        }
      }
      for (const item of Object.values(bundle)) {
        if (item.type !== "asset" || !item.fileName.endsWith(".css")) continue;
        const css =
          typeof item.source === "string" ? item.source : new TextDecoder().decode(item.source);
        if (!css.includes("KaTeX_")) continue;
        item.source = css.replace(
          /,\s*url\([^)]*KaTeX_[^)]*\.(?:woff|ttf)\)\s*format\("(?:woff|truetype)"\)/g,
          "",
        );
      }
      if (dropped) this.warn(`katex-woff2-only: dropped ${dropped} non-woff2 font files`);
    },
  };
}

export default defineConfig({
  plugins: [svelte(), tailwindcss(), katexWoff2Only()],
  build: {
    outDir: "dist",
    emptyOutDir: true,
    target: "esnext",
    assetsInlineLimit: 4096,
    chunkSizeWarningLimit: 3000,
    rollupOptions: {
      output: {
        manualChunks(id) {
          // The review board (FR58) loads on demand for the same reason:
          // a full review surface must never ride along with a card.
          if (id.includes("src/surfaces/Board.svelte") || id.includes("src/lib/board/")) {
            return "board";
          }
          // The artifact runtime is the second on-demand payload: the JSX
          // transform plus React and Tailwind as injected text (M10). It is
          // pinned to its own chunk for the same two reasons as diagrams - a card
          // with a one-line question must not fetch it, and the repository
          // carries one file rather than a handful.
          if (id.includes("src/artifact/generated") || id.includes("src/lib/artifact-runtime")) {
            return "artifacts";
          }
          if (!id.includes("node_modules")) return;
          if (/sucrase|@babel\/parser|lines-and-columns/.test(id)) return "artifacts";
          // KaTeX gets its own chunk rather than riding in diagrams. It arrived
          // here as one of mermaid's dependencies, which is why it used to belong
          // there, but math.go now imports it directly and math in prose is far
          // more common than a diagram: a formula must not drag a 3MB layout
          // engine in behind it. A diagram still gets both, which it needs.
          if (/node_modules[\\/]katex[\\/]/.test(id)) return "math";
          if (/mermaid|cytoscape|dagre|d3|@braintree|khroma|langium|marked|roughjs|ts-dedent|dompurify|internmap|delaunator|robust-predicates/.test(id)) {
            return "diagrams";
          }
        },
      },
    },
  },
});
