package support

import (
	"strings"
	"testing"
)

func TestRedactDeviceIdentifiers(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			"reMarkable 2 serial",
			"User-authenticator configured with 'Config{.deviceId = RM000-000-00000, .x = 1}'",
			"User-authenticator configured with 'Config{.deviceId = [serial redacted], .x = 1}'",
		},
		{
			"Paper Pro serial ending in a digit",
			"Setting authorId for deviceId=RM00A0000000A0A",
			"Setting authorId for deviceId=[serial redacted]",
		},
		{
			"Paper Pro serial of digits only",
			"deviceId = RM00A0000000000",
			"deviceId = [serial redacted]",
		},
		{
			"author id",
			"Set authorId to 00000000-0000-5000-8000-000000000000",
			"Set authorId to [author id redacted]",
		},
		{
			"firmware versions and package names are left alone",
			"osver 3.26.0.68, RM-DEMO-0000, rmfakecloud-proxy 0.0.10-r2",
			"osver 3.26.0.68, RM-DEMO-0000, rmfakecloud-proxy 0.0.10-r2",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := string(redactDeviceIdentifiers([]byte(c.in))); got != c.want {
				t.Errorf("redactDeviceIdentifiers(%q) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestRedactLeavesNoSerialInMultilineLog(t *testing.T) {
	log := strings.Repeat("line\n", 3) + "deviceId = RM00A0000000000\n" + "deviceId=RM000-000-00000\n"
	got := string(redactDeviceIdentifiers([]byte(log)))
	if strings.Contains(got, "RM00A") || strings.Contains(got, "RM000-") {
		t.Errorf("serial survived redaction: %q", got)
	}
}
