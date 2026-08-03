# Bytecode

This document describes the Sprout virtual machine.
It covers the instruction set, the compiler, and the runtime.
You need the syntax tree concepts from `grammar.md` to read this page.

## Pipeline

A program moves through four stages.

```text
source -> parser -> checker -> compiler -> VM
```

The parser builds a syntax tree.
The checker rejects bad programs.
The compiler turns the tree into instructions.
The VM executes those instructions.

The `sprout vm` command runs the full pipeline.
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

Each function also carries a constant pool.
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
| MAKE_ITER | none | Pop an iterable, push an iterator. |
| ITER_NEXT | target | Advance, jump when done. |
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

## Arithmetic

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

## Limitations

- The VM materializes a loop iterable before the loop starts.
- Stack depth is bounded by the call nesting of the program.
- Constant assignment is rejected by the compiler.
- The disassembler renders only compiled functions.

## Test the VM

```text
go test ./internal/vm/
```

Run the engine parity suite.

```text
go test ./test/ -run EnginesAgree
```
