package debug

import "fmt"

const Enabled = enabled

func Printf(format string, args ...any) {
	if enabled {
		fmt.Printf(format, args...)
	}
}

func Println(args ...any) {
	if enabled {
		fmt.Println(args...)
	}
}
