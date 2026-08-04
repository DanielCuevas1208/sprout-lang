// This file renders an AST back to Sprout source text.
//
// The printer is a faithful inverse of the parser. It keeps every expression
// on one line, so a printed program always parses back to the same tree. The
// bundler uses it to inline module files into a single self-contained
// program. It is also handy for tests and future formatting tools.
//
// Comments are not preserved. The output is valid, deterministic source.
package ast

import (
	"strconv"
	"strings"

	"github.com/sprout-lang/sprout/internal/token"
)

// Source renders n as Sprout source text.
//
// A statement list ends each statement with a newline. Blocks use braces and
// a four-space indent. Expressions never span lines.
func Source(n Node) string {
	p := &sourcePrinter{}
	p.node(n)
	return p.b.String()
}

// SourceBody renders n with import statements removed and export modifiers
// dropped. The build tool uses it to inline a module body into a bundle.
func SourceBody(n Node) string {
	p := &sourcePrinter{bundle: true}
	p.node(n)
	return p.b.String()
}

// Operator binding powers, mirroring the parser.
const (
	srcPrecAssign  = 1
	srcPrecOr      = 2
	srcPrecAnd     = 3
	srcPrecEq      = 4
	srcPrecCmp     = 5
	srcPrecTerm    = 6
	srcPrecFactor  = 7
	srcPrecUnary   = 8
	srcPrecPower   = 9
	srcPrecCall    = 10
	srcPrecPrimary = 11
)

// srcOpPrec returns the binding power of a binary operator.
func srcOpPrec(k token.Kind) int {
	switch k {
	case token.ASSIGN:
		return srcPrecAssign
	case token.OR:
		return srcPrecOr
	case token.AND:
		return srcPrecAnd
	case token.EQ, token.NEQ:
		return srcPrecEq
	case token.LT, token.LE, token.GT, token.GE:
		return srcPrecCmp
	case token.PLUS, token.MINUS:
		return srcPrecTerm
	case token.STAR, token.SLASH, token.PERCENT:
		return srcPrecFactor
	case token.CARET:
		return srcPrecPower
	}
	return 0
}

// srcIsRightAssoc reports whether a binary operator groups to the right.
func srcIsRightAssoc(k token.Kind) bool {
	return k == token.ASSIGN || k == token.CARET
}

type sourcePrinter struct {
	b      strings.Builder
	indent int
	// bundle drops import statements and export modifiers.
	bundle bool
}

func (p *sourcePrinter) node(n Node) {
	switch v := n.(type) {
	case *Program:
		p.stmts(v.Stmts)
	case *LetStmt:
		kw := "let"
		if v.IsConst {
			kw = "const"
		}
		if v.Export && !p.bundle {
			kw = "export " + kw
		}
		p.b.WriteString(kw)
		p.space()
		p.name(v.Name)
		if v.Type != nil {
			p.b.WriteString(": ")
			p.b.WriteString(v.Type.Name)
		}
		p.b.WriteString(" = ")
		if v.Value != nil {
			p.expr(v.Value, 0)
		} else {
			p.b.WriteString("nil")
		}
	case *FnStmt:
		if v.Export && !p.bundle {
			p.b.WriteString("export ")
		}
		p.b.WriteString("fn ")
		p.name(v.Name)
		p.params(v.Params)
		p.space()
		p.block(v.Body)
	case *ImportStmt:
		if p.bundle {
			return
		}
		p.b.WriteString("import ")
		p.b.WriteString(strconv.Quote(v.Path))
		if v.Name != nil {
			p.b.WriteString(" as ")
			p.name(v.Name)
		}
	case *IfStmt:
		p.b.WriteString("if ")
		p.expr(v.Cond, 0)
		p.space()
		p.block(v.Then)
		for _, e := range v.Elifs {
			p.b.WriteString(" elif ")
			p.expr(e.Cond, 0)
			p.space()
			p.block(e.Body)
		}
		if v.Else != nil {
			p.b.WriteString(" else")
			p.space()
			p.block(v.Else)
		}
	case *WhileStmt:
		p.b.WriteString("while ")
		p.expr(v.Cond, 0)
		p.space()
		p.block(v.Body)
	case *ForInStmt:
		p.b.WriteString("for ")
		p.name(v.Var)
		p.b.WriteString(" in ")
		p.expr(v.Iterable, 0)
		p.space()
		p.block(v.Body)
	case *ReturnStmt:
		p.b.WriteString("return")
		if v.Value != nil {
			p.b.WriteString(" ")
			p.expr(v.Value, 0)
		}
	case *BreakStmt:
		p.b.WriteString("break")
	case *ContinueStmt:
		p.b.WriteString("continue")
	case *ExprStmt:
		p.expr(v.X, 0)
	case *Block:
		p.block(v)
	default:
		if e, ok := n.(Expr); ok {
			p.expr(e, 0)
		} else {
			p.b.WriteString("nil")
		}
	}
}

func (p *sourcePrinter) stmts(stmts []Stmt) {
	for _, s := range stmts {
		p.writeIndent()
		p.node(s)
		p.nl()
	}
}

func (p *sourcePrinter) block(b *Block) {
	p.b.WriteString("{")
	p.indent++
	for _, s := range b.Stmts {
		p.nl()
		p.writeIndent()
		p.node(s)
	}
	p.indent--
	if len(b.Stmts) > 0 {
		p.nl()
		p.writeIndent()
	}
	p.b.WriteString("}")
}

