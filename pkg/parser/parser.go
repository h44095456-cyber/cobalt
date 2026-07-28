package parser

import (
	"bytes"
	"cobalt/pkg/ast"
	"cobalt/pkg/lexer"
	"cobalt/pkg/token"
	"fmt"
	"strconv"
	"strings"
)

const (
	_ int = iota
	LOWEST
	ASSIGN
	LOGICAL_OR
	LOGICAL_AND
	EQUALS       // == !=
	LESSGREATER  // < > <= >=
	SUM          // + -
	PRODUCT      // * / %
	RANGE        // ..
	PREFIX       // -X !X
	POSTFIX      // X?
	CALL         // myFunction(X)
	INDEX_MEMBER // record.field or arr[idx]
)

var precedences = map[token.TokenType]int{
	token.ASSIGN:   ASSIGN,
	token.EQ:       EQUALS,
	token.NEQ:      EQUALS,
	token.AND:      LOGICAL_AND,
	token.OR:       LOGICAL_OR,
	token.LT:       LESSGREATER,
	token.LTE:      LESSGREATER,
	token.GT:       LESSGREATER,
	token.GTE:      LESSGREATER,
	token.PLUS:     SUM,
	token.MINUS:    SUM,
	token.STAR:     PRODUCT,
	token.SLASH:    PRODUCT,
	token.MOD:      PRODUCT,
	token.DOTDOT:   RANGE,
	token.QUESTION:          POSTFIX,
	token.QUESTION_DOT:      INDEX_MEMBER,
	token.QUESTION_QUESTION: SUM,
	token.LPAREN:            CALL,
	token.LBRACKET: INDEX_MEMBER,
	token.DOT:      INDEX_MEMBER,
}

type (
	prefixParseFn  func() ast.Expression
	infixParseFn   func(ast.Expression) ast.Expression
	postfixParseFn func(ast.Expression) ast.Expression
)

type Parser struct {
	l      *lexer.Lexer
	errors []string

	curToken  token.Token
	peekToken token.Token

	prefixParseFns  map[token.TokenType]prefixParseFn
	infixParseFns   map[token.TokenType]infixParseFn
	postfixParseFns map[token.TokenType]postfixParseFn
}

func New(l *lexer.Lexer) *Parser {
	p := &Parser{
		l:      l,
		errors: []string{},
	}

	p.prefixParseFns = make(map[token.TokenType]prefixParseFn)
	p.registerPrefix(token.IDENT, p.parseIdentifier)
	p.registerPrefix(token.INT, p.parseIntegerLiteral)
	p.registerPrefix(token.FLOAT, p.parseFloatLiteral)
	p.registerPrefix(token.STRING, p.parseStringLiteral)
	p.registerPrefix(token.FSTRING, p.parseFStringLiteral)
	p.registerPrefix(token.BOOL, p.parseBoolLiteral)
	p.registerPrefix(token.LBRACKET, p.parseArrayLiteral)
	p.registerPrefix(token.LBRACE, p.parseMapLiteral)
	p.registerPrefix(token.LPAREN, p.parseGroupedExpression)
	p.registerPrefix(token.AWAIT, p.parseAwaitExpr)
	p.registerPrefix(token.ASM, p.parseAsmExpr)
	p.registerPrefix(token.MINUS, p.parsePrefixExpression)
	p.registerPrefix(token.BANG, p.parsePrefixExpression)
	p.registerPrefix(token.FN, p.parseFnLiteral)

	p.infixParseFns = make(map[token.TokenType]infixParseFn)
	p.registerInfix(token.PLUS, p.parseInfixExpression)
	p.registerInfix(token.MINUS, p.parseInfixExpression)
	p.registerInfix(token.STAR, p.parseInfixExpression)
	p.registerInfix(token.SLASH, p.parseInfixExpression)
	p.registerInfix(token.MOD, p.parseInfixExpression)
	p.registerInfix(token.ASSIGN, p.parseInfixExpression)
	p.registerInfix(token.EQ, p.parseInfixExpression)
	p.registerInfix(token.NEQ, p.parseInfixExpression)
	p.registerInfix(token.LT, p.parseInfixExpression)
	p.registerInfix(token.LTE, p.parseInfixExpression)
	p.registerInfix(token.GT, p.parseInfixExpression)
	p.registerInfix(token.GTE, p.parseInfixExpression)
	p.registerInfix(token.AND, p.parseInfixExpression)
	p.registerInfix(token.OR, p.parseInfixExpression)
	p.registerInfix(token.LPAREN, p.parseCallExpression)
	p.registerInfix(token.LBRACKET, p.parseIndexExpression)
	p.registerInfix(token.DOT, p.parseMemberExpression)
	p.registerInfix(token.QUESTION_DOT, p.parseOptChainExpr)
	p.registerInfix(token.QUESTION_QUESTION, p.parseInfixExpression)
	p.registerInfix(token.DOTDOT, p.parseInfixExpression)

	p.postfixParseFns = make(map[token.TokenType]postfixParseFn)
	p.registerPostfix(token.QUESTION, p.parseTryExpression)

	p.curToken = p.readNonNewlineToken()
	p.peekToken = p.readNonNewlineToken()

	return p
}

