// Package parser builds an abstract syntax tree with a Pratt parser.
//
// The parser is line-aware: a statement ends at a newline unless it is inside
// brackets. This keeps expressions from silently merging across lines while
// still allowing calls, lists, and maps to span several lines.
package parser

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/sprout-lang/sprout/internal/ast"
	"github.com/sprout-lang/sprout/internal/diag"
	"github.com/sprout-lang/sprout/internal/lexer"
	"github.com/sprout-lang/sprout/internal/source"
	"github.com/sprout-lang/sprout/internal/token"
)

// Operator binding powers, from loosest to tightest.
const (
	precLowest = iota
	precAssign
	precOr
	precAnd
	precEquality
	precComparison
	precTerm
	precFactor
	precUnary
	precPower
	precCall
)

var precedences = map[token.Kind]int{
	token.ASSIGN:   precAssign,
	token.OR:       precOr,
	token.AND:      precAnd,
	token.EQ:       precEquality,
	token.NEQ:      precEquality,
	token.LT:       precComparison,
	token.LE:       precComparison,
	token.GT:       precComparison,
	token.GE:       precComparison,
	token.PLUS:     precTerm,
	token.MINUS:    precTerm,
	token.STAR:     precFactor,
	token.SLASH:    precFactor,
	token.PERCENT:  precFactor,
	token.CARET:    precPower,
	token.LPAREN:   precCall,
	token.LBRACKET: precCall,
	token.DOT:      precCall,
}

// Parser turns tokens into an AST.
type Parser struct {
	file  *source.File
	toks  []token.Token
	i     int
	depth int // open bracket count; suppresses newline terminations
	diags []diag.Diagnostic
}

// Parse lexes and parses file into a Program.
func Parse(file *source.File) (*ast.Program, []diag.Diagnostic) {
	toks, ldiags := lexer.New(file).Tokenize()
	p := &Parser{file: file, toks: toks}
	prog := &ast.Program{Stmts: p.parseStatements(false)}
	return prog, append(ldiags, p.diags...)
}

// parseStatements parses a statement sequence. When expectClose is false the
// sequence is the program root, so a stray '}' is reported as an error.
func (p *Parser) parseStatements(expectClose bool) []ast.Stmt {
	var stmts []ast.Stmt
	for {
		p.skipNewlines()
		if p.at(token.EOF) {
			break
		}
		if p.at(token.RBRACE) {
			if !expectClose {
				p.errorf(p.cur().Pos, "unexpected '}', there is no block to close here")
				p.next()
				continue
			}
			break
		}
		if st := p.parseStatement(); st != nil {
			stmts = append(stmts, st)
		}
	}
	return stmts
}

func (p *Parser) parseStatement() ast.Stmt {
	t := p.cur()
	switch t.Kind {
	case token.LET:
		return p.parseLet(false)
	case token.CONST:
		return p.parseLet(true)
	case token.IMPORT:
		return p.parseImport()
	case token.FN:
		if p.peek(1).Kind == token.IDENT {
			return p.parseFnDecl()
		}
	case token.IF:
		return p.parseIf()
	case token.WHILE:
		return p.parseWhile()
	case token.FOR:
		return p.parseFor()
	case token.RETURN:
		return p.parseReturn()
	case token.BREAK:
		p.next()
		return &ast.BreakStmt{Position: t.Pos}
	case token.CONTINUE:
		p.next()
		return &ast.ContinueStmt{Position: t.Pos}
	case token.RBRACE:
		return nil
	case token.EOF:
		return nil
	}
	return p.parseExprStmt()
}

