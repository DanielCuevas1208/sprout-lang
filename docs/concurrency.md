# Concurrency

Sprout 0.14 provides channel ownership, channels, tasks, cancellation, selection, scheduler policies, bounded waits, task isolation, and trace replay.

## Channels

Create a channel with `channel()`.
Pass a non-negative integer for buffered capacity.
A zero-capacity channel waits for a sender and receiver.

Send one value with `send(channel, value)`.
Receive one result with `recv(channel)`.
Close a channel with `close(channel)`.

`recv` returns `ok(value)` while the channel remains open.
It returns `err("channel closed")` after close.

## Channel message ownership

Channel sends use rejection semantics for mutable values.
Scalar values, channels, and tasks can cross a channel.
Results can cross when their contained value follows the same rule.

`send` rejects lists, maps, and struct instances.
It also rejects nested mutable values inside results, modules, methods, and closures.
The check runs before a send can block or deliver a message.

```sprout
let messages = channel(2)
send(messages, 7)
send(messages, "ready")
print(unwrap(recv(messages)))
print(unwrap(recv(messages)))
close(messages)
```

The program prints `7` and `ready`.
See `examples/channel_ownership.spr` for the complete example.

## Tasks

Start a zero-argument function with `spawn(fn() { ... })`.
Wait for completion with `await(task)`.

`await` returns `ok(value)` after normal completion.
It returns `err(message)` after a task error.

## Timed waits

Pass milliseconds as the second argument to `await`.
`await(task, 0)` checks task state without waiting.
A positive timeout waits for completion up to that duration.
An expired wait returns `err("task timed out")`.
The timeout does not cancel the task.
Call `cancel(task)` when the task no longer matters.

```sprout
let gate = channel()
let worker = spawn(fn() {
    recv(gate)
    return "ready"
})

print(unwrap_or(await(worker, 1), "timed out"))
cancel(worker)
print(unwrap_or(await(worker), "cancelled"))
close(gate)
```

The program prints `timed out` and `cancelled`.

## Task isolation

`spawn` checks captured values before it starts a task.
A closure may capture scalar values, channels, and tasks.
A closure must not capture a list, map, or struct instance.
Nested mutable values are rejected.
The check covers all values reachable from the closure environment.
The check returns an error before task creation.
Use channels to share task state.

See `examples/isolation.spr` for a complete example.

## Cancellation

Request cancellation with `cancel(task)`.
Check the current task with `is_cancelled()`.
Cancellation is cooperative.
It does not force arbitrary code to stop.

Blocking `send`, `recv`, and `await` observe cancellation.
A CPU-bound task must poll `is_cancelled()` inside its loop.
A canceled task completes with `err("task cancelled")`.

```sprout
let gate = channel()
let worker = spawn(fn() {
    let signal = recv(gate)
    if is_cancelled() {
        return "stopped"
    }
    return signal
})

cancel(worker)
print(unwrap_or(await(worker), "cancelled"))
```

The program prints the following line.

```text
cancelled
```


## Selection

Use `select` to wait on several channels.
Pass one list of channels.
The function skips closed channels without buffered values.

It returns `ok` with a map when a value arrives.
The map contains `index` and `value` keys.
The index keeps the input list position.

```sprout
let left = channel()
let right = channel(1)
send(right, "ready")
let event = unwrap(select([left, right]))
print(event["index"], event["value"])
close(left)
close(right)
print(unwrap_or(select([left, right]), "all closed"))
```

The program prints this output.

```text
1 ready
all closed
```

When every channel closes, `select` returns `err("all channels closed")`.
Cancellation returns `err("task cancelled")`.

## Scheduler controls

`yield()` gives another runnable task a scheduling opportunity.
It returns nil after the opportunity.

`sleep(milliseconds)` pauses the current task for a wall-clock duration.
It uses milliseconds.
A zero duration acts like `yield()`.
Sleeping does not stop other tasks.
Cancellation stops the current task during either operation.

```sprout
let task = spawn(fn() {
    sleep(1000)
    return "finished"
})
cancel(task)
print(unwrap_or(await(task), "cancelled"))
```

The program prints `cancelled`.

## Scheduler policies

Choose a policy with `-scheduler fair` or `-scheduler direct`.
Use the flag with `run` or `vm`.

`fair` is the default policy.
It asks the Go scheduler to run another task at `yield()`.
`direct` skips that explicit handoff.

Both policies keep cancellation checks.
The interpreter, VM, and child tasks use one policy.

```text
sprout vm -scheduler direct examples/scheduler.spr
```

## Scheduler trace replay

Record yield and sleep decisions while a program runs.
Replay the trace to execute the same logical action sequence without wall-clock sleeps.

```text
sprout run -scheduler-trace record:trace.json examples/scheduler_replay.spr
sprout vm -scheduler-trace replay:trace.json examples/scheduler_replay.spr
```

The trace is JSON. It stores the scheduler policy and an ordered action list.
Replay fails when the program issues a different action or leaves unused trace steps.
Replay requires the same scheduler policy as recording.

Traces cover `yield()` and `sleep()` only.
They do not record task identities or `select()` choices yet.

## Execution model

Each task uses a child interpreter or VM context.
Closures use captured lexical values.
The runtime rejects captured mutable values.
Channels and tasks remain safe to share between contexts.
The parent and child contexts share configured input and output streams.

Use channels to coordinate shared work.
Await every task that the program needs.

## Limitations

Cancellation has no automatic timeout.
`yield()` does not guarantee that another task runs immediately.
The `direct` policy does not force a task handoff.
`sleep()` uses wall-clock time during normal runs, so wake order is not a timing contract.
Replay mode skips wall-clock sleeps and follows the recorded trace instead.
A positive await timeout also uses wall-clock time.
An unbounded `await` can block when a task ignores cancellation.
Channel messages reject mutable values.
They do not copy or transfer mutable values.
