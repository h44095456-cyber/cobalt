package codegen

import (
	"cobalt/pkg/lexer"
	"cobalt/pkg/parser"
	"strings"
	"testing"
)

func TestCodegen(t *testing.T) {
	input := `fn add(a: int, b: int) -> int:
    return a + b

fn main():
    let res = add(10, 20)
    println("Result: " + str(res))
`

	l := lexer.New(input)
	p := parser.New(l)
	prog := p.ParseProgram()

	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}

	cg := New()
	cppCode, err := cg.Generate(prog)
	if err != nil {
		t.Fatalf("codegen error: %v", err)
	}

	if !strings.Contains(cppCode, "long long add(long long a, long long b)") {
		t.Errorf("generated C++ missing add function declaration")
	}

	if !strings.Contains(cppCode, "int main()") {
		t.Errorf("generated C++ missing main function declaration")
	}
}
