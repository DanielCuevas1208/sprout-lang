# Contributing

Thank you for your interest in Sprout. This guide explains how to build and
test the project.

## Build

You need Go 1.22 or newer.

```text
go build ./cmd/sprout
```

## Test

Run the full test suite.

```text
go test ./...
```

Run one package.

```text
go test ./internal/vm/
```

## Format and vet

The CI pipeline checks formatting and vetting. Run both locally first.

```text
gofmt -l .
go vet ./...
```

Format your code before you open a pull request. Files with CRLF line
endings fail the formatting check.

## Project layout

Read `README.md` for the architecture. The repository is a pipeline of small
Go packages. Each package covers one stage of the language.

## Conventions

- Keep public APIs documented.
- Add a deterministic test for each behavior change.
- Run the golden tests after you change output.
- Keep both engines in step. The runtime is the single source of truth.

## Reporting issues

Report bugs and feature requests in the GitHub issue tracker. Include a
minimal example that shows the problem.
