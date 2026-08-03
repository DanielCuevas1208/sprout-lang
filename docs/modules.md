# Modules

Sprout 0.3 adds a module system.
A module is a Sprout source file.
It shares its declarations with other programs.
The `import` expression loads a module and returns its namespace value.

## Import syntax

Write `import` followed by a string path.

```sprout
let text = import "lib/strings.spr"
```

The path is relative to the importing file.
Bind the result to a name.
Use that name to read the module members.

```sprout
print(text.shout("hello"))
```

Brackets read the same member.

```sprout
print(text["shout"]("hello"))
```

## Exports

Every top-level declaration becomes an export.
This rule covers `let`, `const`, and `fn` statements.
Builtin functions are not exports.

This module exports `double` and `answer`.

```sprout
let answer = 42

fn double(x) {
    return x * 2
}
```

A program reads them as members.

```sprout
let math = import "lib/math.spr"
print(math.answer)
print(math.double(4))
```

## Module behavior

A module runs once.
The loader caches each module by its resolved path.
A second import of the same path returns the cached module.

A module may import other modules.
Its nested modules are also exports.
This helps libraries group several files.

The loader detects circular imports.
It stops with a clear error when two modules import each other.

```sprout
let a = import "lib/a.spr"
// a imports b, and b imports a.
// The loader stops with a circular import error.
```

## Member access

The dot operator reads a member.
A module is read-only.
Assignment to a module member is rejected.

```sprout
let math = import "lib/math.spr"
print(math.answer)      // reads the export
math.answer = 5         // error: modules are read-only
```

## The build tool

The `sprout build` command copies a program into a folder.
It also copies every module the program imports.
The output tree matches the source tree.
The built program runs as-is.

```text
sprout build examples/imports.spr -o app
sprout run app/imports.spr
```

The `build` folder is the default output.
Use `-o` to set another folder.

## Limitations

- A module must be a local file.
- There is no package registry.
- A module runs once per process.
- It does not reload when the file changes.
- Imports resolve at runtime, not at check time.
