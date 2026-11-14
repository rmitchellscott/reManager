//go:build linux && !android

package main

import (
	"context"
	"net"
	"time"
)

func dialTablet(ctx context.Context, addr string, timeout time.Duration) (net.Conn, error) {
	d := net.Dialer{Timeout: timeout}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}
	return withQuickAck(conn), nil
}

func (s *ConnectionService) logUSBNetworkDiagnostics(string) {}
