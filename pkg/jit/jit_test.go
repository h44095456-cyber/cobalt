package jit

import (
	"os"
	"path/filepath"
	"testing"
)

func TestJITExecuteIR(t *testing.T) {
	irCode := `
declare i32 @printf(i8*, ...)
declare i32 @fflush(i8*)

@.fmt = private unnamed_addr constant [14 x i8] c"JIT Test: %d\0a\00"

define i32 @main() {
entry:
    %fmt_ptr = getelementptr inbounds [14 x i8], [14 x i8]* @.fmt, i64 0, i64 0
    %r0 = call i32 (i8*, ...) @printf(i8* %fmt_ptr, i32 42)
    %fl = call i32 @fflush(i8* null)
    ret i32 0
}
`
	engine := New()
	out, err := engine.ExecuteIR(irCode)
	if err != nil {
		t.Fatalf("JIT ExecuteIR failed: %v", err)
	}

	expected := "JIT Test: 42\n"
	if out != expected {
		t.Errorf("Expected %q, got %q", expected, out)
	}
}

func TestJITExecuteFile(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "cobalt_jit_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	cbFile := filepath.Join(tmpDir, "test.cb")
	code := `
fn main():
    println("Hello from Cobalt JIT!")
`
	if err := os.WriteFile(cbFile, []byte(code), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	engine := New()
	_, err = engine.ExecuteFile(cbFile)
	if err != nil {
		t.Fatalf("JIT ExecuteFile failed: %v", err)
	}
}
