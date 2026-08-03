// Package diag models source diagnostics and renders them for humans.
//
// A diagnostic carries a message, a severity, and a position in a source
// file. The Reporter renders diagnostics in a rustc-inspired format with a
// gutter, the offending source line, and a caret under the span.
package diag

import (
	"fmt"
	"io"
	"strings"

	"github.com/sprout-lang/sprout/internal/source"
)

// Severity ranks a diagnostic.
type Severity int

const (
	SeverityError Severity = iota
	SeverityWarning
	SeverityNote
)

func (s Severity) String() string {
	switch s {
	case SeverityWarning:
		return "warning"
	case SeverityNote:
		return "note"
	default:
		return "error"
	}
}

// Diagnostic is a single message attached to a source position.
type Diagnostic struct {
	Severity Severity
	Message  string
	File     *source.File
	Pos      source.Pos
	// End is the optional end of the span. It is zero when absent.
	End source.Pos
	// Hint is optional secondary text shown below the span.
	Hint string
}

// Frame is one call-stack entry attached to a runtime error.
type Frame struct {
	Name     string
	FileName string
	Pos      source.Pos
}

// Reporter renders diagnostics.
type Reporter struct {
	// Color enables ANSI colors in the rendered output.
	Color bool
}

// Write renders each diagnostic to w, separated by blank lines.
func (r *Reporter) Write(w io.Writer, ds []Diagnostic) {
	for i := range ds {
		io.WriteString(w, r.Render(&ds[i]))
		if i < len(ds)-1 {
			io.WriteString(w, "\n")
		}
	}
}

// Render returns the formatted text for d.
func (r *Reporter) Render(d *Diagnostic) string {
	var b strings.Builder
	b.WriteString(r.color(d.Severity.String(), severityColor(d.Severity)))
	b.WriteString(": ")
	b.WriteString(d.Message)
	b.WriteString("\n")

	if d.File == nil || !d.Pos.IsValid() {
		return b.String()
	}

	fmt.Fprintf(&b, "  --> %s:%d:%d\n", d.File.Name, d.Pos.Line, d.Pos.Column)

	width := len(fmt.Sprintf("%d", d.Pos.Line))
	if d.End.IsValid() && d.End.Line > d.Pos.Line {
		width = maxInt(width, len(fmt.Sprintf("%d", d.End.Line)))
	}
	gutter := strings.Repeat(" ", width)

	line := d.Pos.Line
	b.WriteString(gutter)
	b.WriteString(" |\n")
	b.WriteString(fmt.Sprintf("%*d | %s\n", width, line, d.File.Line(line)))

	carets := 1
	if d.End.IsValid() && d.End.Line == line && d.End.Column > d.Pos.Column {
		carets = d.End.Column - d.Pos.Column
	}
	// The source line starts after the "%*d | " gutter, so tab stops must be
	// computed from that starting display column.
	pad := caretPadding(d.File.Line(line), d.Pos.Column, width+3)
	b.WriteString(gutter)
	b.WriteString(" | ")
	b.WriteString(pad)
	b.WriteString(r.color(strings.Repeat("^", carets), caretColor))
	b.WriteString("\n")

	if d.Hint != "" {
		b.WriteString(r.color("note", ansiCyan))
		b.WriteString(": ")
		b.WriteString(d.Hint)
		b.WriteString("\n")
	}
	return b.String()
}

// caretPadding returns spaces that align a caret with column col in line.
//
// startCol is the display column where the line begins. Tabs expand to the
// next multiple of 8 from that column so the caret stays aligned.
func caretPadding(line string, col, startCol int) string {
	var b strings.Builder
	display := 0 // display offset within the line
	stop := startCol
	content := 0
	for _, r := range line {
		if content >= col-1 {
			break
		}
		if r == '\t' {
			next := stop + (8 - stop%8)
			n := next - stop
			b.WriteString(strings.Repeat(" ", n))
			stop = next
			display += n
		} else {
			b.WriteRune(' ')
			stop++
			display++
		}
		content++
	}
	return b.String()
}

const (
	ansiReset  = "\x1b[0m"
	ansiRed    = "\x1b[31m"
	ansiYellow = "\x1b[33m"
	ansiCyan   = "\x1b[36m"
)

func severityColor(s Severity) string {
	switch s {
	case SeverityWarning:
		return ansiYellow
	case SeverityNote:
		return ansiCyan
	default:
		return ansiRed
	}
}

const caretColor = ansiRed

func (r *Reporter) color(text, code string) string {
	if !r.Color {
		return text
	}
	return code + text + ansiReset
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
