// The React an artifact runs against, as the entry point of a bundle agentbox
// injects as text.
//
// Why text: an artifact document is sandboxed with an opaque origin and a CSP
// with no network at all, so nothing in it can be fetched - not from a CDN
// (agentbox works offline) and not from agentbox's own asset server either. React has
// to arrive inside the document. React 19 ships no UMD build, so agentbox builds
// its own IIFE from this file (frontend/tools/build-runtime.mjs) into
// generated/react-runtime.js, which artifact-runtime.js imports as a string.
//
// Globals plus a module table, not an import map: an artifact's
// `import React from "react"` is rewritten to require() by the transform in
// artifact-runtime.js, and require() resolves against this table. One mechanism
// then covers both shapes agentbox has to run - a claude.ai module that imports
// react, and a plain document that expects window.React the way a CDN script
// tag would have left it.

import React from "react";
import * as ReactJSXRuntime from "react/jsx-runtime";
import * as ReactDOMClient from "react-dom/client";
import ReactDOM from "react-dom";

// React 19 dropped ReactDOM.render. Artifacts written against the CDN era still
// call it, and failing on a line that used to work is a worse answer than
// forwarding it to the root API.
const dom = {
  ...ReactDOM,
  ...ReactDOMClient,
  render(element, container) {
    const root = ReactDOMClient.createRoot(container);
    root.render(element);
    return root;
  },
};

globalThis.React = React;
globalThis.ReactDOM = dom;
globalThis.__agentboxArtifactModules = {
  react: React,
  "react/jsx-runtime": ReactJSXRuntime,
  "react/jsx-dev-runtime": ReactJSXRuntime,
  "react-dom": dom,
  "react-dom/client": ReactDOMClient,
};
