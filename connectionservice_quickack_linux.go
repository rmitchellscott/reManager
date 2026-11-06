//go:build linux

package main

import (
	"net"
	"syscall"
)

func withQuickAck(conn net.Conn) net.Conn {
	tcpConn, ok := conn.(*net.TCPConn)
	if !ok {
		return conn
	}
	rawConn, err := tcpConn.SyscallConn()
	if err != nil {
		return conn
	}
	return &quickAckConn{TCPConn: tcpConn, rawConn: rawConn}
}

// The tablet's socket-activated dropbear runs with Nagle on, so each small reply
// waits for the ACK of the one before it, and delayed ACK then stalls SFTP. The
// kernel clears TCP_QUICKACK on its own, so it has to be set again after every read.
type quickAckConn struct {
	*net.TCPConn
	rawConn syscall.RawConn
}

func (c *quickAckConn) Read(b []byte) (int, error) {
	n, err := c.TCPConn.Read(b)
	c.rawConn.Control(func(fd uintptr) {
		syscall.SetsockoptInt(int(fd), syscall.IPPROTO_TCP, syscall.TCP_QUICKACK, 1)
	})
	return n, err
}
