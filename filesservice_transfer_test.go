package main

import (
	"context"
	"testing"
	"time"

	"golang.org/x/crypto/ssh"
)

func TestTransferTreatsAnAbortedSocketAsADeadConnection(t *testing.T) {
	if !isTransferConnectionDead(errString("write tcp 10.0.2.16:41234->10.69.0.22:22: write: software caused connection abort")) {
		t.Error("ECONNABORTED from a socket Android destroyed was not treated as a dead connection")
	}
	if isTransferConnectionDead(errString("sftp: permission denied")) {
		t.Error("an SFTP error on a live connection was treated as a dead connection")
	}
}

func TestTransferWaitTakesTheReconnectedClient(t *testing.T) {
	dead := &ssh.Client{}
	c := &core{client: dead}
	replacement := &ssh.Client{}
	go func() {
		time.Sleep(300 * time.Millisecond)
		c.mu.Lock()
		c.client = replacement
		c.mu.Unlock()
	}()

	if got := c.awaitReconnectedClient(context.Background(), c.connGen.Load(), dead); got != replacement {
		t.Fatalf("awaitReconnectedClient returned %p, want the reconnected client %p", got, replacement)
	}
}

func TestTransferWaitEndsWhenTheUserDisconnects(t *testing.T) {
	c := &core{}
	gen := c.connGen.Load()
	go func() {
		time.Sleep(300 * time.Millisecond)
		c.connGen.Add(1)
	}()

	start := time.Now()
	if got := c.awaitReconnectedClient(context.Background(), gen, nil); got != nil {
		t.Fatalf("awaitReconnectedClient returned a client after a disconnect")
	}
	if waited := time.Since(start); waited > 2*time.Second {
		t.Fatalf("awaitReconnectedClient waited %v after the disconnect", waited)
	}
}

func TestTransferWaitEndsWhenTheTransferIsCanceled(t *testing.T) {
	c := &core{}
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(300 * time.Millisecond)
		cancel()
	}()

	start := time.Now()
	if got := c.awaitReconnectedClient(ctx, c.connGen.Load(), nil); got != nil {
		t.Fatalf("awaitReconnectedClient returned a client after a cancel")
	}
	if waited := time.Since(start); waited > 2*time.Second {
		t.Fatalf("awaitReconnectedClient waited %v after the cancel", waited)
	}
}
