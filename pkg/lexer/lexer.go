package lexer

import (
	"bytes"
	"cobalt/pkg/token"
	"strconv"
)

type Lexer struct {
	input        string
	position     int  // current position in input (points to current char)
	readPosition int  // current reading position in input (after current char)
	ch           byte // current char under examination
	line         int
	column       int
	indentStack  []int
	pendingToks  []token.Token
	atLineStart  bool
}

func New(input string) *Lexer {
	l := &Lexer{
		input:       input,
		line:        1,
		column:      0,
		indentStack: []int{0},
		atLineStart: true,
	}
	l.readChar()
	return l
}

func (l *Lexer) readChar() {
	if l.readPosition >= len(l.input) {
		l.ch = 0
	} else {
		l.ch = l.input[l.readPosition]
	}
	l.position = l.readPosition
	l.readPosition++
	l.column++
}

func (l *Lexer) peekChar() byte {
	if l.readPosition >= len(l.input) {
		return 0
	}
	return l.input[l.readPosition]
}

func (l *Lexer) NextToken() token.Token {
	if len(l.pendingToks) > 0 {
		tok := l.pendingToks[0]
		l.pendingToks = l.pendingToks[1:]
		return tok
	}

	if l.atLineStart {
		l.handleIndentation()
		if len(l.pendingToks) > 0 {
			tok := l.pendingToks[0]
			l.pendingToks = l.pendingToks[1:]
			return tok
		}
	}

	l.skipWhitespace()

	var tok token.Token

	switch l.ch {
	case '\n':
		tok = l.newToken(token.NEWLINE, "\n")
		l.line++
		l.column = 0
		l.atLineStart = true
		l.readChar()
		return tok
	case '=':
		if l.peekChar() == '=' {
			ch := l.ch
			l.readChar()
			tok = token.Token{Type: token.EQ, Literal: string(ch) + string(l.ch), Line: l.line, Column: l.column - 1}
		} else if l.peekChar() == '>' {
			ch := l.ch
			l.readChar()
			tok = token.Token{Type: token.FAT_ARROW, Literal: string(ch) + string(l.ch), Line: l.line, Column: l.column - 1}
		} else {
			tok = l.newToken(token.ASSIGN, string(l.ch))
		}
	case '+':
		tok = l.newToken(token.PLUS, string(l.ch))
	case '-':
		if l.peekChar() == '>' {
			ch := l.ch
			l.readChar()
			tok = token.Token{Type: token.ARROW, Literal: string(ch) + string(l.ch), Line: l.line, Column: l.column - 1}
		} else {
			tok = l.newToken(token.MINUS, string(l.ch))
		}
	case '*':
		tok = l.newToken(token.STAR, string(l.ch))
	case '/':
		tok = l.newToken(token.SLASH, string(l.ch))
	case '%':
		tok = l.newToken(token.MOD, string(l.ch))
	case '!':
		if l.peekChar() == '=' {
			ch := l.ch
			l.readChar()
			tok = token.Token{Type: token.NEQ, Literal: string(ch) + string(l.ch), Line: l.line, Column: l.column - 1}
		} else {
			tok = l.newToken(token.BANG, string(l.ch))
		}
	case '&':
		if l.peekChar() == '&' {
			ch := l.ch
			l.readChar()
			tok = token.Token{Type: token.AND, Literal: string(ch) + string(l.ch), Line: l.line, Column: l.column - 1}
		} else {
			tok = l.newToken(token.ILLEGAL, string(l.ch))
		}
	case '|':
		if l.peekChar() == '|' {
			ch := l.ch
			l.readChar()
			tok = token.Token{Type: token.OR, Literal: string(ch) + string(l.ch), Line: l.line, Column: l.column - 1}
		} else {
			tok = l.newToken(token.ILLEGAL, string(l.ch))
		}
	case '<':
		if l.peekChar() == '=' {
			ch := l.ch
			l.readChar()
			tok = token.Token{Type: token.LTE, Literal: string(ch) + string(l.ch), Line: l.line, Column: l.column - 1}
		} else {
			tok = l.newToken(token.LT, string(l.ch))
		}
	case '>':
		if l.peekChar() == '=' {
			ch := l.ch
			l.readChar()
			tok = token.Token{Type: token.GTE, Literal: string(ch) + string(l.ch), Line: l.line, Column: l.column - 1}
		} else {
			tok = l.newToken(token.GT, string(l.ch))
		}
	case '?':
		if l.peekChar() == '.' {
			ch := l.ch
			l.readChar()
			tok = token.Token{Type: token.QUESTION_DOT, Literal: string(ch) + string(l.ch), Line: l.line, Column: l.column - 1}
		} else if l.peekChar() == '?' {
			ch := l.ch
			l.readChar()
			tok = token.Token{Type: token.QUESTION_QUESTION, Literal: string(ch) + string(l.ch), Line: l.line, Column: l.column - 1}
		} else {
			tok = l.newToken(token.QUESTION, string(l.ch))
		}
	case '@':
		tok = l.newToken(token.AT, string(l.ch))
	case ':':
		tok = l.newToken(token.COLON, string(l.ch))
	case ',':
		tok = l.newToken(token.COMMA, string(l.ch))
	case '.':
		if l.peekChar() == '.' {
			ch := l.ch
			l.readChar()
			tok = token.Token{Type: token.DOTDOT, Literal: string(ch) + string(l.ch), Line: l.line, Column: l.column - 1}
		} else {
			tok = l.newToken(token.DOT, string(l.ch))
		}
	case '(':
		tok = l.newToken(token.LPAREN, string(l.ch))
	case ')':
		tok = l.newToken(token.RPAREN, string(l.ch))
	case '[':
		tok = l.newToken(token.LBRACKET, string(l.ch))
	case ']':
		tok = l.newToken(token.RBRACKET, string(l.ch))
	case '{':
		tok = l.newToken(token.LBRACE, string(l.ch))
	case '}':
		tok = l.newToken(token.RBRACE, string(l.ch))
	case '"':
		strLit := l.readString()
		return token.Token{Type: token.STRING, Literal: strLit, Line: l.line, Column: l.column}
	case '#':
		l.skipComment()
		return l.NextToken()
	case 0:
		// EOF - unwind remaining indent levels
		for len(l.indentStack) > 1 {
			l.indentStack = l.indentStack[:len(l.indentStack)-1]
			l.pendingToks = append(l.pendingToks, token.Token{Type: token.DEDENT, Literal: "", Line: l.line, Column: l.column})
		}
		if len(l.pendingToks) > 0 {
			tok = l.pendingToks[0]
			l.pendingToks = l.pendingToks[1:]
			return tok
		}
		tok = token.Token{Type: token.EOF, Literal: "", Line: l.line, Column: l.column}
	default:
		if l.ch == 'f' && l.peekChar() == '"' {
			col := l.column
			l.readChar() // consume 'f'
			tok = token.Token{Type: token.FSTRING, Literal: l.readString(), Line: l.line, Column: col}
			return tok
		}
		if isLetter(l.ch) {
			lit := l.readIdentifier()
			tokType := token.LookupIdent(lit)
			return token.Token{Type: tokType, Literal: lit, Line: l.line, Column: l.column}
		} else if isDigit(l.ch) {
			lit, tokType := l.readNumber()
			return token.Token{Type: tokType, Literal: lit, Line: l.line, Column: l.column}
		} else {
			tok = l.newToken(token.ILLEGAL, string(l.ch))
		}
	}

	l.readChar()
	return tok
}

