package tablettest

import (
	"io"
	"slices"
	"sync"
	"time"

	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
)

const exitCommandNotFound = 127

type replayer struct {
	mu      sync.Mutex
	answers map[string][]*Exchange
	asked   map[string]bool
	files   *replayedFiles
	onMiss  func(command string)
	server  *Server
	dropped map[*Exchange]bool
}

// The last answer to a command repeats once its recorded answers run out, because how often a status is polled depends on timing.
func NewReplayServer(cassette *Cassette, onMiss func(command string)) (*Server, error) {
	r := newReplayer(cassette, onMiss)
	server, err := newServer(func() sessionBackend { return &replaySession{replayer: r} })
	if err != nil {
		return nil, err
	}
	server.unasked = func() []string { return append(r.unasked(), r.files.unasked()...) }
	r.server = server
	return server, nil
}

func newReplayer(cassette *Cassette, onMiss func(command string)) *replayer {
	r := &replayer{answers: map[string][]*Exchange{}, asked: map[string]bool{}, files: newReplayedFiles(cassette, onMiss), onMiss: onMiss, dropped: map[*Exchange]bool{}}
	for _, exchange := range cassette.Exchanges {
		key := normalize(exchange.Command)
		r.answers[key] = append(r.answers[key], exchange)
	}
	return r
}

// A real outage lasts as long as the tablet takes to boot. Replay keeps its shape, a drop and then refused connections, in a few seconds.
const (
	replayDropDelay = time.Second
	replayDownFor   = 3 * time.Second
)

func (r *replayer) dropAfter(exchange *Exchange) {
	r.mu.Lock()
	first := exchange.DropsConnection && !r.dropped[exchange]
	r.dropped[exchange] = true
	r.mu.Unlock()
	if !first {
		return
	}
	go func() {
		time.Sleep(replayDropDelay)
		r.server.goDown()
		time.Sleep(replayDownFor)
		r.server.comeUp()
	}()
}

func (r *replayer) next(command string) *Exchange {
	r.mu.Lock()
	defer r.mu.Unlock()
	key := normalize(command)
	queue := r.answers[key]
	if len(queue) == 0 {
		return nil
	}
	r.asked[key] = true
	if len(queue) > 1 {
		r.answers[key] = queue[1:]
	}
	return queue[0]
}

func (r *replayer) unasked() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return unaskedKeys(r.answers, r.asked)
}

func unaskedKeys[T any](answers map[string][]T, asked map[string]bool) []string {
	var keys []string
	for key := range answers {
		if !asked[key] {
			keys = append(keys, key)
		}
	}
	slices.Sort(keys)
	return keys
}

type replaySession struct {
	replayer *replayer
}

func (s *replaySession) pty(string, uint32, uint32) bool { return true }

func (s *replaySession) exec(channel ssh.Channel, command string) (int, string) {
	go io.Copy(io.Discard, channel)
	exchange := s.replayer.next(command)
	if exchange == nil {
		s.replayer.onMiss(command)
		io.WriteString(channel.Stderr(), "tablettest: no recorded answer for this command\n")
		return exitCommandNotFound, ""
	}
	io.WriteString(channel, exchange.Stdout)
	io.WriteString(channel.Stderr(), exchange.Stderr)
	s.replayer.dropAfter(exchange)
	return exchange.Exit, exchange.Signal
}

func (s *replaySession) subsystem(channel ssh.Channel, name string) bool {
	if name != "sftp" {
		return false
	}
	server := sftp.NewRequestServer(channel, s.replayer.files.handlers())
	server.Serve()
	server.Close()
	return true
}

func (s *replaySession) shell(channel ssh.Channel) bool {
	io.Copy(io.Discard, channel)
	return true
}

func (s *replaySession) signal(string)               {}
func (s *replaySession) windowChange(uint32, uint32) {}
func (s *replaySession) close()                      {}
