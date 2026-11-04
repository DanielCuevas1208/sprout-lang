package runtime

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/sprout-lang/sprout/internal/object"
)

const schedulerTraceVersion = 1

// SchedulerActionKind names one recorded scheduling operation.
type SchedulerActionKind string

const (
	// SchedulerActionYield records a yield() call.
	SchedulerActionYield SchedulerActionKind = "yield"
	// SchedulerActionSleep records a sleep() call.
	SchedulerActionSleep SchedulerActionKind = "sleep"
)

// SchedulerAction is one logical scheduling decision.
type SchedulerAction struct {
	Kind         SchedulerActionKind `json:"kind"`
	Milliseconds int64               `json:"milliseconds,omitempty"`
}

// SchedulerTrace stores the ordered scheduling decisions for one program run.
type SchedulerTrace struct {
	Version int               `json:"version"`
	Policy  SchedulerPolicy   `json:"policy"`
	Actions []SchedulerAction `json:"actions"`
}

// ParseSchedulerTraceFlag parses -scheduler-trace values.
//
// Use record:PATH to capture a trace or replay:PATH to replay one.
func ParseSchedulerTraceFlag(value string) (recordPath, replayPath string, err error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", "", nil
	}
	switch {
	case strings.HasPrefix(value, "record:"):
		path := strings.TrimSpace(strings.TrimPrefix(value, "record:"))
		if path == "" {
			return "", "", fmt.Errorf("scheduler trace record path is empty")
		}
		return path, "", nil
	case strings.HasPrefix(value, "replay:"):
		path := strings.TrimSpace(strings.TrimPrefix(value, "replay:"))
		if path == "" {
			return "", "", fmt.Errorf("scheduler trace replay path is empty")
		}
		return "", path, nil
	default:
		return "", "", fmt.Errorf("scheduler trace %q must start with record: or replay:", value)
	}
}

// OpenSchedulerSession builds the scheduler for an engine run.
//
// Finish must run after the program completes. It writes a recorded trace or
// verifies that a replay trace was fully consumed.
func OpenSchedulerSession(policy SchedulerPolicy, recordPath, replayPath string) (Scheduler, func() error, error) {
	if recordPath != "" && replayPath != "" {
		return nil, nil, fmt.Errorf("choose either scheduler trace record or replay, not both")
	}
	if replayPath != "" {
		trace, err := ReadSchedulerTrace(replayPath)
		if err != nil {
			return nil, nil, err
		}
		if trace.Policy != policy {
			return nil, nil, fmt.Errorf("scheduler replay: trace policy %q does not match requested policy %q", trace.Policy, policy)
		}
		replay, err := NewReplayScheduler(trace)
		if err != nil {
			return nil, nil, err
		}
		return replay, replay.VerifyComplete, nil
	}
	base, err := NewScheduler(policy)
	if err != nil {
		return nil, nil, err
	}
	if recordPath == "" {
		return base, func() error { return nil }, nil
	}
	recorder := NewRecordingScheduler(base, policy)
	return recorder, func() error {
		return WriteSchedulerTrace(recordPath, recorder.Trace())
	}, nil
}

// ReadSchedulerTrace loads a trace from a JSON file.
func ReadSchedulerTrace(path string) (SchedulerTrace, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return SchedulerTrace{}, fmt.Errorf("read scheduler trace %s: %w", path, err)
	}
	var trace SchedulerTrace
	if err := json.Unmarshal(data, &trace); err != nil {
		return SchedulerTrace{}, fmt.Errorf("parse scheduler trace %s: %w", path, err)
	}
	if err := validateSchedulerTrace(trace); err != nil {
		return SchedulerTrace{}, fmt.Errorf("scheduler trace %s: %w", path, err)
	}
	return trace, nil
}

// WriteSchedulerTrace writes trace to a JSON file.
func WriteSchedulerTrace(path string, trace SchedulerTrace) error {
	if err := validateSchedulerTrace(trace); err != nil {
		return err
	}
	data, err := json.MarshalIndent(trace, "", "  ")
	if err != nil {
		return fmt.Errorf("encode scheduler trace: %w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write scheduler trace %s: %w", path, err)
	}
	return nil
}

func validateSchedulerTrace(trace SchedulerTrace) error {
	if trace.Version != schedulerTraceVersion {
		return fmt.Errorf("unsupported trace version %d; want %d", trace.Version, schedulerTraceVersion)
	}
	if _, err := ParseSchedulerPolicy(string(trace.Policy)); err != nil {
		return fmt.Errorf("invalid trace policy %q", trace.Policy)
	}
	for i, action := range trace.Actions {
		if err := validateSchedulerAction(action); err != nil {
			return fmt.Errorf("action %d: %w", i+1, err)
		}
	}
	return nil
}

func validateSchedulerAction(action SchedulerAction) error {
	switch action.Kind {
	case SchedulerActionYield:
		if action.Milliseconds != 0 {
			return fmt.Errorf("yield action must not set milliseconds")
		}
	case SchedulerActionSleep:
		if err := validateSleepMilliseconds(action.Milliseconds); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown action kind %q", action.Kind)
	}
	return nil
}

