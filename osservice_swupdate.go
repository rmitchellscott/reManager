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

// A never-answering tablet or a never-finishing unit would otherwise keep the install active, blocking reboot and partition switch until the app restarts.
const (
	swupdateSilenceTimeout = 15 * time.Minute
	swupdateOverallTimeout = 45 * time.Minute
)

var errSwupdateNoResult = fmt.Errorf("the update did not report a result in time. Check which OS version the standby slot holds before you switch to it")

func (s *OSService) runSwupdateDetached(ctx context.Context, swuPath string, output io.Writer, canceled func() bool) error {
	execNow := func() (rmexecutor.Executor, bool) {
		c := s.getClient()
		if c == nil {
			return nil, false
		}
		return rmexecutor.NewSSH(c, rmexecutor.WithLogger(debug.SlogLogger())), true
	}

	exec, ok := execNow()
	if !ok {
		return fmt.Errorf("not connected")
	}

	// A finished run stays loaded until stopped, and its name would block this one.
	if leftover := swupdateUnitState(ctx, exec); leftover["ActiveState"] == "active" && leftover["SubState"] == "exited" {
		exec.Run(ctx, "systemctl", "stop", swupdateUnit)
	}
	exec.Run(ctx, "systemctl", "reset-failed", swupdateUnit)
	// --no-block so a dropped link does not read as a failed start mid-install; --remain-after-exit so a late poll can still tell success from a start that never ran.
	start, err := exec.Run(ctx, "systemd-run", "--no-block", "--remain-after-exit", "--unit="+swupdateUnit, "--service-type=oneshot",
		"/usr/sbin/swupdate-from-image-file", swuPath)
	if err != nil {
		return fmt.Errorf("failed to start swupdate: %w", err)
	}
	if !start.Success() {
		return fmt.Errorf("failed to start swupdate: %s", strings.TrimSpace(start.Stderr))
	}

	// Pin the invocation ID across polls: the unit name is reused across installs, and a vanished unit stops reporting the ID its journal is keyed on.
	var invocationID string
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	overall := time.After(swupdateOverallTimeout)
	lastStatusAt := time.Now()
	for {
		if canceled() {
			if e, ok := execNow(); ok {
				e.Run(ctx, "systemctl", "stop", swupdateUnit)
			}
			return fmt.Errorf("canceled")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-overall:
			return errSwupdateNoResult
		case <-ticker.C:
		}
		if time.Since(lastStatusAt) > swupdateSilenceTimeout {
			return errSwupdateNoResult
		}

		e, ok := execNow()
		if !ok {
			continue
		}
		props := swupdateUnitState(ctx, e)
		if props == nil {
			continue
		}
		lastStatusAt = time.Now()
		if props["InvocationID"] != "" {
			invocationID = props["InvocationID"]
		}
		switch props["ActiveState"] {
		case "active":
			if props["SubState"] != "exited" {
				continue
			}
			s.appendUnitJournal(ctx, output, invocationID)
			e.Run(ctx, "systemctl", "stop", swupdateUnit)
			if props["Result"] == "success" {
				return nil
			}
			return fmt.Errorf("swupdate exited with status %s (result=%s)", props["ExecMainStatus"], props["Result"])
		case "failed":
			s.appendUnitJournal(ctx, output, invocationID)
			return fmt.Errorf("swupdate failed: exit status %s (result=%s)", props["ExecMainStatus"], props["Result"])
		case "inactive":
			if props["LoadState"] == "not-found" {
				s.appendUnitJournal(ctx, output, invocationID)
				return fmt.Errorf("the update stopped reporting its status. Check the OS version on your reMarkable")
			}
			continue
		default:
			continue
		}
	}
}

func swupdateUnitState(ctx context.Context, e rmexecutor.Executor) map[string]string {
	st, err := e.Run(ctx, "systemctl", "show", "-p", "LoadState", "-p", "ActiveState", "-p", "SubState", "-p", "Result", "-p", "ExecMainStatus", "-p", "InvocationID", swupdateUnit)
	if err != nil || st == nil || !st.Success() {
		return nil
	}
	return parseKeyVals(st.Stdout)
}

func (s *OSService) appendUnitJournal(ctx context.Context, output io.Writer, invocationID string) {
	if output == nil || invocationID == "" {
		return
	}
	c := s.getClient()
	if c == nil {
		return
	}
	exec := rmexecutor.NewSSH(c, rmexecutor.WithLogger(debug.SlogLogger()))
	if res, err := exec.Run(ctx, "journalctl", "--no-pager", "-o", "cat",
		"_SYSTEMD_INVOCATION_ID="+invocationID, "+", "INVOCATION_ID="+invocationID); err == nil && res != nil {
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
