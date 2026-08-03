// Command sprout runs the Sprout language.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/sprout-lang/sprout/internal/ast"
	"github.com/sprout-lang/sprout/internal/checker"
	"github.com/sprout-lang/sprout/internal/code"
	"github.com/sprout-lang/sprout/internal/compiler"
	"github.com/sprout-lang/sprout/internal/diag"
	"github.com/sprout-lang/sprout/internal/interp"
	"github.com/sprout-lang/sprout/internal/lexer"
	"github.com/sprout-lang/sprout/internal/parser"
	"github.com/sprout-lang/sprout/internal/repl"
	"github.com/sprout-lang/sprout/internal/source"
	"github.com/sprout-lang/sprout/internal/token"
	"github.com/sprout-lang/sprout/internal/vm"
)

const version = "0.2.0"

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 {
		return runRepl()
	}
	switch args[0] {
	case "run":
		return runFile(args[1:])
	case "vm":
		return runVM(args[1:])
	case "dis":
		return runDis(args[1:])
	case "repl":
		return runRepl()
	case "lex":
		return runLex(args[1:])
	case "parse":
		return runParse(args[1:])
	case "check":
		return runCheck(args[1:])
	case "version", "--version", "-v":
		fmt.Println("sprout " + version)
		return 0
	case "help", "--help", "-h":
		usage(os.Stdout)
		return 0
	default:
		if looksLikeFile(args[0]) {
			return runFile(args)
		}
		fmt.Fprintf(os.Stderr, "sprout: unknown command %q\n\n", args[0])
		usage(os.Stderr)
		return 2
	}
}

func usage(w io.Writer) {
	fmt.Fprintln(w, "sprout - a friendly little programming language")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Usage:")
	fmt.Fprintln(w, "  sprout [file.spr]          run a program (short for 'run')")
	fmt.Fprintln(w, "  sprout run <file.spr>      run a program on the interpreter")
	fmt.Fprintln(w, "  sprout vm <file.spr>       run a program on the bytecode VM")
	fmt.Fprintln(w, "  sprout dis <file.spr>      show the compiled bytecode")
	fmt.Fprintln(w, "  sprout repl                start an interactive session")
	fmt.Fprintln(w, "  sprout lex <file.spr>      show the tokens of a file")
	fmt.Fprintln(w, "  sprout parse <file.spr>    show the syntax tree of a file")
	fmt.Fprintln(w, "  sprout check <file.spr>    check a file without running it")
	fmt.Fprintln(w, "  sprout version             show the version")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Options for run, vm, dis, lex, parse, and check:")
	fmt.Fprintln(w, "  -color auto|always|never   control colored diagnostics")
}

func looksLikeFile(name string) bool {
	if strings.HasSuffix(name, ".spr") {
		return true
	}
	_, err := os.Stat(name)
	return err == nil
}

type fileRunner func(path string, rep *diag.Reporter) int

func readSource(path string) (*source.File, int) {
	text, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sprout: cannot read %s: %v\n", path, err)
		return nil, 1
	}
	return source.NewFile(path, string(text)), 0
}

// parseCheckArgs reads a file argument and returns a reporter for it.
//
// The caller still parses and checks the source. Sharing this step keeps the
// color flag and file handling identical across the run-style commands.
func parseCheckArgs(desc string, args []string) (*source.File, *diag.Reporter, int) {
	path, color, code := parseArgs(desc, args)
	if code != 0 {
		return nil, nil, code
	}
	file, code := readSource(path)
	if code != 0 {
		return nil, nil, code
	}
	return file, &diag.Reporter{Color: color}, 0
}

