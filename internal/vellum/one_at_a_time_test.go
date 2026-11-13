package vellum

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"reManager/internal/component"
)

type overlapCounter struct {
	running atomic.Int32
	most    atomic.Int32
}

func (o *overlapCounter) run() {
	now := o.running.Add(1)
	for {
		most := o.most.Load()
		if now <= most || o.most.CompareAndSwap(most, now) {
			break
		}
	}
	time.Sleep(20 * time.Millisecond)
	o.running.Add(-1)
}

func (o *overlapCounter) ExecuteWithOutput(cmd string) (string, error) {
	o.run()
	return "", nil
}

func (o *overlapCounter) ExecuteStreaming(cmd string, onOutput func(line string)) error {
	o.run()
	return nil
}

func (o *overlapCounter) Execute(commands []component.CommandResult) error { return nil }

func TestVellumCommandsFromSeparateClientsNeverOverlap(t *testing.T) {
	device := &overlapCounter{}
	first, second := NewClient(device), NewClient(device)

	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(2)
		go func() { defer wg.Done(); _, _ = first.executor.ExecuteWithOutput(VellumBin + " upgrade --simulate -") }()
		go func() { defer wg.Done(); _ = second.executor.ExecuteStreaming(VellumBin+" del 'pkg'", nil) }()
	}
	wg.Wait()

	if got := device.most.Load(); got != 1 {
		t.Errorf("at most %d vellum commands ran at once, want 1", got)
	}
}

func TestOtherCommandsDoNotWaitForVellum(t *testing.T) {
	device := &overlapCounter{}
	client := NewClient(device)

	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = client.executor.ExecuteWithOutput("cat /home/root/.vellum/lib/apk/db/installed")
		}()
	}
	wg.Wait()

	if got := device.most.Load(); got < 2 {
		t.Errorf("at most %d plain commands ran at once, want them to overlap", got)
	}
}
