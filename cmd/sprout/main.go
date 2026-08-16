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
	"github.com/sprout-lang/sprout/internal/build"
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

const version = "0.8.0"

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 {
		return runRepl()
	}
	switch args[0] {
	case "run":
		return runProject("run", args[1:])
	case "vm":
		return runProject("vm", args[1:])
	case "dis":
		return runProject("dis", args[1:])
	case "repl":
		return runRepl()
	case "lex":
		return runLex(args[1:])
	case "parse":
		return runParse(args[1:])
	case "check":
		return runCheck(args[1:])
	case "init":
		return runInit(args[1:])
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
			return runProject("run", args)
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
	fmt.Fprintln(w, "  sprout run [file.spr]      run a program on the interpreter")
	fmt.Fprintln(w, "  sprout vm [file.spr]       run a program on the bytecode VM")
	fmt.Fprintln(w, "  sprout dis [file.spr]      show the compiled bytecode")
	fmt.Fprintln(w, "  sprout repl                start an interactive session")
	fmt.Fprintln(w, "  sprout lex <file.spr>      show the tokens of a file")
	fmt.Fprintln(w, "  sprout parse <file.spr>    show the syntax tree of a file")
	fmt.Fprintln(w, "  sprout check [file.spr]    check a file without running it")
	fmt.Fprintln(w, "  sprout init [dir]          create a new project")
	fmt.Fprintln(w, "  sprout build [out.spr]     bundle a project into one file")
	fmt.Fprintln(w, "  sprout version             show the version")
	fmt.Fprintln(w)
	fmt.Fprintln(w, "For run, vm, dis, and check, omit the file inside a project")
	fmt.Fprintln(w, "directory to use the entry named in sprout.toml.")
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

func readSource(path string) (*source.File, int) {
	text, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sprout: cannot read %s: %v\n", path, err)
		return nil, 1
	}
	return source.NewFile(path, string(text)), 0
}

// resolveEntry returns the entry file and library directories for a command.
//
// A file argument wins. Without one, the command uses the nearest sprout.toml
// project. The color mode follows the -color flag.
func resolveEntry(desc string, args []string) (path string, libDirs []string, color bool, code int) {
	fs := flag.NewFlagSet(desc, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	colorMode := fs.String("color", "auto", "color output: auto, always, or never")
	if err := fs.Parse(args); err != nil {
		return "", nil, false, 2
	}
	switch fs.NArg() {
	case 1:
		path := fs.Arg(0)
		return path, libDirsFor(path), colorEnabled(*colorMode), 0
	case 0:
		dir, m, err := module.FindManifest(".")
		if err != nil {
			fmt.Fprintf(os.Stderr, "sprout: %s expects one file (or run it inside a project)\n", desc)
			return "", nil, false, 2
		}
		return m.EntryPath(dir), m.LibDirs(dir), colorEnabled(*colorMode), 0
	default:
		fmt.Fprintf(os.Stderr, "sprout: %s expects at most one file\n", desc)
		return "", nil, false, 2
	}
}

// libDirsFor finds the library directories of the project that owns path.
//
// The search starts at the file's own directory and walks upward. A file
// outside any project gets nil, so its bare imports only search its own
// directory. This lets a project file resolve lib modules from anywhere.
func libDirsFor(path string) []string {
	dir, m, err := module.FindManifest(filepath.Dir(path))
	if err != nil {
		return nil
	}
	return m.LibDirs(dir)
}

// loadProject reads the entry file and its modules, then checks them all.
func loadProject(desc string, args []string) (*module.Graph, *diag.Reporter, int) {
	path, libDirs, color, code := resolveEntry(desc, args)
	if code != 0 {
		return nil, nil, code
	}
	g, diags := module.Load(path, module.LoadOptions{LibDirs: libDirs})
	if hasErrors(diags) {
		(&diag.Reporter{Color: color}).Write(os.Stderr, diags)
		return nil, nil, 1
	}
	if diags := checker.CheckGraph(g); hasErrors(diags) {
		(&diag.Reporter{Color: color}).Write(os.Stderr, diags)
		return nil, nil, 1
	}
	return g, &diag.Reporter{Color: color}, 0
}

// runProject runs the run, vm, and dis commands on a loaded project.
func runProject(desc string, args []string) int {
	g, rep, code := loadProject(desc, args)
	if code != 0 {
		return code
	}

	switch desc {
	case "run":
		iv := interp.NewWithIO(os.Stdin, os.Stdout, os.Stderr)
		iv.SetModuleResolver(g.Resolve)
		_, rerr := iv.Exec(g.Entry.Source, g.Entry.Prog)
		if rerr != nil {
			printRunError(os.Stderr, rep, rerr.Message, rerr.File, rerr.Pos, rerr.Frames)
			return 1
		}
		return 0

	case "vm":
		compiled, err := compiler.CompileModules(g)
		if err != nil {
			rep.Write(os.Stderr, []diag.Diagnostic{{
				Severity: diag.SeverityError,
				Message:  err.Error(),
				File:     g.Entry.Source,
			}})
			return 1
		}
		machine := vm.NewWithIO(os.Stdin, os.Stdout, os.Stderr)
		_, rerr := machine.Run(g.Entry.Source, compiled)
		if rerr != nil {
			printRunError(os.Stderr, rep, rerr.Message, rerr.File, rerr.Pos, rerr.Frames)
			return 1
		}
		return 0

	case "dis":
		compiled, err := compiler.CompileModules(g)
		if err != nil {
			rep.Write(os.Stderr, []diag.Diagnostic{{
				Severity: diag.SeverityError,
				Message:  err.Error(),
				File:     g.Entry.Source,
			}})
			return 1
		}
		printProgram(os.Stdout, compiled)
		return 0
	}
	return 2
}

// printProgram writes the entry function and every module function.
func printProgram(w io.Writer, p *code.Program) {
	printFunction(w, p.Main, p, "")
	for _, ref := range p.Modules {
		fmt.Fprintf(w, "\n== module %s ==\n", ref.Name)
		printFunction(w, ref.Fn, p, "")
	}
}

// printFunction writes one compiled function and its nested functions.
func printFunction(w io.Writer, fn *code.Function, p *code.Program, indent string) {
	name := fn.Name
	if name == "" {
		name = "<anonymous>"
	}
	fmt.Fprintf(w, "%s== fn %s ==\n", indent, name)
	fmt.Fprintf(w, "%sParams: %s\n", indent, strings.Join(fn.ParamNames, ", "))
	fmt.Fprintf(w, "%sSlots:  %d\n", indent, fn.NumSlots)
	fmt.Fprint(w, code.DisassembleProgram(fn, p))
	for _, c := range fn.Consts {
		if f, ok := c.(*code.Function); ok {
			printFunction(w, f, p, indent+"  ")
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
	_, _, code := loadProject("check", args)
	if code != 0 {
		return code
	}
	fmt.Println("ok")
	return 0
}

func runInit(args []string) int {
	if len(args) > 1 {
		fmt.Fprintf(os.Stderr, "sprout: init expects at most one directory\n")
		return 2
	}
	dir := "."
	if len(args) == 1 {
		dir = args[0]
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "sprout: init: %v\n", err)
		return 1
	}
	name := filepath.Base(dir)
	if name == "." || name == string(filepath.Separator) {
		if cwd, err := os.Getwd(); err == nil {
			name = filepath.Base(cwd)
		}
	}

	manifest := module.DefaultManifest(name)
	if err := manifest.Write(dir); err != nil {
		fmt.Fprintf(os.Stderr, "sprout: init: %v\n", err)
		return 1
	}
	mainSrc := "// " + manifest.Entry + " - the entry point of the project.\n" +
		"//\n" +
		"// Run: sprout run\n\n" +
		"print(\"hello from " + name + "\")\n"
	if err := os.WriteFile(filepath.Join(dir, manifest.Entry), []byte(mainSrc), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "sprout: init: %v\n", err)
		return 1
	}
	if err := os.MkdirAll(filepath.Join(dir, "lib"), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "sprout: init: %v\n", err)
		return 1
	}

	fmt.Printf("created project %s\n", dir)
	fmt.Printf("  %s\n", "sprout.toml")
	fmt.Printf("  %s\n", manifest.Entry)
	fmt.Printf("  %s\n", "lib/")
	return 0
}

