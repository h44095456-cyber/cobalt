package resolver

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestModuleResolver(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "cobalt_mod_test_*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	mathFile := filepath.Join(tmpDir, "math.cb")
	mathContent := `fn add(a: int, b: int) -> int:
    return a + b
`
	if err := os.WriteFile(mathFile, []byte(mathContent), 0644); err != nil {
		t.Fatalf("failed writing math.cb: %v", err)
	}

	mainFile := filepath.Join(tmpDir, "main.cb")
	mainContent := `import math

fn main():
    let res = math.add(10, 20)
    println(res)
`
	if err := os.WriteFile(mainFile, []byte(mainContent), 0644); err != nil {
		t.Fatalf("failed writing main.cb: %v", err)
	}

	res := New()
	prog, err := res.ResolveProgram(mainFile)
	if err != nil {
		t.Fatalf("ResolveProgram error: %v", err)
	}

	progStr := prog.String()
	if !strings.Contains(progStr, "fn math_add(a: int, b: int) -> int") {
		t.Errorf("expected math_add function declaration in resolved AST, got:\n%s", progStr)
	}

	if !strings.Contains(progStr, "math_add(10, 20)") {
		t.Errorf("expected math_add call expression in resolved AST, got:\n%s", progStr)
	}
}
