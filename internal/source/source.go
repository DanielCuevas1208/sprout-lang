// Package source models source files and the positions inside them.
package source

import "strings"

// Pos is a position in a source file.
//
// Line and Column are 1-based. Offset is a 0-based byte offset.
type Pos struct {
	Line   int
	Column int
	Offset int
}

// IsValid reports whether p points into a source file.
func (p Pos) IsValid() bool {
	return p.Line > 0
}

// File is a named chunk of source text.
type File struct {
	Name  string
	Text  string
	lines []int // byte offset of the start of each line
}

// NewFile returns a File for the given name and text.
func NewFile(name, text string) *File {
	f := &File{Name: name, Text: text}
	f.indexLines()
	return f
}

func (f *File) indexLines() {
	f.lines = []int{0}
	for i := 0; i < len(f.Text); i++ {
		if f.Text[i] == '\n' {
			f.lines = append(f.lines, i+1)
		}
	}
}

// LineCount returns the number of lines in the file.
func (f *File) LineCount() int {
	return len(f.lines)
}

// LineStart returns the byte offset of the first character of line.
func (f *File) LineStart(line int) int {
	if line <= 1 {
		return 0
	}
	if line > len(f.lines) {
		return len(f.Text)
	}
	return f.lines[line-1]
}

// LineEnd returns the byte offset just past the last character of line.
func (f *File) LineEnd(line int) int {
	if line >= len(f.lines) {
		return len(f.Text)
	}
	return f.lines[line]
}

// Line returns the text of line without its trailing newline.
func (f *File) Line(line int) string {
	start := f.LineStart(line)
	end := f.LineEnd(line)
	s := f.Text[start:end]
	s = strings.TrimSuffix(s, "\n")
	return strings.TrimSuffix(s, "\r")
}
