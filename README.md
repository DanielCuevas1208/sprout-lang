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
- A module system with `import` and `export`.
- A build tool that makes one self-contained file.
- A bytecode compiler and a stack-based virtual machine.
- A tree-walking interpreter that shares the runtime with the VM.
- A disassembler for the compiled instruction stream.
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

## Modules

Split a program into files with `import` and `export`.
An export makes a top-level name public.
An import binds those names into the current file.

```sprout
// lib/greetings.spr
export fn greet(name) {
    return "hello, " + name
}
```

```sprout
// main.spr
import "lib/greetings.spr"

print(greet("world"))
```

Use the `from` form to keep a module under one name.

```sprout
import calc from "lib/calc.spr"

print(calc["add"](2, 3))
```

The loader resolves paths relative to the importing file.
It reports missing modules, import cycles, and duplicate imports.
The `modules` command shows the graph of a program.

```text
sprout modules examples/deep.spr
```

Read `docs/modules.md` for the full module guide.

## Build tool

The `build` command bundles a program into one file.

```text
sprout build examples/deep.spr -o dist/deep.spr
```

The output is self-contained.
It runs anywhere without the source modules.

```text
sprout run dist/deep.spr
sprout vm  dist/deep.spr
```

Without `-o`, the output sits next to the input.
The name gets `.bundle.spr` added.

## Command line

| Command | Purpose |
|---------|---------|
| `sprout run file.spr` | Runs a program on the interpreter. |
| `sprout vm file.spr` | Runs a program on the bytecode VM. |
| `sprout build file.spr` | Bundles a program and its imports. |
| `sprout modules file.spr` | Shows the module graph. |
| `sprout dis file.spr` | Shows the compiled bytecode. |
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
Both engines report runtime errors in this format.
Errors inside a module point at the module file.

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
- `greeting.spr` imports a module.
- `modules.spr` imports a module under a name.
- `deep.spr` imports a module that imports another module.

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
internal/module  module loading and bundling
internal/runtime value semantics and the standard library
internal/interp  the tree-walking interpreter
internal/code    opcodes and the instruction stream
internal/compiler  bytecode compiler
internal/vm      stack-based virtual machine
internal/diag    diagnostics and rendering
internal/repl    the interactive session
```

Each stage is independent.
The parser feeds the checker and the compiler.
The runtime is the single source of truth for both engines.
Adding a feature means updating the runtime, then both engines stay in step.

The module loader sits between the checker and the engines.
It resolves imports into one bundle.
Both engines run a bundle in the same order.

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
| `internal/module` | Resolution, exports, cycles, and bundling. |
| `internal/runtime` | Arithmetic, comparison, and indexing. |
| `internal/interp` | Evaluation, closures, and runtime errors. |
| `internal/code` | Opcodes, the builder, and disassembly. |
| `internal/compiler` | Bytecode for expressions and control flow. |
| `internal/vm` | Execution, closures, and runtime errors. |
| `internal/diag` | Diagnostic rendering. |
| `test` | Example goldens, engine parity, and command line. |

The parity tests run each program on both engines.
The VM tests match the same goldens as the interpreter.
Module programs must also agree on both engines.
Tests use only the standard library. They need no network or secrets.

## Roadmap

Version 0.2 is complete. It adds the bytecode virtual machine.
It keeps the same parser and checker.

Version 0.3 is complete. It adds the module system and the build tool.
A program can split across files and bundle back into one.

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
- A bundled program uses the reserved names `__module_<n>` and `__m_<n>`.

## License

Sprout is MIT licensed. See `LICENSE`.
