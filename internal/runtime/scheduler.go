package runtime

import (
	goruntime "runtime"
	"time"

	"github.com/sprout-lang/sprout/internal/object"
)

// maxSleepMilliseconds keeps the conversion to time.Duration safe.
const maxSleepMilliseconds = int64(1<<63-1) / int64(time.Millisecond)

// schedulerYield gives another runnable goroutine a scheduling opportunity.
func schedulerYield(ctx *Context) error {
	if ctx != nil && ctx.Cancelled() {
		return object.ErrCancelled
	}
	goruntime.Gosched()
	if ctx != nil && ctx.Cancelled() {
		return object.ErrCancelled
	}
	return nil
}

// schedulerSleep pauses the current task while keeping cancellation wakeable.
func schedulerSleep(ctx *Context, milliseconds int64) error {
	if milliseconds < 0 {
		return FmtErr("sleep() duration cannot be negative")
	}
	if milliseconds > maxSleepMilliseconds {
		return FmtErr("sleep() duration is too large")
	}
	if milliseconds == 0 {
		return schedulerYield(ctx)
	}

	timer := time.NewTimer(time.Duration(milliseconds) * time.Millisecond)
	defer func() {
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
	}()

	var done <-chan struct{}
	if ctx != nil {
		done = ctx.Done
	}
	select {
	case <-timer.C:
		return nil
	case <-done:
		return object.ErrCancelled
	}
}
