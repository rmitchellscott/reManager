//go:build !darwin

package cloudsync

func Open(onChange func(keys []string)) (KeyValueStore, bool) { return nil, false }
