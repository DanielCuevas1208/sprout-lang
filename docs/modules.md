# Modules

Sprout 0.3 adds a module system. A module is a Sprout source file. It shares
functions and values with other programs. The `import` expression loads a
module and returns its namespace.

## Import syntax

Write `import` followed by a string path.

```sprout
let text = import "lib/strings.spr"
```

The path is relative to the importing file. Bind the result to a name. Use
that name to read the module members.

```sprout
print(text.shout("hello"))
```

Use brackets for the same read.

```sprout
print(text["shout"]("hello"))
```

## Exports

Every top-level declaration becomes an export. `let`, `const`, and `fn`
statements are all exported. Builtin functions are not exported.

This module exports `double` and `answer`.

```sprout
let answer = 42

fn double(x) {
    return x * 2
}
```

## Module behavior

A module runs once. The loader caches it by resolved path. A second import
of the same path returns the same module.

A module may import other modules. Its nested modules are also exports. This
is useful for libraries that group several files.

The loader detects circular imports. It stops with a clear error when module
A imports module B and module B imports module A.

## Member access

Use the dot operator to read a member. Use it only for reading. A module is
read-only. Assignment to a module member is rejected.

```sprout
let m = import "lib/math.spr"
print(m.answer)      // reads the export
m.answer = 5         // error: modules are read-only
```

## The build tool

The `sprout build` command copies a program and its modules into a folder.
The output tree matches the source tree. The built program runs as-is.

```text
sprout build examples/imports.spr -o app
sprout run app/imports.spr
```

The `build` folder is the default output. Use `-o` to set another folder.

## Limitations

- A module must be a local file. There is no package registry.
- Map-style keys on a module are read-only.
- A module runs once per process. It does not reload on change.
- Imports resolve at runtime, not at compile time.
