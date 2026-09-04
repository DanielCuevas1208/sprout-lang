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

// runCLIIn runs a command with the given working directory.
func runCLIIn(t *testing.T, dir, stdin string, args ...string) (stdout, stderr string, code int) {
	t.Helper()
	cmd := exec.Command(sproutBin, args...)
	cmd.Dir = dir
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
	if !strings.Contains(out, "0.10.0") {
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

func TestSchedulerPolicyFlag(t *testing.T) {
	want := "started\nfinished\ndone\n"
	for _, command := range []string{"run", "vm"} {
		out, errOut, code := runCLI(t, "", command, "-scheduler", "direct",
			filepath.Join("..", "examples", "scheduler.spr"))
		if code != 0 {
			t.Fatalf("%s exit code %d: %s", command, code, errOut)
		}
		if out != want {
			t.Errorf("%s output = %q, want %q", command, out, want)
		}
	}

	_, errOut, code := runCLI(t, "", "run", "-scheduler", "random",
		filepath.Join("..", "examples", "hello.spr"))
	if code != 2 {
		t.Fatalf("invalid scheduler exit code = %d, want 2", code)
	}
	if !strings.Contains(errOut, "choose fair or direct") {
		t.Errorf("invalid scheduler error: %q", errOut)
	}
}

func TestRunStructProgramOnBothEngines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "struct.spr")
	src := "struct Point { x y }\n" +
		"fn Point.sum() { return self.x + self.y }\n" +
		"let p = Point(3, 4)\n" +
		"print(p.sum())\n" +
		"p.x = 9\n" +
		"print(p)\n"
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	runOut, _, runCode := runCLI(t, "", "run", path)
	if runCode != 0 {
		t.Fatalf("run exit code %d", runCode)
	}
	vmOut, _, vmCode := runCLI(t, "", "vm", path)
	if vmCode != 0 {
		t.Fatalf("vm exit code %d", vmCode)
	}
	if runOut != vmOut {
		t.Errorf("engines differ:\nrun: %q\n vm: %q", runOut, vmOut)
	}
	if !strings.Contains(runOut, "7") || !strings.Contains(runOut, "Point{x: 9, y: 4}") {
		t.Errorf("output: %q", runOut)
	}
}

func TestRunMatchProgramOnBothEngines(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "match.spr")
	src := "fn safe_div(a, b) {\n" +
		"    if b == 0 {\n" +
		"        return err(\"zero\")\n" +
		"    }\n" +
		"    return ok(a / b)\n" +
		"}\n" +
		"print(match safe_div(6, 3) {\n" +
		"    ok(v) => { v },\n" +
		"    err(m) => { 0 },\n" +
		"    _ => { -1 },\n" +
		"})\n" +
		"print(match safe_div(1, 0) {\n" +
		"    ok(v) => { v },\n" +
		"    err(m) => { \"failed: \" + m },\n" +
		"    _ => { \"?\" },\n" +
		"})\n"
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	runOut, _, runCode := runCLI(t, "", "run", path)
	if runCode != 0 {
		t.Fatalf("run exit code %d", runCode)
	}
	vmOut, _, vmCode := runCLI(t, "", "vm", path)
	if vmCode != 0 {
		t.Fatalf("vm exit code %d", vmCode)
	}
	if runOut != vmOut {
		t.Errorf("engines differ:\nrun: %q\n vm: %q", runOut, vmOut)
	}
	if !strings.Contains(runOut, "2") || !strings.Contains(runOut, "failed: zero") {
		t.Errorf("output: %q", runOut)
	}
}

