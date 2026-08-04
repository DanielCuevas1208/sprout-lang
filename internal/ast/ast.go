// Package ast defines the abstract syntax tree of the Sprout language.
package ast

import (
	"strconv"
	"strings"

	"github.com/sprout-lang/sprout/internal/source"
	"github.com/sprout-lang/sprout/internal/token"
)

// Node is anything that appears in a program.
type Node interface {
	Pos() source.Pos
	End() source.Pos
}

// Stmt is a statement.
type Stmt interface {
	Node
	stmt()
}

// Expr is an expression.
type Expr interface {
	Node
	expr()
}

// Program is a sequence of statements.
type Program struct {
	Stmts []Stmt
}

func (p *Program) Pos() source.Pos {
	if len(p.Stmts) == 0 {
		return source.Pos{}
	}
	return p.Stmts[0].Pos()
}
func (p *Program) End() source.Pos {
	if len(p.Stmts) == 0 {
		return source.Pos{}
	}
	return p.Stmts[len(p.Stmts)-1].End()
}

// Ident is a name.
type Ident struct {
	Name     string
	Position source.Pos
}

func (n *Ident) Pos() source.Pos { return n.Position }
func (n *Ident) End() source.Pos {
	return source.Pos{Line: n.Position.Line, Column: n.Position.Column + len(n.Name), Offset: n.Position.Offset + len(n.Name)}
}
func (*Ident) expr() {}

// Param is a function parameter.
type Param struct {
	Name *Ident
	Type *Ident // optional type annotation
}

func (p *Param) Pos() source.Pos { return p.Name.Position }
func (p *Param) End() source.Pos {
	if p.Type != nil {
		return p.Type.End()
	}
	return p.Name.End()
}

// LetStmt declares a variable (IsConst == false) or a constant.
// Export marks the name as visible to importers of the file.
type LetStmt struct {
	KwPos   source.Pos
	IsConst bool
	Export  bool
	Name    *Ident
	Type    *Ident // optional type annotation
	Value   Expr
}

func (s *LetStmt) Pos() source.Pos { return s.KwPos }
func (s *LetStmt) End() source.Pos {
	if s.Value != nil {
		return s.Value.End()
	}
	if s.Name != nil {
		return s.Name.End()
	}
	return s.KwPos
}
func (*LetStmt) stmt() {}

// FnStmt declares a named function.
// Export marks the name as visible to importers of the file.
type FnStmt struct {
	FnPos  source.Pos
	Export bool
	Name   *Ident
	Params []*Param
	Body   *Block
}

func (s *FnStmt) Pos() source.Pos { return s.FnPos }
func (s *FnStmt) End() source.Pos {
	if s.Body != nil {
		return s.Body.End()
	}
	return s.FnPos
}
func (*FnStmt) stmt() {}

// Block is a sequence of statements between braces.
type Block struct {
	Lbrace source.Pos
	Stmts  []Stmt
	Rbrace source.Pos
}

func (b *Block) Pos() source.Pos { return b.Lbrace }
func (b *Block) End() source.Pos { return b.Rbrace }
func (*Block) stmt()             {}

// IfStmt is a conditional with optional elif and else branches.
type IfStmt struct {
	IfPos source.Pos
	Cond  Expr
	Then  *Block
	Elifs []*ElifBranch
	Else  *Block
}

// ElifBranch is one "elif cond { body }" clause.
type ElifBranch struct {
	Cond Expr
	Body *Block
}

func (s *IfStmt) Pos() source.Pos { return s.IfPos }
func (s *IfStmt) End() source.Pos {
	if s.Else != nil {
		return s.Else.End()
	}
	if len(s.Elifs) > 0 {
		return s.Elifs[len(s.Elifs)-1].Body.End()
	}
	return s.Then.End()
}
func (*IfStmt) stmt() {}

// WhileStmt is a condition-controlled loop.
type WhileStmt struct {
	WhilePos source.Pos
	Cond     Expr
	Body     *Block
}

func (s *WhileStmt) Pos() source.Pos { return s.WhilePos }
func (s *WhileStmt) End() source.Pos { return s.Body.End() }
func (*WhileStmt) stmt()             {}

// ForInStmt iterates over a sequence, binding each element to a name.
type ForInStmt struct {
	ForPos   source.Pos
	Var      *Ident
	Iterable Expr
	Body     *Block
}