// parseImport parses "import" string ["as" identifier].
func (p *Parser) parseImport() ast.Stmt {
	kw := p.next() // import

	if !p.at(token.STRING) {
		p.errorf(p.cur().Pos, "expected a module path string after 'import', found %s", tokenString(p.cur()))
		p.recoverStatement()
		return nil
	}
	pathTok := p.next()
	spec := pathTok.Value

	var alias *ast.Ident
	if p.at(token.AS) {
		p.next()
		alias = p.expectIdent("an alias name after 'as'")
		if alias == nil {
			return nil
		}
	} else {
		name := defaultAlias(spec)
		if !validIdent(name) {
			p.errorf(pathTok.Pos, "cannot use %q as a module name, use 'as' to name it", spec)
			p.recoverStatement()
			return nil
		}
		alias = &ast.Ident{Name: name, Position: pathTok.Pos}
	}

	return &ast.ImportStmt{KwPos: kw.Pos, Path: spec, PathPos: pathTok.Pos, Alias: alias}
}

// defaultAlias derives a binding name from a module specifier.
func defaultAlias(spec string) string {
	s := strings.ReplaceAll(spec, "\\", "/")
	s = strings.TrimSuffix(s, "/")
	if i := strings.LastIndexByte(s, '/'); i >= 0 {
		s = s[i+1:]
	}
	return s
}

// validIdent reports whether s is a usable identifier and not a keyword.
func validIdent(s string) bool {
	if s == "" || token.Lookup(s) != token.IDENT {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		ok := c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (i > 0 && c >= '0' && c <= '9')
		if !ok {
			return false
		}
	}
	return true
}

func (p *Parser) parseLet(isConst bool) ast.Stmt {
	kw := p.next() // let or const

	name := p.expectIdent("a name after '%s'", kw.Lexeme)
	if name == nil {
		return nil
	}

	var typeName *ast.Ident
	if p.at(token.COLON) {
		p.next()
		if p.at(token.IDENT) {
			typeName = &ast.Ident{Name: p.cur().Lexeme, Position: p.cur().Pos}
			p.next()
		} else {
			p.errorf(p.cur().Pos, "expected a type after ':', found %s", tokenString(p.cur()))
		}
	}

	if !p.at(token.ASSIGN) {
		p.errorf(p.cur().Pos, "expected '=' in declaration, found %s", tokenString(p.cur()))
		p.recoverStatement()
		return nil
	}
	p.next()
	p.skipNewlines()

	value := p.parseExpression(precLowest)
	if value == nil {
		p.errorf(p.cur().Pos, "expected a value after '=', found %s", tokenString(p.cur()))
	}
	return &ast.LetStmt{KwPos: kw.Pos, IsConst: isConst, Name: name, Type: typeName, Value: value}
}

func (p *Parser) parseFnDecl() ast.Stmt {
	fnTok := p.next() // fn
	name := p.expectIdent("a name after 'fn'")
	if name == nil {
		return nil
	}
	params, ok := p.parseParams(fnTok.Pos)
	if !ok {
		return nil
	}
	body := p.parseBlock()
	if body == nil {
		return nil
	}
	return &ast.FnStmt{FnPos: fnTok.Pos, Name: name, Params: params, Body: body}
}

func (p *Parser) parseIf() ast.Stmt {
	ifTok := p.next() // if
	p.skipNewlines()

	cond := p.parseExpression(precLowest)
	if cond == nil {
		p.errorf(p.cur().Pos, "expected a condition after 'if', found %s", tokenString(p.cur()))
		p.recoverStatement()
		return nil
	}
	thenB := p.parseBlock()
	if thenB == nil {
		return nil
	}

	st := &ast.IfStmt{IfPos: ifTok.Pos, Cond: cond, Then: thenB}
	for {
		p.skipNewlines()
		switch {
		case p.at(token.ELIF):
			p.next()
			p.skipNewlines()
			c := p.parseExpression(precLowest)
			if c == nil {
				p.errorf(p.cur().Pos, "expected a condition after 'elif', found %s", tokenString(p.cur()))
				p.recoverStatement()
				return nil
			}
			b := p.parseBlock()
			if b == nil {
				return nil
			}
			st.Elifs = append(st.Elifs, &ast.ElifBranch{Cond: c, Body: b})
		case p.at(token.ELSE):
			p.next()
			elseB := p.parseBlock()
			if elseB == nil {
				return nil
			}
			st.Else = elseB
			return st
		default:
			return st
		}
	}
}

