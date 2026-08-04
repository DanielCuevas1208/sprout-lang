# Concurrency

Sprout 0.6 provides channels and tasks.
A channel moves values between concurrent functions.
A task represents one spawned function.

## Channels

Create a channel with `channel()`.
Pass a non-negative integer to set its buffer capacity.
A zero-capacity channel waits for a sender and a receiver.
A buffered channel accepts values until its capacity fills.

Send one value with `send(channel, value)`.
Receive one result with `recv(channel)`.
The receive result is `ok(value)` while the channel remains open.
The receive result is `err("channel closed")` after the channel closes.
Close a channel with `close(channel)`.

## Tasks

Start a zero-argument function with `spawn(fn() { ... })`.
Wait for a task with `await(task)`.
The await result is `ok(value)` after normal completion.
The await result is `err(message)` after a task error.

```sprout
let jobs = channel()
let worker = spawn(fn() {
    send(jobs, 21)
    return "worker finished"
})

print(unwrap(recv(jobs)))
print(unwrap(await(worker)))
close(jobs)
print(is_err(recv(jobs)))
```

The program prints the following lines.

```text
21
worker finished
true
```

## Execution model

Each task uses a child interpreter or VM context.
A closure keeps its captured environment.
Channels and tasks remain safe to share between contexts.
The parent and child contexts share the configured input and output streams.

Do not mutate captured lists, maps, or structs from multiple tasks.
Use channels to coordinate shared work.
Await every task that the program needs.

Sprout does not cancel tasks.
Sprout does not provide timeouts or channel selection.
A task that does not finish can block `await` forever.