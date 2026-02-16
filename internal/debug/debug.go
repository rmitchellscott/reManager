package debug

import (
	"fmt"
	"sync"
)

const Enabled = enabled

type FileLogger interface {
	Printf(format string, args ...any)
	Println(args ...any)
}

var (
	fileLogger FileLogger
	flMu       sync.RWMutex
)

func SetFileLogger(l FileLogger) {
	flMu.Lock()
	fileLogger = l
	flMu.Unlock()
}

func Printf(format string, args ...any) {
	if enabled {
		fmt.Printf(format, args...)
	}
	flMu.RLock()
	fl := fileLogger
	flMu.RUnlock()
	if fl != nil {
		fl.Printf(format, args...)
	}
}

func Println(args ...any) {
	if enabled {
		fmt.Println(args...)
	}
	flMu.RLock()
	fl := fileLogger
	flMu.RUnlock()
	if fl != nil {
		fl.Println(args...)
	}
}
