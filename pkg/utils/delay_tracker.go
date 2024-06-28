package utils

import (
	"fmt"
	"os"
	"time"
)

type DelayTracker struct {
	verbose bool
	action  string
	start   time.Time
}

func StartDelayTracker(verbose bool, action string) *DelayTracker {
	dt := DelayTracker{
		verbose: verbose,
		action:  action,
	}
	if dt.verbose {
		dt.start = time.Now()
		fmt.Fprintf(os.Stderr, "Getting %s...\n", dt.action)
	}
	return &dt
}

func (dt *DelayTracker) End() {
	if dt.verbose {
		fmt.Fprintf(os.Stderr, "Got %s within %s\n", dt.action, time.Since(dt.start))
	}
}
