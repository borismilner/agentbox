// Package frontend carries the built web UI into the binary. agentbox stays a
// single file you can copy: the Svelte bundle ships inside it, served to the
// webview from memory rather than from disk.
package frontend

import "embed"

//go:embed all:dist
var Dist embed.FS
