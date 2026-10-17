# Contributing to Sprout

Thank you for helping with Sprout.
This page explains how to build, test, and contribute code.

## The repository

The codebase is a pipeline of small Go packages.
Each package lives under `internal`.
The command line interface lives under `cmd/sprout`.
The `docs` folder holds the language documentation.

## Build the binary

You need Go 1.22 or newer.

```text
go build ./cmd/sprout
```

## Run the tests

Run the whole suite.

```text
go test ./...
```

Run one package.

```text
go test ./internal/parser/
```

Run the race detector.

```text
go test -race ./...
```

## Check the code

Format all files.

```text
gofmt -l .
```

Run the static checks.

```text
go vet ./...
```

## Add a feature

A new feature touches both engines.
Add the value semantics to the runtime package.
Add the syntax to the parser and the grammar doc.
Add tests for the checker, the compiler, and the VM.
Run the engine parity suite.

```text
go test ./test/ -run EnginesAgree
```

Keep the public API stable.
Update the README, the roadmap, and the relevant docs.

## Add a module

A module feature touches the module package.
The loader turns a project into a graph of parsed files.
Add tests for loading, resolution, and cycles.
Add parity tests that run the project on both engines.

## Style

Use `gofmt` formatting.
Write comments that explain the intent, not the code.
Keep public documentation in plain language.
Use active voice and short sentences.
Add a test for every fix.

## Commit messages

Use the imperative mood in the subject line.

```text
Add file-based module resolution
```

Keep the subject under 70 characters.
Describe the reason for the change in the body.

## Report a bug

Open an issue with a small example.
Include the command you ran and the output you saw.
Say which version of Sprout you use.

## Code of conduct

Be respectful and constructive in all discussions.
We aim for a welcoming community for everyone.
