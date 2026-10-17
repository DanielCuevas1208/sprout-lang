package lexer

import (
	"strings"
	"testing"

	"github.com/sprout-lang/sprout/internal/source"
	"github.com/sprout-lang/sprout/internal/token"
)

func lex(src string) []token.Token {
	file := source.NewFile("test.spr", src)
	toks, _ := New(file).Tokenize()
	return toks
}

func kinds(toks []token.Token) []token.Kind {
	var out []token.Kind
	for _, t := range toks {
		out = append(out, t.Kind)
	}
	return out
}

func TestBasicTokens(t *testing.T) {
	toks := lex("let x = 5 + 3.5\n")
	want := []token.Kind{
		token.LET, token.IDENT, token.ASSIGN, token.INT,
		token.PLUS, token.FLOAT, token.NEWLINE, token.EOF,
	}
	got := kinds(toks)
	if len(got) != len(want) {
		t.Fatalf("token count: got %d, want %d\n%v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("token %d: got %v, want %v", i, got[i], want[i])
		}
	}
}

func TestAllOperators(t *testing.T) {
	toks := lex("== != < <= > >= = => + - * / % ^ ( ) [ ] { } , : .\n")
	got := kinds(toks)
	want := []token.Kind{
		token.EQ, token.NEQ, token.LT, token.LE, token.GT, token.GE,
		token.ASSIGN, token.ARROW, token.PLUS, token.MINUS, token.STAR,
		token.SLASH, token.PERCENT, token.CARET, token.LPAREN, token.RPAREN,
		token.LBRACKET, token.RBRACKET, token.LBRACE, token.RBRACE,
		token.COMMA, token.COLON, token.DOT, token.NEWLINE, token.EOF,
	}
	if len(got) != len(want) {
		t.Fatalf("token count: got %d, want %d\n%v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("token %d: got %v, want %v", i, got[i], want[i])
		}
	}
}

func TestKeywords(t *testing.T) {
	src := "let const fn if elif else for in while return break continue true false nil and or not import export as struct interface match\n"
	toks := lex(src)
	got := kinds(toks)
	want := []token.Kind{
		token.LET, token.CONST, token.FN, token.IF, token.ELIF, token.ELSE,
		token.FOR, token.IN, token.WHILE, token.RETURN, token.BREAK,
		token.CONTINUE, token.TRUE, token.FALSE, token.NIL, token.AND,
		token.OR, token.NOT, token.IMPORT, token.EXPORT, token.AS,
		token.STRUCT, token.INTERFACE, token.MATCH,
		token.NEWLINE, token.EOF,
	}
	if len(got) != len(want) {
		t.Fatalf("token count: got %d, want %d\n%v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("token %d: got %v, want %v", i, got[i], want[i])
		}
	}
}

func TestHexEscape(t *testing.T) {
	toks := lex(`"a\x41b" "byte\x00end"`)
	var values []string
	for _, t := range toks {
		if t.Kind == token.STRING {
			values = append(values, t.Value)
		}
	}
	want := []string{"aAb", "byte\x00end"}
	if len(values) != len(want) {
		t.Fatalf("string count: got %d, want %d", len(values), len(want))
	}
	for i := range want {
		if values[i] != want[i] {
			t.Errorf("string %d: got %q, want %q", i, values[i], want[i])
		}
	}
}

func TestBadHexEscape(t *testing.T) {
	if msgs := lexErrors(`"\xZZ"`); len(msgs) == 0 {
		t.Error("expected an error for a malformed hex escape")
	}
}

func TestStringLexemes(t *testing.T) {
	toks := lex(`"hello\nworld" "tab\there" "quote: \""`)
	var values []string
	for _, t := range toks {
		if t.Kind == token.STRING {
			values = append(values, t.Value)
		}
	}
	want := []string{"hello\nworld", "tab\there", `quote: "`}
	if len(values) != len(want) {
		t.Fatalf("string count: got %d, want %d", len(values), len(want))
	}
	for i := range want {
		if values[i] != want[i] {
			t.Errorf("string %d: got %q, want %q", i, values[i], want[i])
		}
	}
}

func TestRawString(t *testing.T) {
	toks := lex("`line one\nline two`\n")
	if toks[0].Kind != token.STRING {
		t.Fatalf("first token: got %v, want STRING", toks[0].Kind)
	}
	if toks[0].Value != "line one\nline two" {
		t.Errorf("raw value: got %q", toks[0].Value)
	}
}

func TestNumbers(t *testing.T) {
	toks := lex("0 42 1_000_000 3.14 1e3 2.5e-2\n")
	var lexemes []string
	for _, t := range toks {
		if t.Kind == token.INT || t.Kind == token.FLOAT {
			lexemes = append(lexemes, t.Lexeme)
		}
	}
	want := []string{"0", "42", "1_000_000", "3.14", "1e3", "2.5e-2"}
	if len(lexemes) != len(want) {
		t.Fatalf("numbers: got %v, want %v", lexemes, want)
	}
	for i := range want {
		if lexemes[i] != want[i] {
			t.Errorf("number %d: got %q, want %q", i, lexemes[i], want[i])
		}
	}
}

func TestCommentsSkipped(t *testing.T) {
	src := `let x = 1 // line comment
/* block
   comment */
let y = 2`
	toks := lex(src)
	var names []string
	for _, t := range toks {
		if t.Kind == token.IDENT {
			names = append(names, t.Lexeme)
		}
	}
	if len(names) != 2 || names[0] != "x" || names[1] != "y" {
		t.Fatalf("identifiers: got %v, want [x y]", names)
	}
}

func TestPositions(t *testing.T) {
	toks := lex("let x\n+ 5")
	if toks[0].Pos.Line != 1 || toks[0].Pos.Column != 1 {
		t.Errorf("let position: %+v", toks[0].Pos)
	}
	if toks[2].Kind != token.NEWLINE {
		t.Fatalf("token 2: got %v, want NEWLINE", toks[2].Kind)
	}
	if toks[2].Pos.Line != 1 || toks[2].Pos.Column != 6 {
		t.Errorf("newline position: %+v", toks[2].Pos)
	}
	if toks[3].Pos.Line != 2 || toks[3].Pos.Column != 1 {
		t.Errorf("plus position: %+v", toks[3].Pos)
	}
}

func lexErrors(src string) []string {
	file := source.NewFile("test.spr", src)
	_, diags := New(file).Tokenize()
	var msgs []string
	for _, d := range diags {
		msgs = append(msgs, d.Message)
	}
	return msgs
}

func TestErrors(t *testing.T) {
	cases := []struct {
		src  string
		want string
	}{
		{`"unterminated`, "unterminated string literal"},
		{"/* never closed", "unterminated block comment"},
		{`let x = #`, "unexpected character"},
		{`1e`, "expected digits in exponent"},
		{`let x = $`, "unexpected character"},
	}
	for _, c := range cases {
		msgs := lexErrors(c.src)
		found := false
		for _, m := range msgs {
			if strings.Contains(m, c.want) {
				found = true
			}
		}
		if !found {
			t.Errorf("%q: expected error containing %q, got %v", c.src, c.want, msgs)
		}
	}
}
