# Standard Library

This page lists the built-in functions of Sprout 0.3.
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

`int` and `float` accept numbers, booleans, and numeric strings.
`type` reports `module` for an imported module's namespace.

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

## Assertions

| Function | Description |
|----------|-------------|
| `assert(cond, msg?)` | Stops on a false condition. |

An assertion error names the file and line of the call.