func (p *Parser) parseWhile() ast.Stmt {
	whileTok := p.next() // while
	p.skipNewlines()

	cond := p.parseExpression(precLowest)
	if cond == nil {
		p.errorf(p.cur().Pos, "expected a condition after 'while', found %s", tokenString(p.cur()))
		p.recoverStatement()
		return nil
	}
	body := p.parseBlock()
	if body == nil {
		return nil
	}
	return &ast.WhileStmt{WhilePos: whileTok.Pos, Cond: cond, Body: body}
}

func (p *Parser) parseFor() ast.Stmt {
	forTok := p.next() // for

	v := p.expectIdent("a loop variable after 'for'")
	if v == nil {
		return nil
	}
	if !p.at(token.IN) {
		p.errorf(p.cur().Pos, "expected 'in' after '%s', found %s", v.Name, tokenString(p.cur()))
		p.recoverStatement()
		return nil
	}
	p.next()
	p.skipNewlines()

	iterable := p.parseExpression(precLowest)
	if iterable == nil {
		p.errorf(p.cur().Pos, "expected a value to iterate over after 'in', found %s", tokenString(p.cur()))
		p.recoverStatement()
		return nil
	}
	body := p.parseBlock()
	if body == nil {
		return nil
	}
	return &ast.ForInStmt{ForPos: forTok.Pos, Var: v, Iterable: iterable, Body: body}
}

func (p *Parser) parseReturn() ast.Stmt {
	retTok := p.next() // return
	if p.at(token.NEWLINE) || p.at(token.RBRACE) || p.at(token.EOF) {
		return &ast.ReturnStmt{ReturnPos: retTok.Pos}
	}
	value := p.parseExpression(precLowest)
	if value == nil {
		p.errorf(p.cur().Pos, "expected a value after 'return', found %s", tokenString(p.cur()))
	}
	return &ast.ReturnStmt{ReturnPos: retTok.Pos, Value: value}
}

func (p *Parser) parseExprStmt() ast.Stmt {
	e := p.parseExpression(precLowest)
	if e == nil {
		p.errorf(p.cur().Pos, "expected a statement, found %s", tokenString(p.cur()))
		p.recoverStatement()
		return nil
	}
	return &ast.ExprStmt{X: e}
}

// parseExpression is the Pratt parser entry point.
func (p *Parser) parseExpression(prec int) ast.Expr {
	left := p.parsePrefix(p.cur())
	if left == nil {
		return nil
	}

	for {
		if p.at(token.NEWLINE) {
			if p.depth > 0 {
				p.next()
				continue
			}
			break
		}

		kind := p.cur().Kind
		infixPrec, isInfix := precedences[kind]
		if !isInfix || infixPrec < prec {
			break
		}

		switch kind {
		case token.LPAREN:
			// parseCall consumes the '(' itself.
			left = p.parseCall(left)
			if left == nil {
				return nil
			}
		case token.LBRACKET:
			// parseIndex consumes the '[' itself.
			left = p.parseIndex(left)
			if left == nil {
				return nil
			}
		case token.DOT:
			dot := p.next().Pos
			if !p.at(token.IDENT) {
				p.errorf(p.cur().Pos, "expected a member name after '.', found %s", tokenString(p.cur()))
				p.recoverStatement()
				return nil
			}
			name := p.next()
			left = &ast.MemberExpr{X: left, Dot: dot, Name: &ast.Ident{Name: name.Lexeme, Position: name.Pos}}
		default:
			op := p.next()
			p.skipNewlines()

			switch kind {
			case token.ASSIGN:
				if !isAssignable(left) {
					p.errorf(op.Pos, "cannot assign to %s", assignTargetDesc(left))
				}
				right := p.parseExpression(precAssign - 1)
				if right == nil {
					return left
				}
				left = &ast.AssignExpr{Target: left, OpPos: op.Pos, Value: right}
			case token.CARET:
				// Right-associative: bind tighter than precUnary so "-2^2" is "-(2^2)".
				right := p.parseExpression(precPower - 1)
				if right == nil {
					return left
				}
				left = &ast.BinaryExpr{Left: left, Op: op.Kind, OpPos: op.Pos, Right: right}
			default:
				right := p.parseExpression(infixPrec + 1)
				if right == nil {
					return left
				}
				left = &ast.BinaryExpr{Left: left, Op: op.Kind, OpPos: op.Pos, Right: right}
			}
		}
	}
	return left
}

