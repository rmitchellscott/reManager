//go:build windows

package main

import (
	"context"
	"net"
	"syscall"
	"time"
)

// SIO_TCP_SET_ACK_FREQUENCY from mstcpip.h, which package syscall does not define.
const sioTCPSetAckFrequency = 0x98000017

func dialTablet(ctx context.Context, addr string, timeout time.Duration) (net.Conn, error) {
	d := net.Dialer{Timeout: timeout}
	conn, err := d.DialContext(ctx, "tcp", addr)
	if err != nil {
		return nil, err
	}
	if tcpConn, ok := conn.(*net.TCPConn); ok {
		ackEveryPacket(tcpConn)
	}
	return conn, nil
}

func (s *ConnectionService) logUSBNetworkDiagnostics(string) {}

// The tablet's socket-activated dropbear runs with Nagle on, so each small reply
// waits for the ACK of the one before it, and delayed ACK then stalls SFTP.
func ackEveryPacket(conn *net.TCPConn) {
	rawConn, err := conn.SyscallConn()
	if err != nil {
		return
	}
	rawConn.Control(func(fd uintptr) {
		frequency := byte(1)
		var returned uint32
		syscall.WSAIoctl(syscall.Handle(fd), sioTCPSetAckFrequency, &frequency, 1, nil, 0, &returned, nil, 0)
	})
}