func (p *Parser) registerPostfix(tokenType token.TokenType, fn postfixParseFn) {
	p.postfixParseFns[tokenType] = fn
}

func (p *Parser) parseTryExpression(left ast.Expression) ast.Expression {
	return &ast.TryExpr{Token: p.curToken, Expr: left}
}

func (p *Parser) parseOptChainExpr(left ast.Expression) ast.Expression {
	tok := p.curToken
	if !p.expectPeek(token.IDENT) {
		return nil
	}
	member := &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}
	return &ast.OptChainExpr{Token: tok, Object: left, Member: member}
}

func (p *Parser) readNonNewlineToken() token.Token {
	tok := p.l.NextToken()
	for tok.Type == token.NEWLINE {
		tok = p.l.NextToken()
	}
	return tok
}

func (p *Parser) nextToken() {
	p.curToken = p.peekToken
	p.peekToken = p.readNonNewlineToken()
}

func (p *Parser) Errors() []string {
	return p.errors
}

func (p *Parser) registerPrefix(tokenType token.TokenType, fn prefixParseFn) {
	p.prefixParseFns[tokenType] = fn
}

func (p *Parser) registerInfix(tokenType token.TokenType, fn infixParseFn) {
	p.infixParseFns[tokenType] = fn
}

func (p *Parser) ParseProgram() *ast.Program {
	program := &ast.Program{Statements: []ast.Statement{}}

	for p.curToken.Type != token.EOF {
		stmt := p.parseStatement()
		if stmt != nil {
			program.Statements = append(program.Statements, stmt)
		}
		p.nextToken()
	}

	return program
}

func (p *Parser) parseStatement() ast.Statement {
	switch p.curToken.Type {
	case token.LET, token.VAR:
		return p.parseVarDeclStmt()
	case token.IMPORT:
		return p.parseImportStmt()
	case token.FN:
		return p.parseFnDeclStmt()
	case token.ASYNC:
		return p.parseFnDeclStmt()
	case token.RPC:
		return p.parseFnDeclStmt()
	case token.RETURN:
		return p.parseReturnStmt()
	case token.IF:
		return p.parseIfStmt()
	case token.WHILE:
		return p.parseWhileStmt()
	case token.FOR:
		return p.parseForInStmt()
	case token.MATCH:
		return p.parseMatchStmt()
	case token.STRUCT:
		return p.parseStructDeclStmt()
	case token.TRAIT:
		return p.parseTraitDeclStmt()
	case token.IMPL:
		return p.parseImplDeclStmt()
	case token.SPAWN:
		return p.parseSpawnStmt()
	case token.EXTERN:
		return p.parseExternFnStmt()
	case token.MACRO:
		return p.parseMacroDeclStmt()
	case token.DEFER:
		return p.parseDeferStmt()
	case token.AT:
		return p.parseDecoratedStmt()
	default:
		return p.parseExpressionStatement()
	}
}

func (p *Parser) parseImportStmt() *ast.ImportStmt {
	stmt := &ast.ImportStmt{Token: p.curToken}

	if !p.expectPeek(token.IDENT) && !p.expectPeek(token.STRING) {
		return nil
	}

	pathParts := []string{p.curToken.Literal}
	for p.peekToken.Literal == "/" || p.peekToken.Literal == "::" || p.peekToken.Literal == "." {
		p.nextToken() // consume separator
		if p.expectPeek(token.IDENT) || p.expectPeek(token.STRING) {
			pathParts = append(pathParts, "/", p.curToken.Literal)
		} else {
			break
		}
	}

	stmt.ModulePath = strings.Join(pathParts, "")
	return stmt
}

