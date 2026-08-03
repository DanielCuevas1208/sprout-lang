// Command sprout runs the Sprout language.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/sprout-lang/sprout/internal/ast"
	"github.com/sprout-lang/sprout/internal/bundle"
	"github.com/sprout-lang/sprout/internal/code"
	"github.com/sprout-lang/sprout/internal/diag"
	"github.com/sprout-lang/sprout/internal/interp"
	"github.com/sprout-lang/sprout/internal/lexer"
	"github.com/sprout-lang/sprout/internal/parser"
	"github.com/sprout-lang/sprout/internal/repl"
	"github.com/sprout-lang/sprout/internal/runner"
	"github.com/sprout-lang/sprout/internal/source"
	"github.com/sprout-lang/sprout/internal/token"
	"github.com/sprout-lang/sprout/internal/vm"
)

const version = "0.3.0"

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
	case "check":
		return runCheck(args[1:])
	case "build":
		return runBuild(args[1:])
	case "repl":
		return runRepl()
	case "lex":
		return runLex(args[1:])
	case "parse":
		return runParse(args[1:])
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
	fmt.Fprintln(w, "  sprout check <file.spr>    check a file without running it")
	fmt.Fprintln(w, "  sprout build <file.spr>    write a bytecode bundle")
	fmt.Fprintln(w, "  sprout repl                start an interactive session")
	fmt.Fprintln(w, "  sprout lex <file.spr>      show the tokens of a file")
	fmt.Fprintln(w, "  sprout parse <file.spr>    show the syntax tree of a file")
	fmt.Fprintln(w, "  sprout version             show the version")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Options for run, vm, dis, lex, parse, check, and build:")
	fmt.Fprintln(w, "  -color auto|always|never   control colored diagnostics")
	fmt.Fprintln(w, "  build -o <path>            set the output bundle path")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "run and vm also run bytecode bundles (files ending in .sprc).")
}

func looksLikeFile(name string) bool {
	if strings.HasSuffix(name, ".spr") {
		return true
	}
	_, err := os.Stat(name)
	return err == nil
}

func readSource(path string) (*source.File, int) {
	text, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sprout: cannot read %s: %v\n", path, err)
		return nil, 1
	}
	return source.NewFile(path, string(text)), 0
}

func parseArgs(desc string, args []string) (string, bool, int) {
	fs := flag.NewFlagSet(desc, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	colorMode := fs.String("color", "auto", "color output: auto, always, or never")
	if err := fs.Parse(reorderFlags(args, colorValueFlag)); err != nil {
		return "", false, 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintf(os.Stderr, "sprout: %s expects one file\n", desc)
		return "", false, 2
	}
	return fs.Arg(0), colorEnabled(*colorMode), 0
}

// reorderFlags moves flags before positional arguments.
//
// The flag package stops parsing at the first positional argument. Moving
// flags to the front lets options appear in any position on the command
// line. valueFlags names the options that take the next argument as their
// value.
func reorderFlags(args []string, valueFlags map[string]bool) []string {
	var flags, rest []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		if strings.HasPrefix(a, "-") && !strings.HasPrefix(a, "--") {
			flags = append(flags, a)
			if !strings.Contains(a, "=") && valueFlags[a] && i+1 < len(args) {
				flags = append(flags, args[i+1])
				i++
			}
			continue
		}
		rest = append(rest, a)
	}
	return append(flags, rest...)
}

var colorValueFlag = map[string]bool{"-color": true}
var buildValueFlags = map[string]bool{"-color": true, "-o": true}

func runFile(args []string) int {
	path, color, code := parseArgs("run", args)
	if code != 0 {
		return code
	}
	rep := &diag.Reporter{Color: color}
	if strings.HasSuffix(path, ".sprc") {
		return runBundle(path, rep)
	}

	graph, diags := runner.Load(path)
	rep.Write(os.Stderr, diags)
	if hasErrors(diags) {
		return 1
	}
	if rerr := runner.RunInterp(graph, os.Stdin, os.Stdout, os.Stderr); rerr != nil {
		printRunError(os.Stderr, rep, rerr.Message, rerr.File, rerr.Pos, rerr.Frames)
		return 1
	}
	return 0
}

