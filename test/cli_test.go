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
	if !strings.Contains(out, "0.1.0") {
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