func (p *Parser) parseDecoratedStmt() ast.Statement {
	var decorators []string
	for p.curToken.Type == token.AT {
		p.nextToken() // consume @
		if p.curToken.Type != token.IDENT {
			return nil
		}
		decName := p.curToken.Literal
		p.nextToken()
		if p.curToken.Type == token.LPAREN {
			decName += "("
			p.nextToken()
			var args []string
			for p.curToken.Type != token.RPAREN && p.curToken.Type != token.EOF {
				if p.curToken.Type != token.COMMA {
					args = append(args, p.curToken.Literal)
				}
				p.nextToken()
			}
			decName += strings.Join(args, ", ") + ")"
			if p.curToken.Type == token.RPAREN {
				p.nextToken()
			}
		}
		decorators = append(decorators, decName)
		if p.curToken.Type == token.NEWLINE {
			p.nextToken()
		}
	}

	stmt := p.parseStatement()
	if fnStmt, ok := stmt.(*ast.FnDeclStmt); ok {
		fnStmt.Decorators = append(fnStmt.Decorators, decorators...)
		return fnStmt
	}
	if structStmt, ok := stmt.(*ast.StructDeclStmt); ok {
		structStmt.Decorators = append(structStmt.Decorators, decorators...)
		return structStmt
	}
	return stmt
}

func (p *Parser) parseVarDeclStmt() ast.Statement {
	isVar := p.curToken.Type == token.VAR
	tok := p.curToken

	if p.peekToken.Type == token.LPAREN {
		p.nextToken() // consume '('
		var names []*ast.Identifier
		for {
			if !p.expectPeek(token.IDENT) {
				return nil
			}
			names = append(names, &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal})
			if p.peekToken.Type == token.COMMA {
				p.nextToken() // consume ','
			} else {
				break
			}
		}
		if !p.expectPeek(token.RPAREN) {
			return nil
		}
		if !p.expectPeek(token.ASSIGN) {
			return nil
		}
		p.nextToken()
		val := p.parseExpression(LOWEST)
		return &ast.TupleVarDeclStmt{
			Token: tok,
			IsVar: isVar,
			Names: names,
			Value: val,
		}
	}

	stmt := &ast.VarDeclStmt{Token: tok, IsVar: isVar}

	if !p.expectPeek(token.IDENT) {
		return nil
	}

	stmt.Name = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}

	if p.peekToken.Type == token.COLON {
		p.nextToken()
		if !p.expectPeek(token.IDENT) {
			return nil
		}
		stmt.Type = p.curToken.Literal
	}

	if p.peekToken.Type == token.ASSIGN {
		p.nextToken()
		p.nextToken()
		stmt.Value = p.parseExpression(LOWEST)
	}

	return stmt
}

func (p *Parser) parseFnDeclStmt() *ast.FnDeclStmt {
	stmt := &ast.FnDeclStmt{Token: p.curToken}
	if p.curToken.Type == token.ASYNC {
		stmt.IsAsync = true
		if !p.expectPeek(token.FN) {
			return nil
		}
	} else if p.curToken.Type == token.RPC {
		stmt.IsRPC = true
		if !p.expectPeek(token.FN) {
			return nil
		}
	}

	if p.peekToken.Type == token.LPAREN {
		p.nextToken()
		p.nextToken()
		nameId := &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}
		if !p.expectPeek(token.COLON) {
			return nil
		}
		p.nextToken()
		typeStr := p.curToken.Literal
		if !p.expectPeek(token.RPAREN) {
			return nil
		}
		stmt.Receiver = &ast.Param{Name: nameId, Type: typeStr}
	}

	if !p.expectPeek(token.IDENT) {
		return nil
	}
	stmt.Name = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}

	if p.peekToken.Type == token.LT {
		p.nextToken() // consume '<'
		var typeParams []string
		for {
			if !p.expectPeek(token.IDENT) {
				return nil
			}
			typeParams = append(typeParams, p.curToken.Literal)
			if p.peekToken.Type == token.COMMA {
				p.nextToken()
			} else {
				break
			}
		}
		if !p.expectPeek(token.GT) {
			return nil
		}
		stmt.TypeParams = typeParams
	}

	if !p.expectPeek(token.LPAREN) {
		return nil
	}

	stmt.Params = p.parseFnParameters()

	if p.peekToken.Type == token.ARROW {
		p.nextToken() // consume '->'
		if p.peekToken.Type == token.LPAREN {
			p.nextToken() // consume '('
			var retParts []string
			retParts = append(retParts, "(")
			for {
				p.nextToken()
				retParts = append(retParts, p.curToken.Literal)
				if p.peekToken.Type == token.COMMA {
					p.nextToken()
					retParts = append(retParts, ",")
				} else {
					break
				}
			}
			if !p.expectPeek(token.RPAREN) {
				return nil
			}
			retParts = append(retParts, ")")
			stmt.ReturnType = strings.Join(retParts, "")
		} else {
			if !p.expectPeek(token.IDENT) {
				return nil
			}
			stmt.ReturnType = p.curToken.Literal
		}
	}

	if p.peekToken.Type == token.COLON {
		p.nextToken() // consume ':'
		if p.peekToken.Type == token.INDENT {
			p.nextToken()
			stmt.Body = p.parseBlockStmt()
		}
	}

	return stmt
}