func parseArgs(desc string, args []string) (string, bool, int) {
	fs := flag.NewFlagSet(desc, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	colorMode := fs.String("color", "auto", "color output: auto, always, or never")
	if err := fs.Parse(args); err != nil {
		return "", false, 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintf(os.Stderr, "sprout: %s expects one file\n", desc)
		return "", false, 2
	}
	return fs.Arg(0), colorEnabled(*colorMode), 0
}

func runFile(args []string) int {
	file, rep, code := parseCheckArgs("run", args)
	if code != 0 {
		return code
	}
	prog, diags := parser.Parse(file)
	if hasErrors(diags) {
		rep.Write(os.Stderr, diags)
		return 1
	}
	if diags := checker.Check(file, prog); hasErrors(diags) {
		rep.Write(os.Stderr, diags)
		return 1
	}

	iv := interp.NewWithIO(os.Stdin, os.Stdout, os.Stderr)
	_, rerr := iv.Exec(file, prog)
	if rerr != nil {
		printRunError(os.Stderr, rep, rerr.Message, rerr.File, rerr.Pos, rerr.Frames)
		return 1
	}
	return 0
}

func runVM(args []string) int {
	file, rep, code := parseCheckArgs("vm", args)
	if code != 0 {
		return code
	}
	prog, diags := parser.Parse(file)
	if hasErrors(diags) {
		rep.Write(os.Stderr, diags)
		return 1
	}
	if diags := checker.Check(file, prog); hasErrors(diags) {
		rep.Write(os.Stderr, diags)
		return 1
	}

	compiled, err := compiler.Compile(file, prog)
	if err != nil {
		rep.Write(os.Stderr, []diag.Diagnostic{{
			Severity: diag.SeverityError,
			Message:  err.Error(),
			File:     file,
		}})
		return 1
	}

	machine := vm.NewWithIO(os.Stdin, os.Stdout, os.Stderr)
	_, rerr := machine.Run(file, compiled)
	if rerr != nil {
		printRunError(os.Stderr, rep, rerr.Message, rerr.File, rerr.Pos, rerr.Frames)
		return 1
	}
	return 0
}

func runDis(args []string) int {
	file, rep, code := parseCheckArgs("dis", args)
	if code != 0 {
		return code
	}
	prog, diags := parser.Parse(file)
	if hasErrors(diags) {
		rep.Write(os.Stderr, diags)
		return 1
	}
	if diags := checker.Check(file, prog); hasErrors(diags) {
		rep.Write(os.Stderr, diags)
		return 1
	}

	compiled, err := compiler.Compile(file, prog)
	if err != nil {
		rep.Write(os.Stderr, []diag.Diagnostic{{
			Severity: diag.SeverityError,
			Message:  err.Error(),
			File:     file,
		}})
		return 1
	}
	printFunction(os.Stdout, compiled.Main, "")
	return 0
}

// printFunction writes one compiled function and its nested functions.
func printFunction(w io.Writer, fn *code.Function, indent string) {
	name := fn.Name
	if name == "" {
		name = "<anonymous>"
	}
	fmt.Fprintf(w, "%s== fn %s ==\n", indent, name)
	fmt.Fprintf(w, "%sParams: %s\n", indent, strings.Join(fn.ParamNames, ", "))
	fmt.Fprintf(w, "%sSlots:  %d\n", indent, fn.NumSlots)
	fmt.Fprint(w, code.Disassemble(fn))
	for _, c := range fn.Consts {
		if f, ok := c.(*code.Function); ok {
			printFunction(w, f, indent+"  ")
		}
	}
}

func runLex(args []string) int {
	path, color, code := parseArgs("lex", args)
	if code != 0 {
		return code
	}
	file, code := readSource(path)
	if code != 0 {
		return code
	}
	rep := &diag.Reporter{Color: color}

	toks, diags := lexer.New(file).Tokenize()
	if hasErrors(diags) {
		rep.Write(os.Stderr, diags)
		return 1
	}
	for _, t := range toks {
		lexeme := t.Lexeme
		if t.Kind == token.STRING {
			lexeme = fmt.Sprintf("%q", t.Value)
		}
		fmt.Printf("%d:%d  %-12s %s\n", t.Pos.Line, t.Pos.Column, t.Kind, lexeme)
	}
	return 0
}

func runParse(args []string) int {
	path, color, code := parseArgs("parse", args)
	if code != 0 {
		return code
	}
	file, code := readSource(path)
	if code != 0 {
		return code
	}
	rep := &diag.Reporter{Color: color}

	prog, diags := parser.Parse(file)
	if hasErrors(diags) {
		rep.Write(os.Stderr, diags)
		return 1
	}
	fmt.Println(ast.Sexp(prog))
	return 0
}

func runCheck(args []string) int {
	path, color, code := parseArgs("check", args)
	if code != 0 {
		return code
	}
	file, code := readSource(path)
	if code != 0 {
		return code
	}
	rep := &diag.Reporter{Color: color}

	prog, diags := parser.Parse(file)
	if hasErrors(diags) {
		rep.Write(os.Stderr, diags)
		return 1
	}
	if diags := checker.Check(file, prog); len(diags) > 0 {
		rep.Write(os.Stderr, diags)
		if hasErrors(diags) {
			return 1
		}
	}
	fmt.Println("ok")
	return 0
}

func runRepl() int {
	iv := interp.NewWithIO(os.Stdin, os.Stdout, os.Stderr)
	if err := repl.Run(iv, os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "sprout: %v\n", err)
		return 1
	}
	return 0
}

func printRunError(w io.Writer, rep *diag.Reporter, message string, file *source.File, pos source.Pos, frames []diag.Frame) {
	rep.Write(w, []diag.Diagnostic{{
		Severity: diag.SeverityError,
		Message:  message,
		File:     file,
		Pos:      pos,
	}})
	for _, f := range frames {
		fmt.Fprintf(w, "   at %s (%s:%d:%d)\n", f.Name, f.FileName, f.Pos.Line, f.Pos.Column)
	}
}

func hasErrors(diags []diag.Diagnostic) bool {
	for _, d := range diags {
		if d.Severity == diag.SeverityError {
			return true
		}
	}
	return false
}

func colorEnabled(mode string) bool {
	switch mode {
	case "always":
		return true
	case "never":
		return false
	}
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	return isTerminal(os.Stderr)
}

func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeCharDevice != 0
}
