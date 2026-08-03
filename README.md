# Sprout

Sprout is a small programming language that runs on Go.
It ships with a lexer, a Pratt parser, and two execution engines.
Version 0.3 adds a module system and a build tool.
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
- A module system that spans a program across files.
- A build tool that writes one portable bytecode bundle.
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
| `sprout build file.spr` | Writes a bytecode bundle. |
| `sprout dis file.spr` | Shows the compiled bytecode. |
| `sprout check file.spr` | Checks a program without running it. |
| `sprout repl` | Starts a session. |
| `sprout lex file.spr` | Shows the tokens. |
| `sprout parse file.spr` | Shows the syntax tree. |
| `sprout version` | Shows the version. |

Pass a file path with no command to run it.
Run `sprout help` to see the full usage.
Add `-color always` to force colored diagnostics.

`run` and `vm` also run `.sprc` bundles.
`build` writes a bundle with the `-o` option.

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
Both engines report runtime errors in this format.
An error inside a module points at that module's file.

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

## Modules

A program can span several files.
The `import` statement loads a module and binds it to a name.

```sprout
import "lib/math" as m
print(m.square(6))   // 36
```

A module exports its top-level declarations.
Read an export with a dot.

```sprout
let nums = [1, 2, 3, 4, 5]
print(stats.mean(nums))      // 3.0
print(stats.sum_of_squares(nums))   // 55
```

The loader resolves modules against the entry directory.
Set `SPROUT_PATH` to add search directories.
Import cycles and missing modules are compile errors.

The `examples/modules.spr` program imports two libraries.
Run it on either engine.

```text
sprout run examples/modules.spr
sprout vm examples/modules.spr
```

See `docs/modules.md` for the full reference.

## Build tool

The `build` command compiles a whole module graph to a bundle.

```text
sprout build app.spr
```

The bundle is one file with every module.
It embeds the source text for diagnostics.
Run the bundle on the VM.

```text
sprout vm app.sprc
```

A bundle is deterministic.
The same program always produces the same bytes.

## Bytecode virtual machine

The compiler turns a syntax tree into bytecode.
The VM executes that bytecode with an operand stack and call frames.
Both engines share the runtime, so they behave identically.

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
- `modules.spr` imports two libraries.
- `lib/math.spr` is a reusable number library.
- `lib/stats.spr` is a reusable statistics library.

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
internal/checker static analysis
internal/modules module loader and dependency graph
internal/runtime value semantics and the standard library
internal/interp  the tree-walking interpreter
internal/code    opcodes and the instruction stream
internal/compiler  bytecode compiler
internal/vm      stack-based virtual machine
internal/runner  the end-to-end pipeline driver
internal/bundle  bytecode serialization
internal/diag    diagnostics and rendering
internal/repl    the interactive session
```

Each stage is independent.
The parser feeds the checker and the compiler.
The runtime is the single source of truth for both engines.
The runner loads a module graph, then drives either engine.
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
| `internal/parser` | Precedence, statements, imports, and recovery. |
| `internal/checker` | Scope, imports, members, and static errors. |
| `internal/modules` | Resolution, cycles, and shared modules. |
| `internal/runtime` | Arithmetic, comparison, and indexing. |
| `internal/interp` | Evaluation, closures, modules, and runtime errors. |
| `internal/code` | Opcodes, the builder, and disassembly. |
| `internal/compiler` | Bytecode for expressions, control flow, and modules. |
| `internal/vm` | Execution, closures, modules, and runtime errors. |
| `internal/runner` | Engine parity on multi-file programs. |
| `internal/bundle` | Bytecode round trips and determinism. |
| `internal/diag` | Diagnostic rendering. |
| `test` | Example goldens, engine parity, and command line. |

The parity tests run each program on both engines.
The VM tests match the same goldens as the interpreter.
Tests use only the standard library. They need no network or secrets.

## Roadmap

Version 0.3 is complete. It adds the module system and the build tool.
It keeps the same parser and checker.

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
- A module exports only its top-level names.
- The build tool does not link a native binary.

## License

Sprout is MIT licensed. See `LICENSE`.
