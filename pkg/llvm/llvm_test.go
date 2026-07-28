package llvm

import (
	"cobalt/pkg/lexer"
	"cobalt/pkg/parser"
	"strings"
	"testing"
)

func TestLLVMGen(t *testing.T) {
	input := `fn fib(n: int) -> int:
    if n <= 1:
        return n
    return fib(n - 1) + fib(n - 2)

fn main():
    let res = fib(10)
    println(res)
`

	l := lexer.New(input)
	p := parser.New(l)
	prog := p.ParseProgram()

	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}

	gen := New()
	ir, err := gen.Generate(prog)
	if err != nil {
		t.Fatalf("LLVM generation error: %v", err)
	}

	if !strings.Contains(ir, "define i64 @fib(i64 %n.arg)") {
		t.Errorf("missing @fib function signature in LLVM IR")
	}

	if !strings.Contains(ir, "call i64 @fib") {
		t.Errorf("missing call to @fib in LLVM IR")
	}

	if !strings.Contains(ir, "call i32 (i8*, ...) @printf") {
		t.Errorf("missing @printf call in LLVM IR")
	}
}
