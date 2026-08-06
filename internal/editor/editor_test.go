package editor

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func target() Target {
	return Target{Dir: "/repo", File: "/repo/pkg/client.go", Line: 1367, Col: 9}
}

func TestExpandJetBrainsShape(t *testing.T) {
	// The order this asserts is the one that was verified live and the one the
	// rejected form got wrong: project directory first, file last.
	got, err := Expand([]string{"goland", phDir, "--line", phLine, "--column", phCol, phFile}, target())
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	want := []string{"goland", "/repo", "--line", "1367", "--column", "9", "/repo/pkg/client.go"}
	if !slices.Equal(got, want) {
		t.Fatalf("argv = %q, want %q", got, want)
	}
}

func TestExpandSubstitutesInsideAWord(t *testing.T) {
	// VS Code's shape. If substitution were whole-word only this would come out
	// as the literal placeholder string and the click would open nothing.
	got, err := Expand([]string{"code", "--goto", phFile + ":" + phLine + ":" + phCol}, target())
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	want := []string{"code", "--goto", "/repo/pkg/client.go:1367:9"}
	if !slices.Equal(got, want) {
		t.Fatalf("argv = %q, want %q", got, want)
	}
}

func TestExpandKeepsASpacedPathAsOneArgument(t *testing.T) {
	got, err := Expand([]string{"code", phFile}, Target{File: "/repo/two words.go", Line: 3})
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	if len(got) != 2 || got[1] != "/repo/two words.go" {
		t.Fatalf("argv = %q, want the path unsplit", got)
	}
}

func TestExpandFloorsThePosition(t *testing.T) {
	// A block with no usable start reduces to the top of the file, not to line 0
	// or a negative one - both of which some editors reject outright.
	got, err := Expand([]string{"e", "--line", phLine, "--column", phCol, phFile}, Target{File: "/f.go"})
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	if got[2] != "1" || got[4] != "1" {
		t.Fatalf("argv = %q, want line and column 1", got)
	}
}

func TestExpandRefusesATemplateWithNoFile(t *testing.T) {
	if _, err := Expand([]string{"goland", phDir}, target()); !errors.Is(err, ErrNoFile) {
		t.Fatalf("err = %v, want ErrNoFile", err)
	}
}

func TestExpandRefusesAnEmptyProgram(t *testing.T) {
	for _, tmpl := range [][]string{nil, {}, {"", phFile}, {"  ", phFile}} {
		if _, err := Expand(tmpl, target()); err == nil {
			t.Fatalf("template %q was accepted", tmpl)
		}
	}
}

func TestResolveTakesTheConfiguredTemplate(t *testing.T) {
	cfg := []string{"my-editor", phFile, "+" + phLine}
	tmpl, src, err := Resolve(cfg)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if src != "config" || !slices.Equal(tmpl, cfg) {
		t.Fatalf("resolve = %q from %q, want the configured template", tmpl, src)
	}
}

func TestResolveRefusesAConfiguredTemplateWithNoFile(t *testing.T) {
	// Caught when the template is read rather than when the editor opens on
	// nothing, so the human is told the config is wrong and not the button.
	if _, _, err := Resolve([]string{"my-editor", phDir}); !errors.Is(err, ErrNoFile) {
		t.Fatalf("err = %v, want ErrNoFile", err)
	}
}

