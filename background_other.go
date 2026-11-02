//go:build !ios && !android

package main

func showNativeBackgroundWork(title, subtitle string, fraction float64) {}

func endNativeBackgroundWork() {}

func endNativeBackgroundWorkFailed(title, message string) {}

func notifyNeedsAttention(title, body string) {}
