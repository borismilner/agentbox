// Package sound plays the agentbox earcons by spawning a system player
// (ADR-0006): no cgo, no audio device held open by the daemon. Assets are
// embedded and extracted once per daemon start.
package sound

//go:generate go run ../../tools/genearcons assets

import (
	"embed"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/borismilner/agentbox/internal/logging"
	"github.com/borismilner/agentbox/internal/proto"
)

//go:embed assets/*.wav
var assetFS embed.FS

// Class names an earcon; see 03-ui-ux.md sound design.
type Class string

const (
	ClassInfo     Class = "pop"
	ClassSuccess  Class = "tick"
	ClassWarning  Class = "twotone"
	ClassQuestion Class = "chime"
	ClassError    Class = "thud"
	ClassUrgent   Class = "insist"
)

// ClassFor maps an item to its earcon: blocking items invite with the
// question chime regardless of level, urgent always insists.
func ClassFor(it *proto.Item) Class {
	if it.EffectiveLevel() == proto.LevelUrgent {
		return ClassUrgent
	}
	if it.Blocking() {
		return ClassQuestion
	}
	switch it.EffectiveLevel() {
	case proto.LevelSuccess:
		return ClassSuccess
	case proto.LevelWarning:
		return ClassWarning
	case proto.LevelError:
		return ClassError
	default:
		return ClassInfo
	}
}

// players in preference order (ADR-0006): PipeWire native, Pulse compat, bare
// ALSA, then the two that only exist elsewhere.
//
// One list rather than a build tag, because resolution is by LookPath and the
// order does the work: on Linux pw-play still wins and nothing below it is ever
// reached, and on a machine with none of the first three the next name that EXISTS
// is the one that plays. A tag here would buy nothing and would have to be kept in
// step with a comment saying what each platform does.
//
//   - afplay ships with macOS.
//   - powershell is the Windows fallback: there is no bundled wav player on the
//     PATH, so the earcon is played through Media.SoundPlayer (see playArgs).
var players = []string{"pw-play", "paplay", "aplay", "afplay", "powershell"}

var ErrNoPlayer = fmt.Errorf("no audio player found (tried %s)", strings.Join(players, ", "))

// playing pairs the process with a channel the reaper closes; everyone
// else waits on the channel because exec.Cmd.Wait must only be called
// once.
type playing struct {
	cmd  *exec.Cmd
	done chan struct{}
}

type Player struct {
	log *slog.Logger
	bin string
	dir string

	mu      sync.Mutex
	enabled bool
	volume  float64
	quiet   func(time.Time) bool
	current *playing
}

// Configure applies the [sound] knobs; safe to call live (config reload).
// quiet, when non-nil, suppresses everything below urgent during the
// window (quiet hours).
func (p *Player) Configure(enabled bool, volume float64, quiet func(time.Time) bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.bin == "" {
		enabled = false // no player binary trumps config
	}
	p.enabled = enabled
	p.volume = volume
	p.quiet = quiet
}

// playerName is the player's bare name out of whatever LookPath answered. It cuts
// on BOTH separators rather than using filepath.Base, which only knows the
// separator of the machine it is compiled for - so a Windows path handed to a Linux
// build (a test, a config file copied between machines) came back whole and matched
// nothing. That was a silent fallthrough to "just play the file", which is the one
// branch that looks like it worked.
func playerName(bin string) string {
	if i := strings.LastIndexAny(bin, `/\`); i >= 0 {
		bin = bin[i+1:]
	}
	return strings.TrimSuffix(bin, ".exe")
}

// playArgs is everything after the binary: the volume knob in whatever dialect the
// resolved player speaks, and the file. aplay has no volume control at all, so it
// plays at asset level - the one player where the knob does nothing, and it is last
// among the Linux three for exactly that reason.
func playArgs(bin string, volume float64, path string) []string {
	switch playerName(bin) {
	case "pw-play":
		return []string{fmt.Sprintf("--volume=%.2f", volume), path}
	case "paplay":
		return []string{fmt.Sprintf("--volume=%d", int(volume*65536)), path}
	case "afplay":
		// -v is a linear gain where 1 is the file's own level, which is the same
		// meaning pw-play's --volume has.
		return []string{"-v", fmt.Sprintf("%.2f", volume), path}
	case "powershell":
		// Media.SoundPlayer has no volume of its own, so the knob is not honoured
		// here - the same gap aplay has, and for the same reason it is acceptable:
		// an earcon at asset level is still an earcon. PlaySync because the process
		// exiting early would cut the sound, and the reaper is already waiting on it.
		// -NoProfile so a user's profile script cannot slow down or break a chime.
		return []string{"-NoProfile", "-NonInteractive", "-Command",
			fmt.Sprintf("(New-Object Media.SoundPlayer '%s').PlaySync()", path)}
	default:
		return []string{path}
	}
}

// New extracts the earcons into dir and resolves the player binary. A
// missing player disables sound but is not fatal: agentbox without audio
// still works (the error is logged once here).
func New(log *slog.Logger, dir string, lookPath func(string) (string, error)) *Player {
	p := &Player{log: log, dir: dir, volume: 0.4}
	if err := extract(dir); err != nil {
		log.Error(logging.EvSoundFailed, "component", "sound", "err", fmt.Sprintf("extract assets: %v", err))
		return p
	}
	for _, candidate := range players {
		if path, err := lookPath(candidate); err == nil {
			p.bin = path
			p.enabled = true
			return p
		}
	}
	log.Error(logging.EvSoundFailed, "component", "sound", "err", ErrNoPlayer.Error())
	return p
}

func extract(dir string) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	entries, err := assetFS.ReadDir("assets")
	if err != nil {
		return err
	}
	for _, e := range entries {
		data, err := assetFS.ReadFile("assets/" + e.Name())
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(dir, e.Name()), data, 0o600); err != nil {
			return err
		}
	}
	return nil
}

// Play starts the earcon without blocking. While something is playing,
// lesser sounds are skipped and urgent replaces (one player at a time,
// ADR-0006).
func (p *Player) Play(class Class) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.enabled {
		return
	}
	if class != ClassUrgent && p.quiet != nil && p.quiet(time.Now()) {
		p.log.Debug(logging.EvSoundPlayed, "component", "sound", "class", string(class), "skipped", "quiet_hours")
		return
	}
	if p.current != nil {
		if class != ClassUrgent {
			p.log.Debug(logging.EvSoundPlayed, "component", "sound", "class", string(class), "skipped", "busy")
			return
		}
		p.current.cmd.Process.Kill()
		p.current = nil
	}
	args := playArgs(p.bin, p.volume, filepath.Join(p.dir, string(class)+".wav"))
	cmd := exec.Command(p.bin, args...)
	if err := cmd.Start(); err != nil {
		p.log.Error(logging.EvSoundFailed, "component", "sound", "class", string(class), "err", err.Error())
		return
	}
	cur := &playing{cmd: cmd, done: make(chan struct{})}
	p.current = cur
	p.log.Debug(logging.EvSoundPlayed, "component", "sound", "class", string(class))
	go func() {
		cmd.Wait()
		close(cur.done)
		p.mu.Lock()
		if p.current == cur {
			p.current = nil
		}
		p.mu.Unlock()
	}()
}

// Wait blocks until the current sound finishes; used by tests and by the
// CLI demo so short-lived processes do not orphan players.
func (p *Player) Wait() {
	p.mu.Lock()
	cur := p.current
	p.mu.Unlock()
	if cur != nil {
		<-cur.done
	}
}
