package optimizer

import (
	"cobalt/pkg/lexer"
	"cobalt/pkg/parser"
	"strings"
	"testing"
)

func TestConstantFoldingAndDeadCodeElimination(t *testing.T) {
	input := `fn main():
    let x = 10 + 20 * 2
    let msg = "Hello, " + "Cobalt!"
    return 0
    let dead = 100
`

	l := lexer.New(input)
	p := parser.New(l)
	prog := p.ParseProgram()

	opt := New()
	optProg := opt.Optimize(prog)
	folded, dead, _ := opt.Stats()

	if folded == 0 {
		t.Errorf("expected constant folding, got 0 folded expressions")
	}

	if dead == 0 {
		t.Errorf("expected dead code elimination, got 0 dead statements")
	}

	optStr := optProg.String()
	if !strings.Contains(optStr, "let x = 50") {
		t.Errorf("expected folded 'let x = 50', got:\n%s", optStr)
	}

	if strings.Contains(optStr, "let dead = 100") {
		t.Errorf("expected dead code 'let dead = 100' to be eliminated, got:\n%s", optStr)
	}
}
