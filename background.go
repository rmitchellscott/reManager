package main

import (
	"slices"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

// A backgrounded mobile app is suspended, dropping its goroutines and the SSH sockets under a running job, so background work keeps the process alive for the job's duration (iOS continued processing task, Android foreground service) and keeps the screen awake, since locking an iPhone can drop its USB network link to the tablet.
//
// When jobs overlap, the newest one still running is the one the system shows.

const unknownProgress = -1.0

// The system rate-limits notification and Live Activity updates, and a transfer reports progress many times a second.
const backgroundProgressInterval = time.Second

const waitingForYouSubtitle = "Waiting for you"

// The same pacing as EstimatedProgress in the frontend, so the system's bar and the app's agree: linear to 80% over the estimate, then slowing toward 95% without ever claiming to be finished.
const (
	estimateMark    = 0.80
	estimateCeiling = 0.95
)

type backgroundWork struct {
	core         *core
	title        string
	subtitle     string
	fraction     float64
	measured     bool
	waitingOnYou bool
	failure      string
	ended        sync.Once
}

type backgroundWorkReport struct {
	title    string
	subtitle string
	fraction float64
	at       time.Time
}

func (c *core) beginBackgroundWork(title string) *backgroundWork {
	work := &backgroundWork{core: c, title: title, fraction: unknownProgress}
	if !isMobile() {
		return work
	}
	c.backgroundWorkMu.Lock()
	defer c.backgroundWorkMu.Unlock()
	c.backgroundWorks = append(c.backgroundWorks, work)
	if len(c.backgroundWorks) == 1 {
		application.Mobile.SetKeepAwake(true)
	}
	c.reportBackgroundWorkLocked(true)
	return work
}

func (w *backgroundWork) Progress(fraction float64, subtitle string) {
	if !isMobile() {
		return
	}
	c := w.core
	c.backgroundWorkMu.Lock()
	defer c.backgroundWorkMu.Unlock()
	if !slices.Contains(c.backgroundWorks, w) {
		return
	}
	w.fraction = min(fraction, 1)
	w.subtitle = subtitle
	w.measured = w.measured || fraction >= 0
	c.reportBackgroundWorkLocked(false)
}

// A job that measures its own progress keeps it rather than a guess.
func (w *backgroundWork) EstimateFor(expectedSeconds int) (stop func()) {
	if !isMobile() || expectedSeconds <= 0 {
		return func() {}
	}
	done := make(chan struct{})
	go func() {
		startedAt := time.Now()
		ticker := time.NewTicker(backgroundProgressInterval)
		defer ticker.Stop()
		for {
			select {
			case <-done:
				return
			case <-ticker.C:
				w.estimate(estimatedFraction(time.Since(startedAt), expectedSeconds))
			}
		}
	}()
	var once sync.Once
	return func() { once.Do(func() { close(done) }) }
}

func (w *backgroundWork) estimate(fraction float64) {
	c := w.core
	c.backgroundWorkMu.Lock()
	defer c.backgroundWorkMu.Unlock()
	if w.measured || !slices.Contains(c.backgroundWorks, w) {
		return
	}
	w.fraction = fraction
	c.reportBackgroundWorkLocked(false)
}

func estimatedFraction(elapsed time.Duration, expectedSeconds int) float64 {
	spent := elapsed.Seconds() / float64(expectedSeconds)
	if spent <= 1 {
		return spent * estimateMark
	}
	return estimateMark + (estimateCeiling-estimateMark)*(1-1/spent)
}

func (w *backgroundWork) WaitOnYou(waiting bool) {
	if !isMobile() {
		return
	}
	c := w.core
	c.backgroundWorkMu.Lock()
	defer c.backgroundWorkMu.Unlock()
	if !slices.Contains(c.backgroundWorks, w) {
		return
	}
	w.waitingOnYou = waiting
	c.reportBackgroundWorkLocked(true)
}

// Recorded before End, so the system's UI can show that the job failed and why rather than a finished job.
func (w *backgroundWork) Fail(message string) {
	if !isMobile() || message == "" {
		return
	}
	c := w.core
	c.backgroundWorkMu.Lock()
	defer c.backgroundWorkMu.Unlock()
	w.failure = sentence(message)
}

func (w *backgroundWork) End() {
	if !isMobile() {
		return
	}
	w.ended.Do(func() {
		c := w.core
		c.backgroundWorkMu.Lock()
		defer c.backgroundWorkMu.Unlock()
		c.backgroundWorks = slices.DeleteFunc(c.backgroundWorks, func(running *backgroundWork) bool {
			return running == w
		})
		if len(c.backgroundWorks) == 0 {
			application.Mobile.SetKeepAwake(false)
			if w.failure != "" {
				c.backgroundReported = backgroundWorkReport{}
				endNativeBackgroundWorkFailed(w.title, w.failure)
				return
			}
		}
		c.reportBackgroundWorkLocked(true)
	})
}

func (c *core) reportBackgroundWorkLocked(force bool) {
	if len(c.backgroundWorks) == 0 {
		c.backgroundReported = backgroundWorkReport{}
		endNativeBackgroundWork()
		return
	}
	shown := c.backgroundWorks[len(c.backgroundWorks)-1]
	report := backgroundWorkReport{title: shown.title, subtitle: shown.subtitle, fraction: shown.fraction}
	if shown.waitingOnYou {
		report.subtitle = waitingForYouSubtitle
	}
	last := c.backgroundReported
	textChanged := report.title != last.title || report.subtitle != last.subtitle
	progressDue := report.fraction != last.fraction && time.Since(last.at) >= backgroundProgressInterval
	if !force && !textChanged && !progressDue {
		return
	}
	report.at = time.Now()
	c.backgroundReported = report
	showNativeBackgroundWork(report.title, report.subtitle, report.fraction)
}

func installProgressFraction(progress InstallProgress) float64 {
	if progress.Total <= 0 {
		return unknownProgress
	}
	itemsDone := progress.Index
	if progress.Status == "completed" || progress.Status == "error" {
		itemsDone++
	}
	return float64(itemsDone) / float64(progress.Total)
}

func installProgressSubtitle(progress InstallProgress) string {
	if progress.Component != "" {
		return progress.Component
	}
	return progress.Message
}