func (p *Parser) parseFnParameters() []ast.Param {
	params := []ast.Param{}

	if p.peekToken.Type == token.RPAREN {
		p.nextToken()
		return params
	}

	p.nextToken()

	for {
		name := &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}
		typeStr := ""

		if p.peekToken.Type == token.COLON {
			p.nextToken() // consume ':'
			if !p.expectPeek(token.IDENT) {
				return nil
			}
			typeStr = p.curToken.Literal
		}

		var defaultVal ast.Expression
		if p.peekToken.Type == token.ASSIGN {
			p.nextToken() // consume '='
			p.nextToken()
			defaultVal = p.parseExpression(LOWEST)
		}

		params = append(params, ast.Param{Name: name, Type: typeStr, DefaultValue: defaultVal})

		if p.peekToken.Type == token.COMMA {
			p.nextToken()
			p.nextToken()
		} else {
			break
		}
	}

	if !p.expectPeek(token.RPAREN) {
		return nil
	}

	return params
}

func (p *Parser) parseReturnStmt() *ast.ReturnStmt {
	stmt := &ast.ReturnStmt{Token: p.curToken}

	if p.peekToken.Type != token.NEWLINE && p.peekToken.Type != token.DEDENT && p.peekToken.Type != token.EOF {
		p.nextToken()
		stmt.Value = p.parseExpression(LOWEST)
	}

	return stmt
}

func (p *Parser) parseBlockStmt() *ast.BlockStmt {
	block := &ast.BlockStmt{Statements: []ast.Statement{}}

	p.nextToken() // consume INDENT

	for p.curToken.Type != token.DEDENT && p.curToken.Type != token.EOF {
		stmt := p.parseStatement()
		if stmt != nil {
			block.Statements = append(block.Statements, stmt)
		}
		p.nextToken()
	}

	return block
}

func (p *Parser) parseIfStmt() *ast.IfStmt {
	stmt := &ast.IfStmt{Token: p.curToken}

	p.nextToken()
	stmt.Condition = p.parseExpression(LOWEST)

	if !p.expectPeek(token.COLON) || !p.expectPeek(token.INDENT) {
		return nil
	}

	stmt.Consequence = p.parseBlockStmt()

	for p.peekToken.Type == token.ELIF {
		p.nextToken() // consume ELIF
		p.nextToken()
		elifCond := p.parseExpression(LOWEST)
		if !p.expectPeek(token.COLON) || !p.expectPeek(token.INDENT) {
			return nil
		}
		elifBody := p.parseBlockStmt()
		stmt.Elifs = append(stmt.Elifs, ast.ElifClause{Condition: elifCond, Consequence: elifBody})
	}

	if p.peekToken.Type == token.ELSE {
		p.nextToken() // consume ELSE
		if !p.expectPeek(token.COLON) || !p.expectPeek(token.INDENT) {
			return nil
		}
		stmt.Alternative = p.parseBlockStmt()
	}

	return stmt
}

func (p *Parser) parseAwaitExpr() ast.Expression {
	expression := &ast.AwaitExpr{Token: p.curToken}
	p.nextToken()
	expression.Expr = p.parseExpression(PREFIX)
	return expression
}

func (p *Parser) parseAsmExpr() ast.Expression {
	expression := &ast.AsmExpr{Token: p.curToken}
	if !p.expectPeek(token.LPAREN) {
		return nil
	}
	if !p.expectPeek(token.STRING) {
		return nil
	}
	expression.Instruction = p.curToken.Literal
	if !p.expectPeek(token.RPAREN) {
		return nil
	}
	return expression
}

