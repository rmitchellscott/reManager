package tablettest

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"unicode/utf8"
)

type Exchange struct {
	Command string `json:"command"`
	Stdout  string `json:"stdout,omitempty"`
	Stderr  string `json:"stderr,omitempty"`
	Exit    int    `json:"exit"`
	Signal  string `json:"signal,omitempty"`
	// Set on the command after which the tablet went away, such as a reboot.
	DropsConnection bool `json:"dropsConnection,omitempty"`
}

type Cassette struct {
	mu        sync.Mutex
	Exchanges []*Exchange   `json:"exchanges"`
	Files     []*FileAnswer `json:"files,omitempty"`
}

func LoadCassette(path string) (*Cassette, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var c Cassette
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

// JSON turns invalid UTF-8 into U+FFFD, so binary output would replay corrupted instead of failing.
func (c *Cassette) Save(path string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, exchange := range c.Exchanges {
		if !utf8.ValidString(exchange.Stdout) || !utf8.ValidString(exchange.Stderr) {
			return fmt.Errorf("%q answered with binary output, which a cassette cannot hold", exchange.Command)
		}
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

// reserve holds the exchange's place in start order, so replay hands out a repeated command's answers in the order they were asked for, not the order the commands finished in.
func (c *Cassette) reserve(command string) *Exchange {
	c.mu.Lock()
	defer c.mu.Unlock()
	exchange := &Exchange{Command: command}
	c.Exchanges = append(c.Exchanges, exchange)
	return exchange
}

// The tablet goes away a moment after the command that rebooted it returns, by which time other commands may have started, so a reboot is preferred over whatever ran last.
func (c *Cassette) markDrop() {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i := len(c.Exchanges) - 1; i >= 0; i-- {
		if strings.Contains(c.Exchanges[i].Command, "reboot") {
			c.Exchanges[i].DropsConnection = true
			return
		}
	}
	if len(c.Exchanges) > 0 {
		c.Exchanges[len(c.Exchanges)-1].DropsConnection = true
	}
}

func (c *Cassette) finish(exchange *Exchange, stdout, stderr string, exit int, signal string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	exchange.Stdout, exchange.Stderr, exchange.Exit, exchange.Signal = stdout, stderr, exit, signal
}

var volatileParts = []struct {
	pattern     *regexp.Regexp
	replacement string
}{
	{regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`), "<uuid>"},
	// The PATH the executor prepends is shell boilerplate, so changing it should not invalidate every recording.
	{regexp.MustCompile(`(export )?PATH=[^ ;]*:\$PATH(; | )`), ""},
}

func normalize(command string) string {
	for _, part := range volatileParts {
		command = part.pattern.ReplaceAllString(command, part.replacement)
	}
	return command
}
