# Modules

Sprout 0.3 adds a module system and a build tool.
A module is one source file.
It shares nothing with other files unless you import them.

Read `docs/grammar.md` for the language reference.
Read `docs/bytecode.md` for the virtual machine.

## Exports

Use `export` to make a top-level name public.
Public names are visible to importing files.
Other names stay private to the file.

```sprout
// lib/greetings.spr
export fn greet(name) {
    return "hello, " + name
}

export const punc = "!"

let hidden = 42   // not exported
```

You can export a `let`, a `const`, or a `fn`.
The `export` keyword only works at the top level of a file.

## Imports

Use `import` to load a module.
The path is relative to the importing file.
The path may omit the `.spr` extension.

The plain form binds every exported name.

```sprout
import "lib/greetings.spr"

print(greet("world") + punc)
```

The `from` form binds the whole module under one name.
Read an export with brackets.

```sprout
import calc from "lib/calc.spr"

print(calc["add"](2, 3))
```

A module is initialized exactly once.
Every file that imports the same module shares one copy.
Imports are hoisted, so a file can use an imported name before the import line.

## Resolution

The loader finds each import relative to the importing file.
It reports these problems before the program runs.

- A missing file: `cannot find module 'name'`.
- An import cycle: `import cycle: a -> b -> a`.
- The same module imported twice in one file: `duplicate import`.

Modules do not see each other's private names.
They cannot see the globals of the entry file.

## Build tool

The `build` command makes one self-contained file.

```text
sprout build examples/deep.spr -o dist/deep.spr
```

The output file contains every module.
It runs anywhere without the source files.

```text
sprout run dist/deep.spr
sprout vm  dist/deep.spr
```

Without `-o`, the output sits next to the input.
The name gets `.bundle.spr` added.
For `examples/deep.spr`, the output is `examples/deep.bundle.spr`.

The bundle turns each module into a function.
An import becomes a call to that function.
The generated names `__module_<n>` and `__m_<n>` are reserved.
Do not declare them in source files that you build.

## Module graph

The `modules` command shows the files of a program.

```text
sprout modules examples/deep.spr
```

It lists each file, its imports, and its exports.
Use it to inspect a program before you build it.

## Try it

Run the module examples.

```text
sprout run examples/greeting.spr
sprout vm  examples/modules.spr
sprout run examples/deep.spr
```

Run the module tests.

```text
go test ./test/ -run Modules
```

## Limitations

- Modules cannot run before the checker approves the graph.
- The bundler uses reserved names for generated code.
- A module map is a plain map value, so its exports stay mutable.
- An import binds a copy of the value, not a live reference.
  A later change inside the module does not reach the importer.
  Containers such as lists and maps stay shared because they are pointers.