func TestCheckMatchNeedsCatchAll(t *testing.T) {
	src := "let r = ok(1)\nmatch r {\n    ok(v) => { print(v) },\n    err(e) => { print(e) },\n}\n"
	dir := t.TempDir()
	path := filepath.Join(dir, "nomatch.spr")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	_, errOut, code := runCLI(t, "", "check", path)
	if code == 0 {
		t.Fatalf("check should fail, exit code 0")
	}
	if !strings.Contains(errOut, "catch-all") {
		t.Errorf("stderr: %q", errOut)
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

func TestCheckStructError(t *testing.T) {
	src := "struct Point { x }\nlet p = Point(1)\nprint(p.missing)\n"
	dir := t.TempDir()
	path := filepath.Join(dir, "badstruct.spr")
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	_, errOut, code := runCLI(t, "", "check", path)
	if code == 0 {
		t.Fatalf("check should fail, exit code 0")
	}
	if !strings.Contains(errOut, "no field or method 'missing'") {
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

// TestRunProjectEntryFromAnywhere runs a project file and resolves its lib
// imports through the nearest sprout.toml.
func TestRunProjectEntryFromAnywhere(t *testing.T) {
	out, _, code := runCLI(t, "", "run", filepath.Join("..", "examples", "project", "main.spr"))
	if code != 0 {
		t.Fatalf("run exit code %d", code)
	}
	if !strings.Contains(out, "average: 5.0") {
		t.Errorf("output: %q", out)
	}
	if !strings.Contains(out, "helper calls: 3") {
		t.Errorf("module state not shared: %q", out)
	}
}

func TestRunProjectInsideDir(t *testing.T) {
	dir := filepath.Join("..", "examples", "project")
	out, _, code := runCLIIn(t, dir, "", "run")
	if code != 0 {
		t.Fatalf("run exit code %d", code)
	}
	if !strings.Contains(out, "first: sprout") {
		t.Errorf("output: %q", out)
	}
}

func TestInitThenRun(t *testing.T) {
	dir := t.TempDir()
	out, _, code := runCLIIn(t, dir, "", "init", "demo")
	if code != 0 {
		t.Fatalf("init exit code %d: %s", code, out)
	}
	if !strings.Contains(out, "created project") {
		t.Errorf("init output: %q", out)
	}
	proj := filepath.Join(dir, "demo")
	for _, want := range []string{"sprout.toml", "main.spr", "lib"} {
		if _, err := os.Stat(filepath.Join(proj, want)); err != nil {
			t.Errorf("init did not create %s: %v", want, err)
		}
	}
	out, _, code = runCLIIn(t, proj, "", "run")
	if code != 0 {
		t.Fatalf("run exit code %d: %s", code, out)
	}
	if !strings.Contains(out, "hello from demo") {
		t.Errorf("run output: %q", out)
	}
}

func TestBuildThenRunBundle(t *testing.T) {
	dir := t.TempDir()
	if _, _, code := runCLIIn(t, dir, "", "init", "demo"); code != 0 {
		t.Fatalf("init failed")
	}
	proj := filepath.Join(dir, "demo")
	out, _, code := runCLIIn(t, proj, "", "build")
	if code != 0 {
		t.Fatalf("build exit code %d: %s", code, out)
	}
	bundle := filepath.Join(proj, "out", "demo.spr")
	if _, err := os.Stat(bundle); err != nil {
		t.Fatalf("bundle not created: %v", err)
	}
	if _, err := os.Stat(filepath.Join(proj, "out", "demo.spr")); err != nil {
		t.Fatalf("bundle path: %v", err)
	}
	out, _, code = runCLIIn(t, dir, "", "run", bundle)
	if code != 0 {
		t.Fatalf("bundle run exit code %d: %s", code, out)
	}
	if !strings.Contains(out, "hello from demo") {
		t.Errorf("bundle output: %q", out)
	}
}

func TestBuildRejectsMissingModule(t *testing.T) {
	dir := t.TempDir()
	if _, _, code := runCLIIn(t, dir, "", "init", "demo"); code != 0 {
		t.Fatalf("init failed")
	}
	proj := filepath.Join(dir, "demo")
	src := "import \"ghost\"\nprint(\"done\")\n"
	if err := os.WriteFile(filepath.Join(proj, "main.spr"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	_, errOut, code := runCLIIn(t, proj, "", "build")
	if code == 0 {
		t.Fatalf("build should fail for a missing module")
	}
	if !strings.Contains(errOut, "cannot find module 'ghost'") {
		t.Errorf("stderr: %q", errOut)
	}
}