func (p *Parser) parseWhileStmt() *ast.WhileStmt {
	stmt := &ast.WhileStmt{Token: p.curToken}

	p.nextToken()
	stmt.Condition = p.parseExpression(LOWEST)

	if !p.expectPeek(token.COLON) || !p.expectPeek(token.INDENT) {
		return nil
	}

	stmt.Body = p.parseBlockStmt()
	return stmt
}

func (p *Parser) parseForInStmt() *ast.ForInStmt {
	stmt := &ast.ForInStmt{Token: p.curToken}

	if !p.expectPeek(token.IDENT) {
		return nil
	}
	stmt.VarName = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}

	if !p.expectPeek(token.IN) {
		return nil
	}

	p.nextToken() // consume IN
	stmt.Iterable = p.parseExpression(LOWEST)

	if !p.expectPeek(token.COLON) || !p.expectPeek(token.INDENT) {
		return nil
	}

	stmt.Body = p.parseBlockStmt()
	return stmt
}

func (p *Parser) parseMatchStmt() *ast.MatchStmt {
	stmt := &ast.MatchStmt{Token: p.curToken}

	p.nextToken() // consume MATCH
	stmt.Expr = p.parseExpression(LOWEST)

	if !p.expectPeek(token.COLON) || !p.expectPeek(token.INDENT) {
		return nil
	}

	p.nextToken() // consume INDENT

	for p.curToken.Type != token.DEDENT && p.curToken.Type != token.EOF {
		pattern := p.parseExpression(LOWEST)

		var guard ast.Expression
		if p.peekToken.Type == token.IF {
			p.nextToken() // consume IF
			p.nextToken()
			guard = p.parseExpression(LOWEST)
		}

		if !p.expectPeek(token.FAT_ARROW) {
			return nil
		}

		p.nextToken() // consume FAT_ARROW
		body := p.parseStatement()

		stmt.Cases = append(stmt.Cases, ast.MatchCase{
			Pattern: pattern,
			Guard:   guard,
			Body:    body,
		})

		p.nextToken()
	}

	return stmt
}

func (p *Parser) parseStructDeclStmt() *ast.StructDeclStmt {
	stmt := &ast.StructDeclStmt{Token: p.curToken}

	if !p.expectPeek(token.IDENT) {
		return nil
	}

	stmt.Name = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}

	if p.peekToken.Type == token.LT {
		p.nextToken() // consume '<'
		var typeParams []string
		for {
			if !p.expectPeek(token.IDENT) {
				return nil
			}
			typeParams = append(typeParams, p.curToken.Literal)
			if p.peekToken.Type == token.COMMA {
				p.nextToken()
			} else {
				break
			}
		}
		if !p.expectPeek(token.GT) {
			return nil
		}
		stmt.TypeParams = typeParams
	}

	if !p.expectPeek(token.COLON) || !p.expectPeek(token.INDENT) {
		return nil
	}

	p.nextToken() // consume INDENT
	for p.curToken.Type != token.DEDENT && p.curToken.Type != token.EOF {
		if p.curToken.Type == token.IDENT {
			fieldName := &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}
			fieldType := ""
			if p.peekToken.Type == token.COLON {
				p.nextToken()
				if p.expectPeek(token.IDENT) {
					fieldType = p.curToken.Literal
					if p.peekToken.Type == token.LBRACKET {
						p.nextToken()
						if p.expectPeek(token.RBRACKET) {
							fieldType += "[]"
						}
					}
				}
			}
			stmt.Fields = append(stmt.Fields, ast.StructField{Name: fieldName, Type: fieldType})
		}
		p.nextToken()
	}

	return stmt
}

func (p *Parser) parseMapLiteral() ast.Expression {
	lit := &ast.MapLiteral{Token: p.curToken}

	if p.peekToken.Type == token.RBRACE {
		p.nextToken()
		return lit
	}

	for {
		p.nextToken()
		key := p.parseExpression(LOWEST)
		if !p.expectPeek(token.COLON) {
			return nil
		}
		p.nextToken()
		val := p.parseExpression(LOWEST)

		lit.Keys = append(lit.Keys, key)
		lit.Values = append(lit.Values, val)

		if p.peekToken.Type == token.COMMA {
			p.nextToken() // consume ','
		} else {
			break
		}
	}

	if !p.expectPeek(token.RBRACE) {
		return nil
	}

	return lit
}

