package hotkey

import "testing"

// Parse is the only part of a key grab that can be tested without a display, and
// it is the part a user gets wrong: the spec comes out of a config file.
func TestParseCombos(t *testing.T) {
	cases := []struct {
		spec string
		mods uint16
		ks   uint32
	}{
		{"Super+grave", modSuper, 0x0060},
		{"super+GRAVE", modSuper, 0x0060},
		{"Grave+Super", modSuper, 0x0060}, // order does not matter
		{"Ctrl+Alt+grave", modCtrl | modAlt, 0x0060},
		{"Super+`", modSuper, 0x0060},
		{"Ctrl+Shift+k", modCtrl | modShift, 0x006b},
		{"Alt+F12", modAlt, 0xffc9},
		{"Super+space", modSuper, 0x0020},
		{"Meta+1", modSuper, 0x0031},
	}
	for _, c := range cases {
		mods, ks, err := Parse(c.spec)
		if err != nil {
			t.Errorf("%q: %v", c.spec, err)
			continue
		}
		if mods != c.mods || uint32(ks) != c.ks {
			t.Errorf("%q = mods %#x keysym %#x, want mods %#x keysym %#x", c.spec, mods, ks, c.mods, c.ks)
		}
	}
}

func TestParseRejectsNonsense(t *testing.T) {
	for _, spec := range []string{
		"",             // nothing at all
		"Super",        // modifier with no key
		"Super+a+b",    // two keys
		"Ctrl+wibble",  // not a key we know
		"Super+euro €", // not Latin-1, and not a name
	} {
		if _, _, err := Parse(spec); err == nil {
			t.Errorf("%q should not parse", spec)
		}
	}
}

// A bare key would be taken from every other application on the desktop, which
// is never what a config meant to say.
func TestOpenRefusesAModifierlessCombo(t *testing.T) {
	if _, err := Open("grave", nil, func() {}); err == nil {
		t.Error("a hotkey with no modifier must be refused")
	}
}
