package main

import "testing"

func TestIsRetryableError(t *testing.T) {
	cases := []struct {
		name string
		msg  string
		want bool
	}{
		{"handshake reset by peer", "ssh: handshake failed: read tcp 10.11.99.16:59146->10.11.99.1:22: read: connection reset by peer", true},
		{"windows conn refused", "dial tcp 192.168.2.46:22: connectex: No connection could be made because the target machine actively refused it.", true},
		{"windows timed out", "read tcp 192.168.2.32:59111->192.168.2.46:22: wsarecv: A connection attempt failed because the connected party did not properly respond after a period of time", true},
		{"unix i/o timeout", "dial tcp 10.11.99.1:22: i/o timeout", true},
		{"auth failure not retryable", "ssh: handshake failed: ssh: unable to authenticate, attempted methods [none password]", false},
		{"permission denied not retryable", "ssh: handshake failed: permission denied", false},
		{"bare handshake not retryable", "ssh: handshake failed", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := isRetryableError(errString(c.msg)); got != c.want {
				t.Errorf("isRetryableError(%q) = %v, want %v", c.msg, got, c.want)
			}
		})
	}
}

func TestIsConnectionDeadError(t *testing.T) {
	dead := []string{
		"read tcp: wsarecv: A connection attempt failed because the connected party did not properly respond",
		"read: connection reset by peer",
		"ssh: unexpected packet in response to channel open: <nil>",
		"use of closed network connection",
	}
	for _, m := range dead {
		if !isConnectionDeadError(errString(m)) {
			t.Errorf("isConnectionDeadError(%q) = false, want true", m)
		}
	}
	if isConnectionDeadError(errString("Process exited with status 1")) {
		t.Errorf("command exit misclassified as dead connection")
	}
}

type errString string

func (e errString) Error() string { return string(e) }