func (p *sourcePrinter) params(params []*Param) {
	p.b.WriteString("(")
	for i, pr := range params {
		if i > 0 {
			p.b.WriteString(", ")
		}
		p.name(pr.Name)
		if pr.Type != nil {
			p.b.WriteString(": ")
			p.b.WriteString(pr.Type.Name)
		}
	}
	p.b.WriteString(")")
}

// expr renders an expression so that it binds at least as tightly as minPrec.
func (p *sourcePrinter) expr(e Expr, minPrec int) {
	if prec := srcExprPrec(e); prec < minPrec {
		p.b.WriteString("(")
		p.expr(e, 0)
		p.b.WriteString(")")
		return
	}
	switch v := e.(type) {
	case *Ident:
		p.b.WriteString(v.Name)
	case *IntLit:
		p.b.WriteString(strconv.FormatInt(v.Value, 10))
	case *FloatLit:
		p.b.WriteString(formatFloat(v.Value))
	case *StrLit:
		p.b.WriteString(quoteString(v.Value))
	case *BoolLit:
		if v.Value {
			p.b.WriteString("true")
		} else {
			p.b.WriteString("false")
		}
	case *NilLit:
		p.b.WriteString("nil")
	case *ListLit:
		p.b.WriteString("[")
		for i, el := range v.Elems {
			if i > 0 {
				p.b.WriteString(", ")
			}
			p.expr(el, 0)
		}
		p.b.WriteString("]")
	case *MapLit:
		p.b.WriteString("{")
		for i, en := range v.Entries {
			if i > 0 {
				p.b.WriteString(", ")
			}
			p.expr(en.Key, 0)
			p.b.WriteString(": ")
			p.expr(en.Value, 0)
		}
		p.b.WriteString("}")
	case *UnaryExpr:
		p.b.WriteString(v.Op.String())
		if v.Op == token.NOT {
			p.b.WriteString(" ")
		}
		p.expr(v.X, srcPrecUnary)
	case *BinaryExpr:
		prec := srcOpPrec(v.Op)
		if srcIsRightAssoc(v.Op) {
			p.expr(v.Left, prec+1)
			p.b.WriteString(" ")
			p.b.WriteString(v.Op.String())
			p.b.WriteString(" ")
			p.expr(v.Right, prec)
		} else {
			p.expr(v.Left, prec+1)
			p.b.WriteString(" ")
			p.b.WriteString(v.Op.String())
			p.b.WriteString(" ")
			p.expr(v.Right, prec+1)
		}
	case *AssignExpr:
		p.expr(v.Target, srcPrecAssign+1)
		p.b.WriteString(" = ")
		p.expr(v.Value, srcPrecAssign)
	case *CallExpr:
		p.expr(v.Callee, srcPrecCall+1)
		p.b.WriteString("(")
		for i, a := range v.Args {
			if i > 0 {
				p.b.WriteString(", ")
			}
			p.expr(a, 0)
		}
		p.b.WriteString(")")
	case *IndexExpr:
		p.expr(v.X, srcPrecCall+1)
		p.b.WriteString("[")
		p.expr(v.Index, 0)
		p.b.WriteString("]")
	case *MemberExpr:
		p.expr(v.X, srcPrecCall+1)
		p.b.WriteString(".")
		p.name(v.Name)
	case *FnExpr:
		p.b.WriteString("fn")
		p.params(v.Params)
		p.space()
		p.block(v.Body)
	default:
		p.b.WriteString("nil")
	}
}

// srcExprPrec returns the binding power of an expression node.
func srcExprPrec(e Expr) int {
	switch v := e.(type) {
	case *BinaryExpr:
		if prec := srcOpPrec(v.Op); prec > 0 {
			return prec
		}
	case *AssignExpr:
		return srcPrecAssign
	case *UnaryExpr:
		return srcPrecUnary
	case *CallExpr, *IndexExpr, *MemberExpr:
		return srcPrecCall
	}
	return srcPrecPrimary
}

func (p *sourcePrinter) name(n *Ident) {
	if n != nil {
		p.b.WriteString(n.Name)
	}
}

func (p *sourcePrinter) writeIndent() {
	p.b.WriteString(strings.Repeat("    ", p.indent))
}

func (p *sourcePrinter) nl()    { p.b.WriteString("\n") }
func (p *sourcePrinter) space() { p.b.WriteString(" ") }

// quoteString renders s as a Sprout string literal.
//
// The escape set matches the lexer, including hex byte escapes. A string
// that contains a newline stays on one line by escaping the newline.
func quoteString(s string) string {
	var b strings.Builder
	b.WriteString("\"")
	for i := 0; i < len(s); i++ {
		ch := s[i]
		switch ch {
		case '"':
			b.WriteString("\\\"")
		case '\\':
			b.WriteString("\\\\")
		case '\n':
			b.WriteString("\\n")
		case '\t':
			b.WriteString("\\t")
		case '\r':
			b.WriteString("\\r")
		case 0:
			b.WriteString("\\0")
		default:
			if ch < 0x20 || ch == 0x7f {
				b.WriteString("\\x")
				const hex = "0123456789abcdef"
				b.WriteByte(hex[ch>>4])
				b.WriteByte(hex[ch&0x0f])
			} else {
				b.WriteByte(ch)
			}
		}
	}
	b.WriteString("\"")
	return b.String()
}
