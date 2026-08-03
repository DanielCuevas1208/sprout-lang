// Package repl provides the interactive Sprout session.
package repl

import (
	"bufio"
	"fmt"
	"io"
	"strings"

	"github.com/sprout-lang/sprout/internal/diag"
	"github.com/sprout-lang/sprout/internal/interp"
	"github.com/sprout-lang/sprout/internal/object"
	"github.com/sprout-lang/sprout/internal/parser"
	"github.com/sprout-lang/sprout/internal/source"
)

const version = "0.2.0"

// Run starts an interactive session and returns when the user leaves.
//
// A single environment persists across lines so the user can build up state.
func Run(iv *interp.Interpreter, stdin io.Reader, stdout, stderr io.Writer) error {
	fmt.Fprintf(stdout, "Sprout %s - a friendly little language\n", version)
	fmt.Fprintln(stdout, "Type :help for help, or :quit to leave.")

	sc := bufio.NewScanner(stdin)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var buffer strings.Builder
	prompt := "sprout> "
	rep := &diag.Reporter{Color: false}

	for {
		fmt.Fprint(stdout, prompt)
		if !sc.Scan() {
			fmt.Fprintln(stdout)
			return nil
		}
		line := sc.Text()

		if strings.TrimSpace(line) == "" {
			if buffer.Len() > 0 {
				flush(&buffer, stdout, stderr, iv, rep)
				prompt = "sprout> "
			}
			continue
		}

		if buffer.Len() == 0 {
			switch strings.TrimSpace(line) {
			case ":help", ":h":
				printHelp(stdout)
				continue
			case ":quit", ":q":
				return nil
			case ":version":
				fmt.Fprintf(stdout, "Sprout %s\n", version)
				continue
			}
		}

		buffer.WriteString(line)
		buffer.WriteString("\n")

		file := source.NewFile("<repl>", buffer.String())
		if wantsMore(file) {
			prompt = "......> "
			continue
		}

		flush(&buffer, stdout, stderr, iv, rep)
		prompt = "sprout> "
	}
}

// flush parses, checks, and runs the accumulated input.
func flush(buffer *strings.Builder, stdout, stderr io.Writer, iv *interp.Interpreter, rep *diag.Reporter) {
	src := buffer.String()
	buffer.Reset()

	file := source.NewFile("<repl>", src)
	prog, diags := parser.Parse(file)
	if len(diags) > 0 {
		rep.Write(stderr, diags)
		return
	}
	val, rerr := iv.Exec(file, prog)
	if rerr != nil {
		printRunError(stderr, rep, rerr)
		return
	}
	if val != nil && val.Type() != object.TypeNil {
		fmt.Fprintln(stdout, object.Repr(val))
	}
}

// wantsMore reports whether the buffered input looks incomplete.
func wantsMore(file *source.File) bool {
	_, diags := parser.Parse(file)
	for i := range diags {
		msg := diags[i].Message
		if strings.Contains(msg, "end of file") || strings.Contains(msg, "unterminated") {
			return true
		}
	}
	return false
}

func printRunError(w io.Writer, rep *diag.Reporter, rerr *interp.RunError) {
	rep.Write(w, []diag.Diagnostic{{
		Severity: diag.SeverityError,
		Message:  rerr.Message,
		File:     rerr.File,
		Pos:      rerr.Pos,
	}})
	for _, f := range rerr.Frames {
		fmt.Fprintf(w, "   at %s (%s:%d:%d)\n", f.Name, f.FileName, f.Pos.Line, f.Pos.Column)
	}
}

func printHelp(w io.Writer) {
	fmt.Fprintln(w, "Commands:")
	fmt.Fprintln(w, "  :help     show this help")
	fmt.Fprintln(w, "  :quit     leave the session")
	fmt.Fprintln(w, "  :version  show the version")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Each line is parsed and run. An expression that is not a")
	fmt.Fprintln(w, "statement prints its value.")
}
