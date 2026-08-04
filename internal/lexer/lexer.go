// Package lexer converts Sprout source text into a stream of tokens.
package lexer

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/sprout-lang/sprout/internal/diag"
	"github.com/sprout-lang/sprout/internal/source"
	"github.com/sprout-lang/sprout/internal/token"
)

// Lexer scans a source file one token at a time.
type Lexer struct {
	file  *source.File
	pos   int // current byte offset
	line  int
	col   int
	diags []diag.Diagnostic
}

// New returns a Lexer for file.
func New(file *source.File) *Lexer {
	return &Lexer{file: file, line: 1, col: 1}
}

// Tokenize scans the whole file and returns its tokens and diagnostics.
func (l *Lexer) Tokenize() ([]token.Token, []diag.Diagnostic) {
	var toks []token.Token
	for {
		t := l.next()
		toks = append(toks, t)
		if t.Kind == token.EOF {
			break
		}
	}
	return toks, l.diags
}

func (l *Lexer) next() token.Token {
	for {
		if l.pos >= len(l.file.Text) {
			return token.Token{Kind: token.EOF, Pos: l.currentPos()}
		}
		switch ch := l.peek(); ch {
		case ' ', '\t', '\r':
			l.advance()
			continue
		case '\n':
			pos := l.currentPos()
			l.advance()
			return token.Token{Kind: token.NEWLINE, Lexeme: "\n", Pos: pos}
		case '/':
			switch l.peekAt(1) {
			case '/':
				l.skipLineComment()
				continue
			case '*':
				l.skipBlockComment()
				continue
			}
		}
		return l.lexToken()
	}
}

func (l *Lexer) lexToken() token.Token {
	pos := l.currentPos()
	ch := l.peek()

	switch {
	case isIdentStart(ch):
		return l.lexIdent(pos)
	case isDigit(ch):
		return l.lexNumber(pos)
	case ch == '"' || ch == '`':
		return l.lexString(pos, ch)
	}

	if op, ok := twoCharOps[ch]; ok {
		if l.peekAt(1) == op.next {
			l.advance()
			l.advance()
			return token.Token{Kind: op.kind, Lexeme: string(ch) + string(op.next), Pos: pos}
		}
	}

	if kind, ok := singleCharOps[ch]; ok {
		l.advance()
		return token.Token{Kind: kind, Lexeme: string(ch), Pos: pos}
	}

	r, _ := utf8.DecodeRuneInString(l.file.Text[l.pos:])
	l.advanceRune()
	l.errorf(pos, "unexpected character %q", r)
	return token.Token{Kind: token.ILLEGAL, Lexeme: l.file.Text[l.pos-1 : l.pos], Pos: pos}
}

var twoCharOps = map[byte]struct {
	next byte
	kind token.Kind
}{
	'=': {next: '=', kind: token.EQ},
	'!': {next: '=', kind: token.NEQ},
	'<': {next: '=', kind: token.LE},
	'>': {next: '=', kind: token.GE},
}

var singleCharOps = map[byte]token.Kind{
	'=': token.ASSIGN, '<': token.LT, '>': token.GT,
	'+': token.PLUS, '-': token.MINUS, '*': token.STAR,
	'/': token.SLASH, '%': token.PERCENT, '^': token.CARET,
	'(': token.LPAREN, ')': token.RPAREN,
	'[': token.LBRACKET, ']': token.RBRACKET,
	'{': token.LBRACE, '}': token.RBRACE,
	',': token.COMMA, ':': token.COLON, '.': token.DOT,
}

func (l *Lexer) lexIdent(pos source.Pos) token.Token {
	start := l.pos
	for isIdentPart(l.peek()) {
		l.advance()
	}
	lexeme := l.file.Text[start:l.pos]
	return token.Token{Kind: token.Lookup(lexeme), Lexeme: lexeme, Pos: pos}
}

func (l *Lexer) lexNumber(pos source.Pos) token.Token {
	start := l.pos
	isFloat := false

	l.consumeDigits()

	if l.peek() == '.' && isDigit(l.peekAt(1)) {
		isFloat = true
		l.advance()
		l.consumeDigits()
	}

	if l.peek() == 'e' || l.peek() == 'E' {
		isFloat = true
		l.advance()
		if l.peek() == '+' || l.peek() == '-' {
			l.advance()
		}
		if !isDigit(l.peek()) {
			l.errorf(l.currentPos(), "expected digits in exponent")
			return l.illegalUntil(pos, start)
		}
		l.consumeDigits()
	}

	lexeme := l.file.Text[start:l.pos]
	if strings.HasPrefix(lexeme, "_") || strings.HasSuffix(lexeme, "_") {
		l.errorf(pos, "underscores must appear between digits in a number")
	}
	clean := strings.ReplaceAll(lexeme, "_", "")

	if isFloat {
		if _, err := strconv.ParseFloat(clean, 64); err != nil {
			l.errorf(pos, "invalid float literal %s", lexeme)
		}
		return token.Token{Kind: token.FLOAT, Lexeme: lexeme, Pos: pos}
	}
	if _, err := strconv.ParseInt(clean, 10, 64); err != nil {
		l.errorf(pos, "invalid integer literal %s", lexeme)
	}
	return token.Token{Kind: token.INT, Lexeme: lexeme, Pos: pos}
}