func (p *Parser) parsePrefix(t token.Token) ast.Expr {
	switch t.Kind {
	case token.IDENT:
		p.next()
		return &ast.Ident{Name: t.Lexeme, Position: t.Pos}
	case token.INT:
		p.next()
		clean := strings.ReplaceAll(t.Lexeme, "_", "")
		v, err := strconv.ParseInt(clean, 10, 64)
		if err != nil {
			p.errorf(t.Pos, "invalid integer literal %s", t.Lexeme)
			v = 0
		}
		return &ast.IntLit{Value: v, Position: t.Pos}
	case token.FLOAT:
		p.next()
		clean := strings.ReplaceAll(t.Lexeme, "_", "")
		v, err := strconv.ParseFloat(clean, 64)
		if err != nil {
			p.errorf(t.Pos, "invalid float literal %s", t.Lexeme)
			v = 0
		}
		return &ast.FloatLit{Value: v, Position: t.Pos}
	case token.STRING:
		p.next()
		return &ast.StrLit{Value: t.Value, Position: t.Pos}
	case token.TRUE:
		p.next()
		return &ast.BoolLit{Value: true, Position: t.Pos}
	case token.FALSE:
		p.next()
		return &ast.BoolLit{Value: false, Position: t.Pos}
	case token.NIL:
		p.next()
		return &ast.NilLit{Position: t.Pos}
	case token.MINUS, token.NOT:
		p.next()
		operand := p.parseExpression(precUnary)
		if operand == nil {
			p.errorf(p.cur().Pos, "expected an operand after '%s'", t.Kind)
			return nil
		}
		return &ast.UnaryExpr{Op: t.Kind, OpPos: t.Pos, X: operand}
	case token.LPAREN:
		return p.parseGroup()
	case token.LBRACKET:
		return p.parseList()
	case token.LBRACE:
		return p.parseMap()
	case token.FN:
		return p.parseFnExpr()
	default:
		p.errorf(t.Pos, "expected an expression, found %s", tokenString(t))
		p.next()
		return nil
	}
}

func (p *Parser) parseGroup() ast.Expr {
	open := p.next() // (
	p.open()
	p.skipNewlines()
	if p.at(token.RPAREN) {
		p.errorf(open.Pos, "expected an expression inside '(', found ')'")
		p.close()
		p.next()
		return nil
	}
	inner := p.parseExpression(precLowest)
	if inner == nil {
		p.recoverStatement()
		return nil
	}
	p.skipNewlines()
	if !p.at(token.RPAREN) {
		p.errorf(p.cur().Pos, "expected ')' to close '(', found %s", tokenString(p.cur()))
		p.recoverStatement()
		return nil
	}
	p.close()
	p.next()
	return inner
}

func (p *Parser) parseCall(callee ast.Expr) ast.Expr {
	p.open()
	open := p.next() // (
	p.skipNewlines()

	var args []ast.Expr
	if p.at(token.RPAREN) {
		p.close()
		p.next()
		return &ast.CallExpr{Callee: callee, Lparen: open.Pos, Args: args}
	}

	for {
		arg := p.parseExpression(precLowest)
		if arg == nil {
			break
		}
		args = append(args, arg)
		p.skipNewlines()
		if p.at(token.COMMA) {
			p.next()
			p.skipNewlines()
			if p.at(token.RPAREN) {
				break // trailing comma
			}
			continue
		}
		break
	}

	p.skipNewlines()
	if !p.at(token.RPAREN) {
		p.errorf(p.cur().Pos, "expected ')' to close the call, found %s", tokenString(p.cur()))
		p.recoverStatement()
		return nil
	}
	p.close()
	p.next()
	return &ast.CallExpr{Callee: callee, Lparen: open.Pos, Args: args}
}

