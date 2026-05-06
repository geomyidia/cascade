package pkgc

import "testing"

func TestLoud(t *testing.T) {
	if got := Loud(); got != "Hello!" {
		t.Errorf("Loud() = %q, want %q", got, "Hello!")
	}
}
