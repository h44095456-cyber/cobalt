package ast

import (
	"bytes"
	"cobalt/pkg/token"
	"fmt"
	"strings"
)

type Node interface {
	TokenLiteral() string
	String() string
}

type Statement interface {
	Node
	statementNode()
}

type Expression interface {
	Node
	expressionNode()
}

type Program struct {
	Statements []Statement
}

func (p *Program) TokenLiteral() string {
	if len(p.Statements) > 0 {
		return p.Statements[0].TokenLiteral()
	}
	return ""
}

func (p *Program) String() string {
	var out bytes.Buffer
	for _, s := range p.Statements {
		out.WriteString(s.String())
		out.WriteString("\n")
	}
	return out.String()
}

// Statements

type ImportStmt struct {
	Token      token.Token // IMPORT
	ModulePath string      // e.g. "math"
}

func (i *ImportStmt) statementNode()       {}
func (i *ImportStmt) TokenLiteral() string { return i.Token.Literal }
func (i *ImportStmt) String() string       { return fmt.Sprintf("import %s", i.ModulePath) }

type VarDeclStmt struct {
	Token token.Token // LET or VAR
	IsVar bool
	Name  *Identifier
	Type  string // optional type annotation (e.g. "int", "string")
	Value Expression
}

func (v *VarDeclStmt) statementNode()       {}
func (v *VarDeclStmt) TokenLiteral() string { return v.Token.Literal }
func (v *VarDeclStmt) String() string {
	keyword := "let"
	if v.IsVar {
		keyword = "var"
	}
	typeStr := ""
	if v.Type != "" {
		typeStr = ": " + v.Type
	}
	valStr := ""
	if v.Value != nil {
		valStr = " = " + v.Value.String()
	}
	return fmt.Sprintf("%s %s%s%s", keyword, v.Name.String(), typeStr, valStr)
}

type TupleVarDeclStmt struct {
	Token token.Token // LET or VAR
	IsVar bool
	Names []*Identifier
	Value Expression
}

func (t *TupleVarDeclStmt) statementNode()       {}
func (t *TupleVarDeclStmt) TokenLiteral() string { return t.Token.Literal }
func (t *TupleVarDeclStmt) String() string {
	keyword := "let"
	if t.IsVar {
		keyword = "var"
	}
	var names []string
	for _, n := range t.Names {
		names = append(names, n.String())
	}
	return fmt.Sprintf("%s (%s) = %s", keyword, strings.Join(names, ", "), t.Value.String())
}

type Param struct {
	Name         *Identifier
	Type         string
	DefaultValue Expression
}

type DeferStmt struct {
	Token token.Token // DEFER
	Expr  Expression
}

func (d *DeferStmt) statementNode()       {}
func (d *DeferStmt) TokenLiteral() string { return d.Token.Literal }
func (d *DeferStmt) String() string       { return "defer " + d.Expr.String() }

type FnDeclStmt struct {
	Token      token.Token // FN or ASYNC
	Decorators []string    // e.g. ["inline"]
	IsAsync    bool
	IsRPC      bool
	Receiver   *Param // optional method receiver e.g. (r: Rectangle)
	Name       *Identifier
	TypeParams []string // optional type parameters e.g. <T, U>
	Params     []Param
	ReturnType string
	Body       *BlockStmt
}

func (f *FnDeclStmt) statementNode()       {}
func (f *FnDeclStmt) TokenLiteral() string { return f.Token.Literal }
func (f *FnDeclStmt) String() string {
	var params []string
	for _, p := range f.Params {
		params = append(params, fmt.Sprintf("%s: %s", p.Name.String(), p.Type))
	}
	retType := ""
	if f.ReturnType != "" {
		retType = " -> " + f.ReturnType
	}
	typeParams := ""
	if len(f.TypeParams) > 0 {
		typeParams = "<" + strings.Join(f.TypeParams, ", ") + ">"
	}
	recvStr := ""
	if f.Receiver != nil {
		recvStr = fmt.Sprintf("(%s: %s) ", f.Receiver.Name.String(), f.Receiver.Type)
	}
	bodyStr := ""
	if f.Body != nil {
		bodyStr = ":\n" + f.Body.String()
	}
	return fmt.Sprintf("fn %s%s%s(%s)%s%s", recvStr, f.Name.String(), typeParams, strings.Join(params, ", "), retType, bodyStr)
}

