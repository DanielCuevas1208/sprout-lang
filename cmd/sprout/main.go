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
	case "build":
		return runBuild(args[1:])
	case "modules":
		return runModules(args[1:])
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
	fmt.Fprintln(w, "  sprout build <file.spr>    bundle a program and its imports into one file")
	fmt.Fprintln(w, "  sprout modules <file.spr>  show the module graph of a program")
	fmt.Fprintln(w, "  sprout repl                start an interactive session")
	fmt.Fprintln(w, "  sprout lex <file.spr>      show the tokens of a file")
	fmt.Fprintln(w, "  sprout parse <file.spr>    show the syntax tree of a file")
	fmt.Fprintln(w, "  sprout check <file.spr>    check a file without running it")
	fmt.Fprintln(w, "  sprout version             show the version")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Options for run, vm, dis, lex, parse, check, and modules:")
	fmt.Fprintln(w, "  -color auto|always|never   control colored diagnostics")
	fmt.Fprintln(w, "Options for build:")
	fmt.Fprintln(w, "  -o <file>                  write the bundle to <file>")
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

// loadBundle reads path and resolves its imports, then checks the whole
// graph. It returns a reporter for the calling command.
func loadBundle(path string, rep *diag.Reporter) (*module.Bundle, int) {
	bundle, diags := module.Load(path)
	if hasErrors(diags) {
		rep.Write(os.Stderr, diags)
		return nil, 1
	}
	if diags := checker.CheckBundle(bundle); hasErrors(diags) {
		rep.Write(os.Stderr, diags)
		return nil, 1
	}
	return bundle, 0
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
	path, color, code := parseArgs("run", args)
	if code != 0 {
		return code
	}
	rep := &diag.Reporter{Color: color}
	bundle, code := loadBundle(path, rep)
	if code != 0 {
		return code
	}

	iv := interp.NewWithIO(os.Stdin, os.Stdout, os.Stderr)
	_, rerr := iv.ExecBundle(bundle)
	if rerr != nil {
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
	bundle, code := loadBundle(path, rep)
	if code != 0 {
		return code
	}

	compiled, err := compiler.CompileBundle(bundle)
	if err != nil {
		rep.Write(os.Stderr, []diag.Diagnostic{{
			Severity: diag.SeverityError,
			Message:  err.Error(),
			File:     bundle.Source(),
		}})
		return 1
	}

	machine := vm.NewWithIO(os.Stdin, os.Stdout, os.Stderr)
	_, rerr := machine.Run(bundle.Source(), compiled)
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
	bundle, code := loadBundle(path, rep)
	if code != 0 {
		return code
	}

	compiled, err := compiler.CompileBundle(bundle)
	if err != nil {
		rep.Write(os.Stderr, []diag.Diagnostic{{
			Severity: diag.SeverityError,
			Message:  err.Error(),
			File:     bundle.Source(),
		}})
		return 1
	}
	for _, fn := range compiled.Modules {
		printFunction(os.Stdout, fn, "")
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
	if _, code := loadBundle(path, rep); code != 0 {
		return code
	}
	fmt.Println("ok")
	return 0
}

// runBuild bundles a program into a single self-contained source file.
//
// The flags -o and -color may appear before or after the file argument, so
// "sprout build file.spr -o out.spr" works like "sprout build -o out.spr
// file.spr".
func runBuild(args []string) int {
	outPath := ""
	colorMode := "auto"
	file := ""
	for i := 0; i < len(args); i++ {
		switch a := args[i]; a {
		case "-o":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "sprout: build expects a path after -o")
				return 2
			}
			outPath = args[i+1]
			i++
		case "-color":
			if i+1 >= len(args) {
				fmt.Fprintln(os.Stderr, "sprout: build expects a mode after -color")
				return 2
			}
			colorMode = args[i+1]
			i++
		default:
			if strings.HasPrefix(a, "-") {
				fmt.Fprintf(os.Stderr, "sprout: build: unknown flag %s\n", a)
				return 2
			}
			if file != "" {
				fmt.Fprintln(os.Stderr, "sprout: build expects one file")
				return 2
			}
			file = a
		}
	}
	if file == "" {
		fmt.Fprintln(os.Stderr, "sprout: build expects one file")
		return 2
	}
	rep := &diag.Reporter{Color: colorEnabled(colorMode)}
	bundle, code := loadBundle(file, rep)
	if code != 0 {
		return code
	}

	out := outPath
	if out == "" {
		out = defaultBundleName(file)
	}
	if dir := filepath.Dir(out); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "sprout: cannot create %s: %v\n", dir, err)
			return 1
		}
	}
	src := bundle.Render()
	if err := os.WriteFile(out, []byte(src), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "sprout: cannot write %s: %v\n", out, err)
		return 1
	}
	fmt.Println("built " + out)
	return 0
}

// runModules prints the module graph of a program.
func runModules(args []string) int {
	path, color, code := parseArgs("modules", args)
	if code != 0 {
		return code
	}
	rep := &diag.Reporter{Color: color}
	bundle, code := loadBundle(path, rep)
	if code != 0 {
		return code
	}
	for _, f := range bundle.Files {
		role := "module"
		if f.Main {
			role = "main"
		}
		fmt.Printf("%2d  %-6s  %s\n", f.Index, role, f.Path)
		for _, stmt := range f.Prog.Stmts {
			if imp, ok := stmt.(*ast.ImportStmt); ok {
				if imp.Alias != nil {
					fmt.Printf("         import %s from %q\n", imp.Alias.Name, imp.Path)
				} else {
					fmt.Printf("         import %q\n", imp.Path)
				}
			}
		}
		for _, name := range f.Exports {
			fmt.Printf("         export %s\n", name)
		}
	}
	return 0
}

// defaultBundleName derives an output path for a bundled program.
func defaultBundleName(path string) string {
	base := strings.TrimSuffix(path, ".spr")
	return base + ".bundle.spr"
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