func (p *Parser) parseIndex(x ast.Expr) ast.Expr {
	p.open()
	open := p.next() // [
	p.skipNewlines()
	idx := p.parseExpression(precLowest)
	if idx == nil {
		p.recoverStatement()
		return nil
	}
	p.skipNewlines()
	if !p.at(token.RBRACKET) {
		p.errorf(p.cur().Pos, "expected ']' to close the index, found %s", tokenString(p.cur()))
		p.recoverStatement()
		return nil
	}
	p.close()
	p.next()
	return &ast.IndexExpr{X: x, Lbracket: open.Pos, Index: idx}
}

func (p *Parser) parseList() ast.Expr {
	p.open()
	open := p.next() // [
	p.skipNewlines()

	var elems []ast.Expr
	if p.at(token.RBRACKET) {
		p.close()
		p.next()
		return &ast.ListLit{Lbracket: open.Pos, Elems: elems}
	}

	for {
		e := p.parseExpression(precLowest)
		if e == nil {
			break
		}
		elems = append(elems, e)
		p.skipNewlines()
		if p.at(token.COMMA) {
			p.next()
			p.skipNewlines()
			if p.at(token.RBRACKET) {
				break // trailing comma
			}
			continue
		}
		break
	}

	p.skipNewlines()
	if !p.at(token.RBRACKET) {
		p.errorf(p.cur().Pos, "expected ']' to close the list, found %s", tokenString(p.cur()))
		p.recoverStatement()
		return nil
	}
	p.close()
	p.next()
	return &ast.ListLit{Lbracket: open.Pos, Elems: elems}
}

func (p *Parser) parseMap() ast.Expr {
	p.open()
	open := p.next() // {
	p.skipNewlines()

	var entries []ast.MapEntry
	if p.at(token.RBRACE) {
		p.close()
		p.next()
		return &ast.MapLit{Lbrace: open.Pos, Entries: entries}
	}

	for {
		key := p.parseExpression(precLowest)
		if key == nil {
			break
		}
		if !p.at(token.COLON) {
			p.errorf(p.cur().Pos, "expected ':' after the map key, found %s", tokenString(p.cur()))
			p.recoverStatement()
			return nil
		}
		p.next()
		value := p.parseExpression(precLowest)
		if value == nil {
			break
		}
		entries = append(entries, ast.MapEntry{Key: key, Value: value})
		p.skipNewlines()
		if p.at(token.COMMA) {
			p.next()
			p.skipNewlines()
			if p.at(token.RBRACE) {
				break // trailing comma
			}
			continue
		}
		break
	}

	p.skipNewlines()
	if !p.at(token.RBRACE) {
		p.errorf(p.cur().Pos, "expected '}' to close the map, found %s", tokenString(p.cur()))
		p.recoverStatement()
		return nil
	}
	p.close()
	p.next()
	return &ast.MapLit{Lbrace: open.Pos, Entries: entries}
}

func (p *Parser) parseFnExpr() ast.Expr {
	fnTok := p.next() // fn
	params, ok := p.parseParams(fnTok.Pos)
	if !ok {
		return nil
	}
	body := p.parseBlock()
	if body == nil {
		return nil
	}
	return &ast.FnExpr{FnPos: fnTok.Pos, Params: params, Body: body}
}

