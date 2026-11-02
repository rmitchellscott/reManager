//go:build integration

package main

import (
	"context"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// Exercises the real SSH stack and the event path against a tablet, which a unit test cannot reach and the GUI cannot be driven to reach; opt in with REMANAGER_TEST_HOST and REMANAGER_TEST_PASSWORD.
func TestConnectToRealDevice(t *testing.T) {
	host := os.Getenv("REMANAGER_TEST_HOST")
	password := os.Getenv("REMANAGER_TEST_PASSWORD")
	if host == "" || password == "" {
		t.Skip("set REMANAGER_TEST_HOST and REMANAGER_TEST_PASSWORD")
	}

	wailsApp := application.New(application.Options{Name: "reManager-test"})
	services := newServices(&core{})
	connection := services.connection
	connection.wails = wailsApp
	connection.startup(context.Background())
	defer connection.shutdown(context.Background())

	result := connection.Connect(host, password)
	if !result.Success {
		t.Fatalf("Connect failed: %s (code %s)", result.Message, result.Code)
	}
	t.Logf("connected: device=%s message=%s", result.Device, result.Message)

	info := services.device.GetDeviceInfo()
	if len(info) == 0 {
		t.Error("GetDeviceInfo returned nothing")
	}
	t.Logf("device info: model=%q machine=%q firmware=%q", info["model"], info["machine"], info["firmware"])

	arch := services.device.GetDeviceArchitecture(result.Device)
	t.Logf("architecture: %q", arch)

	installed, err := services.packages.GetInstalledPackagesWithVersions()
	if err != nil {
		t.Fatalf("GetInstalledPackagesWithVersions: %v", err)
	}
	t.Logf("installed packages: %d", len(installed))
	for name, version := range installed {
		t.Logf("  %s %s", name, version)
	}

	if !connection.isConnected() {
		t.Error("isConnected should report true after a successful connect")
	}
	if connection.inDemoMode() {
		t.Error("a real connection must not report demo mode")
	}

	t.Run("events reach a subscriber", func(t *testing.T) {
		// A broken emit or subscription compiles and simply never fires, so assert a round trip rather than the absence of a panic.
		output := make(chan string, 64)
		done := make(chan OperationResult, 1)

		offOutput := wailsApp.Event.On(eventOperationOutput, func(e *application.CustomEvent) {
			if out, ok := e.Data.(OperationOutput); ok {
				select {
				case output <- out.Text:
				default:
				}
			}
		})
		defer offOutput()

		offDone := wailsApp.Event.On(eventOperationDone, func(e *application.CustomEvent) {
			if result, ok := e.Data.(OperationResult); ok {
				select {
				case done <- result:
				default:
				}
			}
		})
		defer offDone()

		connection.runOperation("round-trip", "Round trip", func(op *Operation) (any, error) {
			return nil, connection.runOperationCommand(op, "echo wails-v3-round-trip", false)
		})

		select {
		case result := <-done:
			if !result.OK {
				t.Errorf("op:done reported failure: %s", result.Error)
			}
		case <-time.After(30 * time.Second):
			t.Fatal("op:done never arrived: the subscription or the emit is broken")
		}

		close(output)
		var lines []string
		for line := range output {
			lines = append(lines, line)
		}
		if len(lines) == 0 {
			t.Error("op:output never arrived")
		}
		if !slices.ContainsFunc(lines, func(l string) bool {
			return strings.Contains(l, "wails-v3-round-trip")
		}) {
			t.Errorf("op:output did not carry the command output: %q", lines)
		}
		t.Logf("op:output delivered %d line(s)", len(lines))
	})

	connection.Disconnect()
}
