package compress_test

import (
	"strings"
	"testing"

	"github.com/kaushal/skim/internal/compress"
)

const goTestOutput = `=== RUN   TestFoo
--- PASS: TestFoo (0.00s)
=== RUN   TestBar
    bar_test.go:42: expected 1, got 2
--- FAIL: TestBar (0.01s)
=== RUN   TestBaz
--- PASS: TestBaz (0.00s)
FAIL
FAIL	github.com/example/pkg	0.123s
`

func TestCompressGoTest_PreservesFailures(t *testing.T) {
	r := compress.Compress(goTestOutput)
	if r.Kind != "go_test" {
		t.Fatalf("expected kind=go_test, got %q", r.Kind)
	}
	if !strings.Contains(r.Output, "bar_test.go:42") {
		t.Error("failure line must be preserved")
	}
	if !strings.Contains(r.Output, "--- FAIL: TestBar") {
		t.Error("FAIL marker must be preserved")
	}
}

func TestCompressGoTest_DropsPassLines(t *testing.T) {
	r := compress.Compress(goTestOutput)
	if strings.Contains(r.Output, "--- PASS: TestFoo") {
		t.Error("individual PASS lines must be dropped")
	}
	if strings.Contains(r.Output, "--- PASS: TestBaz") {
		t.Error("individual PASS lines must be dropped")
	}
}

func TestCompressGoTest_Reduced(t *testing.T) {
	r := compress.Compress(goTestOutput)
	if !r.Reduced {
		t.Error("output with passing tests should be reduced")
	}
	if r.Ratio >= 1.0 {
		t.Errorf("ratio should be < 1.0, got %.2f", r.Ratio)
	}
}

const gitStatusOutput = `On branch main
Your branch is up to date with 'origin/main'.

Changes not staged for commit:
  (use "git add <file>..." to update what will be committed)
	modified:   internal/handler/read.go
	modified:   internal/config/config.go
	modified:   internal/metrics/metrics.go

Untracked files:
  (use "git add <file>..." to include in what will be committed)
	internal/cost/
	internal/compress/

no changes added to commit (use "git add" and/or "git commit -a")
`

func TestCompressGitStatus_RecognisesOutput(t *testing.T) {
	r := compress.Compress(gitStatusOutput)
	if r.Kind != "git_status" {
		t.Fatalf("expected kind=git_status, got %q", r.Kind)
	}
	if !strings.Contains(r.Output, "read.go") {
		t.Error("modified file must be preserved")
	}
}

func TestCompressGitStatus_Reduced(t *testing.T) {
	r := compress.Compress(gitStatusOutput)
	if !r.Reduced {
		t.Error("git status output should be reduced (hint lines dropped)")
	}
}

const buildOutput = `# github.com/example/pkg
internal/handler/read.go:45:3: undefined: foo
internal/handler/read.go:72:12: cannot use x (type int) as type string
internal/cost/cost.go:18:2: declared and not used: pricePer1M
3 errors found
`

func TestCompressBuildOutput_PreservesErrors(t *testing.T) {
	r := compress.Compress(buildOutput)
	if r.Kind != "build" {
		t.Fatalf("expected kind=build, got %q", r.Kind)
	}
	if !strings.Contains(r.Output, "read.go:45:3") {
		t.Error("file:line error reference must be preserved")
	}
	if !strings.Contains(r.Output, "cannot use") {
		t.Error("error message must be preserved")
	}
}

func TestCompress_UnknownInput_Passthrough(t *testing.T) {
	input := "hello world\nthis is just some text\n"
	r := compress.Compress(input)
	if r.Kind != "" {
		t.Errorf("unknown input should have empty Kind, got %q", r.Kind)
	}
	if r.Reduced {
		t.Error("unknown input should not be marked as reduced")
	}
	if r.Output != input {
		t.Error("unknown input should pass through unchanged")
	}
}

func TestCompress_RatioAlwaysSet(t *testing.T) {
	cases := []string{
		goTestOutput,
		gitStatusOutput,
		buildOutput,
		"some unknown text",
		"",
	}
	for _, c := range cases {
		r := compress.Compress(c)
		if r.Ratio < 0 {
			t.Errorf("ratio must not be negative, got %.2f for input %q", r.Ratio, c[:min(len(c), 20)])
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
