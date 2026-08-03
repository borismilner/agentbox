// Package sound plays the agentbox earcons by spawning a system player
// (ADR-0006): no cgo, no audio device held open by the daemon. Assets are
// embedded and extracted once per daemon start.
package sound

//go:generate go run ../../tools/genearcons assets

import (
	"embed"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
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

// players in preference order (ADR-0006): PipeWire native, Pulse compat,
// bare ALSA.
var players = []string{"pw-play", "paplay", "aplay"}

var ErrNoPlayer = errors.New("no audio player found (tried pw-play, paplay, aplay)")

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

// volumeArgs translates the volume knob for the resolved player; aplay has
// no volume control, so it plays at asset level.
func volumeArgs(bin string, volume float64) []string {
	switch filepath.Base(bin) {
	case "pw-play":
		return []string{fmt.Sprintf("--volume=%.2f", volume)}
	case "paplay":
		return []string{fmt.Sprintf("--volume=%d", int(volume*65536))}
	default:
		return nil
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
	args := append(volumeArgs(p.bin, p.volume), filepath.Join(p.dir, string(class)+".wav"))
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