// consumeDigits advances over digits and underscores between digits.
func (l *Lexer) consumeDigits() {
	for isDigit(l.peek()) || l.peek() == '_' {
		if l.peek() == '_' && !isDigit(l.peekAt(1)) {
			break
		}
		l.advance()
	}
}

// illegalUntil returns an ILLEGAL token that runs from start to the current
// position, after consuming the rest of the malformed literal.
func (l *Lexer) illegalUntil(pos source.Pos, start int) token.Token {
	if l.pos < len(l.file.Text) {
		l.advance()
	}
	for isIdentPart(l.peek()) || isDigit(l.peek()) {
		l.advance()
	}
	return token.Token{Kind: token.ILLEGAL, Lexeme: l.file.Text[start:l.pos], Pos: pos}
}

func (l *Lexer) lexString(pos source.Pos, quote byte) token.Token {
	start := l.pos
	l.advance() // opening quote
	var sb strings.Builder

	for {
		if l.pos >= len(l.file.Text) {
			l.errorf(pos, "unterminated string literal")
			return token.Token{Kind: token.STRING, Lexeme: l.file.Text[start:l.pos], Value: sb.String(), Pos: pos}
		}
		if quote == '"' && l.peek() == '\n' {
			l.errorf(pos, "unterminated string literal")
			return token.Token{Kind: token.STRING, Lexeme: l.file.Text[start:l.pos], Value: sb.String(), Pos: pos}
		}
		ch := l.peek()
		if ch == quote {
			l.advance()
			return token.Token{Kind: token.STRING, Lexeme: l.file.Text[start:l.pos], Value: sb.String(), Pos: pos}
		}
		if ch == '\\' && quote == '"' {
			l.advance()
			sb.WriteString(l.consumeEscape(pos))
			continue
		}
		sb.WriteByte(ch)
		l.advance()
	}
}

// consumeEscape decodes the escape sequence that follows a backslash.
func (l *Lexer) consumeEscape(strPos source.Pos) string {
	if l.pos >= len(l.file.Text) {
		l.errorf(strPos, "unterminated string literal")
		return "\\"
	}
	ch := l.peek()
	l.advance()
	switch ch {
	case 'n':
		return "\n"
	case 't':
		return "\t"
	case 'r':
		return "\r"
	case '0':
		return "\x00"
	case 'x':
		return l.consumeHexEscape(strPos)
	case '\\':
		return "\\"
	case '"':
		return "\""
	case '\'':
		return "'"
	default:
		l.errorf(l.currentPos(), "unknown escape sequence '\\%c'", ch)
		return string(ch)
	}
}

// consumeHexEscape decodes a "\xNN" byte escape.
func (l *Lexer) consumeHexEscape(strPos source.Pos) string {
	hi, ok1 := l.peekHex()
	l.advance()
	lo, ok2 := l.peekHex()
	l.advance()
	if !ok1 || !ok2 {
		l.errorf(l.currentPos(), "expected two hex digits in '\\x' escape")
		return "\x00"
	}
	return string(byte(hi<<4 | lo))
}

func (l *Lexer) peekHex() (byte, bool) {
	ch := l.peek()
	switch {
	case ch >= '0' && ch <= '9':
		return ch - '0', true
	case ch >= 'a' && ch <= 'f':
		return ch - 'a' + 10, true
	case ch >= 'A' && ch <= 'F':
		return ch - 'A' + 10, true
	}
	return 0, false
}

func (l *Lexer) skipLineComment() {
	for l.pos < len(l.file.Text) && l.peek() != '\n' {
		l.advance()
	}
}

func (l *Lexer) skipBlockComment() {
	start := l.currentPos()
	l.advance() // /
	l.advance() // *
	for {
		if l.pos >= len(l.file.Text) {
			l.errorf(start, "unterminated block comment")
			return
		}
		if l.peek() == '*' && l.peekAt(1) == '/' {
			l.advance()
			l.advance()
			return
		}
		l.advance()
	}
}

func (l *Lexer) currentPos() source.Pos {
	return source.Pos{Line: l.line, Column: l.col, Offset: l.pos}
}

func (l *Lexer) peek() byte {
	if l.pos < len(l.file.Text) {
		return l.file.Text[l.pos]
	}
	return 0
}

func (l *Lexer) peekAt(n int) byte {
	if l.pos+n < len(l.file.Text) {
		return l.file.Text[l.pos+n]
	}
	return 0
}

func (l *Lexer) advance() {
	if l.pos >= len(l.file.Text) {
		return
	}
	if l.file.Text[l.pos] == '\n' {
		l.line++
		l.col = 1
	} else {
		l.col++
	}
	l.pos++
}

// advanceRune advances over one full UTF-8 rune.
func (l *Lexer) advanceRune() {
	if l.pos >= len(l.file.Text) {
		return
	}
	_, size := utf8.DecodeRuneInString(l.file.Text[l.pos:])
	for i := 0; i < size && l.pos < len(l.file.Text); i++ {
		l.advance()
	}
}

func (l *Lexer) errorf(pos source.Pos, format string, args ...any) {
	l.diags = append(l.diags, diag.Diagnostic{
		Severity: diag.SeverityError,
		Message:  fmt.Sprintf(format, args...),
		File:     l.file,
		Pos:      pos,
	})
}

func isIdentStart(ch byte) bool {
	return ch == '_' || (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z')
}

func isIdentPart(ch byte) bool {
	return isIdentStart(ch) || isDigit(ch)
}

func isDigit(ch byte) bool {
	return ch >= '0' && ch <= '9'
}
