package vellum

import (
	"fmt"
	"strings"
	"testing"

	"reManager/internal/component"
)

type countingRunner struct {
	calls  map[string]int
	output map[string]string
}

func (r *countingRunner) ExecuteWithOutput(cmd string) (string, error) {
	for fragment, out := range r.output {
		if strings.Contains(cmd, fragment) {
			r.calls[fragment]++
			return out, nil
		}
	}
	return "", fmt.Errorf("unexpected command: %s", cmd)
}

func (r *countingRunner) Execute(commands []component.CommandResult) error { return nil }

func (r *countingRunner) ExecuteStreaming(cmd string, onOutput func(line string)) error { return nil }

func newCachingClient() (*Client, *countingRunner) {
	r := &countingRunner{
		calls: map[string]int{},
		output: map[string]string{
			"info -q": "xovi\nappload\n",
			"list -I": "xovi-0.3.3-r2 aarch64 {xovi} (MIT) [installed]\n",
		},
	}
	return &Client{executor: r}, r
}

func TestInstalledReadsAreSharedWithinTTL(t *testing.T) {
	c, r := newCachingClient()

	for i := 0; i < 3; i++ {
		if _, err := c.ListWithOsCheck(); err != nil {
			t.Fatal(err)
		}
		if _, err := c.ListInstalledWithVersions(); err != nil {
			t.Fatal(err)
		}
		if _, err := c.List(); err != nil {
			t.Fatal(err)
		}
	}

	if r.calls["info -q"] != 1 {
		t.Errorf("info -q ran %d times, want 1", r.calls["info -q"])
	}
	if r.calls["list -I"] != 1 {
		t.Errorf("list -I ran %d times, want 1", r.calls["list -I"])
	}
}

func TestMutationInvalidatesInstalledCache(t *testing.T) {
	c, r := newCachingClient()

	c.ListWithOsCheck()
	c.ListInstalledWithVersions()
	c.InvalidateInstalledCache()
	c.ListWithOsCheck()
	c.ListInstalledWithVersions()

	if r.calls["info -q"] != 2 {
		t.Errorf("info -q ran %d times after invalidation, want 2", r.calls["info -q"])
	}
	if r.calls["list -I"] != 2 {
		t.Errorf("list -I ran %d times after invalidation, want 2", r.calls["list -I"])
	}
}

func TestStreamingMutationsInvalidate(t *testing.T) {
	for _, tc := range []struct {
		name string
		run  func(c *Client) error
	}{
		{"add", func(c *Client) error { return c.AddStreaming(func(string) {}, "xovi") }},
		{"del", func(c *Client) error { return c.DelStreaming(func(string) {}, "xovi") }},
	} {
		c, r := newCachingClient()
		c.ListWithOsCheck()
		if err := tc.run(c); err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		c.ListWithOsCheck()
		if r.calls["info -q"] != 2 {
			t.Errorf("%s: info -q ran %d times, want 2 (cache should be dropped)", tc.name, r.calls["info -q"])
		}
	}
}

func TestFailedOSVersionReadIsNotCached(t *testing.T) {
	r := &countingRunner{calls: map[string]int{}, output: map[string]string{}}
	c := &Client{executor: r}

	if state, err := c.GetOSVersionState(); err == nil {
		t.Fatalf("GetOSVersionState with no readable version = %+v, want an error", state)
	}

	r.output["RELEASE_VERSION"] = "RELEASE_VERSION=3.28.0.172\n"
	if state, err := c.GetOSVersionState(); err == nil {
		t.Fatalf("GetOSVersionState with an unreadable stored version = %+v, want an error", state)
	}

	r.output["state/osver"] = "3.26.0.68\n"
	state, err := c.GetOSVersionState()
	if err != nil {
		t.Fatal(err)
	}
	if !state.Mismatch || state.CurrentVersion != "3.28.0.172" || state.StoredVersion != "3.26.0.68" {
		t.Errorf("state = %+v, want a mismatch from 3.26.0.68 to 3.28.0.172", state)
	}
}
