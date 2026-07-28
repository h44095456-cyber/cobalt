package optimizer

import (
	"cobalt/pkg/ast"
	"cobalt/pkg/token"
	"fmt"
)

type Optimizer struct {
	foldedCount   int
	deadCodeCount int
	inlinedCount  int
}

func New() *Optimizer {
	return &Optimizer{}
}

// Optimize runs iterative optimization passes until fixed point convergence.
func (o *Optimizer) Optimize(program *ast.Program) *ast.Program {
	for pass := 0; pass < 5; pass++ {
		prevFolded := o.foldedCount
		prevDead := o.deadCodeCount
		prevInlined := o.inlinedCount

		var newStmts []ast.Statement
		for _, stmt := range program.Statements {
			optStmt := o.optimizeStatement(stmt)
			if optStmt != nil {
				newStmts = append(newStmts, optStmt)
			}
		}
		program.Statements = newStmts

		// Fixed point reached: no new optimizations in this pass
		if o.foldedCount == prevFolded && o.deadCodeCount == prevDead && o.inlinedCount == prevInlined {
			break
		}
	}
	return program
}

func (o *Optimizer) Stats() (int, int, int) {
	return o.foldedCount, o.deadCodeCount, o.inlinedCount
}

func (o *Optimizer) optimizeStatement(stmt ast.Statement) ast.Statement {
	if stmt == nil {
		return nil
	}

	switch s := stmt.(type) {
	case *ast.VarDeclStmt:
		s.Value = o.optimizeExpression(s.Value)
		return s

	case *ast.ReturnStmt:
		if s.Value != nil {
			s.Value = o.optimizeExpression(s.Value)
		}
		return s

	case *ast.ExprStmt:
		if s.Expr != nil {
			s.Expr = o.optimizeExpression(s.Expr)
		}
		return s

	case *ast.FnDeclStmt:
		var optBodyStmts []ast.Statement
		hasReturned := false
		for _, bStmt := range s.Body.Statements {
			if hasReturned {
				o.deadCodeCount++
				continue // Dead code elimination following return/break
			}
			optB := o.optimizeStatement(bStmt)
			if optB != nil {
				optBodyStmts = append(optBodyStmts, optB)
			}
			if _, isRet := bStmt.(*ast.ReturnStmt); isRet {
				hasReturned = true
			}
		}
		s.Body.Statements = optBodyStmts
		return s

	case *ast.IfStmt:
		s.Condition = o.optimizeExpression(s.Condition)
		if boolLit, ok := s.Condition.(*ast.BoolLiteral); ok {
			o.deadCodeCount++
			if boolLit.Value {
				// Condition is always true: inline consequence block
				if len(s.Consequence.Statements) == 1 {
					return o.optimizeStatement(s.Consequence.Statements[0])
				}
			} else {
				// Condition is always false: replace with alternative block
				if s.Alternative != nil && len(s.Alternative.Statements) > 0 {
					if len(s.Alternative.Statements) == 1 {
						return o.optimizeStatement(s.Alternative.Statements[0])
					}
				} else {
					return nil // Completely strip dead if block
				}
			}
		}
		return s

	default:
		return stmt
	}
}

