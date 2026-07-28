package resolver

import (
	"cobalt/pkg/ast"
	"cobalt/pkg/lexer"
	"cobalt/pkg/optimizer"
	"cobalt/pkg/parser"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Resolver struct {
	modules    map[string]*ast.Program
	macros     map[string]*ast.MacroDeclStmt
	currentDir string
	imported   map[string]bool
}

func New() *Resolver {
	return &Resolver{
		modules:  make(map[string]*ast.Program),
		macros:   make(map[string]*ast.MacroDeclStmt),
		imported: make(map[string]bool),
	}
}

func (r *Resolver) ResolveProgram(filePath string) (*ast.Program, error) {
	input, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %v", filePath, err)
	}

	r.currentDir = filepath.Dir(filePath)

	l := lexer.New(string(input))
	p := parser.New(l)
	prog := p.ParseProgram()
	if len(p.Errors()) > 0 {
		return nil, fmt.Errorf("syntax errors in module %s: %v", filePath, p.Errors())
	}

	return r.Resolve(prog)
}

func (r *Resolver) Resolve(program *ast.Program) (*ast.Program, error) {
	// First pass: collect macro definitions
	for _, stmt := range program.Statements {
		if mac, ok := stmt.(*ast.MacroDeclStmt); ok {
			r.macros[mac.Name.Value] = mac
		}
	}

	var newStmts []ast.Statement
	for _, stmt := range program.Statements {
		if _, isMac := stmt.(*ast.MacroDeclStmt); isMac {
			continue
		}
		if imp, ok := stmt.(*ast.ImportStmt); ok {
			modProg, err := r.resolveImport(imp)
			if err != nil {
				return nil, err
			}
			newStmts = append(newStmts, modProg.Statements...)
		} else {
			expandedStmts := r.expandStmt(stmt)
			for _, estmt := range expandedStmts {
				r.transformStmt(estmt)
				newStmts = append(newStmts, estmt)
			}
		}
	}

	program.Statements = newStmts

	opt := optimizer.New()
	program = opt.Optimize(program)

	return program, nil
}

func (r *Resolver) expandStmt(stmt ast.Statement) []ast.Statement {
	switch s := stmt.(type) {
	case *ast.ExprStmt:
		if call, ok := s.Expr.(*ast.CallExpr); ok {
			if fnId, ok := call.Function.(*ast.Identifier); ok {
				if mac, isMac := r.macros[fnId.Value]; isMac {
					return r.expandMacro(mac, call.Arguments)
				}
			}
		}
	case *ast.FnDeclStmt:
		var newBodyStmts []ast.Statement
		for _, bStmt := range s.Body.Statements {
			newBodyStmts = append(newBodyStmts, r.expandStmt(bStmt)...)
		}
		s.Body.Statements = newBodyStmts
	case *ast.IfStmt:
		var newConsequence []ast.Statement
		for _, bStmt := range s.Consequence.Statements {
			newConsequence = append(newConsequence, r.expandStmt(bStmt)...)
		}
		s.Consequence.Statements = newConsequence
		if s.Alternative != nil {
			var newAlt []ast.Statement
			for _, bStmt := range s.Alternative.Statements {
				newAlt = append(newAlt, r.expandStmt(bStmt)...)
			}
			s.Alternative.Statements = newAlt
		}
	case *ast.WhileStmt:
		var newBodyStmts []ast.Statement
		for _, bStmt := range s.Body.Statements {
			newBodyStmts = append(newBodyStmts, r.expandStmt(bStmt)...)
		}
		s.Body.Statements = newBodyStmts
	case *ast.ForInStmt:
		var newBodyStmts []ast.Statement
		for _, bStmt := range s.Body.Statements {
			newBodyStmts = append(newBodyStmts, r.expandStmt(bStmt)...)
		}
		s.Body.Statements = newBodyStmts
	}
	return []ast.Statement{stmt}
}

func (r *Resolver) expandMacro(mac *ast.MacroDeclStmt, args []ast.Expression) []ast.Statement {
	subMap := make(map[string]ast.Expression)
	for i, param := range mac.Params {
		if i < len(args) {
			subMap[param.Value] = args[i]
		}
	}
	var expanded []ast.Statement
	for _, stmt := range mac.Body.Statements {
		subStmt := substituteStmt(stmt, subMap)
		expanded = append(expanded, r.expandStmt(subStmt)...)
	}
	return expanded
}