func (p *Parser) parseParams(openPos source.Pos) ([]*ast.Param, bool) {
	var params []*ast.Param
	if !p.at(token.LPAREN) {
		p.errorf(p.cur().Pos, "expected '(' to begin the parameter list, found %s", tokenString(p.cur()))
		p.recoverStatement()
		return nil, false
	}
	p.open()
	p.next() // (
	p.skipNewlines()

	if p.at(token.RPAREN) {
		p.close()
		p.next()
		return params, true
	}

	for {
		name := p.expectIdent("a parameter name")
		if name == nil {
			return nil, false
		}
		param := &ast.Param{Name: name}
		if p.at(token.COLON) {
			p.next()
			if p.at(token.IDENT) {
				param.Type = &ast.Ident{Name: p.cur().Lexeme, Position: p.cur().Pos}
				p.next()
			} else {
				p.errorf(p.cur().Pos, "expected a type after ':', found %s", tokenString(p.cur()))
			}
		}
		params = append(params, param)
		p.skipNewlines()
		if p.at(token.COMMA) {
			p.next()
			p.skipNewlines()
			if p.at(token.RPAREN) {
				break // trailing comma
			}
			continue
		}
		break
	}

	if p.at(token.RPAREN) {
		p.close()
		p.next()
		return params, true
	}
	p.errorf(p.cur().Pos, "expected ')' to close the parameter list, found %s", tokenString(p.cur()))
	p.recoverStatement()
	return nil, false
}

func (p *Parser) parseBlock() *ast.Block {
	if !p.at(token.LBRACE) {
		p.errorf(p.cur().Pos, "expected '{' to begin a block, found %s", tokenString(p.cur()))
		p.recoverStatement()
		return nil
	}
	open := p.next().Pos
	p.open()
	stmts := p.parseStatements(true)
	p.close()
	if !p.at(token.RBRACE) {
		p.errorf(p.cur().Pos, "expected '}' to close the block, found %s", tokenString(p.cur()))
		p.recoverStatement()
		return &ast.Block{Lbrace: open, Stmts: stmts}
	}
	close := p.next().Pos
	return &ast.Block{Lbrace: open, Rbrace: close, Stmts: stmts}
}

func (p *Parser) expectIdent(what string, args ...any) *ast.Ident {
	if !p.at(token.IDENT) {
		p.errorf(p.cur().Pos, "expected %s, found %s", fmt.Sprintf(what, args...), tokenString(p.cur()))
		p.recoverStatement()
		return nil
	}
	ident := &ast.Ident{Name: p.cur().Lexeme, Position: p.cur().Pos}
	p.next()
	return ident
}

// recoverStatement skips tokens until a statement boundary.
func (p *Parser) recoverStatement() {
	for {
		t := p.cur()
		switch t.Kind {
		case token.EOF, token.RBRACE:
			return
		case token.RPAREN, token.RBRACKET:
			p.next()
			return
		case token.NEWLINE:
			p.next()
			return
		default:
			p.next()
		}
	}
}

func (p *Parser) open()  { p.depth++ }
func (p *Parser) close() { p.depth-- }

func (p *Parser) cur() token.Token { return p.toks[p.i] }

func (p *Parser) peek(n int) token.Token {
	if j := p.i + n; j < len(p.toks) {
		return p.toks[j]
	}
	return p.toks[len(p.toks)-1]
}

func (p *Parser) next() token.Token {
	t := p.toks[p.i]
	if p.i < len(p.toks)-1 {
		p.i++
	}
	return t
}

func (p *Parser) at(k token.Kind) bool { return p.cur().Kind == k }

func (p *Parser) skipNewlines() {
	for p.at(token.NEWLINE) {
		p.next()
	}
}

func (p *Parser) errorf(pos source.Pos, format string, args ...any) {
	p.diags = append(p.diags, diag.Diagnostic{
		Severity: diag.SeverityError,
		Message:  fmt.Sprintf(format, args...),
		File:     p.file,
		Pos:      pos,
	})
}

func isAssignable(e ast.Expr) bool {
	switch e.(type) {
	case *ast.Ident, *ast.IndexExpr:
		return true
	}
	return false
}

func assignTargetDesc(e ast.Expr) string {
	switch e.(type) {
	case *ast.Ident:
		return "a name"
	case *ast.IndexExpr:
		return "an index"
	case *ast.MemberExpr:
		return "a module member"
	case *ast.CallExpr:
		return "a call result"
	}
	return "this expression"
}

func tokenString(t token.Token) string {
	return token.TokenString(t)
}
