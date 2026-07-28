package parser

import (
	"cobalt/pkg/ast"
	"cobalt/pkg/lexer"
	"testing"
)

func TestParseVarStatements(t *testing.T) {
	input := `let x = 5
var y: int = 10
`

	l := lexer.New(input)
	p := New(l)
	program := p.ParseProgram()

	checkParserErrors(t, p)

	if len(program.Statements) != 2 {
		t.Fatalf("program.Statements does not contain 2 statements. got=%d", len(program.Statements))
	}

	stmt0, ok := program.Statements[0].(*ast.VarDeclStmt)
	if !ok || stmt0.Name.Value != "x" || stmt0.IsVar {
		t.Fatalf("statement 0 incorrect. got=%s", program.Statements[0].String())
	}

	stmt1, ok := program.Statements[1].(*ast.VarDeclStmt)
	if !ok || stmt1.Name.Value != "y" || !stmt1.IsVar || stmt1.Type != "int" {
		t.Fatalf("statement 1 incorrect. got=%s", program.Statements[1].String())
	}
}

func TestParseFnDeclaration(t *testing.T) {
	input := `fn multiply(a: int, b: int) -> int:
    return a * b
`

	l := lexer.New(input)
	p := New(l)
	program := p.ParseProgram()

	checkParserErrors(t, p)

	if len(program.Statements) != 1 {
		t.Fatalf("program.Statements does not contain 1 statement. got=%d", len(program.Statements))
	}

	fnStmt, ok := program.Statements[0].(*ast.FnDeclStmt)
	if !ok {
		t.Fatalf("statement is not FnDeclStmt. got=%T", program.Statements[0])
	}

	if fnStmt.Name.Value != "multiply" {
		t.Errorf("fn name wrong. expected='multiply', got=%q", fnStmt.Name.Value)
	}

	if len(fnStmt.Params) != 2 {
		t.Fatalf("fn params count wrong. expected=2, got=%d", len(fnStmt.Params))
	}

	if fnStmt.ReturnType != "int" {
		t.Errorf("fn return type wrong. expected='int', got=%q", fnStmt.ReturnType)
	}
}

func checkParserErrors(t *testing.T, p *Parser) {
	errors := p.Errors()
	if len(errors) == 0 {
		return
	}

	t.Errorf("parser has %d errors", len(errors))
	for _, msg := range errors {
		t.Errorf("parser error: %q", msg)
	}
	t.FailNow()
}
