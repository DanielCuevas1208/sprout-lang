# Sprout

Sprout is a small programming language that runs on Go.
It ships with a lexer, a Pratt parser, and two execution engines.
Version 0.3 adds modules and a build tool.
The interpreter and the VM share one runtime and one standard library.
Sprout produces friendly diagnostics that point at the exact problem.

The goal is a language that is easy to learn and easy to read.
The design favors a short standard library over magic.
The codebase is structured to grow cleanly over time.

## Highlights

- A documented grammar with a line-aware Pratt parser.
- A bytecode compiler and a stack-based virtual machine.
- A tree-walking interpreter that shares the runtime with the VM.
- A disassembler for the compiled instruction stream.
- A module system with imports, exports, and cycle detection.
- A build tool that writes a runnable bytecode artifact.
- Source diagnostics with a gutter, line, and caret.
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

Run the same program on the bytecode virtual machine.

```text
./sprout vm examples/fizzbuzz.spr
```

Build a project into a bytecode artifact, then run it.

```text
./sprout build examples/project/main.spr
./sprout run examples/project/main.sprb
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
| `sprout run file.sprb` | Runs a bytecode artifact on the VM. |
| `sprout vm file.spr` | Runs a program on the bytecode VM. |
| `sprout dis file.spr` | Shows the compiled bytecode. |
| `sprout build file.spr` | Builds a bytecode artifact. |
| `sprout repl` | Starts a session. |
| `sprout lex file.spr` | Shows the tokens. |
| `sprout parse file.spr` | Shows the syntax tree. |
| `sprout check file.spr` | Checks without running. |
| `sprout version` | Shows the version. |

Pass a file path with no command to run it.
Run `sprout help` to see the full usage.
Add `-color always` to force colored diagnostics.

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
It also checks modules and the members read from them.
Both engines report runtime errors in this format.

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
A program can split into modules with `import` and `export`.
See `docs/grammar.md` for the formal grammar.

## Modules

Version 0.3 adds a module system.
A module is a `.spr` file.
The `export` keyword marks a name as public.
A module imports another module with `import`.

This is `greet.spr`.

```sprout
export fn hello(name) {
    return "hello, " + name
}

export const DEFAULT_NAME = "friend"

let private_count = 0
```

This is `main.spr`.

```sprout
import "./greet"

print(greet.hello("world"))
print(greet.DEFAULT_NAME)
```

Imports resolve relative to the importing file.
Use `as` to bind a module to a different name.

```sprout
import "./numbers" as math
print(math.square(5))
```

A module runs once, no matter how many files import it.
An import cycle is an error.
The checker validates imports and exported members before a program runs.
A missing member is a compile error.

```text
error: module 'greet' does not export 'shout'
  --> main.spr:3:7
   |
 3 | print(greet.shout("hi"))
   |       ^
```

Private names stay inside their module.
Module members are read-only.
See `docs/modules.md` for the full reference.

## Bytecode virtual machine

Version 0.2 added a compiler and a stack-based virtual machine.
The compiler turns a syntax tree into bytecode.
The VM executes that bytecode with an operand stack and call frames.
Both engines share the runtime, so they behave identically.
Version 0.3 extends the bytecode with import and member instructions.

The `dis` command shows the compiled instructions.

```text
$ sprout dis examples/hello.spr
== fn <main> ==
Params:
Slots:  1
0000  PUSH_CONST  ; examples/hello.spr:5:12  0  (world)
0003  SET_LOCAL  ; examples/hello.spr:5:1  0
0006  BUILTIN  ; examples/hello.spr:7:1  0  (print)
0009  PUSH_CONST  ; examples/hello.spr:7:7  1  (hello, )
0012  GET_LOCAL  ; examples/hello.spr:7:19  0
0015  ADD  ; examples/hello.spr:7:17
0016  CALL  ; examples/hello.spr:7:6  1
```

Each line shows the offset, the opcode, and the source position.
The engine parity tests prove the VM matches the interpreter.
See `docs/bytecode.md` for the full reference.

## Build tool

The `build` command compiles a project into a bytecode artifact.
The artifact ends in `.sprb`.
It runs on the virtual machine without re-parsing.

```text
$ sprout build examples/project/main.spr
built .../examples/project/main.sprb
$ sprout run examples/project/main.sprb
```

The build validates the whole project first.
It reports a parse, check, or compile error and writes nothing.
The artifact keeps source positions and the main source text.
It loads modules from the source tree at run time.
See `docs/modules.md` for the artifact format.

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
- `project/` is a multi-file module project.

Each example has a golden output in `test/golden`.
Both engines must match the goldens.

## Architecture

The repository is a pipeline of small Go packages.

```text
cmd/sprout     command line interface
internal/source  source files and positions
internal/token   token definitions
internal/lexer   the scanner
internal/ast     the syntax tree
internal/parser  the Pratt parser
internal/checker static analysis and module validation
internal/mod     module resolution and loading
internal/runtime value semantics and the standard library
internal/interp  the tree-walking interpreter
internal/code    opcodes and the instruction stream
internal/compiler  bytecode compiler
internal/vm      stack-based virtual machine
internal/codec   bytecode artifact serialization
internal/diag    diagnostics and rendering
internal/repl    the interactive session
```

Each stage is independent.
The parser feeds the checker and the compiler.
The runtime is the single source of truth for both engines.
Modules load once and share their values across files.
Adding a feature means updating the runtime, then both engines stay in step.

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
| `internal/parser` | Precedence, statements, modules, and recovery. |
| `internal/checker` | Scope, static errors, and module validation. |
| `internal/mod` | Resolution, cycles, and loading. |
| `internal/codec` | Artifact encoding and round trips. |
| `internal/runtime` | Arithmetic, comparison, and indexing. |
| `internal/interp` | Evaluation, closures, and runtime errors. |
| `internal/code` | Opcodes, the builder, and disassembly. |
| `internal/compiler` | Bytecode for expressions and control flow. |
| `internal/vm` | Execution, closures, and runtime errors. |
| `internal/diag` | Diagnostic rendering. |
| `test` | Example goldens, engine parity, and command line. |
| `test` | Module projects, builds, and artifacts. |

The parity tests run each program on both engines.
The VM tests match the same goldens as the interpreter.
Tests use only the standard library. They need no network or secrets.

## Roadmap

Version 0.3 is complete. It adds the module system and the build tool.
It keeps the same parser, checker, and bytecode VM.

Version 0.4 adds structs, methods, and interfaces.
Version 0.5 adds result types and pattern matching.
Version 0.6 adds concurrency with channels.

## Limitations

- The language has no classes or structs yet.
- Type annotations are optional and checked lightly.
- Map keys must be strings.
- Integer division truncates toward zero.
- There is no tail-call optimization.
- The standard library is small by design.
- The VM materializes a loop iterable before the loop starts.
- `break` and `continue` inside a closure are not supported.
- A bytecode artifact loads its modules from the source tree.
- An artifact is not self-contained, so it needs its modules to stay put.
- A module exports names with `export`. There is no wildcard import.

## License

Sprout is MIT licensed. See `LICENSE`.