func (l *Lexer) handleIndentation() {
	l.atLineStart = false
	indent := 0

	for l.ch == ' ' || l.ch == '\t' {
		if l.ch == ' ' {
			indent++
		} else if l.ch == '\t' {
			indent += 4
		}
		l.readChar()
	}

	// Ignore empty lines or comments when evaluating indents
	if l.ch == '\n' || l.ch == '#' || l.ch == 0 {
		return
	}

	currentTop := l.indentStack[len(l.indentStack)-1]

	if indent > currentTop {
		l.indentStack = append(l.indentStack, indent)
		l.pendingToks = append(l.pendingToks, token.Token{Type: token.INDENT, Literal: "", Line: l.line, Column: l.column})
	} else if indent < currentTop {
		for len(l.indentStack) > 1 && l.indentStack[len(l.indentStack)-1] > indent {
			l.indentStack = l.indentStack[:len(l.indentStack)-1]
			l.pendingToks = append(l.pendingToks, token.Token{Type: token.DEDENT, Literal: "", Line: l.line, Column: l.column})
		}
	}
}

func (l *Lexer) skipWhitespace() {
	for l.ch == ' ' || l.ch == '\t' || l.ch == '\r' {
		l.readChar()
	}
}

func (l *Lexer) skipComment() {
	for l.ch != '\n' && l.ch != 0 {
		l.readChar()
	}
}

func (l *Lexer) readIdentifier() string {
	position := l.position
	for isLetter(l.ch) || isDigit(l.ch) {
		l.readChar()
	}
	return l.input[position:l.position]
}

func (l *Lexer) readNumber() (string, token.TokenType) {
	position := l.position
	isFloat := false
	for isDigit(l.ch) || (l.ch == '.' && isDigit(l.peekChar())) {
		if l.ch == '.' {
			isFloat = true
		}
		l.readChar()
	}
	tokType := token.INT
	if isFloat {
		tokType = token.FLOAT
	}
	return l.input[position:l.position], tokType
}

func (l *Lexer) readString() string {
	l.readChar() // skip opening quote
	var sb bytes.Buffer
	for l.ch != '"' && l.ch != 0 {
		if l.ch == '\\' {
			l.readChar()
			if l.ch == 'n' {
				sb.WriteByte('\n')
			} else if l.ch == 't' {
				sb.WriteByte('\t')
			} else if l.ch == 'r' {
				sb.WriteByte('\r')
			} else if l.ch == 'e' {
				sb.WriteByte(0x1B)
			} else if l.ch == 'x' {
				l.readChar()
				ch1 := l.ch
				l.readChar()
				ch2 := l.ch
				val, _ := strconv.ParseUint(string([]byte{ch1, ch2}), 16, 8)
				sb.WriteByte(byte(val))
			} else if l.ch == '"' {
				sb.WriteByte('"')
			} else if l.ch == '\\' {
				sb.WriteByte('\\')
			} else {
				sb.WriteByte('\\')
				sb.WriteByte(l.ch)
			}
		} else {
			sb.WriteByte(l.ch)
		}
		l.readChar()
	}
	l.readChar() // skip closing quote
	return sb.String()
}

func (l *Lexer) newToken(tokenType token.TokenType, ch string) token.Token {
	return token.Token{Type: tokenType, Literal: ch, Line: l.line, Column: l.column}
}

func isLetter(ch byte) bool {
	return 'a' <= ch && ch <= 'z' || 'A' <= ch && ch <= 'Z' || ch == '_'
}

func isDigit(ch byte) bool {
	return '0' <= ch && ch <= '9'
}
