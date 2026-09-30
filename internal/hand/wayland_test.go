package hand

import (
	"testing"

	"github.com/jezek/xgb/xproto"
)

func TestSourceFor(t *testing.T) {
	ids := []string{"il", "us"}
	cases := []struct {
		r    rune
		want string
	}{
		{'a', "us"}, {'Z', "us"}, {'é', "us"},
		{'ש', "il"}, {'ם', "il"}, {'א', "il"}, {'ת', "il"},
		{'1', ""}, {' ', ""}, {'.', ""}, {'\n', ""},
	}
	for _, c := range cases {
		if got, ok := sourceFor(c.r, ids); got != c.want || !ok {
			t.Errorf("sourceFor(%q) = %q %v, want %q", c.r, got, ok, c.want)
		}
	}
	// A letter no configured layout carries is refused, never sent to be dropped.
	for _, r := range []rune{'ש', 'Ж'} {
		if _, ok := sourceFor(r, []string{"us"}); ok {
			t.Errorf("sourceFor(%q) with only us must be refused", r)
		}
	}
}

func TestKeysymWL(t *testing.T) {
	// The il layout carries the legacy keysyms, not the Unicode ones.
	if got := keysymWL('א'); got != 0x0ce0 {
		t.Errorf("aleph = %#x, want 0xce0", got)
	}
	if got := keysymWL('ת'); got != 0x0cfa {
		t.Errorf("taw = %#x, want 0xcfa", got)
	}
	if got := keysymWL('a'); got != 'a' {
		t.Errorf("a = %#x", got)
	}
	if got := keysymWL('\n'); got != KeyReturn {
		t.Errorf("newline = %#x, want Return", got)
	}
}

func TestParseAccels(t *testing.T) {
	mods, ks, spec, err := parseAccels("['<Alt>Shift_L']")
	if err != nil || mods != uint16(xproto.ModMask1) || ks != 0xffe1 || spec != "<Alt>Shift_L" {
		t.Errorf("got %#x %#x %q %v", mods, ks, spec, err)
	}
	mods, ks, _, err = parseAccels("['XF86Keyboard', '<Super>space']")
	if err != nil || mods != uint16(xproto.ModMask4) || ks != 0x20 {
		t.Errorf("super+space: got %#x %#x %v", mods, ks, err)
	}
	if _, _, _, err := parseAccels("@as []"); err == nil {
		t.Error("an empty binding list must be an error, not a silent no-op")
	}
}

func TestPlanTextNilLayoutPacesEverything(t *testing.T) {
	strokes, skipped := PlanText("aש1 .", nil, Typing{WPM: 300})
	if len(strokes) != 5 || len(skipped) != 0 {
		t.Fatalf("got %d strokes, %d skipped", len(strokes), len(skipped))
	}
}
