# Sprout Tour

Sprout is a small, friendly programming language. This tour shows the core
features with runnable examples.

## Hello world

```sprout
let name = "world"
print("hello, " + name)
```

Save the code as `hello.spr`. Run it with `sprout run hello.spr`.

## Values and types

Sprout has thirteen value types.

- `int` holds a 64-bit integer.
- `float` holds a 64-bit number.
- `string` holds text.
- `bool` holds true or false.
- `nil` means no value.
- `list` holds an ordered collection.
- `map` holds keys and values. Keys are strings.
- `function` is a callable value.
- `range` is a sequence of integers.
- `struct` is an instance of a struct type.
- `struct type` is a struct declaration.
- `method` is a method with a bound receiver.
- `result` holds a value or an error message.

Use the `type` function to ask for the type of a value.

```sprout
print(type(42))        // int
print(type("hi"))      // string
print(type([1, 2]))    // list
```

## Variables

Use `let` for a variable. Use `const` for a fixed value.

```sprout
let count = 0
count = count + 1      // allowed

const pi = 3.14
pi = 3                  // error: pi is a constant
```

Names may carry an optional type annotation.

```sprout
let temperature: float = 19.5
```

## Numbers

Sprout splits integer math from float math. An operation on two integers
stays integer. An operation with a float becomes a float.

```sprout
print(7 / 2)            // 3   (integer division)
print(7.0 / 2)          // 3.5
print(2 ^ 10)           // 1024
print(1_000_000)        // 1000000
```

## Strings

The plus operator joins strings. A backtick string may span lines.

```sprout
let greeting = "hello"
print(greeting + " again")
print(upper(greeting))          // HELLO
print(join("-", ["a", "b"]))    // a-b
```

## Conditions

Only `nil` and `false` are falsy. Every other value is truthy.

```sprout
let score = 75
if score >= 90 {
    print("excellent")
} elif score >= 60 {
    print("pass")
} else {
    print("keep trying")
}
```

## Loops

Use `while` for a condition loop. Use `for` for a fixed count.

```sprout
let i = 0
while i < 3 {
    print(i)
    i = i + 1
}

for n in range(1, 4) {
    print(n)
}
```

A `for` loop can iterate lists, strings, and ranges.

```sprout
for ch in "ab" {
    print(ch)
}
```

Use `break` to stop a loop. Use `continue` to skip an iteration.

## Functions

Declare a function with `fn`. Call it with parentheses.

```sprout
fn add(a, b) {
    return a + b
}

print(add(2, 3))
```

A function is a value. Store it in a variable. Pass it to another function.

```sprout
let doubler = fn(x) {
    return x * 2
}

print(doubler(21))
```

## Closures

A nested function keeps access to the scope where it was created.

```sprout
fn make_counter() {
    let count = 0
    return fn() {
        count = count + 1
        return count
    }
}

let next = make_counter()
print(next())   // 1
print(next())   // 2
```

Each iteration of a `for` loop gets its own loop variable.

```sprout
let fns = []
for i in range(0, 3) {
    push(fns, fn() {
        return i
    })
}
print(fns[0](), fns[1](), fns[2]())   // 0 1 2
```

## Lists

Lists are ordered and mutable. Use brackets to read and write elements.

```sprout
let fruit = ["apple", "banana"]
push(fruit, "cherry")
print(fruit)            // [apple, banana, cherry]
print(fruit[0])         // apple
fruit[1] = "blueberry"
print(len(fruit))       // 3
```

## Maps

Maps keep insertion order. Reading a missing key returns `nil`.

```sprout
let scores = {"alice": 10, "bob": 20}
scores["carol"] = 30
print(scores["alice"])          // 10
print(scores["missing"])        // nil
print(keys(scores))             // [alice, bob, carol]
```

## Higher-order functions

The standard library has `map`, `filter`, and `fold`.

```sprout
let nums = [1, 2, 3, 4]
print(map(nums, fn(x) { return x * 2 }))
print(filter(nums, fn(x) { return x % 2 == 0 }))
print(fold(nums, 0, fn(acc, x) { return acc + x }))
```

## Structs and methods

A struct groups related values into one value.
Declare it with the `struct` keyword.

```sprout
struct Point {
    x
    y
}

let origin = Point(0, 0)
let labeled = Point(x: 1, y: 2)
print(origin)          // Point{x: 0, y: 0}
print(labeled.y)       // 2
```

