//go:build !android

package main

import (
	"context"
	"net"
	"time"
)

func dialTablet(ctx context.Context, addr string, timeout time.Duration) (net.Conn, error) {
	d := net.Dialer{Timeout: timeout}
	return d.DialContext(ctx, "tcp", addr)
}

func (s *ConnectionService) logUSBNetworkDiagnostics(string) {}
