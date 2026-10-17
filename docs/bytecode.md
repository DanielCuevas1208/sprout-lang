# Bytecode

This document describes the Sprout virtual machine.
It covers the compiler, the instruction set, and the runtime.
Read `docs/grammar.md` first for the syntax tree concepts.

## Pipeline

A program moves through five stages.

```text
source -> parser -> checker -> compiler -> VM
```

The parser builds a syntax tree.
The checker rejects bad programs.
The compiler turns the tree into instructions.
The VM executes those instructions.

The `sprout vm` command runs the whole pipeline.
The `sprout dis` command shows the compiled instructions.

## Bytecode format

A compiled function is a stream of bytes.
Each instruction starts with a one-byte opcode.
Operands follow the opcode.

Integer operands use big-endian encoding.
The format is stable across platforms.

The VM is a stack machine.
Every instruction operates on a shared operand stack.
Loops and conditionals use absolute jump targets.

Each function carries a constant pool.
Literals such as numbers and strings live in the pool.
Instructions reference the pool by index.

## Functions

A compiled function is a `code.Function`.
It records these fields.

- Name.
- File name.
- Parameter names.
- Number of environment slots.
- Instruction stream.
- Constant pool.
- Source position of each instruction.

The compiler stores a nested function in the enclosing pool.
A closure instruction captures the current environment.
The VM wraps a function and its environment into a closure value.

## Scope model

The compiler mirrors the interpreter scoping rules.
Each block becomes an environment in the VM.
Each environment is a list of slots.

An identifier resolves to a depth and a slot.
Depth counts the environments between use and definition.
Slot is the variable index in its environment.

Local instructions read the current environment.
Upvalue instructions read an enclosing environment.
The VM walks the parent chain to find the value.

A loop variable gets a fresh environment each iteration.
This matches the interpreter, so closures see their own copy.

## Instruction set

| Opcode | Operands | Effect |
|--------|----------|--------|
| PUSH_CONST | index | Push pool value. |
| PUSH_NIL | none | Push nil. |
| PUSH_TRUE | none | Push true. |
| PUSH_FALSE | none | Push false. |
| POP | none | Pop and discard. |
| DUP | none | Duplicate the top. |
| NEW_ENV | slots | Enter a block scope. |
| END_ENV | none | Leave the block scope. |
| GET_LOCAL | slot | Push a local value. |
| SET_LOCAL | slot | Store the top in a local. |
| GET_UP | depth, slot | Push an enclosing value. |
| SET_UP | depth, slot | Store in an enclosing slot. |
| BUILTIN | index | Push a standard function. |
| CLOSURE | index | Capture the current environment. |
| CALL | count | Call the top value. |
| RETURN | none | Return nil. |
| RETURN_VALUE | none | Return the top. |
| JUMP | target | Jump to target. |
| JUMP_IF_FALSE | target | Pop, jump when falsy. |
| JUMP_IF_TRUE | target | Pop, jump when truthy. |
| GET_INDEX | none | Pop index and container. |
| SET_INDEX | none | Pop value, index, and container. |
| BUILD_LIST | count | Build a list. |
| BUILD_MAP | count | Build a map. |
| IMPORT | module | Load a module and push its value. |
| MAKE_MODULE | name | Build a module from a map and a name. |
| MAKE_ITER | none | Pop an iterable, push an iterator. |
| ITER_NEXT | target | Advance, jump when done. |
| MAKE_STRUCT | name, fields | Build a struct type from field names. |
| ADD_METHOD | name | Register a method on a struct type. |
| GET_MEMBER | name | Push a field, method, or module member. |
| SET_MEMBER | name | Store a value in a struct field. |
| BUILD_STRUCT | count | Build a struct from named values. |
| TEST_RESULT | ok-flag, target | Unwrap a matching result, else jump. |
| NEG | none | Negate the top. |
| NOT | none | Invert the truthiness. |
| ADD | none | Add the top two values. |
| SUB | none | Subtract. |
| MUL | none | Multiply. |
| DIV | none | Divide. |
| MOD | none | Take the remainder. |
| POW | none | Raise to a power. |
| EQ | none | Test equality. |
| NEQ | none | Test inequality. |
| LT | none | Test less than. |
| LE | none | Test less or equal. |
| GT | none | Test greater than. |
| GE | none | Test greater or equal. |

