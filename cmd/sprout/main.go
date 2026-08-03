// Command sprout runs the Sprout language.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/sprout-lang/sprout/internal/ast"
	"github.com/sprout-lang/sprout/internal/checker"
	"github.com/sprout-lang/sprout/internal/code"
	"github.com/sprout-lang/sprout/internal/compiler"
	"github.com/sprout-lang/sprout/internal/diag"
	"github.com/sprout-lang/sprout/internal/interp"
	"github.com/sprout-lang/sprout/internal/lexer"
	"github.com/sprout-lang/sprout/internal/module"
	"github.com/sprout-lang/sprout/internal/parser"
	"github.com/sprout-lang/sprout/internal/repl"
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
	case "repl":
		return runRepl()
	case "lex":
		return runLex(args[1:])
	case "parse":
		return runParse(args[1:])
	case "check":
		return runCheck(args[1:])
	case "build":
		return runBuild(args[1:])
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
	fmt.Fprintln(w, "  sprout build <file.spr>    copy a program and its modules into build/")
	fmt.Fprintln(w, "  sprout version             show the version")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Options for run, vm, dis, lex, parse, and check:")
	fmt.Fprintln(w, "  -color auto|always|never   control colored diagnostics")
	fmt.Fprintln(w, "Options for build:")
	fmt.Fprintln(w, "  -o <dir>                   set the output directory (default build)")
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
	iv.SetModuleLoader(module.NewInterpLoader(os.Stdin, os.Stdout, os.Stderr))
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
	machine.SetModuleLoader(module.NewVMLoader(os.Stdin, os.Stdout, os.Stderr))
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
	iv.SetModuleLoader(module.NewInterpLoader(os.Stdin, os.Stdout, os.Stderr))
	if err := repl.Run(iv, os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "sprout: %v\n", err)
		return 1
	}
	return 0
}

// runBuild copies a program and its module graph into an output directory.
//
// The output tree mirrors the source tree, so the built program runs as-is
// with imports resolved relative to the entry file.
func runBuild(args []string) int {
	outDir := "build"
	var paths []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "-o", "--out":
			if i+1 >= len(args) {
				fmt.Fprintf(os.Stderr, "sprout: build: %s needs a directory\n", args[i])
				return 2
			}
			i++
			outDir = args[i]
		default:
			if strings.HasPrefix(args[i], "-") {
				fmt.Fprintf(os.Stderr, "sprout: build: unknown option %s\n", args[i])
				return 2
			}
			paths = append(paths, args[i])
		}
	}
	if len(paths) != 1 {
		fmt.Fprintf(os.Stderr, "sprout: build expects one file\n")
		return 2
	}

	files, err := module.Graph(paths[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "sprout: build: %v\n", err)
		return 1
	}
	mainAbs, err := filepath.Abs(paths[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "sprout: build: cannot resolve %s\n", paths[0])
		return 1
	}
	mainDir := filepath.Dir(mainAbs)

	for _, src := range files {
		rel, err := filepath.Rel(mainDir, src)
		if err != nil || strings.HasPrefix(rel, "..") {
			fmt.Fprintf(os.Stderr, "sprout: build: module '%s' is outside the project directory\n", src)
			return 1
		}
		dest := filepath.Join(outDir, rel)
		if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "sprout: build: cannot create %s: %v\n", filepath.Dir(dest), err)
			return 1
		}
		data, err := os.ReadFile(src)
		if err != nil {
			fmt.Fprintf(os.Stderr, "sprout: build: cannot read %s: %v\n", src, err)
			return 1
		}
		if err := os.WriteFile(dest, data, 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "sprout: build: cannot write %s: %v\n", dest, err)
			return 1
		}
	}

	fmt.Printf("wrote %d file(s) to %s\n", len(files), outDir)
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
