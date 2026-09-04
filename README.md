# Sprout

Sprout is a small programming language that runs on Go.
It ships with a lexer, a Pratt parser, and two execution engines.
Version 0.2 added a stack-based bytecode virtual machine.
Version 0.3 added file-based modules and a build tool.
Version 0.4 added structs, methods, and interfaces.
Version 0.5 added result types and pattern matching.
Version 0.6 added channels and tasks.
Version 0.7 added cooperative task cancellation.
Version 0.8 added channel selection.
Version 0.9 added scheduler controls.
Version 0.10 adds configurable scheduler policies.
The interpreter and the VM share one runtime and one standard library.
Sprout produces friendly diagnostics that point at the exact problem.

The goal is a language that is easy to learn and easy to read.
The design favors a short standard library over magic.
The codebase is structured to grow cleanly over time.

## Highlights

- A documented grammar with a line-aware Pratt parser.
- Structs with methods and interface contracts.
- Result values with `ok` and `err`.
- Blocking channels and joinable tasks for concurrent functions.
- Cooperative task cancellation that wakes blocked operations.
- Channel selection for deterministic fan-in programs.
- Scheduler controls for fair task handoff and cancellable pauses.
- A `match` expression with wildcard and variable patterns.
- A bytecode compiler and a stack-based virtual machine.
- A tree-walking interpreter that shares the runtime with the VM.
- A disassembler for the compiled instruction stream.
- A module system with imports, exports, and cycle checks.
- A build tool that bundles a project into one file.
- A deterministic source printer used by the bundler.
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
| `sprout dis file.spr` | Shows the compiled bytecode. |
| `sprout repl` | Starts a session. |
| `sprout lex file.spr` | Shows the tokens. |
| `sprout parse file.spr` | Shows the syntax tree. |
| `sprout check file.spr` | Checks without running. |
| `sprout init [dir]` | Creates a new project. |
| `sprout build [out.spr]` | Bundles a project into one file. |
| `sprout version` | Shows the version. |

Pass a file path with no command to run it.
Inside a project, omit the file for `run`, `vm`, `dis`, and `check`.
Those commands then use the entry named in `sprout.toml`.
Run `sprout help` to see the full usage.
Add `-color always` to force colored diagnostics.
Use `-scheduler fair` or `-scheduler direct` with `run` and `vm`.

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

## Structs, methods, and interfaces

Version 0.4 adds named types.
A struct groups related values into one value.
A method belongs to a struct type.
An interface lists the methods a struct must provide.

```sprout
struct Point {
    x
    y
}

fn Point.sum() {
    return self.x + self.y
}

let a = Point(3, 4)
let b = Point(x: 1, y: 2)
print(a.sum())   // 7
print(b.y)       // 2

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

Call a struct type to build an instance.
Use positional values or named values.
A method body reads its receiver through `self`.
The checker verifies interface satisfaction before a program runs.
See `docs/structs.md` for the full reference.

## Results and pattern matching

Version 0.5 added result types and pattern matching.
A result carries a value or an error message.
Sprout has no exceptions.
A function that can fail returns a result.

```sprout
fn safe_div(a, b) {
    if b == 0 {
        return err("division by zero")
    }
    return ok(a / b)
}

print(safe_div(10, 2))   // ok(5)
```

Use `match` to take a value apart.
Arms run in order. The first match wins.
The `_` pattern matches any value.

```sprout
let r = safe_div(10, 0)

print(match r {
    ok(v) => { "result: " + str(v) },
    err(m) => { "failed: " + m },
    _ => { "unknown" },
})
```

Match works on any value, not just results.
A pattern can bind one name, or test a literal.
A match must end with a catch-all arm.
See `docs/results.md` for the full reference.

## Concurrency

Version 0.6 added channels and tasks.
Version 0.7 added cooperative cancellation.
Version 0.8 added channel selection.
Version 0.9 added scheduler controls.
A channel moves values between spawned functions.
A task reports one spawned function.

```sprout
let jobs = channel()
let worker = spawn(fn() {
    send(jobs, 42)
    return "done"
})

