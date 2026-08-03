package proto

import "testing"

func TestWalkthroughID(t *testing.T) {
	id, err := NewWalkthroughID()
	if err != nil {
		t.Fatal(err)
	}
	if !ValidWalkthroughID(id) {
		t.Errorf("minted id %q does not validate", id)
	}
	for _, bad := range []string{"", "w", "wxyz", "a3f9c2a1b4d5e", "w3f9c2a1b4d", "w3F9C2A1B4D5"} {
		if ValidWalkthroughID(bad) {
			t.Errorf("ValidWalkthroughID(%q) = true", bad)
		}
	}
}
