# Modules

Modules split a program across several files.
This release adds them to Sprout 0.3.
Read `docs/grammar.md` for the syntax.
Read `docs/bytecode.md` for how the VM runs them.

## The import statement

The `import` statement loads a module.
It binds the module to a name.

```sprout
import "lib/math"
```

The module name comes from the path.
Here the name is `math`.

Use `as` for a shorter or clearer name.

```sprout
import "lib/math" as m
print(m.square(5))
```

Each import binds one name.
Two modules with the same name need `as`.

## Module resolution

The loader finds a module from its specifier.
A specifier has no file extension.

```text
import "lib/math"
```

The loader tries these files in order.

```text
lib/math
lib/math.spr
lib/math/main.spr
```

A relative specifier starts with a dot.
It resolves against the importing file.

```sprout
import "../shared/util"
```

Other specifiers resolve against the importing file.
They also resolve against the entry directory.
Set `SPROUT_PATH` to add more search directories.

A module loads once per run.
Two files may import the same module.

## Exports

A module exports its top-level declarations.
This means every `let`, `const`, and `fn` at the top level.

```sprout
// lib/math.spr
fn square(n) {
    return n * n
}

let version = 1
```

Another file reads these exports with a dot.

```sprout
import "lib/math" as m
print(m.square(4))   // 16
print(m.version)     // 1
```

An import alias is private to the module.
It never becomes an export.

## Member access

The dot reads one member of a module.

```sprout
m.square(4)
```

Modules are read-only.
You cannot assign to a member.

A missing member is a compile error.
The checker reports it before the program runs.

```text
error: module 'math' has no member 'nope'
```

## Import cycles

A cycle is an error.
Module `a` cannot import module `b` if `b` imports `a`.

The loader reports the cycle path.

```text
error: import cycle: a -> b -> a
```

## Run a program with modules

Run the interpreter on the entry file.

```text
sprout run app.spr
```

Run the bytecode VM on the same file.

```text
sprout vm app.spr
```

Both engines load the same modules.
They run each module once, in dependency order.

## The build tool

The `build` command compiles a program to a bundle.

```text
sprout build app.spr
```

The bundle is one file with all modules.
It has the `.sprc` extension.

Set the output path with `-o`.

```text
sprout build app.spr -o app.sprc
```

Run a bundle on the VM.

```text
sprout vm app.sprc
```

A bundle embeds the source text.
Runtime errors still show the file, line, and caret.

The `dis` command shows the compiled graph.
It prints every module and the entry function.

```text
sprout dis app.spr
```

## Limitations

- A module exports only its top-level names.
- Module specifiers use forward slashes.
- The build tool does not link a native binary.
- Modules do not support wildcard imports.
