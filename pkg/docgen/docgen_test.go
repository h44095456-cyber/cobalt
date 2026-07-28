package docgen

import (
	"cobalt/pkg/lexer"
	"cobalt/pkg/parser"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDocGenerator(t *testing.T) {
	input := `# Calculates sum of two integers.
fn add(a: int, b: int) -> int:
    return a + b

# Data representation structure.
struct Point:
    x: int
    y: int
`

	l := lexer.New(input)
	p := parser.New(l)
	prog := p.ParseProgram()

	g := New()
	g.InspectProgram(prog, input)

	md := g.GenerateMarkdown()
	if !strings.Contains(md, "Calculates sum of two integers.") {
		t.Errorf("expected docstring in Markdown output, got:\n%s", md)
	}

	tmpDir := t.TempDir()
	if err := g.GenerateHTML(tmpDir); err != nil {
		t.Fatalf("failed to generate HTML docs: %v", err)
	}

	if _, err := os.Stat(filepath.Join(tmpDir, "index.html")); os.IsNotExist(err) {
		t.Errorf("expected index.html to be created")
	}
}
