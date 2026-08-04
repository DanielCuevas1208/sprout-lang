# Modules and the Build Tool

Sprout 0.3 adds file-based modules and a build tool.
A program can split into several files and still run as one project.
The build tool bundles those files into a single program.

## Imports and exports

A module is any Sprout file that exports names. Export a declaration with
the `export` keyword.

```sprout
// lib/math.spr
export let TAU = 6.283185
export fn double(x) {
    return x * 2
}

let hidden = 42      // private to this module
```

Another file imports the module and reads its names with a dot.

```sprout
// main.spr
import "math" as m
print(m.double(m.TAU))     // 12.56637
```

The import binds the base name of the path. The `as` clause renames it.

```sprout
import "lib/strings.spr" as text
```

## Resolution

Module resolution is file based. The search order depends on the specifier.

- A specifier that starts with `.` resolves relative to the importing file.
- A bare specifier searches the importing file's directory first.
- A bare specifier then searches the project's `lib` directories.
- A missing `.spr` suffix is added before each check.

Only top-level statements may import or export.
An export may only appear in a module file.
A module body runs exactly once, even when several files import it.
Two imports of one module share its state.

## The manifest

A project is a folder with a `sprout.toml` manifest.

```toml
name = "report"
version = "0.1.0"
entry = "main.spr"
lib = ["lib"]
```

The `entry` names the program that `sprout run` and `sprout build` execute.
The `lib` list names directories that bare specifiers search.
Create a new project with `sprout init`.

```text
sprout init hello
```

The command writes a manifest, a `main.spr`, and a `lib` directory.
Run the project from any folder inside it.

```text
cd hello
sprout run
sprout vm
```

Omit the file argument to use the entry named in the manifest.

## The build tool

The build tool turns a project into one self-contained file.
The bundle inlines every module and keeps its load-once behavior.

```text
sprout build out/report.spr
```

Without an argument, the bundle lands at `out/<name>.spr`.
The bundle runs on both engines and needs no external files.

```text
sprout run out/report.spr
sprout vm out/report.spr
```

## Diagnostics

A missing module points at the import.

```text
error: cannot find module 'ghost'
  --> main.spr:1:8
   |
 1 | import "ghost"
   |        ^
```

An import cycle names the files in order.

```text
error: import cycle: main -> a -> b -> a
  --> lib/b.spr:1:1
   |
 1 | import "a"
   | ^
```

A member that is not exported fails before the program runs.

```text
error: module 'math' has no exported member 'hidden'
  --> main.spr:2:11
   |
 2 | print(m.hidden)
   |           ^
```

## The module value

The `module` builtin wraps a map into a module value.

```sprout
let m = module("temp", {"x": 1})
print(m.x)     // 1
```

The build tool uses this function to reconstruct modules inside a bundle.
Programs can use it to build module values dynamically.
Member access on a value that is not a module stops at runtime.
