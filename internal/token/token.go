// Package token defines the lexical tokens of the Sprout language.
package token

import (
	"fmt"
	"strconv"

	"github.com/sprout-lang/sprout/internal/source"
)

// Kind identifies the class of a token.
type Kind int

const (
	ILLEGAL Kind = iota
	EOF
	NEWLINE
	IDENT
	INT
	FLOAT
	STRING

	// Keywords.
	LET
	CONST
	FN
	IF
	ELIF
	ELSE
	FOR
	IN
	WHILE
	RETURN
	BREAK
	CONTINUE
	TRUE
	FALSE
	NIL
	AND
	OR
	NOT
	IMPORT
	EXPORT

	// Operators and punctuation.
	ASSIGN   // =
	EQ       // ==
	NEQ      // !=
	LT       // <
	LE       // <=
	GT       // >
	GE       // >=
	PLUS     // +
	MINUS    // -
	STAR     // *
	SLASH    // /
	PERCENT  // %
	CARET    // ^
	LPAREN   // (
	RPAREN   // )
	LBRACKET // [
	RBRACKET // ]
	LBRACE   // {
	RBRACE   // }
	COMMA    // ,
	COLON    // :
	DOT      // .
)

var keywords = map[string]Kind{
	"let":      LET,
	"const":    CONST,
	"fn":       FN,
	"if":       IF,
	"elif":     ELIF,
	"else":     ELSE,
	"for":      FOR,
	"in":       IN,
	"while":    WHILE,
	"return":   RETURN,
	"break":    BREAK,
	"continue": CONTINUE,
	"true":     TRUE,
	"false":    FALSE,
	"nil":      NIL,
	"and":      AND,
	"or":       OR,
	"not":      NOT,
	"import":   IMPORT,
	"export":   EXPORT,
}

// Lookup returns the keyword kind for name, or IDENT.
func Lookup(name string) Kind {
	if k, ok := keywords[name]; ok {
		return k
	}
	return IDENT
}

// IsKeyword reports whether k is a language keyword.
func (k Kind) IsKeyword() bool {
	return k >= LET && k <= EXPORT
}

// String returns a human-readable name for k.
//
// Operators and punctuation render as their source text. For example, the
// PLUS kind renders as "+".
func (k Kind) String() string {
	switch k {
	case ILLEGAL:
		return "illegal token"
	case EOF:
		return "end of file"
	case NEWLINE:
		return "newline"
	case IDENT:
		return "identifier"
	case INT:
		return "integer"
	case FLOAT:
		return "float"
	case STRING:
		return "string"
	case LET:
		return "let"
	case CONST:
		return "const"
	case FN:
		return "fn"
	case IF:
		return "if"
	case ELIF:
		return "elif"
	case ELSE:
		return "else"
	case FOR:
		return "for"
	case IN:
		return "in"
	case WHILE:
		return "while"
	case RETURN:
		return "return"
	case BREAK:
		return "break"
	case CONTINUE:
		return "continue"
	case TRUE:
		return "true"
	case FALSE:
		return "false"
	case NIL:
		return "nil"
	case AND:
		return "and"
	case OR:
		return "or"
	case NOT:
		return "not"
	case IMPORT:
		return "import"
	case EXPORT:
		return "export"
	case ASSIGN:
		return "="
	case EQ:
		return "=="
	case NEQ:
		return "!="
	case LT:
		return "<"
	case LE:
		return "<="
	case GT:
		return ">"
	case GE:
		return ">="
	case PLUS:
		return "+"
	case MINUS:
		return "-"
	case STAR:
		return "*"
	case SLASH:
		return "/"
	case PERCENT:
		return "%"
	case CARET:
		return "^"
	case LPAREN:
		return "("
	case RPAREN:
		return ")"
	case LBRACKET:
		return "["
	case RBRACKET:
		return "]"
	case LBRACE:
		return "{"
	case RBRACE:
		return "}"
	case COMMA:
		return ","
	case COLON:
		return ":"
	case DOT:
		return "."
	}
	return fmt.Sprintf("token(%d)", int(k))
}

// Token is a single lexical token.
type Token struct {
	Kind   Kind
	Lexeme string
	Pos    source.Pos
	// Value holds the decoded text of a STRING token.
	Value string
}

// TokenString describes a token for use in diagnostics.
func TokenString(t Token) string {
	switch t.Kind {
	case EOF:
		return "end of file"
	case NEWLINE:
		return "end of line"
	case IDENT:
		return fmt.Sprintf("'%s'", t.Lexeme)
	case INT, FLOAT:
		return fmt.Sprintf("number %s", t.Lexeme)
	case STRING:
		return fmt.Sprintf("string %s", strconv.Quote(t.Value))
	default:
		return fmt.Sprintf("'%s'", t.Kind)
	}
}
