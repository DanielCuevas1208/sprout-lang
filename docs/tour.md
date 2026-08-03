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

Sprout has nine value types.

- `int` holds a 64-bit integer.
- `float` holds a 64-bit number.
- `string` holds text.
- `bool` holds true or false.
- `nil` means no value.
- `list` holds an ordered collection.
- `map` holds keys and values. Keys are strings.
- `function` is a callable value.
- `range` is a sequence of integers.

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

## Assertions

The `assert` function checks a condition. It stops the program when the
condition is false. It is useful for tests and examples.

```sprout
assert(2 + 2 == 4, "math still works")
```

## Modules

Split a program into files with modules.
A module exports names with `export`.
Import it with `import`.

```sprout
// greet.spr
export fn hello(name) {
    return "hello, " + name
}

// main.spr
import "./greet"
print(greet.hello("world"))
```

A relative path resolves against the importing file.
Use `as` to bind a different name.

```sprout
import "./numbers" as math
print(math.square(5))
```

A module runs once. Its private names stay hidden.
Read `docs/modules.md` for the full reference.

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

Sprout 0.2 adds a stack-based bytecode virtual machine.
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

## Builds

Compile a project into a bytecode artifact.

```text
sprout build examples/project/main.spr
```

Run the artifact on the virtual machine.

```text
sprout run examples/project/main.sprb
```

The build checks the whole project first.
Read `docs/modules.md` for the build reference.

## Next steps

Read the standard library reference in `docs/stdlib.md`.
Read the formal grammar in `docs/grammar.md`.
Read the bytecode virtual machine in `docs/bytecode.md`.
Read the module system and build tool in `docs/modules.md`.
Run the example programs in the `examples` directory.
