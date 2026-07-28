package debugger

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDebuggerBreakpointsAndStepping(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "cobalt_db_test_*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	file := filepath.Join(tmpDir, "test.cb")
	content := `fn add(a: int, b: int) -> int:
    let sum = a + b
    return sum

fn main():
    let res = add(10, 20)
    println(res)
`
	if err := os.WriteFile(file, []byte(content), 0644); err != nil {
		t.Fatalf("Failed to write test file: %v", err)
	}

	db, err := New(file)
	if err != nil {
		t.Fatalf("New debugger failed: %v", err)
	}

	bp := db.SetBreakpoint(2)
	if bp.ID != 1 || bp.Line != 2 {
		t.Errorf("Unexpected breakpoint: %+v", bp)
	}

	if len(db.breakpoints) != 1 {
		t.Errorf("Expected 1 breakpoint, got %d", len(db.breakpoints))
	}

	removed := db.RemoveBreakpoint(2)
	if !removed {
		t.Errorf("Expected breakpoint to be removed")
	}

	if len(db.breakpoints) != 0 {
		t.Errorf("Expected 0 breakpoints after removal, got %d", len(db.breakpoints))
	}
}