type TraitDeclStmt struct {
	Token   token.Token // TRAIT
	Name    *Identifier
	Methods []*FnDeclStmt
}

func (t *TraitDeclStmt) statementNode()       {}
func (t *TraitDeclStmt) TokenLiteral() string { return t.Token.Literal }
func (t *TraitDeclStmt) String() string {
	var out bytes.Buffer
	out.WriteString(fmt.Sprintf("trait %s:\n", t.Name.String()))
	for _, m := range t.Methods {
		out.WriteString("    " + m.String() + "\n")
	}
	return out.String()
}

type ImplDeclStmt struct {
	Token      token.Token // IMPL
	TraitName  *Identifier
	TargetType string
	Methods    []*FnDeclStmt
}

func (i *ImplDeclStmt) statementNode()       {}
func (i *ImplDeclStmt) TokenLiteral() string { return i.Token.Literal }
func (i *ImplDeclStmt) String() string {
	var out bytes.Buffer
	out.WriteString(fmt.Sprintf("impl %s for %s:\n", i.TraitName.String(), i.TargetType))
	for _, m := range i.Methods {
		out.WriteString("    " + m.String() + "\n")
	}
	return out.String()
}

type SpawnStmt struct {
	Token token.Token // SPAWN
	Call  Expression
}

func (s *SpawnStmt) statementNode()       {}
func (s *SpawnStmt) TokenLiteral() string { return s.Token.Literal }
func (s *SpawnStmt) String() string       { return fmt.Sprintf("spawn %s", s.Call.String()) }

type ExternFnStmt struct {
	Token      token.Token // EXTERN
	Abi        string      // e.g. "C"
	Name       *Identifier
	Params     []Param
	ReturnType string
}

func (e *ExternFnStmt) statementNode()       {}
func (e *ExternFnStmt) TokenLiteral() string { return e.Token.Literal }
func (e *ExternFnStmt) String() string {
	var params []string
	for _, p := range e.Params {
		params = append(params, fmt.Sprintf("%s: %s", p.Name.String(), p.Type))
	}
	retType := ""
	if e.ReturnType != "" {
		retType = " -> " + e.ReturnType
	}
	return fmt.Sprintf("extern %q fn %s(%s)%s", e.Abi, e.Name.String(), strings.Join(params, ", "), retType)
}

type MacroDeclStmt struct {
	Token  token.Token // MACRO
	Name   *Identifier
	Params []*Identifier
	Body   *BlockStmt
}

func (m *MacroDeclStmt) statementNode()       {}
func (m *MacroDeclStmt) TokenLiteral() string { return m.Token.Literal }
func (m *MacroDeclStmt) String() string {
	var params []string
	for _, p := range m.Params {
		params = append(params, p.String())
	}
	return fmt.Sprintf("macro %s(%s):\n%s", m.Name.String(), strings.Join(params, ", "), m.Body.String())
}

type ReturnStmt struct {
	Token token.Token // RETURN
	Value Expression
}

func (r *ReturnStmt) statementNode()       {}
func (r *ReturnStmt) TokenLiteral() string { return r.Token.Literal }
func (r *ReturnStmt) String() string {
	if r.Value != nil {
		return "return " + r.Value.String()
	}
	return "return"
}

type BlockStmt struct {
	Statements []Statement
}

func (b *BlockStmt) statementNode()       {}
func (b *BlockStmt) TokenLiteral() string { return "" }
func (b *BlockStmt) String() string {
	var out bytes.Buffer
	for _, s := range b.Statements {
		lines := strings.Split(s.String(), "\n")
		for _, line := range lines {
			if line != "" {
				out.WriteString("    " + line + "\n")
			}
		}
	}
	return out.String()
}

