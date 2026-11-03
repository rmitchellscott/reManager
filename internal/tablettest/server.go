package tablettest

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"net"
	"sync"

	"golang.org/x/crypto/ssh"
)

const Password = "test"

type sessionBackend interface {
	pty(term string, rows, cols uint32) bool
	exec(channel ssh.Channel, command string) (exit int, signal string)
	subsystem(channel ssh.Channel, name string) bool
	shell(channel ssh.Channel) bool
	signal(name string)
	windowChange(rows, cols uint32)
	close()
}

type Server struct {
	addr       string
	config     *ssh.ServerConfig
	newBackend func() sessionBackend
	wg         sync.WaitGroup
	mu         sync.Mutex
	listener   net.Listener
	closed     bool
	conns      []net.Conn
	onClose    func()
	unasked    func() []string
}

func newServer(newBackend func() sessionBackend) (*Server, error) {
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	signer, err := ssh.NewSignerFromKey(key)
	if err != nil {
		return nil, err
	}
	config := &ssh.ServerConfig{
		PasswordCallback: func(_ ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
			if string(password) != Password {
				return nil, errors.New("wrong password")
			}
			return nil, nil
		},
		PublicKeyCallback: func(ssh.ConnMetadata, ssh.PublicKey) (*ssh.Permissions, error) {
			return nil, nil
		},
	}
	config.AddHostKey(signer)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	s := &Server{addr: listener.Addr().String(), listener: listener, config: config, newBackend: newBackend}
	s.wg.Add(1)
	go s.accept(listener)
	return s, nil
}

func (s *Server) Addr() string { return s.addr }

func (s *Server) DropConnections() {
	s.mu.Lock()
	conns := s.conns
	s.conns = nil
	s.mu.Unlock()
	for _, conn := range conns {
		conn.Close()
	}
}

func (s *Server) goDown() {
	s.mu.Lock()
	listener := s.listener
	s.listener = nil
	s.mu.Unlock()
	if listener != nil {
		listener.Close()
	}
	s.DropConnections()
}

func (s *Server) comeUp() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.listener != nil {
		return nil
	}
	listener, err := net.Listen("tcp", s.addr)
	if err != nil {
		return err
	}
	s.listener = listener
	s.wg.Add(1)
	go s.accept(listener)
	return nil
}

// Polling makes how often a command repeats vary, but a recorded command never asked for at all means the app skipped a step.
func (s *Server) Unasked() []string {
	if s.unasked == nil {
		return nil
	}
	return s.unasked()
}

func (s *Server) isClosed() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.closed
}

func (s *Server) Close() {
	s.mu.Lock()
	s.closed = true
	s.mu.Unlock()
	s.goDown()
	if s.onClose != nil {
		s.onClose()
	}
	s.wg.Wait()
}

func (s *Server) accept(listener net.Listener) {
	defer s.wg.Done()
	for {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		s.mu.Lock()
		s.conns = append(s.conns, conn)
		s.mu.Unlock()
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.serveConn(conn)
		}()
	}
}

func (s *Server) serveConn(conn net.Conn) {
	defer conn.Close()
	_, channels, requests, err := ssh.NewServerConn(conn, s.config)
	if err != nil {
		return
	}
	go ssh.DiscardRequests(requests)
	var sessions sync.WaitGroup
	for newChannel := range channels {
		if newChannel.ChannelType() != "session" {
			newChannel.Reject(ssh.UnknownChannelType, "only sessions")
			continue
		}
		channel, requests, err := newChannel.Accept()
		if err != nil {
			continue
		}
		sessions.Add(1)
		go func() {
			defer sessions.Done()
			s.serveSession(channel, requests)
		}()
	}
	sessions.Wait()
}

func (s *Server) serveSession(channel ssh.Channel, requests <-chan *ssh.Request) {
	backend := s.newBackend()
	defer backend.close()
	defer channel.Close()
	for request := range requests {
		switch request.Type {
		case "pty-req":
			var pty struct {
				Term             string
				Cols, Rows, W, H uint32
				Modes            string
			}
			ok := ssh.Unmarshal(request.Payload, &pty) == nil && backend.pty(pty.Term, pty.Rows, pty.Cols)
			request.Reply(ok, nil)
		case "env":
			request.Reply(true, nil)
		case "window-change":
			var size struct{ Cols, Rows, W, H uint32 }
			if ssh.Unmarshal(request.Payload, &size) == nil {
				backend.windowChange(size.Rows, size.Cols)
			}
		case "signal":
			var signal struct{ Name string }
			if ssh.Unmarshal(request.Payload, &signal) == nil {
				backend.signal(signal.Name)
			}
		case "exec":
			var exec struct{ Command string }
			if ssh.Unmarshal(request.Payload, &exec) != nil {
				request.Reply(false, nil)
				continue
			}
			request.Reply(true, nil)
			go func() {
				exit, signal := backend.exec(channel, exec.Command)
				sendExit(channel, exit, signal)
				channel.Close()
			}()
		case "subsystem":
			var subsystem struct{ Name string }
			ok := ssh.Unmarshal(request.Payload, &subsystem) == nil
			request.Reply(ok, nil)
			if ok {
				go func() {
					backend.subsystem(channel, subsystem.Name)
					channel.Close()
				}()
			}
		case "shell":
			request.Reply(true, nil)
			go func() {
				backend.shell(channel)
				channel.Close()
			}()
		default:
			if request.WantReply {
				request.Reply(false, nil)
			}
		}
	}
}

func sendExit(channel ssh.Channel, exit int, signal string) {
	if signal != "" {
		channel.SendRequest("exit-signal", false, ssh.Marshal(struct {
			Signal     string
			CoreDumped bool
			Message    string
			Lang       string
		}{signal, false, "", ""}))
		return
	}
	payload := make([]byte, 4)
	binary.BigEndian.PutUint32(payload, uint32(exit))
	channel.SendRequest("exit-status", false, payload)
}
