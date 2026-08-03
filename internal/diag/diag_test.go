package diag

import (
	"strings"
	"testing"

	"github.com/sprout-lang/sprout/internal/source"
)

func render(d Diagnostic) string {
	return (&Reporter{}).Render(&d)
}

func TestRenderSingleLine(t *testing.T) {
	file := source.NewFile("demo.spr", "let x = 5\nprint(y)\n")
	d := Diagnostic{
		Severity: SeverityError,
		Message:  "undefined name 'y'",
		File:     file,
		Pos:      source.Pos{Line: 2, Column: 7, Offset: 13},
	}
	want := `error: undefined name 'y'
  --> demo.spr:2:7
  |
2 | print(y)
  |       ^
`
	if got := render(d); got != want {
		t.Errorf("render:\n got:\n%q\nwant:\n%q", got, want)
	}
}

func TestRenderSpan(t *testing.T) {
	file := source.NewFile("demo.spr", "let total = [1, 2]\n")
	d := Diagnostic{
		Severity: SeverityError,
		Message:  "expected ']'",
		File:     file,
		Pos:      source.Pos{Line: 1, Column: 14},
		End:      source.Pos{Line: 1, Column: 18},
	}
	got := render(d)
	if !strings.Contains(got, "^^^^") {
		t.Errorf("expected a 4-wide caret, got:\n%s", got)
	}
}

func TestRenderWideGutter(t *testing.T) {
	file := source.NewFile("demo.spr", strings.Repeat("x\n", 9)+"print(y)\n")
	d := Diagnostic{
		Severity: SeverityError,
		Message:  "boom",
		File:     file,
		Pos:      source.Pos{Line: 10, Column: 7, Offset: 18},
	}
	got := render(d)
	if !strings.Contains(got, "10 | print(y)") {
		t.Errorf("expected two-digit gutter, got:\n%s", got)
	}
}

func TestRenderWithoutFile(t *testing.T) {
	got := render(Diagnostic{Severity: SeverityWarning, Message: "be careful"})
	if got != "warning: be careful\n" {
		t.Errorf("got %q", got)
	}
}

func TestRenderHint(t *testing.T) {
	file := source.NewFile("demo.spr", "let x = 5\n")
	d := Diagnostic{
		Severity: SeverityError,
		Message:  "cannot assign to constant",
		File:     file,
		Pos:      source.Pos{Line: 1, Column: 1},
		Hint:     "declare it with 'let' to allow changes",
	}
	got := render(d)
	if !strings.Contains(got, "note: declare it with 'let' to allow changes") {
		t.Errorf("expected hint note, got:\n%s", got)
	}
}

func TestCaretAlignsWithTabs(t *testing.T) {
	file := source.NewFile("demo.spr", "if x {\n\tprint(y)\n}\n")
	d := Diagnostic{
		Severity: SeverityError,
		Message:  "undefined name 'y'",
		File:     file,
		Pos:      source.Pos{Line: 2, Column: 8, Offset: 13},
	}
	got := render(d)
	// The caret must land under the 'y' once tabs expand to 8 columns.
	lines := strings.Split(got, "\n")
	textLine := expandTabs(lines[3])
	caretLine := expandTabs(lines[4])
	caretIdx := strings.Index(caretLine, "^")
	textIdx := strings.Index(textLine, "y")
	if caretIdx != textIdx {
		t.Errorf("caret at %d, want %d\n%s", caretIdx, textIdx, got)
	}
}

func expandTabs(s string) string {
	var b strings.Builder
	col := 0
	for _, r := range s {
		if r == '\t' {
			next := col + (8 - col%8)
			b.WriteString(strings.Repeat(" ", next-col))
			col = next
		} else {
			b.WriteRune(r)
			col++
		}
	}
	return b.String()
}