type ElifClause struct {
	Condition   Expression
	Consequence *BlockStmt
}

type IfStmt struct {
	Token       token.Token // IF
	Condition   Expression
	Consequence *BlockStmt
	Elifs       []ElifClause
	Alternative *BlockStmt
}

func (i *IfStmt) statementNode()       {}
func (i *IfStmt) TokenLiteral() string { return i.Token.Literal }
func (i *IfStmt) String() string {
	var out bytes.Buffer
	out.WriteString(fmt.Sprintf("if %s:\n%s", i.Condition.String(), i.Consequence.String()))
	for _, elif := range i.Elifs {
		out.WriteString(fmt.Sprintf("elif %s:\n%s", elif.Condition.String(), elif.Consequence.String()))
	}
	if i.Alternative != nil {
		out.WriteString(fmt.Sprintf("else:\n%s", i.Alternative.String()))
	}
	return out.String()
}

type WhileStmt struct {
	Token     token.Token // WHILE
	Condition Expression
	Body      *BlockStmt
}

func (w *WhileStmt) statementNode()       {}
func (w *WhileStmt) TokenLiteral() string { return w.Token.Literal }
func (w *WhileStmt) String() string {
	return fmt.Sprintf("while %s:\n%s", w.Condition.String(), w.Body.String())
}

type ForInStmt struct {
	Token    token.Token // FOR
	VarName  *Identifier
	Iterable Expression
	Body     *BlockStmt
}

func (f *ForInStmt) statementNode()       {}
func (f *ForInStmt) TokenLiteral() string { return f.Token.Literal }
func (f *ForInStmt) String() string {
	return fmt.Sprintf("for %s in %s:\n%s", f.VarName.String(), f.Iterable.String(), f.Body.String())
}

type MatchCase struct {
	Pattern Expression
	Guard   Expression
	Body    Statement
}

type MatchStmt struct {
	Token token.Token // MATCH
	Expr  Expression
	Cases []MatchCase
}

func (m *MatchStmt) statementNode()       {}
func (m *MatchStmt) TokenLiteral() string { return m.Token.Literal }
func (m *MatchStmt) String() string {
	var out bytes.Buffer
	out.WriteString(fmt.Sprintf("match %s:\n", m.Expr.String()))
	for _, c := range m.Cases {
		guardStr := ""
		if c.Guard != nil {
			guardStr = " if " + c.Guard.String()
		}
		out.WriteString(fmt.Sprintf("    %s%s => %s\n", c.Pattern.String(), guardStr, c.Body.String()))
	}
	return out.String()
}

type ExprStmt struct {
	Token token.Token
	Expr  Expression
}

func (e *ExprStmt) statementNode()       {}
func (e *ExprStmt) TokenLiteral() string { return e.Token.Literal }
func (e *ExprStmt) String() string {
	if e.Expr != nil {
		return e.Expr.String()
	}
	return ""
}

type StructField struct {
	Name *Identifier
	Type string
}

type StructDeclStmt struct {
	Token      token.Token // STRUCT
	Decorators []string    // e.g. ["derive(Debug)", "derive(Clone)", "derive(Eq)"]
	IsReflect  bool
	Name       *Identifier
	TypeParams []string // optional type parameters e.g. <T>
	Fields     []StructField
}

func (s *StructDeclStmt) statementNode()       {}
func (s *StructDeclStmt) TokenLiteral() string { return s.Token.Literal }
func (s *StructDeclStmt) String() string {
	var out bytes.Buffer
	typeParams := ""
	if len(s.TypeParams) > 0 {
		typeParams = "<" + strings.Join(s.TypeParams, ", ") + ">"
	}
	out.WriteString(fmt.Sprintf("struct %s%s:\n", s.Name.String(), typeParams))
	for _, f := range s.Fields {
		out.WriteString(fmt.Sprintf("    %s: %s\n", f.Name.String(), f.Type))
	}
	return out.String()
}

// Expressions

type Identifier struct {
	Token token.Token // IDENT
	Value string
}