func (s *ForInStmt) Pos() source.Pos { return s.ForPos }
func (s *ForInStmt) End() source.Pos { return s.Body.End() }
func (*ForInStmt) stmt()             {}

// ReturnStmt exits the current function with an optional value.
type ReturnStmt struct {
	ReturnPos source.Pos
	Value     Expr
}

func (s *ReturnStmt) Pos() source.Pos { return s.ReturnPos }
func (s *ReturnStmt) End() source.Pos {
	if s.Value != nil {
		return s.Value.End()
	}
	return s.ReturnPos
}
func (*ReturnStmt) stmt() {}

// BreakStmt exits the innermost loop.
type BreakStmt struct {
	Position source.Pos
}

func (s *BreakStmt) Pos() source.Pos { return s.Position }
func (s *BreakStmt) End() source.Pos { return s.Position }
func (*BreakStmt) stmt()             {}

// ContinueStmt skips to the next loop iteration.
type ContinueStmt struct {
	Position source.Pos
}

func (s *ContinueStmt) Pos() source.Pos { return s.Position }
func (s *ContinueStmt) End() source.Pos { return s.Position }
func (*ContinueStmt) stmt()             {}

// ExprStmt is an expression used for its side effect.
type ExprStmt struct {
	X Expr
}

func (s *ExprStmt) Pos() source.Pos { return s.X.Pos() }
func (s *ExprStmt) End() source.Pos { return s.X.End() }
func (*ExprStmt) stmt()             {}

// IntLit is an integer literal.
type IntLit struct {
	Value    int64
	Position source.Pos
}

func (n *IntLit) Pos() source.Pos { return n.Position }
func (n *IntLit) End() source.Pos { return n.Position }
func (*IntLit) expr()             {}

// FloatLit is a floating-point literal.
type FloatLit struct {
	Value    float64
	Position source.Pos
}

func (n *FloatLit) Pos() source.Pos { return n.Position }
func (n *FloatLit) End() source.Pos { return n.Position }
func (*FloatLit) expr()             {}

// StrLit is a string literal.
type StrLit struct {
	Value    string
	Position source.Pos
}

func (n *StrLit) Pos() source.Pos { return n.Position }
func (n *StrLit) End() source.Pos { return n.Position }
func (*StrLit) expr()             {}

// BoolLit is a boolean literal.
type BoolLit struct {
	Value    bool
	Position source.Pos
}

func (n *BoolLit) Pos() source.Pos { return n.Position }
func (n *BoolLit) End() source.Pos { return n.Position }
func (*BoolLit) expr()             {}

// NilLit is the nil literal.
type NilLit struct {
	Position source.Pos
}

func (n *NilLit) Pos() source.Pos { return n.Position }
func (n *NilLit) End() source.Pos { return n.Position }
func (*NilLit) expr()             {}

// ListLit is a list literal.
type ListLit struct {
	Lbracket source.Pos
	Elems    []Expr
}

func (n *ListLit) Pos() source.Pos { return n.Lbracket }
func (n *ListLit) End() source.Pos {
	if len(n.Elems) > 0 {
		return n.Elems[len(n.Elems)-1].End()
	}
	return n.Lbracket
}
func (*ListLit) expr() {}

// MapEntry is one "key: value" pair in a map literal.
type MapEntry struct {
	Key   Expr
	Value Expr
}

// MapLit is a map literal.
type MapLit struct {
	Lbrace  source.Pos
	Entries []MapEntry
}

func (n *MapLit) Pos() source.Pos { return n.Lbrace }
func (n *MapLit) End() source.Pos {
	if len(n.Entries) > 0 {
		return n.Entries[len(n.Entries)-1].Value.End()
	}
	return n.Lbrace
}
func (*MapLit) expr() {}

// UnaryExpr applies a unary operator to its operand.
type UnaryExpr struct {
	Op    token.Kind
	OpPos source.Pos
	X     Expr
}

func (n *UnaryExpr) Pos() source.Pos { return n.OpPos }
func (n *UnaryExpr) End() source.Pos { return n.X.End() }
func (*UnaryExpr) expr()             {}

// BinaryExpr combines two operands with an infix operator.
type BinaryExpr struct {
	Left  Expr
	Op    token.Kind
	OpPos source.Pos
	Right Expr
}

func (n *BinaryExpr) Pos() source.Pos { return n.OpPos }
func (n *BinaryExpr) End() source.Pos { return n.Right.End() }
func (*BinaryExpr) expr()             {}

