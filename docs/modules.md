# Modules

This page describes the Sprout module system.
It covers imports, exports, and how the loader resolves files.
Modules let you split a program into reusable files.

## Overview

A module is a Sprout source file.
The `import` statement loads a module and binds it to a name.
The `export` keyword marks declarations as visible to importers.

A module runs once, in an isolated scope.
Its exported names form a read-only namespace.
Other files cannot see private names.

## Imports

The `import` statement takes a module path.

```sprout
import "lib/numbers"
```

The bound name comes from the file name.
The path `lib/numbers` binds the name `numbers`.
A missing `.spr` extension is added when needed.

Use `as` to choose a different name.

```sprout
import "lib/strings" as text
```

The path is relative to the file that contains the import.

## Exports

The `export` keyword marks a top-level declaration as public.

```sprout
export let answer = 42
export const pi = 3.14
export fn square(x) {
    return x * x
}
```

Only `let`, `const`, and `fn` declarations can be exported.
Imports and exports only appear at the top level of a file.
Private names still work inside the module.

## Using a module

An imported module is a value of type `module`.
Reach an export with index access, then call it like any function.

```sprout
import "lib/numbers"

print(numbers["square"](5))   // 25
print(numbers["answer"])      // 42
```

Exported functions share the module scope.
They can call the module private helpers.
They can read and write the module state.

## Example

Save two files in a `lib` directory.

```sprout
// lib/mathx.spr
export fn double(x) {
    return x * 2
}
```

```sprout
// main.spr
import "lib/mathx"

print(mathx["double"](21))    // 42
```

Run the main file.

```text
sprout run main.spr
```

## Loading rules

The loader follows a few simple rules.

- A module runs once per program.
  Later imports return the same value.
- Paths resolve against the importing file directory.
- Circular imports are an error.
- A missing or broken module is an error.
- The loader uses the same engine that runs the program.

These rules keep module behavior deterministic.
Both the interpreter and the VM follow them.

## Checking

The `sprout check` command validates the whole import graph.
It loads every imported module and checks it without running it.
A broken or missing module fails the check.

```text
sprout check main.spr
```

## Errors

Import errors point at the import statement.
The message names the module and the cause.

```text
error: circular import of "b.spr"
```

A parse or check error in a module reports the module path and line.
Run `sprout check` to see the full picture before running.

## Limitations

- A module value is read-only. Writes to it are an error.
- Import paths cannot reach outside the file system.
- A module with a circular dependency is rejected.
- Module state is shared by reference. Values like lists are not copied.
- There is no package index yet. Imports use file paths only.