## Modules

An import compiles to `IMPORT` with the module index.
The operand names the module in the program's module table.
The VM runs the module body once and caches the result.
Later imports reuse the cached module value.
A module that imports itself is an import cycle and fails.

```text
0000  IMPORT  ; main.spr:1:1  0  (math)
0003  SET_LOCAL  ; main.spr:1:19  0
```

A member read compiles to `GET_MEMBER` with the member name.
A struct field read and a module export read use the same instruction.
The `dis` command lists every module below the main function.

## Structs

A struct declaration compiles to `MAKE_STRUCT`.
The field names are pushed first, then the instruction builds the type.
A method declaration compiles to `CLOSURE` and `ADD_METHOD`.
The method reserves slot zero for the `self` receiver.

A struct literal with positional values is a normal call.
The VM constructs the instance when it calls the type value.
A named struct literal compiles to `BUILD_STRUCT`.

```text
0000  PUSH_CONST  ; test.spr:2:8  0  (x)
0003  PUSH_CONST  ; test.spr:2:8  1  (y)
0006  MAKE_STRUCT  ; test.spr:2:1  2  (Point) fields 2
0011  SET_LOCAL  ; test.spr:2:8  1
0014  GET_LOCAL  ; test.spr:6:4  1
0017  CLOSURE  ; test.spr:6:1  4  (Point.sum)
0020  ADD_METHOD  ; test.spr:6:10  3  (sum)
```

A method call reads the method with `GET_MEMBER`.
The VM binds the receiver when it calls the method value.

## Short-circuiting

`and` and `or` evaluate lazily.
The compiler emits a duplicate and a conditional jump.
The right operand runs only when needed.

```text
print(0 and 1)   // prints 0
print(1 or 2)    // prints 1
```

## Pattern matching

A match expression compiles into a subject push and an arm chain.
Each arm starts by duplicating the subject.
Its pattern test either unwraps the value or jumps to the next arm.

A result pattern compiles to `TEST_RESULT`.
The instruction reads the ok-flag and the failure target.
A matching result is unwrapped and stays on the stack.
Any other value jumps to the target.

A literal pattern duplicates the value and compares it.
The comparison is `==`; an integer matches an equal float.

```text
0000  GET_LOCAL  ; test.spr:2:13  0
0003  DUP  ; test.spr:3:5
0004  TEST_RESULT  ; test.spr:3:5  ok -> 0032
0008  NEW_ENV  ; test.spr:3:14  1
0011  SET_LOCAL  ; test.spr:3:5  0
0014  POP  ; test.spr:3:5
0015  GET_LOCAL  ; test.spr:3:16  0
0018  PUSH_CONST  ; test.spr:3:20  1  (2)
0021  MUL  ; test.spr:3:18
0022  END_ENV  ; test.spr:3:14
0023  JUMP  ; test.spr:3:14  -> 0068
0026  DUP  ; test.spr:4:5
0027  TEST_RESULT  ; test.spr:4:5  err -> 0046
```

Each arm body runs in its own environment.
Pattern variables bind in that environment.
The arm body leaves its value on the stack.

## Shared runtime

The VM does not implement arithmetic itself.
It delegates to the shared runtime package.
Both the interpreter and the VM call the same functions.
This guarantees identical results.

The runtime covers arithmetic, comparison, indexing, and iteration.
It also owns the standard library.
A builtin is a runtime value with a name and a Go function.

## Errors

The VM reports errors with a source position and a call stack.
It uses the same diagnostic format as the interpreter.
A runtime error prints the message, the position, and the frames.

## Try it

Compile and run a program on the VM.

```text
sprout vm examples/fibonacci.spr
```

Show the compiled instructions.

```text
sprout dis examples/hello.spr
```

Run the VM unit tests.

```text
go test ./internal/vm/
```

Run the engine parity suite.

```text
go test ./test/ -run EnginesAgree
```

## Limitations

- The VM materializes a loop iterable before the loop starts.
- Stack depth is bounded by the call nesting of the program.
- Constant assignment is rejected by the compiler.
- `break` and `continue` inside a closure are not supported.
- A function call carries at most 255 arguments.