print(unwrap(recv(jobs)))
print(unwrap(await(worker)))
close(jobs)
```

The example prints this output.

```text
42
done
```

A canceled task returns an error result.

```text
sprout run examples/cancellation.spr
cancelled
```

`channel()` creates an unbuffered channel.
Pass a non-negative integer to create a buffered channel.
`recv` returns `ok(value)`, or `err("channel closed")` after close.
`await` returns the worker value or its error as a result.
Use `close` after all sends finish.
Call `cancel(task)` to request cooperative cancellation.
Call `is_cancelled()` inside long-running tasks.

A selection waits on several channels.
It skips closed channels without buffered values.
It returns `ok({"index": i, "value": v})` for a value.
It returns `err("all channels closed")` when no channel remains.

```sprout
let left = channel()
let right = channel(1)
send(right, "ready")
let event = unwrap(select([left, right]))
print(event["index"], event["value"])
close(left)
close(right)
print(unwrap_or(select([left, right]), "all closed"))
```

The example prints this output.

```text
1 ready
all closed
```

Use `yield()` to give another task a scheduling opportunity.
Use `sleep(milliseconds)` to pause the current task without blocking its peers.
Both controls observe cooperative cancellation.

```sprout
let events = channel(1)
let worker = spawn(fn() {
    send(events, "started")
    yield()
    sleep(1)
    send(events, "finished")
    return "done"
})

