//go:build ignore

// Throwaway: emits the palette-picker's data blob - the code sample rendered
// through the real markdown pipeline plus every code theme resolved through
// the real BuildTheme, so the picker previews exactly what agentbox will show.
// Run from the repo root: go run tools/mockups/gen-picker-data.go > data.json
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/borismilner/agentbox/internal/config"
	"github.com/borismilner/agentbox/internal/webui"
)

const sample = "```go\n" +
	`//go:build linux

// present decides the treatment an item deserves.
func (u *UI) present(v *daemon.View, retries int) (string, error) {
	const warmBudget = 300 * time.Millisecond
	kind, hue := treatment(v.Item), IdentityHue(v.Agent, true)
	if v.Item == nil || retries > maxRetries {
		return "", fmt.Errorf("nothing to show after %d tries: %w", retries, ErrEmpty)
	}
	labels := map[string]float64{"cold": 1.5, "warm": 0.3}
	for name, budget := range labels {
		u.log.Info("webui.show", "kind", kind, "budget", budget*float64(retries))
		go u.emit(name, fmt.Sprintf("\x1b[3%dm%s\x1b[0m", 2, hue))
	}
	return kind, nil
}
` + "```\n\n```python\n" + `@dataclass(frozen=True)
class Station:
    """One stop on the review route."""
    name: str
    verdict: Verdict = Verdict.PENDING

    @property
    def label(self) -> str:
        count = len(self.name) if self.name else 0
        return f"station {self.name!r} ({count} chars, {count / 2:.1f} pairs)"
` + "```\n\n```diff\n" + `@@ -381,7 +381,9 @@ func (h *Hand) Type(s string)
-	old := latin_layout(s)
-	h.press(old)
+	group := h.lockGroup(s)
+	defer group.release()
+	h.press(group.plan(s))
` + "```\n"

func main() {
	themes := map[string][2]webui.Theme{}
	for _, name := range config.CodeThemes {
		cfg := config.Default()
		cfg.Markdown.CodeTheme = name
		cfg.Theme.Mode = "dark"
		dark := webui.BuildTheme(cfg)
		cfg.Theme.Mode = "light"
		light := webui.BuildTheme(cfg)
		themes[name] = [2]webui.Theme{dark, light}
	}
	blob := map[string]any{
		"sample": webui.RenderMarkdown(sample),
		"order":  config.CodeThemes,
		"themes": themes,
	}
	out, err := json.Marshal(blob)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	os.Stdout.Write(out)
}
