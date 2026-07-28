package lexer

import (
	"cobalt/pkg/token"
	"testing"
)

func TestLexer(t *testing.T) {
	input := `let x = 10
var y = 20.5
fn add(a: int, b: int) -> int:
    return a + b
`

	l := New(input)

	expected := []struct {
		expectedType    token.TokenType
		expectedLiteral string
	}{
		{token.LET, "let"},
		{token.IDENT, "x"},
		{token.ASSIGN, "="},
		{token.INT, "10"},
		{token.NEWLINE, "\n"},

		{token.VAR, "var"},
		{token.IDENT, "y"},
		{token.ASSIGN, "="},
		{token.FLOAT, "20.5"},
		{token.NEWLINE, "\n"},

		{token.FN, "fn"},
		{token.IDENT, "add"},
		{token.LPAREN, "("},
		{token.IDENT, "a"},
		{token.COLON, ":"},
		{token.IDENT, "int"},
		{token.COMMA, ","},
		{token.IDENT, "b"},
		{token.COLON, ":"},
		{token.IDENT, "int"},
		{token.RPAREN, ")"},
		{token.ARROW, "->"},
		{token.IDENT, "int"},
		{token.COLON, ":"},
		{token.NEWLINE, "\n"},

		{token.INDENT, ""},
		{token.RETURN, "return"},
		{token.IDENT, "a"},
		{token.PLUS, "+"},
		{token.IDENT, "b"},
		{token.NEWLINE, "\n"},

		{token.DEDENT, ""},
		{token.EOF, ""},
	}

	for i, tt := range expected {
		tok := l.NextToken()
		if tok.Type != tt.expectedType {
			t.Fatalf("tests[%d] - token type wrong. expected=%q, got=%q (literal=%q)",
				i, tt.expectedType, tok.Type, tok.Literal)
		}
		if tt.expectedLiteral != "" && tok.Literal != tt.expectedLiteral {
			t.Fatalf("tests[%d] - literal wrong. expected=%q, got=%q",
				i, tt.expectedLiteral, tok.Literal)
		}
	}
}