func runBuild(args []string) int {
	if len(args) > 1 {
		fmt.Fprintf(os.Stderr, "sprout: build expects at most one output file\n")
		return 2
	}
	outPath := ""
	if len(args) == 1 {
		outPath = args[0]
	}

	dir, m, err := module.FindManifest(".")
	if err != nil {
		fmt.Fprintf(os.Stderr, "sprout: build: %v\n", err)
		return 1
	}
	entry := m.EntryPath(dir)
	g, diags := module.Load(entry, module.LoadOptions{LibDirs: m.LibDirs(dir)})
	if hasErrors(diags) {
		(&diag.Reporter{Color: false}).Write(os.Stderr, diags)
		return 1
	}
	if diags := checker.CheckGraph(g); hasErrors(diags) {
		(&diag.Reporter{Color: false}).Write(os.Stderr, diags)
		return 1
	}
	text, err := build.Bundle(g)
	if err != nil {
		fmt.Fprintf(os.Stderr, "sprout: build: %v\n", err)
		return 1
	}
	if outPath == "" {
		outPath = filepath.Join(dir, "out", m.Name+".spr")
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "sprout: build: %v\n", err)
		return 1
	}
	if err := os.WriteFile(outPath, []byte(text), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "sprout: build: %v\n", err)
		return 1
	}
	fmt.Printf("built %s\n", outPath)
	return 0
}

func runRepl() int {
	iv := interp.NewWithIO(os.Stdin, os.Stdout, os.Stderr)
	iv.SetModuleResolver(projectResolver())
	if err := repl.Run(iv, os.Stdin, os.Stdout, os.Stderr); err != nil {
		fmt.Fprintf(os.Stderr, "sprout: %v\n", err)
		return 1
	}
	return 0
}

// projectResolver resolves imports from the current directory.
func projectResolver() interp.ModuleResolver {
	res := module.NewResolver(nil)
	return func(spec, fromPath string) (*module.File, bool) {
		dir := filepath.Dir(fromPath)
		if dir == "." {
			dir = "."
		}
		path, ok := res.Resolve(spec, dir)
		if !ok {
			return nil, false
		}
		file := &module.File{Source: readSourceFile(path), Spec: spec}
		if file.Source == nil {
			return nil, false
		}
		prog, diags := parser.Parse(file.Source)
		if hasErrors(diags) {
			return nil, false
		}
		file.Prog = prog
		file.Path = path
		return file, true
	}
}

// readSourceFile reads a file and returns nil on failure.
func readSourceFile(path string) *source.File {
	text, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return source.NewFile(path, string(text))
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

// parseArgs reads a single file argument with the standard color flag.
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