func (p *Parser) parseTraitDeclStmt() *ast.TraitDeclStmt {
	stmt := &ast.TraitDeclStmt{Token: p.curToken}

	if !p.expectPeek(token.IDENT) {
		return nil
	}
	stmt.Name = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}

	if !p.expectPeek(token.COLON) || !p.expectPeek(token.INDENT) {
		return nil
	}

	p.nextToken() // consume INDENT
	for p.curToken.Type != token.DEDENT && p.curToken.Type != token.EOF {
		if p.curToken.Type == token.FN {
			fn := p.parseFnDeclStmt()
			if fn != nil {
				stmt.Methods = append(stmt.Methods, fn)
			}
		}
		p.nextToken()
	}

	return stmt
}

func (p *Parser) parseImplDeclStmt() *ast.ImplDeclStmt {
	stmt := &ast.ImplDeclStmt{Token: p.curToken}

	if !p.expectPeek(token.IDENT) {
		return nil
	}

	firstName := p.curToken.Literal

	if p.peekToken.Type == token.FOR {
		p.nextToken() // consume FOR
		if !p.expectPeek(token.IDENT) {
			return nil
		}
		stmt.TraitName = &ast.Identifier{Token: p.curToken, Value: firstName}
		stmt.TargetType = p.curToken.Literal
	} else {
		stmt.TargetType = firstName
	}

	if !p.expectPeek(token.COLON) || !p.expectPeek(token.INDENT) {
		return nil
	}

	p.nextToken() // consume INDENT
	for p.curToken.Type != token.DEDENT && p.curToken.Type != token.EOF {
		if p.curToken.Type == token.FN {
			fn := p.parseFnDeclStmt()
			if fn != nil {
				if len(fn.Params) > 0 && fn.Params[0].Name.Value == "self" {
					fn.Params[0].Type = stmt.TargetType
				} else {
					fn.Receiver = &ast.Param{
						Name: &ast.Identifier{Token: p.curToken, Value: strings.ToLower(stmt.TargetType[:1])},
						Type: stmt.TargetType,
					}
				}
				stmt.Methods = append(stmt.Methods, fn)
			}
		}
		p.nextToken()
	}

	return stmt
}

func (p *Parser) parseSpawnStmt() *ast.SpawnStmt {
	stmt := &ast.SpawnStmt{Token: p.curToken}
	p.nextToken()
	stmt.Call = p.parseExpression(LOWEST)
	return stmt
}

func (p *Parser) parseExternFnStmt() *ast.ExternFnStmt {
	stmt := &ast.ExternFnStmt{Token: p.curToken}

	if !p.expectPeek(token.STRING) {
		return nil
	}
	stmt.Abi = p.curToken.Literal

	if !p.expectPeek(token.FN) {
		return nil
	}

	if !p.expectPeek(token.IDENT) {
		return nil
	}
	stmt.Name = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}

	if !p.expectPeek(token.LPAREN) {
		return nil
	}

	stmt.Params = p.parseFnParameters()

	if p.peekToken.Type == token.ARROW {
		p.nextToken() // consume '->'
		if !p.expectPeek(token.IDENT) {
			return nil
		}
		stmt.ReturnType = p.curToken.Literal
	}

	return stmt
}

func (p *Parser) parseMacroDeclStmt() *ast.MacroDeclStmt {
	stmt := &ast.MacroDeclStmt{Token: p.curToken}

	if !p.expectPeek(token.IDENT) {
		return nil
	}
	stmt.Name = &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}

	if !p.expectPeek(token.LPAREN) {
		return nil
	}

	stmt.Params = []*ast.Identifier{}
	if p.peekToken.Type != token.RPAREN {
		for {
			if !p.expectPeek(token.IDENT) {
				return nil
			}
			stmt.Params = append(stmt.Params, &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal})
			if p.peekToken.Type == token.COMMA {
				p.nextToken()
			} else {
				break
			}
		}
	}

	if !p.expectPeek(token.RPAREN) {
		return nil
	}

	if !p.expectPeek(token.COLON) || !p.expectPeek(token.INDENT) {
		return nil
	}

	stmt.Body = p.parseBlockStmt()
	return stmt
}

func (p *Parser) parseExpressionStatement() *ast.ExprStmt {
	stmt := &ast.ExprStmt{Token: p.curToken}
	stmt.Expr = p.parseExpression(LOWEST)
	return stmt
}