A method belongs to a struct type.
The method body reads the receiver through `self`.

```sprout
fn Point.sum() {
    return self.x + self.y
}

print(labeled.sum())   // 3
```

## Interfaces

An interface lists the methods a struct must provide.
A struct satisfies it when it declares every method.

```sprout
interface Shape {
    area()
}

struct Square { side }

fn Square.area() {
    return self.side * self.side
}

fn report(s: Shape) {
    return "area " + str(s.area())
}

print(report(Square(side: 3)))   // area 9
```

Read `docs/structs.md` for the full reference.

## Results and pattern matching

A result carries a value or an error message.
Use `ok` to mark success. Use `err` to mark failure.

```sprout
fn safe_div(a, b) {
    if b == 0 {
        return err("division by zero")
    }
    return ok(a / b)
}

print(safe_div(10, 2))   // ok(5)
print(safe_div(1, 0))    // err("division by zero")
```

Use `match` to take a result apart.

```sprout
let r = safe_div(10, 2)

print(match r {
    ok(v) => { "result: " + str(v) },
    err(m) => { "failed: " + m },
    _ => { "unknown" },
})
```

The `_` pattern matches any value.
It is the catch-all arm that a match must end with.

Match any value, not just results.

```sprout
fn describe(n) {
    return match n {
        0 => { "zero" },
        1 => { "one" },
        x => { "the number " + str(x) },
    }
}

print(describe(1))       // one
print(describe(42))      // the number 42
```

Read `docs/results.md` for the full reference.
Read `docs/concurrency.md` for channels, tasks, and selection.

## Assertions

The `assert` function checks a condition. It stops the program when the
condition is false. It is useful for tests and examples.

```sprout
assert(2 + 2 == 4, "math still works")
```

## Modules

A module is a separate Sprout file. A program imports it and reads its
exported names with a dot.

Save `lib/math.spr`.

```sprout
export fn double(x) {
    return x * 2
}
```

Save `main.spr`.

```sprout
import "math" as m
print(m.double(21))     // 42
```

The import binds the base name of the path. Use `as` to rename it.
An import may only appear at the top level.
An export may only appear in a module file.
A module body runs once, even when several files import it.

A project is a folder with a `sprout.toml` manifest. Create one with
`sprout init`. Run it with `sprout run` from inside the project.

```toml
name = "my-project"
entry = "main.spr"
lib = ["lib"]
```

Bare imports search the project's `lib` directories.

## Interactive mode

Run `sprout repl` to start a session. The session keeps its state between
lines. Type `:quit` to leave. Type `:help` for help.

```text
sprout> let x = 6
sprout> x * 7
42
sprout> :quit
```

## Bytecode virtual machine

Sprout 0.2 added a stack-based bytecode virtual machine.
Sprout 0.3 runs modules on both engines.
Sprout 0.4 runs structs and methods on both engines.
Sprout 0.5 runs results and match on both engines.
Sprout 0.6 runs channels and tasks on both engines.
Sprout 0.7 runs cooperative cancellation on both engines.
Sprout 0.8 runs channel selection on both engines.
Sprout 0.9 runs scheduler controls on both engines.
It shares the parser, the checker, and the standard library with the
interpreter. The two engines produce the same results.

Run a program on the VM.

```text
sprout vm examples/fizzbuzz.spr
```

Show the compiled instructions of a program.

```text
sprout dis examples/hello.spr
```

Read `docs/bytecode.md` for the full VM reference.

## Scheduler controls

Use `yield()` to give another task a scheduling opportunity.
Use `sleep(milliseconds)` to pause the current task.
Both controls observe cooperative cancellation.

```sprout
let events = channel(1)
let worker = spawn(fn() {
    send(events, "ready")
    yield()
    sleep(1)
    send(events, "done")
})
print(unwrap(recv(events)))
print(unwrap(recv(events)))
print(unwrap(await(worker)))
close(events)
```

The program prints `ready`, `done`, and `nil`.
Read `docs/concurrency.md` for cancellation and scheduling details.

## Next steps

Read the standard library reference in `docs/stdlib.md`.
Read the structs and interfaces reference in `docs/structs.md`.
Read the results and pattern matching reference in `docs/results.md`.
Read the formal grammar in `docs/grammar.md`.
Read the bytecode virtual machine in `docs/bytecode.md`.
Read the module system in `docs/modules.md`.
Read concurrency and cancellation in `docs/concurrency.md`.
Run the example programs in the `examples` directory.
