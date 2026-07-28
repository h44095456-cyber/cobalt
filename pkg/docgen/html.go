package docgen

import (
	"cobalt/pkg/ast"
	"fmt"
	"strings"
)

type HTMLDocGenerator struct{}

func NewHTMLDocGenerator() *HTMLDocGenerator {
	return &HTMLDocGenerator{}
}

func (h *HTMLDocGenerator) GenerateHTML(program *ast.Program, title string) string {
	var fns []string
	var structs []string

	for _, stmt := range program.Statements {
		if fn, ok := stmt.(*ast.FnDeclStmt); ok {
			var params []string
			for _, p := range fn.Params {
				params = append(params, fmt.Sprintf("%s: %s", p.Name.Value, p.Type))
			}
			ret := fn.ReturnType
			if ret == "" {
				ret = "void"
			}
			decs := ""
			if len(fn.Decorators) > 0 {
				decs = fmt.Sprintf("<span class=\"tag\">@%s</span> ", strings.Join(fn.Decorators, " "))
			}
			fns = append(fns, fmt.Sprintf(`
				<div class="doc-card">
					<div class="fn-sig">%sfn <strong>%s</strong>(%s) -&gt; %s</div>
					<div class="fn-desc">Exported Cobalt core function contract.</div>
				</div>`, decs, fn.Name.Value, strings.Join(params, ", "), ret))
		} else if st, ok := stmt.(*ast.StructDeclStmt); ok {
			var fields []string
			for _, f := range st.Fields {
				fields = append(fields, fmt.Sprintf("    %s: %s", f.Name.Value, f.Type))
			}
			structs = append(structs, fmt.Sprintf(`
				<div class="doc-card">
					<div class="fn-sig">struct <strong>%s</strong></div>
					<pre class="struct-body">{\n%s\n}</pre>
				</div>`, st.Name.Value, strings.Join(fields, "\n")))
		}
	}

	return fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <title>Cobalt API Reference - %s</title>
    <link href="https://fonts.googleapis.com/css2?family=Outfit:wght@400;600;700&family=Fira+Code:wght@400;500&display=swap" rel="stylesheet">
    <style>
        body { font-family: 'Outfit', sans-serif; background: #090d16; color: #f8fafc; margin: 0; padding: 32px; }
        .container { max-width: 900px; margin: 0 auto; }
        h1 { font-size: 36px; color: #38bdf8; margin-bottom: 24px; border-bottom: 1px solid #1e293b; padding-bottom: 12px; }
        h2 { font-size: 24px; color: #818cf8; margin-top: 32px; margin-bottom: 16px; }
        .doc-card { background: #0f172a; border: 1px solid #1e293b; border-radius: 12px; padding: 20px; margin-bottom: 16px; }
        .fn-sig { font-family: 'Fira Code', monospace; font-size: 16px; color: #38bdf8; }
        .fn-desc { color: #94a3b8; font-size: 14px; margin-top: 8px; }
        .struct-body { font-family: 'Fira Code', monospace; color: #4ade80; background: #020617; padding: 12px; border-radius: 8px; font-size: 14px; }
        .tag { background: #0284c7; color: white; padding: 2px 8px; border-radius: 6px; font-size: 12px; }
    </style>
</head>
<body>
    <div class="container">
        <h1>Cobalt API Reference Documentation</h1>
        <h2>Struct Declarations</h2>
        %s
        <h2>Function Signatures</h2>
        %s
    </div>
</body>
</html>`, title, strings.Join(structs, ""), strings.Join(fns, ""))
}