func (i *Identifier) expressionNode()      {}
func (i *Identifier) TokenLiteral() string { return i.Token.Literal }
func (i *Identifier) String() string       { return i.Value }

type IntegerLiteral struct {
	Token token.Token
	Value int64
}

func (il *IntegerLiteral) expressionNode()      {}
func (il *IntegerLiteral) TokenLiteral() string { return il.Token.Literal }
func (il *IntegerLiteral) String() string       { return il.Token.Literal }

type FloatLiteral struct {
	Token token.Token
	Value float64
}

func (fl *FloatLiteral) expressionNode()      {}
func (fl *FloatLiteral) TokenLiteral() string { return fl.Token.Literal }
func (fl *FloatLiteral) String() string       { return fl.Token.Literal }

type StringLiteral struct {
	Token token.Token
	Value string
}

func (sl *StringLiteral) expressionNode()      {}
func (sl *StringLiteral) TokenLiteral() string { return sl.Token.Literal }
func (sl *StringLiteral) String() string       { return fmt.Sprintf("%q", sl.Value) }

type OptChainExpr struct {
	Token  token.Token
	Object Expression
	Member *Identifier
}

func (oc *OptChainExpr) expressionNode()      {}
func (oc *OptChainExpr) TokenLiteral() string { return oc.Token.Literal }
func (oc *OptChainExpr) String() string       { return oc.Object.String() + "?." + oc.Member.Value }

type FStringLiteral struct {
	Token token.Token
	Parts []Expression
}

func (fs *FStringLiteral) expressionNode()      {}
func (fs *FStringLiteral) TokenLiteral() string { return fs.Token.Literal }
func (fs *FStringLiteral) String() string       { return "f\"" + fs.Token.Literal + "\"" }

type BoolLiteral struct {
	Token token.Token
	Value bool
}

func (b *BoolLiteral) expressionNode()      {}
func (b *BoolLiteral) TokenLiteral() string { return b.Token.Literal }
func (b *BoolLiteral) String() string       { return b.Token.Literal }

type ArrayLiteral struct {
	Token    token.Token // [
	Elements []Expression
}

func (a *ArrayLiteral) expressionNode()      {}
func (a *ArrayLiteral) TokenLiteral() string { return a.Token.Literal }
func (a *ArrayLiteral) String() string {
	var elems []string
	for _, el := range a.Elements {
		elems = append(elems, el.String())
	}
	return fmt.Sprintf("[%s]", strings.Join(elems, ", "))
}

type MapLiteral struct {
	Token  token.Token // {
	Keys   []Expression
	Values []Expression
}

func (m *MapLiteral) expressionNode()      {}
func (m *MapLiteral) TokenLiteral() string { return m.Token.Literal }
func (m *MapLiteral) String() string {
	var pairs []string
	for i := 0; i < len(m.Keys); i++ {
		pairs = append(pairs, fmt.Sprintf("%s: %s", m.Keys[i].String(), m.Values[i].String()))
	}
	return fmt.Sprintf("{%s}", strings.Join(pairs, ", "))
}

type TupleLiteral struct {
	Token    token.Token // (
	Elements []Expression
}

func (t *TupleLiteral) expressionNode()      {}
func (t *TupleLiteral) TokenLiteral() string { return t.Token.Literal }
func (t *TupleLiteral) String() string {
	var elems []string
	for _, el := range t.Elements {
		elems = append(elems, el.String())
	}
	return fmt.Sprintf("(%s)", strings.Join(elems, ", "))
}

type TupleIndexExpr struct {
	Token token.Token // DOT
	Left  Expression
	Index int
}

func (t *TupleIndexExpr) expressionNode()      {}
func (t *TupleIndexExpr) TokenLiteral() string { return t.Token.Literal }
func (t *TupleIndexExpr) String() string       { return fmt.Sprintf("%s.%d", t.Left.String(), t.Index) }

type IndexExpr struct {
	Token token.Token // [
	Left  Expression
	Index Expression
}

func (i *IndexExpr) expressionNode()      {}
func (i *IndexExpr) TokenLiteral() string { return i.Token.Literal }
func (i *IndexExpr) String() string       { return fmt.Sprintf("%s[%s]", i.Left.String(), i.Index.String()) }

