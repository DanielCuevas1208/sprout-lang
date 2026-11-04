package runtime

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestSchedulerTraceRoundTrip(t *testing.T) {
	trace := SchedulerTrace{
		Version: schedulerTraceVersion,
		Policy:  SchedulerFair,
		Actions: []SchedulerAction{
			{Kind: SchedulerActionYield},
			{Kind: SchedulerActionSleep, Milliseconds: 0},
			{Kind: SchedulerActionSleep, Milliseconds: 12},
		},
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "trace.json")
	if err := WriteSchedulerTrace(path, trace); err != nil {
		t.Fatalf("WriteSchedulerTrace: %v", err)
	}
	got, err := ReadSchedulerTrace(path)
	if err != nil {
		t.Fatalf("ReadSchedulerTrace: %v", err)
	}
	if len(got.Actions) != len(trace.Actions) {
		t.Fatalf("actions = %d, want %d", len(got.Actions), len(trace.Actions))
	}
}

func TestReplaySchedulerDetectsMismatch(t *testing.T) {
	trace := SchedulerTrace{
		Version: schedulerTraceVersion,
		Policy:  SchedulerFair,
		Actions: []SchedulerAction{
			{Kind: SchedulerActionYield},
			{Kind: SchedulerActionSleep, Milliseconds: 5},
		},
	}
	replay, err := NewReplayScheduler(trace)
	if err != nil {
		t.Fatalf("NewReplayScheduler: %v", err)
	}
	if err := replay.Yield(nil); err != nil {
		t.Fatalf("first yield: %v", err)
	}
	if err := replay.Sleep(nil, 1); err == nil || !strings.Contains(err.Error(), "expected sleep(5)") {
		t.Fatalf("sleep mismatch error = %v", err)
	}
}

func TestReplaySchedulerDetectsUnusedActions(t *testing.T) {
	trace := SchedulerTrace{
		Version: schedulerTraceVersion,
		Policy:  SchedulerFair,
		Actions: []SchedulerAction{
			{Kind: SchedulerActionYield},
			{Kind: SchedulerActionYield},
		},
	}
	replay, err := NewReplayScheduler(trace)
	if err != nil {
		t.Fatalf("NewReplayScheduler: %v", err)
	}
	if err := replay.Yield(nil); err != nil {
		t.Fatalf("yield: %v", err)
	}
	if err := replay.VerifyComplete(); err == nil || !strings.Contains(err.Error(), "unused trace action") {
		t.Fatalf("VerifyComplete = %v", err)
	}
}

func TestRecordingSchedulerCapturesActions(t *testing.T) {
	recorder := NewRecordingScheduler(&policyScheduler{policy: SchedulerDirect}, SchedulerDirect)
	if err := recorder.Yield(nil); err != nil {
		t.Fatalf("yield: %v", err)
	}
	if err := recorder.Sleep(nil, 0); err != nil {
		t.Fatalf("sleep: %v", err)
	}
	trace := recorder.Trace()
	if len(trace.Actions) != 2 {
		t.Fatalf("actions = %d, want 2", len(trace.Actions))
	}
	if trace.Policy != SchedulerDirect {
		t.Fatalf("policy = %q, want direct", trace.Policy)
	}
}

func TestParseSchedulerTraceFlag(t *testing.T) {
	record, replay, err := ParseSchedulerTraceFlag("record:trace.json")
	if err != nil || record != "trace.json" || replay != "" {
		t.Fatalf("record parse = %q %q %v", record, replay, err)
	}
	record, replay, err = ParseSchedulerTraceFlag("replay:trace.json")
	if err != nil || record != "" || replay != "trace.json" {
		t.Fatalf("replay parse = %q %q %v", record, replay, err)
	}
	if _, _, err := ParseSchedulerTraceFlag("bad"); err == nil {
		t.Fatal("expected error for bad flag")
	}
}

func TestOpenSchedulerSessionRecordAndFinish(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "trace.json")
	scheduler, finish, err := OpenSchedulerSession(SchedulerFair, path, "")
	if err != nil {
		t.Fatalf("OpenSchedulerSession: %v", err)
	}
	if err := scheduler.Yield(nil); err != nil {
		t.Fatalf("yield: %v", err)
	}
	if err := finish(); err != nil {
		t.Fatalf("finish: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("stat trace: %v", err)
	}
}

func TestOpenSchedulerSessionReplayFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "trace.json")
	trace := SchedulerTrace{
		Version: schedulerTraceVersion,
		Policy:  SchedulerFair,
		Actions: []SchedulerAction{{Kind: SchedulerActionYield}},
	}
	if err := WriteSchedulerTrace(path, trace); err != nil {
		t.Fatalf("WriteSchedulerTrace: %v", err)
	}
	scheduler, finish, err := OpenSchedulerSession(SchedulerFair, "", path)
	if err != nil {
		t.Fatalf("OpenSchedulerSession: %v", err)
	}
	if err := scheduler.Yield(nil); err != nil {
		t.Fatalf("yield: %v", err)
	}
	if err := finish(); err != nil {
		t.Fatalf("finish: %v", err)
	}
}

func TestOpenSchedulerSessionRejectsPolicyMismatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "trace.json")
	trace := SchedulerTrace{
		Version: schedulerTraceVersion,
		Policy:  SchedulerDirect,
		Actions: []SchedulerAction{{Kind: SchedulerActionYield}},
	}
	if err := WriteSchedulerTrace(path, trace); err != nil {
		t.Fatalf("WriteSchedulerTrace: %v", err)
	}
	if _, _, err := OpenSchedulerSession(SchedulerFair, "", path); err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("policy mismatch error = %v", err)
	}
}

func TestRecordingSchedulerIsSafeForConcurrentTasks(t *testing.T) {
	recorder := NewRecordingScheduler(&policyScheduler{policy: SchedulerDirect}, SchedulerDirect)
	const workers = 8
	const actionsPerWorker = 16
	var wg sync.WaitGroup
	for range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range actionsPerWorker {
				if err := recorder.Yield(nil); err != nil {
					t.Errorf("yield: %v", err)
				}
			}
		}()
	}
	wg.Wait()
	if got := len(recorder.Trace().Actions); got != workers*actionsPerWorker {
		t.Fatalf("actions = %d, want %d", got, workers*actionsPerWorker)
	}
}

func TestReplaySchedulerIsSafeForConcurrentTasks(t *testing.T) {
	const actions = 128
	trace := SchedulerTrace{
		Version: schedulerTraceVersion,
		Policy:  SchedulerDirect,
		Actions: make([]SchedulerAction, actions),
	}
	for i := range trace.Actions {
		trace.Actions[i] = SchedulerAction{Kind: SchedulerActionYield}
	}
	replay, err := NewReplayScheduler(trace)
	if err != nil {
		t.Fatalf("NewReplayScheduler: %v", err)
	}
	var wg sync.WaitGroup
	for range actions {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := replay.Yield(nil); err != nil {
				t.Errorf("yield: %v", err)
			}
		}()
	}
	wg.Wait()
	if err := replay.VerifyComplete(); err != nil {
		t.Fatalf("VerifyComplete: %v", err)
	}
}
