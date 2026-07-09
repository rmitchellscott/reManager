package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"reManager/internal/debug"

	rmexecutor "github.com/rmitchellscott/remarkable-go/executor"
)

const swupdateUnit = "remanager-swupdate"

func (a *App) runSwupdateDetached(ctx context.Context, swuPath string, output io.Writer, cancelled func() bool) error {
	execNow := func() (rmexecutor.Executor, bool) {
		c := a.getClient()
		if c == nil {
			return nil, false
		}
		return rmexecutor.NewSSH(c, rmexecutor.WithLogger(debug.SlogLogger())), true
	}

	exec, ok := execNow()
	if !ok {
		return fmt.Errorf("not connected")
	}

	exec.Run(ctx, "systemctl", "reset-failed", swupdateUnit)
	start, err := exec.Run(ctx, "systemd-run", "--unit="+swupdateUnit, "--service-type=oneshot",
		"/usr/sbin/swupdate-from-image-file", swuPath)
	if err != nil {
		return fmt.Errorf("failed to start swupdate: %w", err)
	}
	if !start.Success() {
		return fmt.Errorf("failed to start swupdate: %s", strings.TrimSpace(start.Stderr))
	}

	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		if cancelled() {
			if e, ok := execNow(); ok {
				e.Run(ctx, "systemctl", "stop", swupdateUnit)
			}
			return fmt.Errorf("cancelled")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}

		e, ok := execNow()
		if !ok {
			continue
		}
		st, err := e.Run(ctx, "systemctl", "show", "-p", "ActiveState", "-p", "Result", "-p", "ExecMainStatus", swupdateUnit)
		if err != nil || st == nil || !st.Success() {
			continue
		}
		props := parseKeyVals(st.Stdout)
		switch props["ActiveState"] {
		case "inactive":
			a.appendUnitJournal(ctx, output)
			if props["Result"] == "success" {
				return nil
			}
			return fmt.Errorf("swupdate exited with status %s (result=%s)", props["ExecMainStatus"], props["Result"])
		case "failed":
			a.appendUnitJournal(ctx, output)
			return fmt.Errorf("swupdate failed: exit status %s (result=%s)", props["ExecMainStatus"], props["Result"])
		default:
			continue
		}
	}
}

func (a *App) appendUnitJournal(ctx context.Context, output io.Writer) {
	if output == nil {
		return
	}
	c := a.getClient()
	if c == nil {
		return
	}
	exec := rmexecutor.NewSSH(c, rmexecutor.WithLogger(debug.SlogLogger()))
	if res, err := exec.Run(ctx, "journalctl", "-u", swupdateUnit, "--no-pager", "-o", "cat"); err == nil && res != nil {
		io.WriteString(output, res.Stdout)
	}
}

func parseKeyVals(s string) map[string]string {
	m := make(map[string]string)
	for _, line := range strings.Split(s, "\n") {
		if i := strings.IndexByte(line, '='); i > 0 {
			m[strings.TrimSpace(line[:i])] = strings.TrimSpace(line[i+1:])
		}
	}
	return m
}