func (o *Optimizer) optimizeExpression(expr ast.Expression) ast.Expression {
	if expr == nil {
		return nil
	}

	switch e := expr.(type) {
	case *ast.InfixExpr:
		e.Left = o.optimizeExpression(e.Left)
		e.Right = o.optimizeExpression(e.Right)

		// Constant Folding for Integer Arithmetic
		leftInt, leftIsInt := e.Left.(*ast.IntegerLiteral)
		rightInt, rightIsInt := e.Right.(*ast.IntegerLiteral)
		if leftIsInt && rightIsInt {
			o.foldedCount++
			switch e.Operator {
			case "+":
				return &ast.IntegerLiteral{Token: token.Token{Literal: fmt.Sprintf("%d", leftInt.Value+rightInt.Value)}, Value: leftInt.Value + rightInt.Value}
			case "-":
				return &ast.IntegerLiteral{Token: token.Token{Literal: fmt.Sprintf("%d", leftInt.Value-rightInt.Value)}, Value: leftInt.Value - rightInt.Value}
			case "*":
				return &ast.IntegerLiteral{Token: token.Token{Literal: fmt.Sprintf("%d", leftInt.Value*rightInt.Value)}, Value: leftInt.Value * rightInt.Value}
			case "/":
				if rightInt.Value != 0 {
					return &ast.IntegerLiteral{Token: token.Token{Literal: fmt.Sprintf("%d", leftInt.Value/rightInt.Value)}, Value: leftInt.Value / rightInt.Value}
				}
			case "%":
				if rightInt.Value != 0 {
					return &ast.IntegerLiteral{Token: token.Token{Literal: fmt.Sprintf("%d", leftInt.Value%rightInt.Value)}, Value: leftInt.Value % rightInt.Value}
				}
			case ">":
				return &ast.BoolLiteral{Token: token.Token{Literal: fmt.Sprintf("%t", leftInt.Value > rightInt.Value)}, Value: leftInt.Value > rightInt.Value}
			case "<":
				return &ast.BoolLiteral{Token: token.Token{Literal: fmt.Sprintf("%t", leftInt.Value < rightInt.Value)}, Value: leftInt.Value < rightInt.Value}
			case ">=":
				return &ast.BoolLiteral{Token: token.Token{Literal: fmt.Sprintf("%t", leftInt.Value >= rightInt.Value)}, Value: leftInt.Value >= rightInt.Value}
			case "<=":
				return &ast.BoolLiteral{Token: token.Token{Literal: fmt.Sprintf("%t", leftInt.Value <= rightInt.Value)}, Value: leftInt.Value <= rightInt.Value}
			case "==":
				return &ast.BoolLiteral{Token: token.Token{Literal: fmt.Sprintf("%t", leftInt.Value == rightInt.Value)}, Value: leftInt.Value == rightInt.Value}
			case "!=":
				return &ast.BoolLiteral{Token: token.Token{Literal: fmt.Sprintf("%t", leftInt.Value != rightInt.Value)}, Value: leftInt.Value != rightInt.Value}
			}
		}

		// Constant Folding for String Concatenation
		leftStr, leftIsStr := e.Left.(*ast.StringLiteral)
		rightStr, rightIsStr := e.Right.(*ast.StringLiteral)
		if leftIsStr && rightIsStr && e.Operator == "+" {
			o.foldedCount++
			return &ast.StringLiteral{Token: e.Token, Value: leftStr.Value + rightStr.Value}
		}

		// Constant Folding for Boolean Operations
		leftBool, leftIsBool := e.Left.(*ast.BoolLiteral)
		rightBool, rightIsBool := e.Right.(*ast.BoolLiteral)
		if leftIsBool && rightIsBool {
			o.foldedCount++
			switch e.Operator {
			case "and", "&&":
				return &ast.BoolLiteral{Token: token.Token{Literal: "true"}, Value: leftBool.Value && rightBool.Value}
			case "or", "||":
				return &ast.BoolLiteral{Token: token.Token{Literal: "true"}, Value: leftBool.Value || rightBool.Value}
			}
		}

		return e

	case *ast.PrefixExpr:
		e.Right = o.optimizeExpression(e.Right)
		if boolLit, ok := e.Right.(*ast.BoolLiteral); ok && (e.Operator == "!" || e.Operator == "not") {
			o.foldedCount++
			return &ast.BoolLiteral{Token: boolLit.Token, Value: !boolLit.Value}
		}
		return e

	case *ast.CallExpr:
		e.Function = o.optimizeExpression(e.Function)
		for i, arg := range e.Arguments {
			e.Arguments[i] = o.optimizeExpression(arg)
		}
		return e

	default:
		return expr
	}
}