func (p *Parser) parseExpression(precedence int) ast.Expression {
	prefix := p.prefixParseFns[p.curToken.Type]
	if prefix == nil {
		p.noPrefixParseFnError(p.curToken.Type)
		return nil
	}

	leftExp := prefix()

	for p.peekToken.Type != token.NEWLINE && p.peekToken.Type != token.EOF && precedence < p.peekPrecedence() {
		postfix := p.postfixParseFns[p.peekToken.Type]
		if postfix != nil {
			p.nextToken()
			leftExp = postfix(leftExp)
			continue
		}

		infix := p.infixParseFns[p.peekToken.Type]
		if infix == nil {
			return leftExp
		}

		p.nextToken()
		leftExp = infix(leftExp)
	}

	return leftExp
}

func (p *Parser) parseIdentifier() ast.Expression {
	return &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal}
}

func (p *Parser) parseIntegerLiteral() ast.Expression {
	lit := &ast.IntegerLiteral{Token: p.curToken}

	val, err := strconv.ParseInt(p.curToken.Literal, 0, 64)
	if err != nil {
		p.errors = append(p.errors, fmt.Sprintf("could not parse %q as integer", p.curToken.Literal))
		return nil
	}

	lit.Value = val
	return lit
}

func (p *Parser) parseFloatLiteral() ast.Expression {
	lit := &ast.FloatLiteral{Token: p.curToken}

	val, err := strconv.ParseFloat(p.curToken.Literal, 64)
	if err != nil {
		p.errors = append(p.errors, fmt.Sprintf("could not parse %q as float", p.curToken.Literal))
		return nil
	}

	lit.Value = val
	return lit
}

func (p *Parser) parseStringLiteral() ast.Expression {
	return &ast.StringLiteral{Token: p.curToken, Value: p.curToken.Literal}
}

func (p *Parser) parseBoolLiteral() ast.Expression {
	return &ast.BoolLiteral{Token: p.curToken, Value: p.curToken.Literal == "true"}
}

func (p *Parser) parseArrayLiteral() ast.Expression {
	array := &ast.ArrayLiteral{Token: p.curToken}
	array.Elements = p.parseExpressionList(token.RBRACKET)
	return array
}

func (p *Parser) parseExpressionList(end token.TokenType) []ast.Expression {
	list := []ast.Expression{}

	if p.peekToken.Type == end {
		p.nextToken()
		return list
	}

	p.nextToken()
	list = append(list, p.parseExpression(LOWEST))

	for p.peekToken.Type == token.COMMA {
		p.nextToken()
		p.nextToken()
		list = append(list, p.parseExpression(LOWEST))
	}

	if !p.expectPeek(end) {
		return nil
	}

	return list
}

func (p *Parser) parseIndexExpression(left ast.Expression) ast.Expression {
	exp := &ast.IndexExpr{Token: p.curToken, Left: left}

	p.nextToken()
	exp.Index = p.parseExpression(LOWEST)

	if !p.expectPeek(token.RBRACKET) {
		return nil
	}

	return exp
}

func (p *Parser) parsePrefixExpression() ast.Expression {
	expression := &ast.PrefixExpr{
		Token:    p.curToken,
		Operator: p.curToken.Literal,
	}

	p.nextToken()
	expression.Right = p.parseExpression(PREFIX)

	return expression
}

func (p *Parser) parseFnLiteral() ast.Expression {
	lit := &ast.FnLiteral{Token: p.curToken}

	if !p.expectPeek(token.LPAREN) {
		return nil
	}

	lit.Params = p.parseFnParameters()

	if p.peekToken.Type == token.ARROW {
		p.nextToken() // consume '->'
		p.nextToken()
		lit.ReturnType = p.curToken.Literal
	}

	if !p.expectPeek(token.COLON) {
		return nil
	}

	if p.peekToken.Type == token.INDENT {
		p.nextToken()
		lit.Body = p.parseBlockStmt()
	} else {
		p.nextToken()
		stmt := p.parseStatement()
		lit.Body = &ast.BlockStmt{Statements: []ast.Statement{stmt}}
	}

	return lit
}

func (p *Parser) parseInfixExpression(left ast.Expression) ast.Expression {
	expression := &ast.InfixExpr{
		Token:    p.curToken,
		Operator: p.curToken.Literal,
		Left:     left,
	}

	if expression.Operator == ".." {
		precedence := p.curPrecedence()
		p.nextToken()
		right := p.parseExpression(precedence)
		return &ast.RangeExpr{
			Token: expression.Token,
			Start: left,
			End:   right,
		}
	}

	precedence := p.curPrecedence()
	p.nextToken()
	expression.Right = p.parseExpression(precedence)

	return expression
}

