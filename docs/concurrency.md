# Concurrency

Sprout 0.9 provides channels, tasks, cancellation, selection, and scheduler controls.

## Channels

Create a channel with `channel()`.
Pass a non-negative integer for buffered capacity.
A zero-capacity channel waits for a sender and receiver.

Send one value with `send(channel, value)`.
Receive one result with `recv(channel)`.
Close a channel with `close(channel)`.

`recv` returns `ok(value)` while the channel remains open.
It returns `err("channel closed")` after close.

## Tasks

Start a zero-argument function with `spawn(fn() { ... })`.
Wait for completion with `await(task)`.

`await` returns `ok(value)` after normal completion.
It returns `err(message)` after a task error.

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

## Execution model

Each task uses a child interpreter or VM context.
Closures keep their captured environment.
Channels and tasks remain safe to share between contexts.
The parent and child contexts share configured input and output streams.

Use channels to coordinate shared work.
Await every task that the program needs.

## Limitations

Cancellation has no timeout operation.
The scheduler policy cannot be configured.
`yield()` does not guarantee that another task runs immediately.
`sleep()` uses wall-clock time, so wake order is not a timing contract.
A task that ignores cancellation can block `await` forever.
Do not mutate captured lists, maps, or structs from multiple tasks.
