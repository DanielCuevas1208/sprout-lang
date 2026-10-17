# Results and Pattern Matching

This page explains result values and the `match` expression.
They are the headline feature of Sprout 0.5.

## Results

A result is a value that carries one of two states.

- An ok result holds a value.
- An error result holds a message string.

Sprout has no exceptions.
A function that can fail returns a result.
The caller decides how to handle failure.

Build a result with one of two functions.

```sprout
let good = ok(42)
let bad = err("the value is out of range")
```

Print a result to see its state.

```text
ok(42)
err("the value is out of range")
```

The `type` function reports `result` for both states.

## Reading a result

Four functions work with results.

| Function | Returns |
|----------|---------|
| `is_ok(r)` | true when r is an ok result. |
| `is_err(r)` | true when r is an error result. |
| `unwrap(r)` | The ok value. Stops on an error. |
| `unwrap_or(r, fallback)` | The ok value, or fallback on an error. |

```sprout
print(is_ok(ok(1)))        // true
print(unwrap(ok(1)))       // 1
print(unwrap_or(err("x"), 0))   // 0
```

`unwrap` stops the program on an error result.
Its message becomes the runtime error.

```sprout
print(unwrap(err("oops")))
```

```text
error: oops
```

## Pattern matching

A `match` expression picks one arm by the shape of a value.
It evaluates the subject once, then tries each arm in order.
The first arm whose pattern matches runs.
Its block value is the match value.

```sprout
let r = ok(7)

print(match r {
    ok(v) => { v * 2 },
    err(e) => { 0 },
    _ => { -1 },
})
```

```text
14
```

A match needs four parts.

- The `match` keyword.
- A subject expression.
- A list of arms in braces.
- A comma after every arm but the last.

## Patterns

A pattern describes the shape a value must have.

| Pattern | Matches |
|---------|---------|
| `_` | Any value. Binds nothing. |
| `x` | Any value. Binds the name `x`. |
| `1`, `"hi"`, `true`, `nil` | A value equal to the literal. |
| `ok(p)` | An ok result. Tests the inner value with `p`. |
| `err(p)` | An error result. Tests the message with `p`. |

A pattern binds at most one name.
The bound name lives in the arm block only.

```sprout
let n = 3

print(match n {
    1 => { "one" },
    2 => { "two" },
    x => { "the number " + str(x) },
})
```

```text
the number 3
```

## Catch-all arms

A match must end with a catch-all arm.
A catch-all pattern is `_` or a plain name.
It covers any value the earlier arms do not match.
The checker reports a match without one.

```sprout
match r {
    ok(v) => { print(v) },
    err(e) => { print(e) },
}
```

```text
error: match must end with a catch-all arm that uses '_' or a variable name
```

## Nested patterns

A result pattern may hold another pattern.
The chain can test any number of result levels.

```sprout
let nested = ok(err("deep"))

print(match nested {
    ok(err(m)) => { "nested error: " + m },
    ok(v) => { "shallow" },
    err(m) => { "error: " + m },
    _ => { "unknown" },
})
```

```text
nested error: deep
```

Literal tests use the same equality as `==`.
An integer pattern matches a float with the same value.

## Match as a value

A match is an expression.
Use it anywhere an expression fits.

```sprout
let label = match score {
    5 => { "five" },
    _ => { "other" },
}

let total = 0
for i in range(0, 4) {
    total = total + match i {
        0 => { 10 },
        _ => { i },
    }
}
```

The value of an arm is the value of its last expression statement.
An arm that ends in any other statement is worth nil.

## Example program

Run the full demo.

```text
sprout run examples/results.spr
sprout vm examples/results.spr
```

Both engines print the same output.
