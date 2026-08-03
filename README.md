# Sprout

Sprout is a small programming language that runs on Go.
It ships with a lexer, a Pratt parser, a checker, and two execution engines.
The engines are a tree-walking interpreter and a bytecode virtual machine.
Both engines share one value model and one standard library.
Sprout produces friendly diagnostics that point at the exact problem.

The goal is a language that is easy to learn and easy to read.
The design favors a short standard library over magic.
The bytecode pipeline is a foundation for a faster runtime later.

## Highlights

- A documented grammar with a line-aware Pratt parser.
- Source diagnostics with a gutter, line, and caret.
- A bytecode virtual machine with a documented instruction set.
- A tree-walking interpreter for teaching and debugging.
- A shared runtime that keeps both engines consistent.
- A small standard library for real example programs.
- A REPL for interactive experiments.
- Deterministic tests for every stage of the pipeline.

## Quick start

You need Go 1.22 or newer.

```text
go build ./cmd/sprout
```

Run an example program on the interpreter.

```text
./sprout run examples/fizzbuzz.spr
```

Run the same program on the bytecode VM.

```text
./sprout vm examples/fizzbuzz.spr
```

Start an interactive session.

```text
./sprout repl
```

On Windows the binary is `sprout.exe`.

## Try it

Save this file as `hello.spr`.

```sprout
let name = "world"
print("hello, " + name)
```

Run it.

```text
sprout run hello.spr
```

You see this output.

```text
hello, world
```

Run the tour in `docs/tour.md` for a full walkthrough.

## Command line

| Command | Purpose |
|---------|---------|
| `sprout run file.spr` | Runs a program on the interpreter. |
| `sprout vm file.spr` | Runs a program on the bytecode VM. |
| `sprout dis file.spr` | Shows the compiled bytecode. |
| `sprout repl` | Starts a session. |
| `sprout lex file.spr` | Shows the tokens. |
| `sprout parse file.spr` | Shows the syntax tree. |
| `sprout check file.spr` | Checks without running. |
| `sprout version` | Shows the version. |

Pass a file path with no command to run it.
Run `sprout help` to see the full usage.
Add `-color always` to force colored diagnostics.

## Two engines, one behavior

Sprout has two ways to run a program.
`run` uses the tree-walking interpreter.
`vm` compiles the program to bytecode and runs it on a stack machine.

The two engines share a package called `runtime`.
It holds arithmetic, comparison, indexing, iteration, and the standard library.
Because both engines use it, they produce identical output.

Each example program has a golden output.
The test suite runs every example on both engines.
The suite fails if the engines ever disagree.

See `docs/bytecode.md` for the instruction set and the compiler design.

## Diagnostics

Sprout reports errors with context. A compile error looks like this.

```text
error: undefined name 'y'
  --> demo.spr:2:7
   |
 2 | print(y)
   |       ^
```

A runtime error includes the call stack.

```text
error: cannot divide by zero
  --> demo.spr:3:9
   |
 3 |     return n / 0
   |             ^
   |
   at inner (demo.spr:5:12)
   at outer (demo.spr:9:5)
```

The checker finds problems before the program runs.
It reports undefined names, bad constants, and misplaced control flow.

## Language at a glance

Sprout has integers, floats, strings, booleans, and nil.
Lists and maps are first-class and mutable.
Functions are values. They capture their scope as closures.

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

Only `nil` and `false` are falsy.
An expression ends at a newline unless it is inside brackets.
See `docs/grammar.md` for the formal grammar.

## Standard library

The standard library is small and documented.
It covers output, conversion, lists, maps, strings, and numbers.
It also provides `assert` for tests and examples.

```sprout
let nums = [1, 2, 3, 4]
print(map(nums, fn(x) { return x * 2 }))
print(fold(nums, 0, fn(acc, x) { return acc + x }))
```

See `docs/stdlib.md` for the full reference.

## Examples

The `examples` directory holds documented programs.

- `hello.spr` prints a greeting.
- `fizzbuzz.spr` plays the classic game.
- `fibonacci.spr` uses recursion.
- `primes.spr` finds primes below 30.
- `counters.spr` shows closures.
- `collections.spr` works with lists and maps.
- `strings.spr` shows string functions.
- `math.spr` shows numbers and rounding.
- `higher_order.spr` uses map, filter, and fold.
- `guess.spr` is an interactive game.

Each example has a golden output in `test/golden`.

## Architecture

The repository is a small pipeline of Go packages.

```text
cmd/sprout     command line interface
internal/source  source files and positions
internal/token   token definitions
internal/lexer   the scanner
internal/ast     the syntax tree
internal/parser  the Pratt parser
internal/checker static analysis
internal/runtime shared value semantics and standard library
internal/object  runtime values
internal/interp  the tree-walking interpreter
internal/compiler  AST to bytecode compiler
internal/code    bytecode instructions and disassembly
internal/vm      the stack-based virtual machine
internal/diag    diagnostics and rendering
internal/repl    the interactive session
```

Each stage is independent.
The interpreter and the VM read from the same parser and checker.
The compiler turns the checked tree into instructions.
The VM executes those instructions on a stack.

## Development

Format the code.

```text
gofmt -l .
```

Check the code.

```text
go vet ./...
```

Run all tests.

```text
go test ./...
```

Run a single package.

```text
go test ./internal/vm/
```

## Test status

All tests pass on Go 1.22 and newer.

| Suite | Scope |
|-------|-------|
| `internal/lexer` | Tokens, positions, and lexer errors. |
| `internal/parser` | Precedence, statements, and recovery. |
| `internal/checker` | Scope and static errors. |
| `internal/interp` | Evaluation, closures, and runtime errors. |
| `internal/runtime` | Shared value semantics and builtins. |
| `internal/compiler` | Bytecode emission and patching. |
| `internal/code` | Instruction encoding and disassembly. |
| `internal/vm` | Bytecode execution and stack traces. |
| `internal/diag` | Diagnostic rendering. |
| `test` | Example goldens, engine parity, and command line. |

The suite runs every example on both engines.
It proves the interpreter and the VM produce identical output.
Tests use only the standard library. They need no network or secrets.

## Roadmap

Version 0.2 is complete.
It added the bytecode VM, the compiler, and the shared runtime.

| Version | Status | Work |
|---------|--------|------|
| 0.1 | Complete | Parser, checker, interpreter, diagnostics. |
| 0.2 | Complete | Bytecode VM, compiler, shared runtime. |
| 0.3 | Next | Module system and a build tool. |
| 0.4 | Planned | Structs, methods, and interfaces. |
| 0.5 | Planned | Result types and pattern matching. |
| 0.6 | Planned | Concurrency with channels. |

## Limitations

- The language has no classes or structs yet.
- Type annotations are optional and checked lightly.
- Map keys must be strings.
- Integer division truncates toward zero.
- There is no tail-call optimization.
- The VM materializes iterations as sequences.
- The standard library is small by design.

## License

Sprout is MIT licensed. See `LICENSE`.
