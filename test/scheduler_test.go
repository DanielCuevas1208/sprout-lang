package test

import (
	"strings"
	"sync/atomic"
	"testing"

	"github.com/sprout-lang/sprout/internal/checker"
	"github.com/sprout-lang/sprout/internal/compiler"
	"github.com/sprout-lang/sprout/internal/interp"
	"github.com/sprout-lang/sprout/internal/parser"
	"github.com/sprout-lang/sprout/internal/runtime"
	"github.com/sprout-lang/sprout/internal/source"
	"github.com/sprout-lang/sprout/internal/vm"
)

type countingScheduler struct {
	yieldCalls atomic.Int32
	sleepCalls atomic.Int32
}

func (s *countingScheduler) Yield(ctx *runtime.Context) error {
	s.yieldCalls.Add(1)
	return nil
}

func (s *countingScheduler) Sleep(ctx *runtime.Context, milliseconds int64) error {
	s.sleepCalls.Add(1)
	return nil
}

func TestConfiguredSchedulerReachesChildTasks(t *testing.T) {
	const src = `let task = spawn(fn() {
    yield()
    sleep(0)
    return 7
})
print(unwrap(await(task)))`

	interpScheduler := &countingScheduler{}
	interpOutput, interpErr := runWithSchedulerInterp(src, interpScheduler)
	if interpErr != nil {
		t.Fatalf("interpreter error: %v", interpErr)
	}

	vmScheduler := &countingScheduler{}
	vmOutput, vmErr := runWithSchedulerVM(src, vmScheduler)
	if vmErr != nil {
		t.Fatalf("VM error: %v", vmErr)
	}

	if interpOutput != "7\n" || vmOutput != interpOutput {
		t.Fatalf("outputs = %q and %q, want identical 7 output", interpOutput, vmOutput)
	}
	if interpScheduler.yieldCalls.Load() != 1 || interpScheduler.sleepCalls.Load() != 1 {
		t.Fatalf("interpreter scheduler calls = %d yield, %d sleep",
			interpScheduler.yieldCalls.Load(), interpScheduler.sleepCalls.Load())
	}
	if vmScheduler.yieldCalls.Load() != 1 || vmScheduler.sleepCalls.Load() != 1 {
		t.Fatalf("VM scheduler calls = %d yield, %d sleep",
			vmScheduler.yieldCalls.Load(), vmScheduler.sleepCalls.Load())
	}
}

func runWithSchedulerInterp(src string, scheduler runtime.Scheduler) (string, error) {
	file := source.NewFile("scheduler-test.spr", src)
	prog, diags := parser.Parse(file)
	if len(diags) > 0 {
		return "", &testError{message: diags[0].Message}
	}
	var output strings.Builder
	iv := interp.NewWithIO(strings.NewReader(""), &output, &strings.Builder{})
	iv.SetScheduler(scheduler)
	_, rerr := iv.Exec(file, prog)
	if rerr != nil {
		return output.String(), rerr
	}
	return output.String(), nil
}

func runWithSchedulerVM(src string, scheduler runtime.Scheduler) (string, error) {
	file := source.NewFile("scheduler-test.spr", src)
	prog, diags := parser.Parse(file)
	if len(diags) > 0 {
		return "", &testError{message: diags[0].Message}
	}
	if cdiags := checker.Check(file, prog); len(cdiags) > 0 {
		return "", &testError{message: cdiags[0].Message}
	}
	compiled, err := compiler.Compile(file, prog)
	if err != nil {
		return "", err
	}
	var output strings.Builder
	machine := vm.NewWithIO(strings.NewReader(""), &output, &strings.Builder{})
	machine.SetScheduler(scheduler)
	_, rerr := machine.Run(file, compiled)
	if rerr != nil {
		return output.String(), rerr
	}
	return output.String(), nil
}

type testError struct {
	message string
}

func (e *testError) Error() string {
	return e.message
}
