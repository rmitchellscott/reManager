package executor

import (
	"os/exec"
	"strings"
	"testing"
)

func TestQuoteArgsPassesEachValueThroughLiterally(t *testing.T) {
	values := []string{
		"plain",
		"$(echo injected)",
		"`echo injected`",
		"$HOME",
		"it's",
		"'!name'",
		"a b\tc",
		"; echo injected",
		"line\nbreak",
		"",
	}

	out, err := exec.Command("sh", "-c", `printf '%s\0' `+QuoteArgs(values)).Output()
	if err != nil {
		t.Fatalf("script failed: %v", err)
	}

	got := strings.Split(strings.TrimSuffix(string(out), "\x00"), "\x00")
	if len(got) != len(values) {
		t.Fatalf("expected %d arguments, got %d: %q", len(values), len(got), got)
	}
	for i := range values {
		if got[i] != values[i] {
			t.Errorf("argument %d: got %q, want %q", i, got[i], values[i])
		}
	}
}
