# Sprout Roadmap

This roadmap records release evidence and bounded future work.

## Completed releases

### release-0.2 — complete

Outcome: The project runs programs on a bytecode virtual machine.

Deliverable: The compiler and VM share the parser and checker.

Acceptance: VM programs match interpreter behavior.

Verify: go test ./internal/vm

### release-0.3 — complete

Outcome: Programs can load modules and build projects.

Deliverable: File modules, imports, cycle checks, and bundles.

Acceptance: Module projects run on both engines.

Verify: go test ./internal/module ./internal/build

### release-0.4 — complete

Outcome: Programs can define structs, methods, and interfaces.

Deliverable: Runtime structs and static interface checks.

Acceptance: Struct examples run on both engines.

Verify: go test ./internal/interp ./internal/vm -run Struct

### release-0.5 — complete

Outcome: Programs can return results and match patterns.

Deliverable: Result values and match expressions.

Acceptance: Result examples agree across engines.

Verify: go test ./test -run Match

### release-0.6 — complete

Outcome: Programs can coordinate joinable tasks with channels.

Deliverable: Blocking channels, task handles, and await.

Acceptance: Channel examples finish on both engines.

Verify: go test ./test -run Concurrency

### release-0.7 — complete

Outcome: Programs can stop blocked tasks cooperatively.

Deliverable: Cancellation signals and cancellation-aware blocking calls.

Acceptance: Canceled tasks return deterministic errors.

Verify: go test ./test -run Cancellation

### release-0.8 — complete

Outcome: Programs can wait on several channels.

Deliverable: Deterministic channel selection.

Acceptance: Selection reports the chosen input index.

Verify: go test ./test -run Select

### release-0.9 — complete

Outcome: Programs can yield and sleep inside tasks.

Deliverable: Scheduler controls and cancellation checks.

Acceptance: Both controls reach child tasks.

Verify: go test ./test -run SchedulerControls

### release-0.10 — complete

Outcome: Embedders can choose a scheduler policy.

Deliverable: Fair and direct scheduler policies.

Acceptance: Both engines use one configured policy.

Verify: go test ./test -run Scheduler

### release-0.11 — complete

Outcome: Programs can bound task waits.

Deliverable: Optional await timeouts.

Acceptance: Timeouts do not cancel the waiting task.

Verify: go test ./test -run TimedAwait

## Current release

### task-isolation — complete

Outcome: Spawned closures have explicit mutable-state rules.

Deliverable: A shared validator rejects captured lists, maps, and struct instances.

Policy: Scalar values, channels, and tasks remain shareable.

Acceptance:

- Accepted scalar and channel captures run on both engines.
- Captured lists, maps, and structs fail before task creation.
- Cancellation still wakes an accepted blocked task.
- Both engines return the same output and error text.

Verify:

- `go test ./internal/runtime -run TestValidateTaskCapture`
- `go test ./test -run TestEnginesAgreeOnTaskIsolation`
- `go run ./cmd/sprout run examples/isolation.spr`
- `go run ./cmd/sprout vm examples/isolation.spr`

## Future milestones

### scheduler-replay — planned

Outcome: Developers can reproduce one logical task schedule.

Deliverable: Record and replay scheduler choices without wall-clock timing.

Acceptance:

- A concurrency example replays from a recorded trace.
- Incompatible traces fail with a clear error.
- Interpreter and VM outputs match for replayed scenarios.

Verify: go test ./test -run TestEnginesAgreeOnSchedulerReplay

### channel-value-ownership — planned

Outcome: Channel messages have explicit mutable-value ownership.

Deliverable: Choose transfer, copy, or rejection semantics for mutable payloads.

Acceptance:

- The policy covers lists, maps, and structs.
- Accepted and rejected messages have deterministic results.
- Interpreter and VM behavior matches.

Verify: go test ./test -run TestEnginesAgreeOnChannelValueOwnership
