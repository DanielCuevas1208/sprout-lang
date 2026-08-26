# Standard Library

This page lists the built-in functions of Sprout 0.9.
They are available in every program without an import.

## Output

| Function | Description |
|----------|-------------|
| `print(...)` | Prints values with spaces and a newline. |
| `write(...)` | Prints values without a newline. |
| `eprint(...)` | Prints values to the error stream. |

## Conversion

| Function | Description |
|----------|-------------|
| `str(x)` | Returns the display text of x. |
| `int(x)` | Converts x to an integer. |
| `float(x)` | Converts x to a float. |
| `bool(x)` | Converts x to a boolean. |
| `type(x)` | Returns the type name of x as a string. |

`type` reports `struct` for an instance, `struct type` for a struct
declaration, and `method` for a bound method.
`int` and `float` accept numbers, booleans, and numeric strings.

## Values

| Function | Description |
|----------|-------------|
| `len(x)` | Returns the length of a string, list, or map. |
| `range(a, b, step?)` | Returns a range from a to b, exclusive of b. |
| `input(prompt?)` | Reads one line from the input. |

`len` counts runes in a string. It counts elements in a list or map.

## Lists

| Function | Description |
|----------|-------------|
| `push(list, x)` | Adds x to the end. Returns the list. |
| `pop(list)` | Removes the last element. Returns it. |
| `get(list, i)` | Reads element i. Returns nil when out of range. |
| `set(list, i, x)` | Writes element i. Returns the list. |
| `has(list, i)` | Reports whether i is a valid index. |
| `map(list, fn)` | Applies fn to each element. |
| `filter(list, fn)` | Keeps elements where fn returns truthy. |
| `fold(list, init, fn)` | Reduces the list with fn. |

## Maps

| Function | Description |
|----------|-------------|
| `get(map, key)` | Reads a key. Returns nil when missing. |
| `set(map, key, x)` | Writes a key. Returns the map. |
| `has(map, key)` | Reports whether a key exists. |
| `keys(map)` | Returns the keys as a list. |
| `values(map)` | Returns the values as a list. |

Map keys are strings. Maps keep insertion order.

## Strings

| Function | Description |
|----------|-------------|
| `join(sep, list)` | Joins a list into a string. |
| `split(str, sep)` | Splits a string into a list. |
| `upper(str)` | Returns the uppercase copy. |
| `lower(str)` | Returns the lowercase copy. |
| `trim(str)` | Removes surrounding whitespace. |
| `starts_with(str, part)` | Reports a prefix match. |
| `ends_with(str, part)` | Reports a suffix match. |
| `contains(str, part)` | Reports a substring match. |
| `repeat(str, n)` | Returns str repeated n times. |

## Numbers

| Function | Description |
|----------|-------------|
| `abs(n)` | Returns the absolute value. |
| `min(a, b, ...)` | Returns the smallest number. |
| `max(a, b, ...)` | Returns the largest number. |
| `floor(n)` | Rounds down to an integer. |
| `ceil(n)` | Rounds up to an integer. |
| `round(n)` | Rounds to the nearest integer. |
| `sqrt(n)` | Returns the square root. |

## Results

| Function | Returns |
|----------|---------|
| `ok(x)` | An ok result that holds x. |
| `err(msg)` | An error result that holds msg. |
| `is_ok(r)` | true when r is an ok result. |
| `is_err(r)` | true when r is an error result. |
| `unwrap(r)` | The ok value. Stops on an error. |
| `unwrap_or(r, fallback)` | The ok value, or fallback. |

`ok` holds any value. `err` holds a message string.
`unwrap` stops the program when the result is an error.
See `docs/results.md` for the full reference.

## Modules

| Function | Description |
|----------|-------------|
| `module(name, exports)` | Wraps a map into a module value. |

The build tool uses `module` to reconstruct modules inside a bundle.
Programs can use it to build module values dynamically.

## Concurrency

| Function | Description |
|----------|-------------|
| `channel(capacity?)` | Creates a blocking or buffered channel. |
| `send(channel, value)` | Sends one value and waits for a receiver. |
| `recv(channel)` | Receives a result from a channel. |
| `select(channels)` | Waits for a value from several channels. |
| `close(channel)` | Closes a channel. |
| `spawn(fn)` | Starts a zero-argument function as a task. |
| `await(task)` | Waits for a task and returns a result. |
| `cancel(task)` | Requests cooperative cancellation. |
| `is_cancelled()` | Reports current task cancellation. |
| `yield()` | Gives another task a scheduling opportunity. |
| `sleep(milliseconds)` | Pauses the current task for milliseconds. |

`recv` returns `ok(value)` for a value.
It returns `err("channel closed")` after close.
`await` returns the worker value or an error result.
`cancel` wakes blocked operations when cancellation reaches a task.
`select` returns `ok({"index": i, "value": v})` for a value.
It returns `err("all channels closed")` when no channel remains.
It skips closed channels without buffered values.
`yield` does not guarantee a task switch.
`sleep` observes cancellation and uses wall-clock time.
See `docs/concurrency.md` for the full reference.

## Assertions

| Function | Description |
|----------|-------------|
| `assert(cond, msg?)` | Stops on a false condition. |

An assertion error names the file and line of the call.