func runVM(args []string) int {
	path, color, code := parseArgs("vm", args)
	if code != 0 {
		return code
	}
	rep := &diag.Reporter{Color: color}
	if strings.HasSuffix(path, ".sprc") {
		return runBundle(path, rep)
	}

	graph, diags := runner.Load(path)
	rep.Write(os.Stderr, diags)
	if hasErrors(diags) {
		return 1
	}
	if rerr := runner.RunVM(graph, os.Stdin, os.Stdout, os.Stderr); rerr != nil {
		printRunError(os.Stderr, rep, rerr.Message, rerr.File, rerr.Pos, rerr.Frames)
		return 1
	}
	return 0
}

// runBundle executes a bytecode bundle on the virtual machine.
func runBundle(path string, rep *diag.Reporter) int {
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sprout: cannot read %s: %v\n", path, err)
		return 1
	}
	compiled, err := bundle.Decode(data)
	if err != nil {
		rep.Write(os.Stderr, []diag.Diagnostic{{Severity: diag.SeverityError, Message: err.Error()}})
		return 1
	}
	machine := vm.NewWithIO(os.Stdin, os.Stdout, os.Stderr)
	_, rerr := machine.Run(compiled.EntryFile(), compiled)
	if rerr != nil {
		printRunError(os.Stderr, rep, rerr.Message, rerr.File, rerr.Pos, rerr.Frames)
		return 1
	}
	return 0
}

func runDis(args []string) int {
	path, color, code := parseArgs("dis", args)
	if code != 0 {
		return code
	}
	rep := &diag.Reporter{Color: color}

	graph, diags := runner.Load(path)
	rep.Write(os.Stderr, diags)
	if hasErrors(diags) {
		return 1
	}
	compiled, err := runner.Compile(graph)
	if err != nil {
		rep.Write(os.Stderr, []diag.Diagnostic{{Severity: diag.SeverityError, Message: err.Error()}})
		return 1
	}
	for i, m := range compiled.Modules {
		if i > 0 {
			fmt.Fprintln(os.Stdout)
		}
		fmt.Fprintf(os.Stdout, "== module %s ==\n", m.Path)
		printFunction(os.Stdout, m.Init, "")
	}
	if len(compiled.Modules) > 0 {
		fmt.Fprintln(os.Stdout)
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
	rep := &diag.Reporter{Color: color}

	_, diags := runner.Load(path)
	rep.Write(os.Stderr, diags)
	if hasErrors(diags) {
		return 1
	}
	fmt.Println("ok")
	return 0
}

// runBuild compiles a program and writes a bytecode bundle.
func runBuild(args []string) int {
	fs := flag.NewFlagSet("build", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	colorMode := fs.String("color", "auto", "color output: auto, always, or never")
	out := fs.String("o", "", "output bundle path")
	if err := fs.Parse(reorderFlags(args, buildValueFlags)); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintf(os.Stderr, "sprout: build expects one file\n")
		return 2
	}
	path := fs.Arg(0)
	rep := &diag.Reporter{Color: colorEnabled(*colorMode)}

	graph, diags := runner.Load(path)
	rep.Write(os.Stderr, diags)
	if hasErrors(diags) {
		return 1
	}
	compiled, err := runner.Compile(graph)
	if err != nil {
		rep.Write(os.Stderr, []diag.Diagnostic{{Severity: diag.SeverityError, Message: err.Error()}})
		return 1
	}
	data, err := bundle.Encode(compiled)
	if err != nil {
		rep.Write(os.Stderr, []diag.Diagnostic{{Severity: diag.SeverityError, Message: err.Error()}})
		return 1
	}

	outPath := *out
	if outPath == "" {
		outPath = strings.TrimSuffix(path, ".spr") + ".sprc"
		if outPath == path {
			outPath = path + ".sprc"
		}
	}
	if err := os.WriteFile(outPath, data, 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "sprout: cannot write %s: %v\n", outPath, err)
		return 1
	}
	fmt.Fprintf(os.Stdout, "built %s (%d modules, %d bytes)\n", outPath, len(compiled.Modules), len(data))
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