func (p *Parser) parseGroupedExpression() ast.Expression {
	tok := p.curToken
	p.nextToken()

	exp := p.parseExpression(LOWEST)

	if p.peekToken.Type == token.COMMA {
		var elems []ast.Expression
		elems = append(elems, exp)
		for p.peekToken.Type == token.COMMA {
			p.nextToken() // consume ','
			p.nextToken()
			elems = append(elems, p.parseExpression(LOWEST))
		}
		if !p.expectPeek(token.RPAREN) {
			return nil
		}
		return &ast.TupleLiteral{Token: tok, Elements: elems}
	}

	if !p.expectPeek(token.RPAREN) {
		return nil
	}

	return exp
}

func (p *Parser) parseCallExpression(function ast.Expression) ast.Expression {
	exp := &ast.CallExpr{Token: p.curToken, Function: function}
	exp.Arguments = p.parseCallArguments()
	return exp
}

func (p *Parser) parseCallArguments() []ast.Expression {
	args := []ast.Expression{}

	if p.peekToken.Type == token.RPAREN {
		p.nextToken()
		return args
	}

	p.nextToken()
	args = append(args, p.parseExpression(LOWEST))

	for p.peekToken.Type == token.COMMA {
		p.nextToken()
		p.nextToken()
		args = append(args, p.parseExpression(LOWEST))
	}

	if !p.expectPeek(token.RPAREN) {
		return nil
	}

	return args
}

func (p *Parser) parseMemberExpression(left ast.Expression) ast.Expression {
	tok := p.curToken
	if p.peekToken.Type == token.INT {
		p.nextToken()
		idxVal, _ := strconv.Atoi(p.curToken.Literal)
		return &ast.TupleIndexExpr{
			Token: tok,
			Left:  left,
			Index: idxVal,
		}
	}
	p.nextToken()
	return &ast.MemberExpr{
		Token:  tok,
		Object: left,
		Member: &ast.Identifier{Token: p.curToken, Value: p.curToken.Literal},
	}
}

func (p *Parser) peekPrecedence() int {
	if p, ok := precedences[p.peekToken.Type]; ok {
		return p
	}
	return LOWEST
}

func (p *Parser) curPrecedence() int {
	if p, ok := precedences[p.curToken.Type]; ok {
		return p
	}
	return LOWEST
}

func (p *Parser) expectPeek(t token.TokenType) bool {
	if p.peekToken.Type == t {
		p.nextToken()
		return true
	}
	p.peekError(t)
	return false
}

func (p *Parser) peekError(t token.TokenType) {
	msg := fmt.Sprintf("line %d: expected next token to be %s, got %s instead",
		p.peekToken.Line, t, p.peekToken.Type)
	p.errors = append(p.errors, msg)
}

func (p *Parser) noPrefixParseFnError(t token.TokenType) {
	msg := fmt.Sprintf("line %d: no prefix parse function for %s found", p.curToken.Line, t)
	p.errors = append(p.errors, msg)
}

func (p *Parser) parseDeferStmt() *ast.DeferStmt {
	stmt := &ast.DeferStmt{Token: p.curToken}
	p.nextToken()
	stmt.Expr = p.parseExpression(LOWEST)
	return stmt
}

func (p *Parser) parseFStringLiteral() ast.Expression {
	tok := p.curToken
	raw := tok.Literal
	node := &ast.FStringLiteral{Token: tok, Parts: []ast.Expression{}}

	var curText bytes.Buffer
	for i := 0; i < len(raw); i++ {
		if raw[i] == '{' {
			if curText.Len() > 0 {
				node.Parts = append(node.Parts, &ast.StringLiteral{Token: tok, Value: curText.String()})
				curText.Reset()
			}
			i++
			var exprText bytes.Buffer
			depth := 1
			for i < len(raw) && depth > 0 {
				if raw[i] == '{' {
					depth++
				} else if raw[i] == '}' {
					depth--
					if depth == 0 {
						break
					}
				} else {
					exprText.WriteByte(raw[i])
				}
				i++
			}
			subLexer := lexer.New(exprText.String())
			subParser := New(subLexer)
			subExpr := subParser.parseExpression(LOWEST)
			if subExpr != nil {
				node.Parts = append(node.Parts, subExpr)
			}
		} else {
			curText.WriteByte(raw[i])
		}
	}
	if curText.Len() > 0 {
		node.Parts = append(node.Parts, &ast.StringLiteral{Token: tok, Value: curText.String()})
	}

	return node
}

