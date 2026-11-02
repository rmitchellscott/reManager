package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"reManager/internal/debug"
)

// An existing file keeps its permissions; perm applies only to a new one.
func writeFileAtomic(path string, data []byte, perm os.FileMode) error {
	if info, err := os.Stat(path); err == nil {
		perm = info.Mode().Perm()
	}

	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	renamed := false
	defer func() {
		if !renamed {
			_ = os.Remove(tmpPath)
		}
	}()

	if err := tmp.Chmod(perm); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	renamed = true

	syncDir(dir)
	return nil
}

// Some platforms cannot fsync a directory. The rename is still atomic there.
func syncDir(dir string) {
	d, err := os.Open(dir)
	if err != nil {
		return
	}
	_ = d.Sync()
	_ = d.Close()
}

func setAsideUnreadable(path string, parseErr error) error {
	asidePath := fmt.Sprintf("%s.unreadable-%d", path, time.Now().UnixMilli())
	if err := os.Rename(path, asidePath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("%s does not parse (%v) and could not be moved aside: %w", filepath.Base(path), parseErr, err)
	}
	debug.Println("[DEBUG]", filepath.Base(path), "does not parse, moved to", filepath.Base(asidePath)+":", parseErr)
	return nil
}