func substituteStmt(stmt ast.Statement, subMap map[string]ast.Expression) ast.Statement {
	switch s := stmt.(type) {
	case *ast.VarDeclStmt:
		return &ast.VarDeclStmt{
			Token: s.Token,
			Name:  s.Name,
			Type:  s.Type,
			Value: substituteExpr(s.Value, subMap),
		}
	case *ast.ReturnStmt:
		if s.Value != nil {
			return &ast.ReturnStmt{
				Token: s.Token,
				Value: substituteExpr(s.Value, subMap),
			}
		}
		return s
	case *ast.ExprStmt:
		return &ast.ExprStmt{
			Token: s.Token,
			Expr:  substituteExpr(s.Expr, subMap),
		}
	case *ast.IfStmt:
		var newElifs []ast.ElifClause
		for _, e := range s.Elifs {
			newElifs = append(newElifs, ast.ElifClause{
				Condition:   substituteExpr(e.Condition, subMap),
				Consequence: substituteBlock(e.Consequence, subMap),
			})
		}
		var newAlt *ast.BlockStmt
		if s.Alternative != nil {
			newAlt = substituteBlock(s.Alternative, subMap)
		}
		return &ast.IfStmt{
			Token:       s.Token,
			Condition:   substituteExpr(s.Condition, subMap),
			Consequence: substituteBlock(s.Consequence, subMap),
			Elifs:       newElifs,
			Alternative: newAlt,
		}
	default:
		return stmt
	}
}

func substituteBlock(block *ast.BlockStmt, subMap map[string]ast.Expression) *ast.BlockStmt {
	var newStmts []ast.Statement
	for _, stmt := range block.Statements {
		newStmts = append(newStmts, substituteStmt(stmt, subMap))
	}
	return &ast.BlockStmt{Statements: newStmts}
}

func substituteExpr(expr ast.Expression, subMap map[string]ast.Expression) ast.Expression {
	if expr == nil {
		return nil
	}
	switch e := expr.(type) {
	case *ast.Identifier:
		if sub, ok := subMap[e.Value]; ok {
			return sub
		}
		return e
	case *ast.PrefixExpr:
		return &ast.PrefixExpr{
			Token:    e.Token,
			Operator: e.Operator,
			Right:    substituteExpr(e.Right, subMap),
		}
	case *ast.InfixExpr:
		return &ast.InfixExpr{
			Token:    e.Token,
			Left:     substituteExpr(e.Left, subMap),
			Operator: e.Operator,
			Right:    substituteExpr(e.Right, subMap),
		}
	case *ast.CallExpr:
		var newArgs []ast.Expression
		for _, a := range e.Arguments {
			newArgs = append(newArgs, substituteExpr(a, subMap))
		}
		return &ast.CallExpr{
			Token:     e.Token,
			Function:  substituteExpr(e.Function, subMap),
			Arguments: newArgs,
		}
	case *ast.MemberExpr:
		return &ast.MemberExpr{
			Token:  e.Token,
			Object: substituteExpr(e.Object, subMap),
			Member: e.Member,
		}
	default:
		return expr
	}
}

func (r *Resolver) resolveImport(imp *ast.ImportStmt) (*ast.Program, error) {
	targetPath := filepath.Join(r.currentDir, imp.ModulePath+".cb")
	if _, err := os.Stat(targetPath); os.IsNotExist(err) {
		targetPath = imp.ModulePath + ".cb"
	}

	if r.imported[imp.ModulePath] {
		return &ast.Program{}, nil
	}

	input, err := os.ReadFile(targetPath)
	if err != nil {
		return nil, fmt.Errorf("module resolution error: import %s not found (path: %s)", imp.ModulePath, targetPath)
	}

	r.imported[imp.ModulePath] = true
	modPrefix := strings.ReplaceAll(imp.ModulePath, "/", "_")

	l := lexer.New(string(input))
	p := parser.New(l)
	modProg := p.ParseProgram()
	if len(p.Errors()) > 0 {
		return nil, fmt.Errorf("syntax errors in module %s: %v", imp.ModulePath, p.Errors())
	}

	var modStmts []ast.Statement
	for _, stmt := range modProg.Statements {
		if fn, ok := stmt.(*ast.FnDeclStmt); ok {
			fn.Name.Value = modPrefix + "_" + fn.Name.Value
			modStmts = append(modStmts, fn)
		} else if st, ok := stmt.(*ast.StructDeclStmt); ok {
			st.Name.Value = modPrefix + "_" + st.Name.Value
			modStmts = append(modStmts, st)
		} else {
			modStmts = append(modStmts, stmt)
		}
	}
	modProg.Statements = modStmts

	return r.Resolve(modProg)
}