// RecordingScheduler wraps a scheduler and records yield() and sleep() calls.
type RecordingScheduler struct {
	mu      sync.Mutex
	base    Scheduler
	policy  SchedulerPolicy
	actions []SchedulerAction
}

// NewRecordingScheduler records scheduling operations from base.
func NewRecordingScheduler(base Scheduler, policy SchedulerPolicy) *RecordingScheduler {
	if base == nil {
		base = DefaultScheduler()
	}
	return &RecordingScheduler{base: base, policy: policy}
}

// Trace returns the captured scheduling decisions.
func (r *RecordingScheduler) Trace() SchedulerTrace {
	r.mu.Lock()
	defer r.mu.Unlock()
	actions := append([]SchedulerAction(nil), r.actions...)
	if actions == nil {
		actions = []SchedulerAction{}
	}
	return SchedulerTrace{
		Version: schedulerTraceVersion,
		Policy:  r.policy,
		Actions: actions,
	}
}

// Yield records and delegates to the wrapped scheduler.
func (r *RecordingScheduler) Yield(ctx *Context) error {
	if err := r.base.Yield(ctx); err != nil {
		return err
	}
	r.mu.Lock()
	r.actions = append(r.actions, SchedulerAction{Kind: SchedulerActionYield})
	r.mu.Unlock()
	return nil
}

// Sleep records and delegates to the wrapped scheduler.
func (r *RecordingScheduler) Sleep(ctx *Context, milliseconds int64) error {
	if err := r.base.Sleep(ctx, milliseconds); err != nil {
		return err
	}
	r.mu.Lock()
	r.actions = append(r.actions, SchedulerAction{
		Kind:         SchedulerActionSleep,
		Milliseconds: milliseconds,
	})
	r.mu.Unlock()
	return nil
}

// ReplayScheduler enforces a recorded trace without wall-clock sleeps.
type ReplayScheduler struct {
	mu    sync.Mutex
	trace SchedulerTrace
	index int
}

// NewReplayScheduler builds a scheduler that replays trace.
func NewReplayScheduler(trace SchedulerTrace) (*ReplayScheduler, error) {
	if err := validateSchedulerTrace(trace); err != nil {
		return nil, err
	}
	return &ReplayScheduler{trace: trace}, nil
}

// Yield consumes the next yield action from the trace.
func (r *ReplayScheduler) Yield(ctx *Context) error {
	if ctx != nil && ctx.Cancelled() {
		return object.ErrCancelled
	}
	if err := r.consume(SchedulerActionYield, 0); err != nil {
		return err
	}
	if ctx != nil && ctx.Cancelled() {
		return object.ErrCancelled
	}
	return nil
}

// Sleep consumes the next sleep action and skips wall-clock waiting.
func (r *ReplayScheduler) Sleep(ctx *Context, milliseconds int64) error {
	if err := validateSleepMilliseconds(milliseconds); err != nil {
		return err
	}
	if ctx != nil && ctx.Cancelled() {
		return object.ErrCancelled
	}
	if milliseconds == 0 {
		if err := r.consume(SchedulerActionSleep, 0); err != nil {
			return err
		}
		if ctx != nil && ctx.Cancelled() {
			return object.ErrCancelled
		}
		return nil
	}
	if err := r.consume(SchedulerActionSleep, milliseconds); err != nil {
		return err
	}
	if ctx != nil && ctx.Cancelled() {
		return object.ErrCancelled
	}
	return nil
}

// VerifyComplete reports unused trace actions after the program finishes.
func (r *ReplayScheduler) VerifyComplete() error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.index < len(r.trace.Actions) {
		remaining := len(r.trace.Actions) - r.index
		return fmt.Errorf("scheduler replay: %d unused trace action(s) remain starting at step %d", remaining, r.index+1)
	}
	return nil
}

func (r *ReplayScheduler) consume(kind SchedulerActionKind, milliseconds int64) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	step := r.index + 1
	if r.index >= len(r.trace.Actions) {
		return fmt.Errorf("scheduler replay: trace ended at step %d but program issued %s", step, describeSchedulerAction(kind, milliseconds))
	}
	want := r.trace.Actions[r.index]
	r.index++
	if want.Kind != kind {
		return fmt.Errorf("scheduler replay: at step %d expected %s, got %s", step, describeSchedulerAction(want.Kind, want.Milliseconds), describeSchedulerAction(kind, milliseconds))
	}
	if kind == SchedulerActionSleep && want.Milliseconds != milliseconds {
		return fmt.Errorf("scheduler replay: at step %d expected sleep(%d), got sleep(%d)", step, want.Milliseconds, milliseconds)
	}
	return nil
}

func describeSchedulerAction(kind SchedulerActionKind, milliseconds int64) string {
	switch kind {
	case SchedulerActionYield:
		return "yield"
	case SchedulerActionSleep:
		return fmt.Sprintf("sleep(%d)", milliseconds)
	default:
		return string(kind)
	}
}
