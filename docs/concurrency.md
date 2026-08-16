# Concurrency

Sprout 0.8 provides channels, tasks, cooperative cancellation, and channel selection.

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

## Execution model

Each task uses a child interpreter or VM context.
Closures keep their captured environment.
Channels and tasks remain safe to share between contexts.
The parent and child contexts share configured input and output streams.

Use channels to coordinate shared work.
Await every task that the program needs.

## Limitations

Cancellation has no timeout operation.
Sprout has no scheduler controls.
A task that ignores cancellation can block `await` forever.
Do not mutate captured lists, maps, or structs from multiple tasks.
