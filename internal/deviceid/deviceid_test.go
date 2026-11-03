package deviceid

import (
	"encoding/hex"
	"strings"
	"testing"
)

const rmppMachineID = "190c6209dabc71e9190c6209dabc71e9"

func TestDeriveIsStable(t *testing.T) {
	first, err := Derive(rmppMachineID)
	if err != nil {
		t.Fatalf("Derive: %v", err)
	}

	second, err := Derive(rmppMachineID + "\n")
	if err != nil {
		t.Fatalf("Derive with trailing newline: %v", err)
	}
	if first != second {
		t.Fatalf("a trailing newline changed the result: %q vs %q", first, second)
	}
}

func TestDeriveDoesNotLeakTheMachineID(t *testing.T) {
	derived, err := Derive(rmppMachineID)
	if err != nil {
		t.Fatalf("Derive: %v", err)
	}
	if strings.Contains(derived, rmppMachineID) {
		t.Fatalf("derived value contains the machine ID verbatim")
	}
	// The device's own ID is one 64-bit value written twice, so a derivation that carried that structure through would not be hiding much.
	if derived[:16] == derived[16:] {
		t.Fatalf("derived value repeats its own halves: %q", derived)
	}
}

func TestDeriveDistinguishesDevices(t *testing.T) {
	first, err := Derive(rmppMachineID)
	if err != nil {
		t.Fatalf("Derive: %v", err)
	}
	second, err := Derive("0123456789abcdef0123456789abcdef")
	if err != nil {
		t.Fatalf("Derive: %v", err)
	}
	if first == second {
		t.Fatalf("two machine IDs derived to the same value")
	}
}

func TestDeriveStampsAVersion4UUID(t *testing.T) {
	derived, err := Derive(rmppMachineID)
	if err != nil {
		t.Fatalf("Derive: %v", err)
	}
	raw, err := hex.DecodeString(derived)
	if err != nil {
		t.Fatalf("derived value is not hexadecimal: %v", err)
	}
	if raw[6]&0xf0 != 0x40 {
		t.Fatalf("version nibble is %#x, want 0x4", raw[6]>>4)
	}
	if raw[8]&0xc0 != 0x80 {
		t.Fatalf("variant bits are %#x, want DCE", raw[8]>>6)
	}
}

func TestDeriveRejectsUnusableInput(t *testing.T) {
	for _, input := range []string{"", "   ", "not-hex-at-all", "abcd"} {
		if _, err := Derive(input); err == nil {
			t.Fatalf("expected an error for %q", input)
		}
	}
}
