package debug

import (
	"fmt"
	"strings"
	"testing"
)

type capture struct{ out strings.Builder }

func (c *capture) Printf(f string, a ...any) { c.out.WriteString(fmt.Sprintf(f, a...)) }
func (c *capture) Println(a ...any)          {}

func TestSlogBridgeFormatsCommands(t *testing.T) {
	c := &capture{}
	SetFileLogger(c)
	defer SetFileLogger(nil)

	lg := SlogLogger()
	lg.Debug("exec", "cmd", "fw_setenv active_partition 2", "exit", 1, "stderr", "Error opening lock file")

	got := c.out.String()
	for _, want := range []string{"[remarkable-go]", "exec", "cmd=fw_setenv active_partition 2", "exit=1", "stderr=Error opening lock file"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in: %s", want, got)
		}
	}
}
