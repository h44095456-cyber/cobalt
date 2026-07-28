package token

type TokenType string

const (
	// Special
	EOF     TokenType = "EOF"
	ILLEGAL TokenType = "ILLEGAL"
	NEWLINE TokenType = "NEWLINE"
	INDENT  TokenType = "INDENT"
	DEDENT  TokenType = "DEDENT"

	// Identifiers & Literals
	IDENT  TokenType = "IDENT"
	INT    TokenType = "INT"
	FLOAT  TokenType = "FLOAT"
	STRING TokenType = "STRING"
	BOOL   TokenType = "BOOL"

	QUESTION          = "?"
	QUESTION_DOT      = "?."
	QUESTION_QUESTION = "??"
	AT                = "@"

	// Keywords
	LET    TokenType = "LET"
	VAR    TokenType = "VAR"
	CONST  TokenType = "CONST"
	FN     TokenType = "FN"
	RETURN TokenType = "RETURN"
	IF     TokenType = "IF"
	ELIF   TokenType = "ELIF"
	ELSE   TokenType = "ELSE"
	WHILE  TokenType = "WHILE"
	FOR    TokenType = "FOR"
	IN     TokenType = "IN"
	STRUCT TokenType = "STRUCT"
	ENUM   TokenType = "ENUM"
	MATCH  TokenType = "MATCH"
	TRAIT  TokenType = "TRAIT"
	IMPL   TokenType = "IMPL"
	SPAWN  TokenType = "SPAWN"
	EXTERN TokenType = "EXTERN"
	MACRO  TokenType = "MACRO"
	ASYNC  TokenType = "ASYNC"
	AWAIT  TokenType = "AWAIT"
	ASM    TokenType = "ASM"
	RPC    TokenType = "RPC"
	DEFER  TokenType = "DEFER"
	FSTRING TokenType = "FSTRING"
	IMPORT TokenType = "IMPORT"

	// Operators & Symbols
	ASSIGN   TokenType = "="
	PLUS     TokenType = "+"
	MINUS    TokenType = "-"
	STAR     TokenType = "*"
	SLASH    TokenType = "/"
	MOD      TokenType = "%"

	EQ  TokenType = "=="
	NEQ TokenType = "!="
	LT  TokenType = "<"
	LTE TokenType = "<="
	GT  TokenType = ">"
	GTE TokenType = ">="

	AND  TokenType = "&&"
	OR   TokenType = "||"
	BANG TokenType = "!"

	ARROW     TokenType = "->"
	FAT_ARROW TokenType = "=>"
	COLON     TokenType = ":"
	DOTDOT    TokenType = ".."
	COMMA     TokenType = ","
	DOT       TokenType = "."

	LPAREN   TokenType = "("
	RPAREN   TokenType = ")"
	LBRACKET TokenType = "["
	RBRACKET TokenType = "]"
	LBRACE   TokenType = "{"
	RBRACE   TokenType = "}"
)

type Token struct {
	Type    TokenType
	Literal string
	Line    int
	Column  int
}

var keywords = map[string]TokenType{
	"let":    LET,
	"var":    VAR,
	"const":  CONST,
	"fn":     FN,
	"return": RETURN,
	"if":     IF,
	"elif":   ELIF,
	"else":   ELSE,
	"while":  WHILE,
	"for":    FOR,
	"in":     IN,
	"struct": STRUCT,
	"enum":   ENUM,
	"match":  MATCH,
	"trait":  TRAIT,
	"impl":   IMPL,
	"spawn":  SPAWN,
	"extern": EXTERN,
	"macro":  MACRO,
	"async":  ASYNC,
	"await":  AWAIT,
	"asm":    ASM,
	"rpc":    RPC,
	"defer":  DEFER,
	"import": IMPORT,
	"and":    AND,
	"or":     OR,
	"not":    BANG,
	"true":   BOOL,
	"false":  BOOL,
}

func LookupIdent(ident string) TokenType {
	if tok, ok := keywords[ident]; ok {
		return tok
	}
	return IDENT
}