func (r *Resolver) transformStmt(stmt ast.Statement) {
	switch s := stmt.(type) {
	case *ast.VarDeclStmt:
		s.Value = r.transformExpr(s.Value)
	case *ast.TupleVarDeclStmt:
		s.Value = r.transformExpr(s.Value)
	case *ast.FnDeclStmt:
		for _, bStmt := range s.Body.Statements {
			r.transformStmt(bStmt)
		}
	case *ast.ReturnStmt:
		if s.Value != nil {
			s.Value = r.transformExpr(s.Value)
		}
	case *ast.IfStmt:
		s.Condition = r.transformExpr(s.Condition)
		for _, bStmt := range s.Consequence.Statements {
			r.transformStmt(bStmt)
		}
		for _, elif := range s.Elifs {
			elif.Condition = r.transformExpr(elif.Condition)
			for _, bStmt := range elif.Consequence.Statements {
				r.transformStmt(bStmt)
			}
		}
		if s.Alternative != nil {
			for _, bStmt := range s.Alternative.Statements {
				r.transformStmt(bStmt)
			}
		}
	case *ast.WhileStmt:
		s.Condition = r.transformExpr(s.Condition)
		for _, bStmt := range s.Body.Statements {
			r.transformStmt(bStmt)
		}
	case *ast.ForInStmt:
		s.Iterable = r.transformExpr(s.Iterable)
		for _, bStmt := range s.Body.Statements {
			r.transformStmt(bStmt)
		}
	case *ast.MatchStmt:
		s.Expr = r.transformExpr(s.Expr)
		for _, c := range s.Cases {
			c.Pattern = r.transformExpr(c.Pattern)
			r.transformStmt(c.Body)
		}
	case *ast.ExprStmt:
		if s.Expr != nil {
			s.Expr = r.transformExpr(s.Expr)
		}
	case *ast.SpawnStmt:
		s.Call = r.transformExpr(s.Call)
	case *ast.ExternFnStmt:
		return
	}
}

func (r *Resolver) transformExpr(expr ast.Expression) ast.Expression {
	if expr == nil {
		return nil
	}
	switch e := expr.(type) {
	case *ast.TryExpr:
		e.Expr = r.transformExpr(e.Expr)
	case *ast.InfixExpr:
		e.Left = r.transformExpr(e.Left)
		e.Right = r.transformExpr(e.Right)
	case *ast.PrefixExpr:
		e.Right = r.transformExpr(e.Right)
	case *ast.CallExpr:
		if mem, ok := e.Function.(*ast.MemberExpr); ok {
			if objId, ok := mem.Object.(*ast.Identifier); ok {
				if r.imported[objId.Value] {
					modPrefix := strings.ReplaceAll(objId.Value, "/", "_")
					e.Function = &ast.Identifier{
						Token: objId.Token,
						Value: modPrefix + "_" + mem.Member.Value,
					}
				}
			}
		}
		e.Function = r.transformExpr(e.Function)
		for i, arg := range e.Arguments {
			e.Arguments[i] = r.transformExpr(arg)
		}
	case *ast.MemberExpr:
		if objId, ok := e.Object.(*ast.Identifier); ok {
			if r.imported[objId.Value] {
				modPrefix := strings.ReplaceAll(objId.Value, "/", "_")
				return &ast.Identifier{
					Token: objId.Token,
					Value: modPrefix + "_" + e.Member.Value,
				}
			}
		}
		e.Object = r.transformExpr(e.Object)
	case *ast.ArrayLiteral:
		for i, el := range e.Elements {
			e.Elements[i] = r.transformExpr(el)
		}
	case *ast.MapLiteral:
		for i := range e.Keys {
			e.Keys[i] = r.transformExpr(e.Keys[i])
			e.Values[i] = r.transformExpr(e.Values[i])
		}
	case *ast.TupleLiteral:
		for i, el := range e.Elements {
			e.Elements[i] = r.transformExpr(el)
		}
	case *ast.TupleIndexExpr:
		e.Left = r.transformExpr(e.Left)
	case *ast.IndexExpr:
		e.Left = r.transformExpr(e.Left)
		e.Index = r.transformExpr(e.Index)
	case *ast.FnLiteral:
		for _, bStmt := range e.Body.Statements {
			r.transformStmt(bStmt)
		}
	}
	return expr
}
