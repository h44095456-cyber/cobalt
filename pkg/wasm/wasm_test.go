package wasm

import (
	"cobalt/pkg/lexer"
	"cobalt/pkg/parser"
	"strings"
	"testing"
)

func TestWasmGenerator(t *testing.T) {
	input := `fn add(a: int, b: int) -> int:
    return a + b
`

	l := lexer.New(input)
	p := parser.New(l)
	prog := p.ParseProgram()

	g := New()
	wat, err := g.Generate(prog)
	if err != nil {
		t.Fatalf("failed to generate WAT code: %v", err)
	}

	if !strings.Contains(wat, "(func $add (export \"add\")") {
		t.Errorf("expected func $add export in WAT output, got:\n%s", wat)
	}

	if !strings.Contains(wat, "i64.add") {
		t.Errorf("expected i64.add instruction in WAT output, got:\n%s", wat)
	}
}
