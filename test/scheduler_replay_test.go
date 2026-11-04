package test

import (
	"reflect"
	"testing"

	"github.com/sprout-lang/sprout/internal/runtime"
)

const schedulerReplayProgram = `let events = channel(1)
let worker = spawn(fn() {
    send(events, "started")
    yield()
    sleep(5)
    send(events, "finished")
    return "done"
})
print(unwrap(recv(events)))
print(unwrap(recv(events)))
print(unwrap(await(worker)))
close(events)`

func TestEnginesAgreeOnSchedulerReplay(t *testing.T) {
	want := "started\nfinished\ndone\n"

	interpBase, err := runtime.NewScheduler(runtime.SchedulerFair)
	if err != nil {
		t.Fatalf("interp base scheduler: %v", err)
	}
	interpRecorder := runtime.NewRecordingScheduler(interpBase, runtime.SchedulerFair)
	interpOut, interpErr := runWithSchedulerInterp(schedulerReplayProgram, interpRecorder)
	if interpErr != nil {
		t.Fatalf("record interpreter: %v", interpErr)
	}

	vmBase, err := runtime.NewScheduler(runtime.SchedulerFair)
	if err != nil {
		t.Fatalf("vm base scheduler: %v", err)
	}
	vmRecorder := runtime.NewRecordingScheduler(vmBase, runtime.SchedulerFair)
	vmOut, vmErr := runWithSchedulerVM(schedulerReplayProgram, vmRecorder)
	if vmErr != nil {
		t.Fatalf("record vm: %v", vmErr)
	}

	if interpOut != want || vmOut != interpOut {
		t.Fatalf("recorded outputs = %q and %q, want %q", interpOut, vmOut, want)
	}
	if !reflect.DeepEqual(interpRecorder.Trace().Actions, vmRecorder.Trace().Actions) {
		t.Fatalf("recorded traces differ:\ninterp: %#v\nvm: %#v", interpRecorder.Trace().Actions, vmRecorder.Trace().Actions)
	}

	trace := interpRecorder.Trace()
	interpReplay, err := runtime.NewReplayScheduler(trace)
	if err != nil {
		t.Fatalf("interp replay scheduler: %v", err)
	}
	replayOut, replayErr := runWithSchedulerInterp(schedulerReplayProgram, interpReplay)
	if replayErr != nil {
		t.Fatalf("replay interpreter: %v", replayErr)
	}
	if err := interpReplay.VerifyComplete(); err != nil {
		t.Fatalf("interp replay verify: %v", err)
	}

	vmReplay, err := runtime.NewReplayScheduler(trace)
	if err != nil {
		t.Fatalf("vm replay scheduler: %v", err)
	}
	vmReplayOut, vmReplayErr := runWithSchedulerVM(schedulerReplayProgram, vmReplay)
	if vmReplayErr != nil {
		t.Fatalf("replay vm: %v", vmReplayErr)
	}
	if err := vmReplay.VerifyComplete(); err != nil {
		t.Fatalf("vm replay verify: %v", err)
	}

	if replayOut != want || vmReplayOut != replayOut {
		t.Fatalf("replayed outputs = %q and %q, want %q", replayOut, vmReplayOut, want)
	}
}
