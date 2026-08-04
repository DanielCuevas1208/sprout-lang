package test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

var sproutBin string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "sprout-cli-")
	if err != nil {
		panic(err)
	}
	sproutBin = filepath.Join(dir, "sprout"+exeSuffix())
	build := exec.Command("go", "build", "-o", sproutBin, "../cmd/sprout")
	build.Stderr = os.Stderr
	if err := build.Run(); err != nil {
		panic("build sprout: " + err.Error())
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}

func exeSuffix() string {
	if os.PathSeparator == '\\' {
		return ".exe"
	}
	return ""
}

func runCLI(t *testing.T, stdin string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	cmd := exec.Command(sproutBin, args...)
	cmd.Stdin = strings.NewReader(stdin)
	var out, errb strings.Builder
	cmd.Stdout = &out
	cmd.Stderr = &errb
	err := cmd.Run()
	code = 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else {
			t.Fatalf("run %v: %v", args, err)
		}
	}
	return out.String(), errb.String(), code
}

func TestVersion(t *testing.T) {
	out, _, code := runCLI(t, "", "version")
	if code != 0 {
		t.Fatalf("version exit code %d", code)
	}
	if !strings.Contains(out, "0.3.0") {
		t.Errorf("version output: %q", out)
	}
}

func TestRunHello(t *testing.T) {
	out, _, code := runCLI(t, "", "run", filepath.Join("..", "examples", "hello.spr"))
	if code != 0 {
		t.Fatalf("run exit code %d", code)
	}
	if !strings.Contains(out, "hello, world") {
		t.Errorf("output: %q", out)
	}
}

func TestRunVM(t *testing.T) {
	out, _, code := runCLI(t, "", "vm", filepath.Join("..", "examples", "fizzbuzz.spr"))
	if code != 0 {
		t.Fatalf("vm exit code %d", code)
	}
	if !strings.Contains(out, "fizzbuzz") {
		t.Errorf("output: %q", out)
	}
}

func TestRunVMClosures(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "counter.spr")
	src := "fn make_counter() {\n" +
		"\tlet count = 0\n" +
		"\treturn fn() {\n" +
		"\t\tcount = count + 1\n" +
		"\t\treturn count\n" +
		"\t}\n" +
		"}\n" +
		"let next = make_counter()\n" +
		"print(next(), next(), next())\n"
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	out, _, code := runCLI(t, "", "vm", path)
	if code != 0 {
		t.Fatalf("vm exit code %d", code)
	}
	if !strings.Contains(out, "1 2 3") {
		t.Errorf("output: %q", out)
	}
}

func TestRunDis(t *testing.T) {
	out, _, code := runCLI(t, "", "dis", filepath.Join("..", "examples", "hello.spr"))
	if code != 0 {
		t.Fatalf("dis exit code %d", code)
	}
	if !strings.Contains(out, "== fn <main> ==") {
		t.Errorf("expected main function header, output: %q", out)
	}
	if !strings.Contains(out, "PUSH_CONST") {
		t.Errorf("expected instructions, output: %q", out)
	}
}

func TestRunVMErrorHasStack(t *testing.T) {
	src := "fn inner() { return 1 / 0 }\ninner()\n"
	dir := t.TempDir()
	path := filepath.Join(dir, "boom.spr")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	_, errOut, code := runCLI(t, "", "vm", path)
	if code == 0 {
		t.Fatalf("vm should fail, exit code 0")
	}
	if !strings.Contains(errOut, "cannot divide by zero") {
		t.Errorf("stderr: %q", errOut)
	}
	if !strings.Contains(errOut, "at inner") {
		t.Errorf("expected stack frame, stderr: %q", errOut)
	}
}

func TestDefaultRun(t *testing.T) {
	out, _, code := runCLI(t, "", filepath.Join("..", "examples", "primes.spr"))
	if code != 0 {
		t.Fatalf("default run exit code %d", code)
	}
	if !strings.Contains(out, "[2, 3, 5, 7, 11, 13, 17, 19, 23, 29]") {
		t.Errorf("output: %q", out)
	}
}

func TestCheckBadProgram(t *testing.T) {
	src := "print(missing_name)\n"
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.spr")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	_, errOut, code := runCLI(t, "", "check", path)
	if code == 0 {
		t.Fatalf("check should fail, exit code 0")
	}
	if !strings.Contains(errOut, "undefined name 'missing_name'") {
		t.Errorf("stderr: %q", errOut)
	}
}

func TestRunRuntimeErrorHasStack(t *testing.T) {
	src := "fn inner() { return 1 / 0 }\ninner()\n"
	dir := t.TempDir()
	path := filepath.Join(dir, "boom.spr")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	_, errOut, code := runCLI(t, "", "run", path)
	if code == 0 {
		t.Fatalf("run should fail, exit code 0")
	}
	if !strings.Contains(errOut, "cannot divide by zero") {
		t.Errorf("stderr: %q", errOut)
	}
	if !strings.Contains(errOut, "at inner") {
		t.Errorf("expected stack frame, stderr: %q", errOut)
	}
}

func TestLexAndParse(t *testing.T) {
	lexOut, _, code := runCLI(t, "", "lex", filepath.Join("..", "examples", "hello.spr"))
	if code != 0 {
		t.Fatalf("lex exit code %d", code)
	}
	if !strings.Contains(lexOut, "identifier") || !strings.Contains(lexOut, "string") {
		t.Errorf("lex output: %q", lexOut)
	}

	parseOut, _, code := runCLI(t, "", "parse", filepath.Join("..", "examples", "hello.spr"))
	if code != 0 {
		t.Fatalf("parse exit code %d", code)
	}
	if !strings.Contains(parseOut, "(program") {
		t.Errorf("parse output: %q", parseOut)
	}
}