// AssignExpr stores a value into a target.
type AssignExpr struct {
	Target Expr
	OpPos  source.Pos
	Value  Expr
}

func (n *AssignExpr) Pos() source.Pos { return n.OpPos }
func (n *AssignExpr) End() source.Pos { return n.Value.End() }
func (*AssignExpr) expr()             {}

// CallExpr invokes a function with arguments.
type CallExpr struct {
	Callee Expr
	Lparen source.Pos
	Args   []Expr
}

func (n *CallExpr) Pos() source.Pos {
	if n.Callee != nil {
		return n.Callee.Pos()
	}
	return n.Lparen
}
func (n *CallExpr) End() source.Pos { return n.Lparen }
func (*CallExpr) expr()             {}

// ImportStmt loads a module and binds it to a local name.
type ImportStmt struct {
	ImportPos source.Pos
	// Path is the module specifier, for example "lib/util" or "./util.spr".
	Path    string
	PathPos source.Pos
	// Name is the name the module is bound to in the importing scope.
	Name *Ident
}

func (s *ImportStmt) Pos() source.Pos { return s.ImportPos }
func (s *ImportStmt) End() source.Pos {
	if s.Name != nil {
		return s.Name.End()
	}
	return s.ImportPos
}
func (*ImportStmt) stmt() {}

// MemberExpr reads a named member of a module value.
//
// The member is stored in the AST as a name so the checker can validate it
// against the module's export table without re-tokenizing the source.
type MemberExpr struct {
	X      Expr
	DotPos source.Pos
	Name   *Ident
}

func (n *MemberExpr) Pos() source.Pos { return n.X.Pos() }
func (n *MemberExpr) End() source.Pos { return n.Name.End() }
func (*MemberExpr) expr()             {}

// IndexExpr reads an element from a container.
type IndexExpr struct {
	X        Expr
	Lbracket source.Pos
	Index    Expr
}

func (n *IndexExpr) Pos() source.Pos {
	if n.X != nil {
		return n.X.Pos()
	}
	return n.Lbracket
}
func (n *IndexExpr) End() source.Pos { return n.Lbracket }
func (*IndexExpr) expr()             {}

// FnExpr is an anonymous function literal.
type FnExpr struct {
	FnPos  source.Pos
	Params []*Param
	Body   *Block
}

func (n *FnExpr) Pos() source.Pos { return n.FnPos }
func (n *FnExpr) End() source.Pos { return n.Body.End() }
func (*FnExpr) expr()             {}

// Sexp renders a node as an S-expression.
//
// The format is used by the "sprout parse" command and by parser tests.
func Sexp(n Node) string {
	p := &printer{indent: 0}
	p.node(n)
	return p.b.String()
}

type printer struct {
	b      strings.Builder
	indent int
}