type PrefixExpr struct {
	Token    token.Token // e.g. - or !
	Operator string
	Right    Expression
}

func (p *PrefixExpr) expressionNode()      {}
func (p *PrefixExpr) TokenLiteral() string { return p.Token.Literal }
func (p *PrefixExpr) String() string       { return fmt.Sprintf("(%s%s)", p.Operator, p.Right.String()) }

type TryExpr struct {
	Token token.Token // ?
	Expr  Expression
}

func (t *TryExpr) expressionNode()      {}
func (t *TryExpr) TokenLiteral() string { return t.Token.Literal }
func (t *TryExpr) String() string       { return fmt.Sprintf("(%s?)", t.Expr.String()) }

type AwaitExpr struct {
	Token token.Token // AWAIT
	Expr  Expression
}

func (a *AwaitExpr) expressionNode()      {}
func (a *AwaitExpr) TokenLiteral() string { return a.Token.Literal }
func (a *AwaitExpr) String() string       { return fmt.Sprintf("await %s", a.Expr.String()) }

type FnLiteral struct {
	Token      token.Token // FN
	Params     []Param
	ReturnType string
	Body       *BlockStmt
}

func (f *FnLiteral) expressionNode()      {}
func (f *FnLiteral) TokenLiteral() string { return f.Token.Literal }
func (f *FnLiteral) String() string {
	var params []string
	for _, p := range f.Params {
		params = append(params, fmt.Sprintf("%s: %s", p.Name.String(), p.Type))
	}
	retType := ""
	if f.ReturnType != "" {
		retType = " -> " + f.ReturnType
	}
	return fmt.Sprintf("fn(%s)%s:\n%s", strings.Join(params, ", "), retType, f.Body.String())
}

type InfixExpr struct {
	Token    token.Token
	Left     Expression
	Operator string
	Right    Expression
}

func (i *InfixExpr) expressionNode()      {}
func (i *InfixExpr) TokenLiteral() string { return i.Token.Literal }
func (i *InfixExpr) String() string {
	return fmt.Sprintf("%s %s %s", i.Left.String(), i.Operator, i.Right.String())
}

type CallExpr struct {
	Token     token.Token // '('
	Function  Expression  // Identifier or MemberExpr
	TypeArgs  []string    // optional generic type arguments e.g. <int>
	Arguments []Expression
}

func (c *CallExpr) expressionNode()      {}
func (c *CallExpr) TokenLiteral() string { return c.Token.Literal }
func (c *CallExpr) String() string {
	var args []string
	for _, a := range c.Arguments {
		args = append(args, a.String())
	}
	typeArgs := ""
	if len(c.TypeArgs) > 0 {
		typeArgs = "<" + strings.Join(c.TypeArgs, ", ") + ">"
	}
	return fmt.Sprintf("%s%s(%s)", c.Function.String(), typeArgs, strings.Join(args, ", "))
}

type MemberExpr struct {
	Token  token.Token // DOT
	Object Expression
	Member *Identifier
}

func (m *MemberExpr) expressionNode()      {}
func (m *MemberExpr) TokenLiteral() string { return m.Token.Literal }
func (m *MemberExpr) String() string       { return fmt.Sprintf("%s.%s", m.Object.String(), m.Member.String()) }

type AsmExpr struct {
	Token       token.Token // ASM
	Instruction string
}

func (a *AsmExpr) expressionNode()      {}
func (a *AsmExpr) TokenLiteral() string { return a.Token.Literal }
func (a *AsmExpr) String() string       { return fmt.Sprintf("asm(%q)", a.Instruction) }

type RangeExpr struct {
	Token token.Token // ..
	Start Expression
	End   Expression
}

func (r *RangeExpr) expressionNode()      {}
func (r *RangeExpr) TokenLiteral() string { return r.Token.Literal }
func (r *RangeExpr) String() string       { return fmt.Sprintf("%s..%s", r.Start.String(), r.End.String()) }

