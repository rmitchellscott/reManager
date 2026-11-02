//go:build !darwin || ios || mas

package platform

import "errors"

func IsOutsideApplicationsFolder() bool { return false }

func MoveToApplications() error { return errors.New("moving to Applications is macOS only") }
