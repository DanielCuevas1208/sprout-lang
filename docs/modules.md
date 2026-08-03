# Modules and Builds

This guide covers the Sprout module system and the build tool.
Read `docs/grammar.md` first for the syntax concepts.

## Why modules

A single file grows until it is hard to read.
Modules split a program into files.
Each file is one module.
A module hides its private names and shows only its exports.

## Writing a module

Use `export` before a top-level `let`, `const`, or `fn`.
An exported name is public.
A name without `export` is private.

```sprout
export fn square(n) {
    return n * n
}

export const NAME = "sprout"

let hidden = 42
```

Only a top-level declaration can be exported.
The checker rejects `export` inside a block or function.
A module file is a normal Sprout file.

## Importing a module

Use `import` with a module path.

```sprout
import "./mathlib"

print(mathlib.square(5))
```

The bound name comes from the last path segment.
`import "./mathlib"` binds the name `mathlib`.
Drop the `.spr` suffix, or keep it.
Both forms work.

Use `as` to choose a different name.

```sprout
import "./mathlib" as m
print(m.square(5))
```

A path without `.spr` gets the suffix added.
A relative path resolves against the importing file.

## How loading works

A module runs once per process.
The first import loads it and runs its body.
Later imports reuse the loaded value.
So modules keep their state across files.

A module may import another module.
Imports may nest to any depth.
A module sees the exports of its own imports only.

An import cycle is an error.
The loader reports the chain.

```text
import cycle: a.spr -> b.spr -> a.spr
```

## Checking

The checker loads every imported module before a program runs.
It finds these problems.

- A module path that does not resolve.
- A parse or check error inside an imported module.
- A member that a module does not export.

```sprout
import "./greet"

print(greet.shout("hi"))   // error: module 'greet' does not export 'shout'
```

A member read on a non-module is also an error when the checker can prove it.

```sprout
let x = 5
print(x.size)              // error: cannot read a member of a int
```

## Module values

A module is a value.
`type` reports it as `module`.

```sprout
import "./lib"
print(type(lib))           // module
```

Members are read-only.
Assignment to a member is rejected.
A module value is not a map and has no builtin helpers.

## The build tool

The `build` command compiles a project into one artifact.

```text
sprout build main.spr
```

It writes `main.sprb` next to the source.
Use `-o` to choose the output path.

```text
sprout build main.spr -o dist/app.sprb
```

A build runs the parser, the checker, and the compiler.
Any error stops the build.
A failed build writes no artifact.

The artifact contains the compiled bytecode.
It keeps the source positions and the main source text.
Run it on the virtual machine.

```text
sprout run main.sprb
```

The `run` command detects the artifact format.
You can also run a `.spr` source file the usual way.
The artifact loads its modules from the source tree at run time.
Move the modules and the artifact breaks.
The artifact is not self-contained.

## Examples

Run the sample project in `examples/project`.

```text
sprout run examples/project/main.spr
sprout vm examples/project/main.spr
sprout build examples/project/main.spr
sprout run examples/project/main.sprb
```

The project has four files.
`main.spr` imports three modules.
`greet.spr` imports another module.
The build compiles the whole project.
