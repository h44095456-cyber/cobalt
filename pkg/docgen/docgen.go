package docgen

import (
	"cobalt/pkg/ast"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type DocItem struct {
	Kind       string   // Function, Struct, Trait, Impl, Macro
	Name       string
	Signature  string
	Docstring  string
	Params     []string
	ReturnType string
}

type Generator struct {
	items []DocItem
}

func New() *Generator {
	return &Generator{}
}

func (g *Generator) InspectProgram(program *ast.Program, rawSource string) {
	lines := strings.Split(rawSource, "\n")

	for _, stmt := range program.Statements {
		switch s := stmt.(type) {
		case *ast.FnDeclStmt:
			lineIdx := s.Token.Line - 1
			doc := g.extractDocComments(lines, lineIdx)

			var params []string
			for _, p := range s.Params {
				params = append(params, fmt.Sprintf("%s: %s", p.Name.Value, p.Type))
			}

			sig := fmt.Sprintf("fn %s(%s)", s.Name.Value, strings.Join(params, ", "))
			if s.ReturnType != "" {
				sig += " -> " + s.ReturnType
			}
			if s.IsAsync {
				sig = "async " + sig
			}

			g.items = append(g.items, DocItem{
				Kind:       "Function",
				Name:       s.Name.Value,
				Signature:  sig,
				Docstring:  doc,
				Params:     params,
				ReturnType: s.ReturnType,
			})

		case *ast.StructDeclStmt:
			lineIdx := s.Token.Line - 1
			doc := g.extractDocComments(lines, lineIdx)

			var fields []string
			for _, f := range s.Fields {
				fields = append(fields, fmt.Sprintf("%s: %s", f.Name.Value, f.Type))
			}

			g.items = append(g.items, DocItem{
				Kind:      "Struct",
				Name:      s.Name.Value,
				Signature: fmt.Sprintf("struct %s", s.Name.Value),
				Docstring: doc,
				Params:    fields,
			})

		case *ast.TraitDeclStmt:
			lineIdx := s.Token.Line - 1
			doc := g.extractDocComments(lines, lineIdx)

			var methods []string
			for _, m := range s.Methods {
				methods = append(methods, m.Name.Value)
			}

			g.items = append(g.items, DocItem{
				Kind:      "Trait",
				Name:      s.Name.Value,
				Signature: fmt.Sprintf("trait %s", s.Name.Value),
				Docstring: doc,
				Params:    methods,
			})

		case *ast.MacroDeclStmt:
			lineIdx := s.Token.Line - 1
			doc := g.extractDocComments(lines, lineIdx)

			var macroParams []string
			for _, p := range s.Params {
				macroParams = append(macroParams, p.Value)
			}

			g.items = append(g.items, DocItem{
				Kind:      "Macro",
				Name:      s.Name.Value,
				Signature: fmt.Sprintf("macro %s(%s)", s.Name.Value, strings.Join(macroParams, ", ")),
				Docstring: doc,
				Params:    macroParams,
			})
		}
	}
}

func (g *Generator) extractDocComments(lines []string, targetLine int) string {
	var comments []string
	curr := targetLine - 1
	for curr >= 0 {
		if curr >= len(lines) {
			curr--
			continue
		}
		line := strings.TrimSpace(lines[curr])
		if strings.HasPrefix(line, "#") {
			commentText := strings.TrimSpace(strings.TrimPrefix(line, "#"))
			comments = append([]string{commentText}, comments...)
			curr--
		} else {
			break
		}
	}
	return strings.Join(comments, "\n")
}

func (g *Generator) GenerateHTML(outDir string) error {
	if err := os.MkdirAll(outDir, 0755); err != nil {
		return err
	}

	var sb strings.Builder
	sb.WriteString("<!DOCTYPE html>\n<html lang=\"en\">\n<head>\n")
	sb.WriteString("  <meta charset=\"UTF-8\">\n")
	sb.WriteString("  <title>Cobalt API Documentation</title>\n")
	sb.WriteString("  <style>\n")
	sb.WriteString("    body { font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif; background: #0f172a; color: #f8fafc; padding: 2rem; max-width: 900px; margin: 0 auto; }\n")
	sb.WriteString("    h1 { color: #38bdf8; border-bottom: 2px solid #334155; padding-bottom: 0.5rem; }\n")
	sb.WriteString("    .doc-card { background: #1e293b; border: 1px solid #334155; border-radius: 8px; padding: 1.5rem; margin-bottom: 1.5rem; }\n")
	sb.WriteString("    .badge { display: inline-block; padding: 0.25rem 0.5rem; border-radius: 4px; font-size: 0.75rem; font-weight: bold; text-transform: uppercase; background: #0284c7; color: #fff; margin-bottom: 0.5rem; }\n")
	sb.WriteString("    .badge.struct { background: #d97706; }\n")
	sb.WriteString("    .badge.trait { background: #059669; }\n")
	sb.WriteString("    .badge.macro { background: #7c3aed; }\n")
	sb.WriteString("    .signature { font-family: 'Courier New', monospace; background: #0f172a; padding: 0.75rem; border-radius: 6px; color: #a5f3fc; font-size: 1rem; overflow-x: auto; }\n")
	sb.WriteString("    .docstring { color: #cbd5e1; margin-top: 1rem; line-height: 1.6; whitespace: pre-wrap; }\n")
	sb.WriteString("  </style>\n</head>\n<body>\n")
	sb.WriteString("  <h1>Cobalt Project API Documentation</h1>\n")

	for _, item := range g.items {
		kindClass := strings.ToLower(item.Kind)
		sb.WriteString(fmt.Sprintf("  <div class=\"doc-card\">\n"))
		sb.WriteString(fmt.Sprintf("    <span class=\"badge %s\">%s</span>\n", kindClass, item.Kind))
		sb.WriteString(fmt.Sprintf("    <div class=\"signature\">%s</div>\n", item.Signature))
		if item.Docstring != "" {
			sb.WriteString(fmt.Sprintf("    <div class=\"docstring\">%s</div>\n", item.Docstring))
		} else {
			sb.WriteString(fmt.Sprintf("    <div class=\"docstring\"><em>No documentation comments provided.</em></div>\n"))
		}
		sb.WriteString("  </div>\n")
	}

	sb.WriteString("</body>\n</html>\n")

	outFile := filepath.Join(outDir, "index.html")
	return os.WriteFile(outFile, []byte(sb.String()), 0644)
}

func (g *Generator) GenerateMarkdown() string {
	var sb strings.Builder
	sb.WriteString("# Cobalt Project API Documentation\n\n")

	for _, item := range g.items {
		sb.WriteString(fmt.Sprintf("### `%s` (%s)\n\n", item.Name, item.Kind))
		sb.WriteString("```cobalt\n")
		sb.WriteString(item.Signature + "\n")
		sb.WriteString("```\n\n")

		if item.Docstring != "" {
			sb.WriteString(item.Docstring + "\n\n")
		}
		sb.WriteString("---\n\n")
	}

	return sb.String()
}