func (p *printer) node(n Node) {
	if n == nil {
		p.b.WriteString("(nil)")
		return
	}
	switch v := n.(type) {
	case *Program:
		p.group("program", func() { p.stmts(v.Stmts) })
	case *LetStmt:
		kw := "let"
		if v.IsConst {
			kw = "const"
		}
		if v.Export {
			kw = "export " + kw
		}
		p.group(kw, func() {
			p.name(v.Name)
			if v.Type != nil {
				p.field(":type " + v.Type.Name)
			}
			p.exprField("value", v.Value)
		})
	case *FnStmt:
		head := "fn " + v.Name.Name
		if v.Export {
			head = "export " + head
		}
		p.group(head, func() {
			p.params(v.Params)
			p.block(v.Body)
		})
	case *IfStmt:
		p.group("if", func() {
			p.exprField("cond", v.Cond)
			p.block(v.Then)
			for _, e := range v.Elifs {
				p.group("elif", func() {
					p.exprField("cond", e.Cond)
					p.block(e.Body)
				})
			}
			if v.Else != nil {
				p.group("else", func() { p.block(v.Else) })
			}
		})
	case *WhileStmt:
		p.group("while", func() {
			p.exprField("cond", v.Cond)
			p.block(v.Body)
		})
	case *ForInStmt:
		p.group("for", func() {
			p.field("var " + v.Var.Name)
			p.exprField("in", v.Iterable)
			p.block(v.Body)
		})
	case *ReturnStmt:
		p.group("return", func() { p.exprField("value", v.Value) })
	case *ImportStmt:
		p.group("import", func() {
			p.field(strconv.Quote(v.Path))
			if v.Name != nil {
				p.field("as " + v.Name.Name)
			}
		})
	case *BreakStmt:
		p.group("break", nil)
	case *ContinueStmt:
		p.group("continue", nil)
	case *ExprStmt:
		p.node(v.X)
	case *Block:
		p.block(v)
	case *Ident:
		p.atom(v.Name)
	case *IntLit:
		p.group("int", func() { p.atom(strconv.FormatInt(v.Value, 10)) })
	case *FloatLit:
		p.group("float", func() { p.atom(formatFloat(v.Value)) })
	case *StrLit:
		p.group("string", func() { p.atom(strconv.Quote(v.Value)) })
	case *BoolLit:
		if v.Value {
			p.atom("true")
		} else {
			p.atom("false")
		}
	case *NilLit:
		p.atom("nil")
	case *ListLit:
		p.group("list", func() {
			for _, e := range v.Elems {
				p.node(e)
			}
		})
	case *MapLit:
		p.group("map", func() {
			for _, en := range v.Entries {
				p.group("entry", func() {
					p.node(en.Key)
					p.node(en.Value)
				})
			}
		})
	case *UnaryExpr:
		p.group("unary "+v.Op.String(), func() { p.node(v.X) })
	case *BinaryExpr:
		p.group("binary "+v.Op.String(), func() {
			p.node(v.Left)
			p.node(v.Right)
		})
	case *AssignExpr:
		p.group("assign", func() {
			p.node(v.Target)
			p.node(v.Value)
		})
	case *CallExpr:
		p.group("call", func() {
			p.node(v.Callee)
			for _, a := range v.Args {
				p.node(a)
			}
		})
	case *IndexExpr:
		p.group("index", func() {
			p.node(v.X)
			p.node(v.Index)
		})
	case *MemberExpr:
		p.group("member", func() {
			p.node(v.X)
			p.field("." + v.Name.Name)
		})
	case *FnExpr:
		p.group("fn", func() {
			p.params(v.Params)
			p.block(v.Body)
		})
	default:
		p.atom("?")
	}
}

func (p *printer) group(head string, body func()) {
	p.open(head)
	if body != nil {
		body()
	}
	p.close()
}

func (p *printer) stmts(stmts []Stmt) {
	for _, s := range stmts {
		p.node(s)
	}
}

func (p *printer) open(head string) {
	p.nl()
	p.b.WriteString("(")
	p.b.WriteString(head)
	p.indent += 2
}

func (p *printer) close() {
	p.indent -= 2
	p.nl()
	p.b.WriteString(")")
}

func (p *printer) nl() {
	p.b.WriteString("\n")
	p.b.Write(spaces(p.indent))
}

func (p *printer) atom(s string) {
	p.nl()
	p.b.WriteString(s)
}

func (p *printer) field(name string) {
	p.nl()
	p.b.WriteString(name)
}

func (p *printer) name(n *Ident) {
	if n == nil {
		return
	}
	p.atom(n.Name)
}

func (p *printer) exprField(name string, e Expr) {
	if e == nil {
		p.atom("(nothing)")
		return
	}
	p.field(name)
	p.node(e)
}

func (p *printer) params(params []*Param) {
	if len(params) == 0 {
		p.field("(params)")
		return
	}
	var names []string
	for _, pr := range params {
		if pr.Type != nil {
			names = append(names, pr.Name.Name+":"+pr.Type.Name)
		} else {
			names = append(names, pr.Name.Name)
		}
	}
	p.field("(params " + strings.Join(names, " ") + ")")
}

func (p *printer) block(b *Block) {
	if b == nil {
		p.atom("(nothing)")
		return
	}
	p.open("block")
	p.stmts(b.Stmts)
	p.close()
}

func spaces(n int) []byte {
	s := make([]byte, n)
	for i := range s {
		s[i] = ' '
	}
	return s
}

// formatFloat renders f as a Sprout float literal.
//
// A value like 1.0 must keep a decimal point or exponent so that re-parsing
// yields a float, not an integer. The lexer reads "1" as an integer.
func formatFloat(f float64) string {
	s := strconv.FormatFloat(f, 'g', -1, 64)
	if !strings.ContainsAny(s, ".eE") {
		s += ".0"
	}
	return s
}
