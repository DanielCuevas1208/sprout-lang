package runtime

import (
	"fmt"
	goruntime "runtime"
	"time"

	"github.com/sprout-lang/sprout/internal/object"
)

// maxSleepMilliseconds keeps the conversion to time.Duration safe.
const maxSleepMilliseconds = int64(1<<63-1) / int64(time.Millisecond)

// SchedulerPolicy selects how yield() hands control to the Go scheduler.
type SchedulerPolicy string

const (
	// SchedulerFair performs an explicit cooperative handoff.
	SchedulerFair SchedulerPolicy = "fair"
	// SchedulerDirect keeps running the current task after yield().
	SchedulerDirect SchedulerPolicy = "direct"
)

// Scheduler owns the policy used by scheduler builtins.
//
// Schedulers created by NewScheduler are safe to share between an engine
// and its child tasks. The policy is immutable after construction.
type Scheduler interface {
	Yield(ctx *Context) error
	Sleep(ctx *Context, milliseconds int64) error
}

type policyScheduler struct {
	policy SchedulerPolicy
}

// ParseSchedulerPolicy validates a command-line or embedding policy name.
func ParseSchedulerPolicy(name string) (SchedulerPolicy, error) {
	switch SchedulerPolicy(name) {
	case SchedulerFair, SchedulerDirect:
		return SchedulerPolicy(name), nil
	default:
		return "", fmt.Errorf("unknown scheduler policy %q; choose fair or direct", name)
	}
}

// NewScheduler creates a scheduler with policy.
func NewScheduler(policy SchedulerPolicy) (Scheduler, error) {
	validated, err := ParseSchedulerPolicy(string(policy))
	if err != nil {
		return nil, err
	}
	return &policyScheduler{policy: validated}, nil
}

// DefaultScheduler returns the scheduler used by new execution engines.
func DefaultScheduler() Scheduler {
	return defaultScheduler
}

// schedulerFor returns the configured scheduler or the default policy for a
// context created by an embedding application.
func schedulerFor(ctx *Context) Scheduler {
	if ctx != nil && ctx.Scheduler != nil {
		return ctx.Scheduler
	}
	return defaultScheduler
}

var defaultScheduler = &policyScheduler{policy: SchedulerFair}

// Yield gives another runnable goroutine a scheduling opportunity.
func (s *policyScheduler) Yield(ctx *Context) error {
	if ctx != nil && ctx.Cancelled() {
		return object.ErrCancelled
	}
	if s.policy == SchedulerFair {
		goruntime.Gosched()
	}
	if ctx != nil && ctx.Cancelled() {
		return object.ErrCancelled
	}
	return nil
}

// Sleep pauses the current task while keeping cancellation wakeable.
func (s *policyScheduler) Sleep(ctx *Context, milliseconds int64) error {
	if err := validateSleepMilliseconds(milliseconds); err != nil {
		return err
	}
	if milliseconds == 0 {
		return s.Yield(ctx)
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

// validateSleepMilliseconds keeps timer conversion bounds in one place.
func validateSleepMilliseconds(milliseconds int64) error {
	if milliseconds < 0 {
		return FmtErr("sleep() duration cannot be negative")
	}
	if milliseconds > maxSleepMilliseconds {
		return FmtErr("sleep() duration is too large")
	}
	return nil
}

// schedulerYield gives another runnable goroutine a scheduling opportunity.
func schedulerYield(ctx *Context) error {
	if ctx != nil && ctx.Cancelled() {
		return object.ErrCancelled
	}
	if err := schedulerFor(ctx).Yield(ctx); err != nil {
		return err
	}
	if ctx != nil && ctx.Cancelled() {
		return object.ErrCancelled
	}
	return nil
}

// schedulerSleep pauses the current task while keeping cancellation wakeable.
func schedulerSleep(ctx *Context, milliseconds int64) error {
	if err := validateSleepMilliseconds(milliseconds); err != nil {
		return err
	}
	if ctx != nil && ctx.Cancelled() {
		return object.ErrCancelled
	}
	if err := schedulerFor(ctx).Sleep(ctx, milliseconds); err != nil {
		return err
	}
	if ctx != nil && ctx.Cancelled() {
		return object.ErrCancelled
	}
	return nil
}
