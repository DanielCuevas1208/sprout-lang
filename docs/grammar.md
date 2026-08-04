# Sprout Grammar

This document is the formal grammar of Sprout version 0.3.

## Notation

The grammar uses EBNF notation.

- `A | B` means A or B.
- `{ X }` means X repeated zero or more times.
- `[ X ]` means X is optional.
- Literal keywords appear in double quotes.

A newline ends a statement. The parser ignores newlines inside brackets.
Statements may share a line, but newlines separate them by default.

## Lexical structure

A source file is a sequence of tokens. The lexer skips whitespace and
comments.

```
comment       := "//" {any char except newline}
               | "/*" {any char} "*/"
string        := '"' {char | escape} '"'
               | '`' {any char except '`'} '`'
escape        := "\n" | "\t" | "\r" | "\0" | "\\" | '"' | "'"
               | "\x" hex_digit hex_digit
integer       := digit {digit | "_"}
float         := integer "." digit {digit | "_"} [exponent]
               | integer exponent
exponent      := ("e" | "E") ["+" | "-"] digit {digit}
identifier    := letter {letter | digit | "_"}
```

A `\x` escape writes one byte from two hex digits.
Numbers do not start or end with an underscore.
A floating-point literal needs a digit before the decimal point.

## Program structure

```
program       := statement*
statement     := import_stmt | let_decl | const_decl | fn_decl
               | export_stmt
               | if_stmt | while_stmt | for_stmt
               | return_stmt | break_stmt | continue_stmt
               | expr_stmt
```

## Imports and exports

An import loads another Sprout file and binds it to a name.
A module exposes names with `export`.

```
import_stmt   := "import" string ["as" identifier]
export_stmt   := "export" (let_decl | const_decl | fn_decl)
```

An import and an export may only appear at the top level.
An export may only appear in a module file, which is a file loaded by an
import. The binding name of an import is the base name of its path unless
the `as` clause renames it.

## Declarations

```
let_decl      := "let" identifier [":" type] "=" expr
const_decl    := "const" identifier [":" type] "=" expr
fn_decl       := "fn" identifier "(" params ")" block
params        := [ param {"," param} [","] ]
param         := identifier [":" type]
type          := identifier
```

The `fn` keyword also creates an anonymous function as an expression.

```
fn_expr       := "fn" "(" params ")" block
```

## Control flow

```
if_stmt       := "if" expr block {"elif" expr block} ["else" block]
while_stmt    := "while" expr block
for_stmt      := "for" identifier "in" expr block
return_stmt   := "return" [expr]
break_stmt    := "break"
continue_stmt := "continue"
block         := "{" statement* "}"
```

## Expressions

Operators bind from loosest to tightest. Assignment and power are
right-associative. All other binary operators are left-associative.

```
expr          := assign
assign        := or_expr ["=" assign]
or_expr       := and_expr {"or" and_expr}
and_expr      := equality {"and" equality}
equality      := comparison {("==" | "!=") comparison}
comparison    := term {("<" | "<=" | ">" | ">=") term}
term          := factor {("+" | "-") factor}
factor        := unary {("*" | "/" | "%") unary}
unary         := ("-" | "not") unary | power
power         := primary ["^" unary]
primary       := integer | float | string | "true" | "false" | "nil"
               | identifier | fn_expr
               | "(" expr ")"
               | list_lit | map_lit
               | call | index | member
```

The unary minus binds looser than power. So `-3 ^ 2` means `-(3 ^ 2)`.

```
list_lit      := "[" [ expr {"," expr} [","] ] "]"
map_lit       := "{" [ map_entry {"," map_entry} [","] ] "}"
map_entry     := expr ":" expr
call          := primary "(" [ expr {"," expr} [","] ] ")"
index         := primary "[" expr "]"
member        := primary "." identifier
```

A member access reads an exported name of a module value.

## Precedence table

| Level | Operators | Associativity |
|-------|-----------|---------------|
| 1     | `=`       | right         |
| 2     | `or`      | left          |
| 3     | `and`     | left          |
| 4     | `==` `!=` | left          |
| 5     | `<` `<=` `>` `>=` | left |
| 6     | `+` `-`   | left          |
| 7     | `*` `/` `%` | left        |
| 8     | `-` `not` (prefix) | right |
| 9     | `^`       | right         |
| 10    | call, index, member | left |

## Line continuation

A line may continue in three cases.

1. Inside brackets. `(`, `[`, and `{` suppress newlines.
2. After an operator. The next line may hold the operand.
3. After `=` or `in`.

A `return` value must stay on the same line as `return`.