// stubPath puts a fake executable on PATH and nothing else, so detection is
// tested against a known machine rather than against whatever this one has.
func stubPath(t *testing.T, bins ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, b := range bins {
		p := filepath.Join(dir, b)
		if err := os.WriteFile(p, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", dir)
	return dir
}

func TestResolveDetectsAKnownLauncher(t *testing.T) {
	stubPath(t, "zed")
	tmpl, src, err := Resolve(nil)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	want := []string{"zed", "/repo/pkg/client.go:1367:9"}
	got, err := Expand(tmpl, target())
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	if src != "zed" || !slices.Equal(got, want) {
		t.Fatalf("detected %q -> %q, want zed -> %q", src, got, want)
	}
}

func TestResolvePrefersTheProjectAwareLauncher(t *testing.T) {
	// Both installed. The JetBrains one wins because it routes to the window
	// that is already open, which is the whole point of the feature.
	stubPath(t, "code", "goland", "subl")
	_, src, err := Resolve(nil)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if src != "goland" {
		t.Fatalf("detected %q, want goland", src)
	}
}

func TestResolveFallsBackToXdgOpenWithoutTheLine(t *testing.T) {
	stubPath(t, "xdg-open")
	tmpl, src, err := Resolve(nil)
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	got, err := Expand(tmpl, target())
	if err != nil {
		t.Fatalf("expand: %v", err)
	}
	if src != "xdg-open" || !slices.Equal(got, []string{"xdg-open", "/repo/pkg/client.go"}) {
		t.Fatalf("fallback = %q from %q", got, src)
	}
}

func TestResolveOnAMachineWithNothing(t *testing.T) {
	stubPath(t)
	if _, _, err := Resolve(nil); err == nil {
		t.Fatal("resolve found an editor on an empty PATH")
	}
}

func TestCommandRejectsAProgramThatIsNotThere(t *testing.T) {
	stubPath(t, "xdg-open")
	if _, _, err := Command([]string{"nosucheditor", phFile}, target()); err == nil {
		t.Fatal("a missing program was accepted")
	}
}

func TestCommandEndToEnd(t *testing.T) {
	dir := stubPath(t, "goland")
	argv, src, err := Command(nil, target())
	if err != nil {
		t.Fatalf("command: %v", err)
	}
	want := []string{"goland", "/repo", "--line", "1367", "--column", "9", "/repo/pkg/client.go"}
	if src != "goland" || !slices.Equal(argv, want) {
		t.Fatalf("argv = %q from %q, want %q", argv, src, want)
	}
	_ = dir
}

func TestEscapeLeavesTheServiceCgroup(t *testing.T) {
	// The daemon is KillMode=control-group, so an unwrapped child dies with it on
	// the next deploy. Assert the wrap is present and that the editor's own argv
	// survives it intact behind the -- separator.
	stubPath(t, "systemd-run")
	got := escape([]string{"goland", "/repo", "--line", "12", "/repo/f.go"})
	if got[0] != "systemd-run" || !slices.Contains(got, "--scope") {
		t.Fatalf("escape = %q, want a systemd-run scope", got)
	}
	i := slices.Index(got, "--")
	if i < 0 || !slices.Equal(got[i+1:], []string{"goland", "/repo", "--line", "12", "/repo/f.go"}) {
		t.Fatalf("escape = %q, want the editor argv after --", got)
	}
	// --unit would refuse the second file opened while the first scope lives.
	if slices.Contains(got, "--unit") {
		t.Fatalf("escape names a fixed unit: %q", got)
	}
}

func TestEscapeIsAPassThroughWithoutSystemd(t *testing.T) {
	stubPath(t)
	argv := []string{"goland", "/repo/f.go"}
	if got := escape(argv); !slices.Equal(got, argv) {
		t.Fatalf("escape = %q, want it untouched", got)
	}
}

func TestStartRunsTheProgram(t *testing.T) {
	// No systemd-run on this PATH, so the argv runs directly and the test can
	// wait on it - the wrapped path is asserted by TestEscape* instead.
	dir := stubPath(t)
	stamp := filepath.Join(dir, "argv")
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\" > " + stamp + "\n"
	if err := os.WriteFile(filepath.Join(dir, "fake-editor"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	cmd, err := Start([]string{"fake-editor", "--line", "12", "/repo/f.go"})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("wait: %v", err)
	}
	got, err := os.ReadFile(stamp)
	if err != nil {
		t.Fatalf("the program did not run: %v", err)
	}
	if string(got) != "--line\n12\n/repo/f.go\n" {
		t.Fatalf("the program saw %q", got)
	}
}