print(unwrap(recv(events)))
print(unwrap(recv(events)))
print(unwrap(await(worker)))
close(events)
```

The example prints this output.

```text
started
finished
done
```

Choose a task handoff policy from the command line.

```text
sprout run -scheduler direct examples/scheduler.spr
```

`fair` is the default policy.
It calls the Go scheduler at `yield()`.
`direct` skips that explicit handoff.
Both policies keep cancellation checks.

See `docs/concurrency.md` for the full reference.

## Modules

A module is a separate Sprout file.
A file exports names with the `export` keyword.
Another file imports them with the `import` statement.

Save this file as `lib/math.spr`.

```sprout
export fn double(x) {
    return x * 2
}
```

Save this file as `main.spr`.

```sprout
import "math" as m
print(m.double(21))
```

The import binds the base name of the path.
The `as` clause renames the binding.
A module body runs exactly once, even when several files import it.
Two imports of one module share its state.

A project is a folder with a `sprout.toml` manifest.
Create one with `sprout init`.

```text
sprout init hello
```

The manifest names the entry program and the library directories.

```toml
name = "hello"
entry = "main.spr"
lib = ["lib"]
```

Bare imports search the project's `lib` directories.
A missing module, an import cycle, and an unknown member all fail early.
See `docs/modules.md` for the full reference.

## Build tool

The build tool turns a project into one self-contained file.

```text
sprout build
```

The bundle lands at `out/<name>.spr`.
It inlines every module and keeps load-once behavior.
It runs on both engines with no external files.

```text
sprout run out/hello.spr
sprout vm out/hello.spr
```

The `examples/project` folder is a complete module project.
Run it from inside the folder.

```text
cd examples/project
sprout run
```

## Bytecode virtual machine

Version 0.2 added the compiler and the stack-based VM.
The compiler turns a syntax tree into bytecode.
The VM executes that bytecode with an operand stack and call frames.
Both engines share the runtime, so they behave identically.
Version 0.3 runs modules on both engines.
Version 0.4 runs structs and methods on both engines.
Version 0.5 runs results and match on both engines.
Version 0.6 runs channels and tasks on both engines.
Version 0.7 runs cooperative cancellation on both engines.
Version 0.8 runs channel selection on both engines.
Version 0.9 runs scheduler controls on both engines.
Version 0.10 runs both scheduler policies through the same runtime interface.

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
Version 0.3 adds `module` for building module values.
Version 0.4 adds no new builtins. The `type` function reports structs.
Version 0.5 adds `ok`, `err`, `is_ok`, `is_err`, `unwrap`, and `unwrap_or`.
Version 0.6 adds `channel`, `send`, `recv`, `close`, `spawn`, and `await`.
Version 0.7 added `cancel` and `is_cancelled`.
Version 0.8 adds `select`.
Version 0.9 added `yield` and `sleep`.
Version 0.10 adds `fair` and `direct` scheduler policies.

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
- `structs.spr` uses structs, methods, and interfaces.
- `results.spr` uses results and pattern matching.
- `concurrency.spr` coordinates workers with channels and tasks.
- `cancellation.spr` stops a blocked task cooperatively.
- `select.spr` fans in values from several channels.
- `scheduler.spr` gives tasks explicit scheduling opportunities.
- `guess.spr` is an interactive game.
- `project` is a multi-file module project.

Each example has a golden output in `test/golden`.
Both engines must match the goldens.

## Architecture

The repository is a pipeline of small Go packages.

```text
cmd/sprout     command line interface
internal/source  source files and positions
internal/token   token definitions
internal/lexer   the scanner
internal/ast     the syntax tree and the source printer
internal/parser  the Pratt parser
internal/checker static analysis
internal/module  the module loader and the manifest
internal/build   the bundler
internal/object  runtime values, tasks, cancellation, selection, and modules
internal/runtime value semantics, standard library, and scheduler policies
internal/interp  the tree-walking interpreter
internal/code    opcodes and the instruction stream
internal/compiler  bytecode compiler
internal/vm      stack-based virtual machine
internal/diag    diagnostics and rendering
internal/repl    the interactive session
```

Each stage is independent.
The parser feeds the checker and the compiler.
The module loader turns a project into a graph of parsed files.
The runtime is the single source of truth for both engines.
A struct type, its methods, and its field access live in one place.
Channels and tasks live in the object and runtime packages.
Adding a feature means updating the runtime, then both engines stay in step.
The CLI creates one scheduler for the selected engine.
Child tasks inherit that scheduler.

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

CI checks formatting, module files, vet, builds, tests, and race safety.

| Suite | Scope |
|-------|-------|
| `internal/lexer` | Tokens, positions, and lexer errors. |
| `internal/parser` | Precedence, statements, and recovery. |
| `internal/checker` | Scope, imports, exports, structs, and static errors. |
| `internal/ast` | Source printer round trips. |
| `internal/module` | Loading, resolution, cycles, and manifests. |
| `internal/build` | Bundle output and reserved names. |
| `internal/runtime` | Arithmetic, comparison, indexing, members, results, concurrency, and scheduling. |
| `internal/interp` | Evaluation, closures, structs, and runtime errors. |
| `internal/code` | Opcodes, the builder, and disassembly. |
| `internal/compiler` | Bytecode for expressions, structs, and control flow. |
| `internal/vm` | Execution, closures, structs, and runtime errors. |
| `internal/diag` | Diagnostic rendering. |
| `internal/object` | Channels, tasks, cancellation, selection, and shared runtime values. |
| `test` | Example goldens, engine parity, cancellation, and command line. |

The parity tests run each program on both engines.
The VM tests match the same goldens as the interpreter.
Module tests run projects and bundles on both engines.
Concurrency tests cover blocking, close, cancellation, selection, scheduler policies, task completion, and engine parity.
Match programs run on both engines and must agree.
Tests use only the standard library. They need no network or secrets.
Scheduler tests inject one policy into both engines and child tasks.

## Roadmap

Version 0.2 is complete. It added the bytecode virtual machine.
It kept the same parser and checker.

Version 0.3 is complete. It added the module system and the build tool.
It runs modules on the interpreter and the VM.
It detects missing modules and import cycles.

Version 0.4 is complete. It added structs, methods, and interfaces.
Structs and methods run on both engines.
Interfaces are a static contract checked before a program runs.

Version 0.5 is complete. It added result types and pattern matching.
A function can return `ok` or `err`.
The `match` expression tests a value against patterns.
It binds pattern names in the arm that wins.
Results and match run on both engines.

Version 0.6 is complete. It added channels and joinable tasks.
Channels wake blocked operations when callers close them.
Both engines run the same concurrency examples.

Version 0.7 is complete. It added cooperative task cancellation.
Cancellation wakes blocked channel operations.
Both engines run the same cancellation examples.

Version 0.8 is complete. It added channel selection.
Selection waits on several channels and reports the selected index.
Both engines run the same selection examples.

Version 0.9 is complete. It added scheduler controls.
`yield()` gives another task a scheduling opportunity.
`sleep(milliseconds)` pauses one task and observes cancellation.
Both engines run the same scheduler examples.

Version 0.10 is complete. It added configurable scheduler policies.
The `fair` policy hands off at `yield()`.
The `direct` policy skips that explicit handoff.
The interpreter and VM share the policy interface.

Version 1.0 remains open for stronger task isolation and additional policies.

## Limitations

- Structs have no class inheritance. Methods are bound by name.
- Interface methods check names and arity only.
- Interface types cannot be exported from a module.
- Type annotations are optional and checked lightly.
- Map keys must be strings.
- Integer division truncates toward zero.
- There is no tail-call optimization.
- The standard library is small by design.
- The VM materializes a loop iterable before the loop starts.
- `break` and `continue` inside a closure are not supported.
- Module resolution is file based. There is no package registry.
- A bundle reprints module bodies. It does not compress them.
- A match arm is a block. It cannot be a bare expression.
- Cancellation is cooperative and has no timeout operation.
- `yield()` does not guarantee that another task runs immediately.
- The `direct` policy does not force a task handoff.
- `sleep()` uses wall-clock time, so wake order is not a timing contract.
- A spawned function shares captured mutable values with its parent.
- Do not mutate captured lists, maps, or structs from multiple tasks.
- A program must await tasks that it needs before it exits.
- A pattern binds at most one variable.
- Match patterns cover literals, names, and results only.

## License

Sprout is MIT licensed. See `LICENSE`.
