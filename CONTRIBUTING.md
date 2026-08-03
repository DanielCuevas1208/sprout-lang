# Contributing

Thank you for helping with Sprout.
This guide explains how the repository works.
It also explains how to make a change that fits.

## Prerequisites

You need Go 1.22 or newer.
You need a way to run the `git` command.

## The pipeline

Sprout is a pipeline of small Go packages.
Each stage has one job.

```text
lexer -> parser -> checker -> compiler -> VM
```

The interpreter and the VM share one runtime.
Change the runtime when you change value behavior.
Then both engines stay in step.

The module loader lives in `internal/module`.
It runs each engine kind with the same rules.
Add tests there when you change module behavior.

## Build and test

Build the command.

```text
go build ./cmd/sprout
```

Run all tests.

```text
go test ./...
```

Run the tests with the race detector.

```text
go test -race ./...
```

Run one package.

```text
go test ./internal/vm/
```

Check formatting.

```text
gofmt -l .
```

## Example programs

The `examples` folder holds runnable programs.
Each program needs a golden output file.
The golden file lives in `test/golden`.
Name it after the program.

Run an example with the interpreter.

```text
go run ./cmd/sprout run examples/hello.spr
```

Run it on the bytecode VM.

```text
go run ./cmd/sprout vm examples/hello.spr
```

Record the golden output when you add an example.
Both engines must match the golden.

## Code style

- Run `gofmt` before you commit.
- Keep functions small and focused.
- Write package comments in plain English.
- Prefer the standard library over new dependencies.
- Keep tests deterministic.
- Use no network in tests.

## Tests

Add a test for every new behavior.
Match the existing test style in the package.
Use table-driven tests when a case list fits.
Cover the interpreter and the VM for shared behavior.
Cover the loader for module behavior.

## Documentation

Update the docs when you change the language.
The grammar lives in `docs/grammar.md`.
The standard library lives in `docs/stdlib.md`.
The virtual machine lives in `docs/bytecode.md`.
The module system lives in `docs/modules.md`.
Update the README roadmap when a milestone ships.

Write public docs in plain English.
Use short sentences and active voice.
Keep instructions under 20 words.
Keep descriptive sentences under 25 words.
Do not use emojis in the docs.

## Commits

Write a clear commit message.
Explain what changed and why.
Keep each commit focused on one change.
Do not commit build output or secrets.
