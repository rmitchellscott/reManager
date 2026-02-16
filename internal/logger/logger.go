package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	maxFileSize  = 5 * 1024 * 1024 // 5 MB
	maxFileCount = 5
	logFileName  = "remanager.log"
)

type Logger struct {
	logDir string
	file   *os.File
	mu     sync.Mutex
}

func New(configDir string) (*Logger, error) {
	logDir := filepath.Join(configDir, "logs")
	if err := os.MkdirAll(logDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create log directory: %w", err)
	}

	l := &Logger{logDir: logDir}
	if err := l.openLogFile(); err != nil {
		return nil, err
	}
	return l, nil
}

func (l *Logger) openLogFile() error {
	logPath := filepath.Join(l.logDir, logFileName)
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return fmt.Errorf("failed to open log file: %w", err)
	}
	l.file = f
	return nil
}

func (l *Logger) rotate() error {
	if l.file != nil {
		l.file.Close()
	}

	for i := maxFileCount - 1; i >= 1; i-- {
		old := filepath.Join(l.logDir, fmt.Sprintf("remanager.%d.log", i))
		new := filepath.Join(l.logDir, fmt.Sprintf("remanager.%d.log", i+1))
		os.Rename(old, new)
	}

	current := filepath.Join(l.logDir, logFileName)
	first := filepath.Join(l.logDir, "remanager.1.log")
	os.Rename(current, first)

	// Remove excess files
	excess := filepath.Join(l.logDir, fmt.Sprintf("remanager.%d.log", maxFileCount))
	os.Remove(excess)

	return l.openLogFile()
}

func (l *Logger) checkRotation() error {
	if l.file == nil {
		return l.openLogFile()
	}
	info, err := l.file.Stat()
	if err != nil {
		return err
	}
	if info.Size() >= maxFileSize {
		return l.rotate()
	}
	return nil
}

func (l *Logger) write(category, message string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.file == nil {
		return
	}

	if err := l.checkRotation(); err != nil {
		return
	}

	ts := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	fmt.Fprintf(l.file, "%s [%s] %s\n", ts, category, message)
}

func (l *Logger) Printf(format string, args ...any) {
	l.write("APP", fmt.Sprintf(format, args...))
}

func (l *Logger) Println(args ...any) {
	l.write("APP", fmt.Sprintln(args...))
}

type CommandLog struct {
	file *os.File
	mu   sync.Mutex
}

func (l *Logger) StartCommandLog(deviceID, cmd string) *CommandLog {
	ts := time.Now().Unix()
	safe := sanitizeForFilename(cmd)
	name := fmt.Sprintf("%s-%d.log", safe, ts)

	dir := l.logDir
	if deviceID != "" {
		dir = filepath.Join(l.logDir, deviceID)
		os.MkdirAll(dir, 0700)
	}
	path := filepath.Join(dir, name)

	f, err := os.Create(path)
	if err != nil {
		return nil
	}

	l.write("CMD", fmt.Sprintf("started: %s → %s", cmd, name))

	return &CommandLog{file: f}
}

func (cl *CommandLog) Write(data string) {
	if cl == nil {
		return
	}
	cl.mu.Lock()
	defer cl.mu.Unlock()
	if cl.file != nil {
		cl.file.WriteString(data)
	}
}

func (cl *CommandLog) WriteExitCode(err error) {
	if cl == nil {
		return
	}
	cl.mu.Lock()
	defer cl.mu.Unlock()
	if cl.file == nil {
		return
	}
	if err != nil {
		fmt.Fprintf(cl.file, "\n[exit: %s]\n", err)
	} else {
		fmt.Fprintln(cl.file, "\n[exit: 0]")
	}
}

func (cl *CommandLog) Close() {
	if cl == nil {
		return
	}
	cl.mu.Lock()
	defer cl.mu.Unlock()
	if cl.file != nil {
		cl.file.Close()
		cl.file = nil
	}
}

func sanitizeForFilename(s string) string {
	parts := strings.Fields(s)
	if len(parts) > 0 {
		dir := filepath.Base(filepath.Dir(parts[0]))
		base := filepath.Base(parts[0])
		if dir != "." && dir != "/" && dir != base {
			parts[0] = dir + "-" + base
		} else {
			parts[0] = base
		}
	}
	s = strings.Join(parts, "-")

	if len(s) > 40 {
		s = s[:40]
	}
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
			b.WriteRune(r)
		}
	}
	return b.String()
}

func (l *Logger) LogEvent(category, message string) {
	l.write(category, message)
}

func (l *Logger) LogInstall(action, pkg, message string) {
	l.write("INSTALL", fmt.Sprintf("%s %s: %s", action, pkg, message))
}

func (l *Logger) LogConnection(event, detail string) {
	l.write("CONN", fmt.Sprintf("%s: %s", event, detail))
}

func (l *Logger) GetLogDir() string {
	return l.logDir
}

func (l *Logger) CleanupOldCommandLogs(maxAge time.Duration) {
	cutoff := time.Now().Add(-maxAge)
	entries, err := os.ReadDir(l.logDir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		deviceDir := filepath.Join(l.logDir, entry.Name())
		files, err := os.ReadDir(deviceDir)
		if err != nil {
			continue
		}
		for _, f := range files {
			if f.IsDir() || !strings.HasSuffix(f.Name(), ".log") {
				continue
			}
			info, err := f.Info()
			if err != nil {
				continue
			}
			if info.ModTime().Before(cutoff) {
				os.Remove(filepath.Join(deviceDir, f.Name()))
			}
		}
		remaining, _ := os.ReadDir(deviceDir)
		if len(remaining) == 0 {
			os.Remove(deviceDir)
		}
	}
}

func (l *Logger) DeleteAllCommandLogs() error {
	entries, err := os.ReadDir(l.logDir)
	if err != nil {
		return fmt.Errorf("failed to read log directory: %w", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if err := os.RemoveAll(filepath.Join(l.logDir, entry.Name())); err != nil {
			return fmt.Errorf("failed to remove %s: %w", entry.Name(), err)
		}
	}
	return nil
}

func (l *Logger) Close() error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file != nil {
		err := l.file.Close()
		l.file = nil
		return err
	}
	return nil
}