func TestReplSession(t *testing.T) {
	input := "let x = 6\nprint(x * 7)\nx + 1\n:quit\n"
	out, _, code := runCLI(t, input, "repl")
	if code != 0 {
		t.Fatalf("repl exit code %d", code)
	}
	if !strings.Contains(out, "42") {
		t.Errorf("expected 42 in repl output: %q", out)
	}
	if !strings.Contains(out, "7") {
		t.Errorf("expected echo 7 in repl output: %q", out)
	}
}

func TestReplMultiLine(t *testing.T) {
	input := "let t = [\n  1,\n  2,\n]\nprint(len(t))\n:quit\n"
	out, _, code := runCLI(t, input, "repl")
	if code != 0 {
		t.Fatalf("repl exit code %d", code)
	}
	if !strings.Contains(out, "2") {
		t.Errorf("expected 2 in repl output: %q", out)
	}
}

func TestUnknownCommand(t *testing.T) {
	_, errOut, code := runCLI(t, "", "frobnicate")
	if code == 0 {
		t.Fatalf("unknown command should fail")
	}
	if !strings.Contains(errOut, "unknown command") {
		t.Errorf("stderr: %q", errOut)
	}
}

// writeFile writes src to path, creating parent directories.
func writeFile(t *testing.T, path, src string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestRunProgramWithImports(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "lib", "g.spr"),
		"export fn greet(name) { return \"hello, \" + name }\n")
	writeFile(t, filepath.Join(dir, "main.spr"),
		"import \"lib/g.spr\"\nprint(greet(\"cli\"))\n")
	main := filepath.Join(dir, "main.spr")

	out, _, code := runCLI(t, "", "run", main)
	if code != 0 {
		t.Fatalf("run exit code %d", code)
	}
	if !strings.Contains(out, "hello, cli") {
		t.Errorf("run output: %q", out)
	}

	out, _, code = runCLI(t, "", "vm", main)
	if code != 0 {
		t.Fatalf("vm exit code %d", code)
	}
	if !strings.Contains(out, "hello, cli") {
		t.Errorf("vm output: %q", out)
	}
}

func TestCheckProgramWithImports(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "lib", "g.spr"),
		"export fn greet(name) { return \"hello, \" + name }\n")
	writeFile(t, filepath.Join(dir, "main.spr"),
		"import \"lib/g.spr\"\nprint(greet(\"x\"))\n")
	out, _, code := runCLI(t, "", "check", filepath.Join(dir, "main.spr"))
	if code != 0 {
		t.Fatalf("check exit code %d", code)
	}
	if !strings.Contains(out, "ok") {
		t.Errorf("check output: %q", out)
	}
}

func TestCheckMissingImportFails(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "main.spr"), "import \"nope.spr\"\n")
	_, errOut, code := runCLI(t, "", "check", filepath.Join(dir, "main.spr"))
	if code == 0 {
		t.Fatalf("check should fail on a missing module")
	}
	if !strings.Contains(errOut, "cannot find module") {
		t.Errorf("stderr: %q", errOut)
	}
}

func TestBuildProducesRunnableFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "lib", "g.spr"),
		"export fn greet(name) { return \"hello, \" + name }\n")
	writeFile(t, filepath.Join(dir, "main.spr"),
		"import \"lib/g.spr\"\nprint(greet(\"built\"))\n")
	main := filepath.Join(dir, "main.spr")
	outFile := filepath.Join(dir, "dist", "main.bundle.spr")

	out, _, code := runCLI(t, "", "build", "-o", outFile, main)
	if code != 0 {
		t.Fatalf("build exit code %d, output: %s", code, out)
	}
	if !strings.Contains(out, "built") {
		t.Errorf("build output: %q", out)
	}
	if _, err := os.Stat(outFile); err != nil {
		t.Fatalf("bundle not written: %v", err)
	}

	// The bundle runs without the source modules present.
	os.RemoveAll(filepath.Join(dir, "lib"))
	os.Remove(main)
	bundleOut, _, code := runCLI(t, "", "run", outFile)
	if code != 0 {
		t.Fatalf("bundle run exit code %d", code)
	}
	if !strings.Contains(bundleOut, "hello, built") {
		t.Errorf("bundle run output: %q", bundleOut)
	}
}

func TestBuildDefaultName(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "main.spr"), "print(\"hi\")\n")
	main := filepath.Join(dir, "main.spr")
	out, _, code := runCLI(t, "", "build", main)
	if code != 0 {
		t.Fatalf("build exit code %d, output: %s", code, out)
	}
	want := filepath.Join(dir, "main.bundle.spr")
	if _, err := os.Stat(want); err != nil {
		t.Errorf("default bundle not at %s: %v", want, err)
	}
}

func TestModulesListsGraph(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "lib", "g.spr"),
		"export fn greet(name) { return name }\nexport const punc = \"!\"\n")
	writeFile(t, filepath.Join(dir, "main.spr"),
		"import \"lib/g.spr\"\nprint(greet(\"x\") + punc)\n")
	out, _, code := runCLI(t, "", "modules", filepath.Join(dir, "main.spr"))
	if code != 0 {
		t.Fatalf("modules exit code %d", code)
	}
	for _, want := range []string{"main", "module", "import", "export", "greet", "punc"} {
		if !strings.Contains(out, want) {
			t.Errorf("modules output missing %q: %q", want, out)
		}
	}
}
