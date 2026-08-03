// Command genearcons synthesizes the agentbox earcons into
// internal/sound/assets. Run via go:generate from internal/sound; the WAVs
// are committed so builds never depend on this tool.
package main

import (
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"path/filepath"
)

const (
	sampleRate = 44100
	amp        = 0.35
)

type note struct {
	freq  float64 // Hz; 0 = silence
	durMS int
	decay float64 // exponential decay rate; higher dies faster
}

// Each earcon is a tiny melodic figure; sounds stay under 600 ms and quiet
// (03-ui-ux.md sound design).
var earcons = map[string][]note{
	"pop":     {{660, 90, 18}},                                                           // info: single soft pop
	"tick":    {{880, 70, 14}, {1175, 90, 14}},                                           // success: quick ascent
	"twotone": {{740, 120, 10}, {554, 140, 10}},                                          // warning: descending pair
	"chime":   {{659, 150, 8}, {988, 190, 8}},                                            // question: rising invitation
	"thud":    {{150, 220, 12}},                                                          // error: low landing
	"insist":  {{880, 110, 10}, {0, 30, 0}, {880, 110, 10}, {0, 30, 0}, {1109, 150, 10}}, // urgent
}

func synth(notes []note) []int16 {
	var out []int16
	for _, n := range notes {
		count := sampleRate * n.durMS / 1000
		for i := range count {
			t := float64(i) / sampleRate
			if n.freq == 0 {
				out = append(out, 0)
				continue
			}
			// Fundamental plus a soft second harmonic so the tone
			// is not a bare sine; 5 ms attack ramp kills clicks.
			v := math.Sin(2*math.Pi*n.freq*t) + 0.3*math.Sin(4*math.Pi*n.freq*t)
			env := math.Exp(-n.decay * t)
			if attack := 0.005; t < attack {
				env *= t / attack
			}
			out = append(out, int16(amp*env*v/1.3*math.MaxInt16))
		}
	}
	return out
}

func writeWAV(path string, samples []int16) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	dataLen := uint32(len(samples) * 2)
	w := func(v any) { binary.Write(f, binary.LittleEndian, v) }
	f.WriteString("RIFF")
	w(uint32(36 + dataLen))
	f.WriteString("WAVEfmt ")
	w(uint32(16))
	w(uint16(1)) // PCM
	w(uint16(1)) // mono
	w(uint32(sampleRate))
	w(uint32(sampleRate * 2))
	w(uint16(2))
	w(uint16(16))
	f.WriteString("data")
	w(dataLen)
	w(samples)
	return nil
}

func main() {
	dir := "assets"
	if len(os.Args) > 1 {
		dir = os.Args[1]
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	for name, notes := range earcons {
		path := filepath.Join(dir, name+".wav")
		if err := writeWAV(path, synth(notes)); err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", path, err)
			os.Exit(1)
		}
		fmt.Println("wrote", path)
	}
}
