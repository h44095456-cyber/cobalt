package lsp

import (
	"bufio"
	"cobalt/pkg/lexer"
	"cobalt/pkg/parser"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

type Request struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type Response struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   interface{} `json:"error,omitempty"`
}

type Notification struct {
	JSONRPC string      `json:"jsonrpc"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params"`
}

type TextDocumentItem struct {
	URI  string `json:"uri"`
	Text string `json:"text"`
}

type DidOpenTextDocumentParams struct {
	TextDocument TextDocumentItem `json:"textDocument"`
}

type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

type TextDocumentPositionParams struct {
	TextDocument struct {
		URI string `json:"uri"`
	} `json:"textDocument"`
	Position Position `json:"position"`
}

type Diagnostic struct {
	Range    Range  `json:"range"`
	Severity int    `json:"severity"` // 1 = Error
	Message  string `json:"message"`
}

type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

type PublishDiagnosticsParams struct {
	URI         string       `json:"uri"`
	Diagnostics []Diagnostic `json:"diagnostics"`
}

type CompletionItem struct {
	Label string `json:"label"`
	Kind  int    `json:"kind"` // 3 = Function, 6 = Variable, 14 = Keyword
	Detail string `json:"detail,omitempty"`
}

type HoverResult struct {
	Contents struct {
		Kind  string `json:"kind"`
		Value string `json:"value"`
	} `json:"contents"`
}

type Server struct {
	documents map[string]string
}

func NewServer() *Server {
	return &Server{
		documents: make(map[string]string),
	}
}

func (s *Server) Run() {
	reader := bufio.NewReader(os.Stdin)

	for {
		header, err := s.readHeader(reader)
		if err != nil {
			if err == io.EOF {
				break
			}
			continue
		}

		contentLength := header["Content-Length"]
		if contentLength == 0 {
			continue
		}

		body := make([]byte, contentLength)
		_, err = io.ReadFull(reader, body)
		if err != nil {
			break
		}

		var req Request
		if err := json.Unmarshal(body, &req); err != nil {
			continue
		}

		s.handleRequest(req)
	}
}

func (s *Server) readHeader(reader *bufio.Reader) (map[string]int, error) {
	headers := make(map[string]int)
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return nil, err
		}
		line = strings.TrimSpace(line)
		if line == "" {
			break
		}
		parts := strings.SplitN(line, ": ", 2)
		if len(parts) == 2 && parts[0] == "Content-Length" {
			val, _ := strconv.Atoi(parts[1])
			headers["Content-Length"] = val
		}
	}
	return headers, nil
}

func (s *Server) handleRequest(req Request) {
	switch req.Method {
	case "initialize":
		s.sendResponse(req.ID, map[string]interface{}{
			"capabilities": map[string]interface{}{
				"textDocumentSync": 1, // Full sync
				"completionProvider": map[string]interface{}{
					"triggerCharacters": []string{".", ":"},
				},
				"hoverProvider":      true,
				"definitionProvider": true,
			},
		})

	case "textDocument/didOpen":
		var params DidOpenTextDocumentParams
		if err := json.Unmarshal(req.Params, &params); err == nil {
			s.documents[params.TextDocument.URI] = params.TextDocument.Text
			s.diagnoseDocument(params.TextDocument.URI, params.TextDocument.Text)
		}

	case "textDocument/didChange":
		var params struct {
			TextDocument struct {
				URI string `json:"uri"`
			} `json:"textDocument"`
			ContentChanges []struct {
				Text string `json:"text"`
			} `json:"contentChanges"`
		}
		if err := json.Unmarshal(req.Params, &params); err == nil && len(params.ContentChanges) > 0 {
			text := params.ContentChanges[len(params.ContentChanges)-1].Text
			s.documents[params.TextDocument.URI] = text
			s.diagnoseDocument(params.TextDocument.URI, text)
		}

	case "textDocument/completion":
		items := []CompletionItem{
			{Label: "fn", Kind: 14, Detail: "Function declaration"},
			{Label: "let", Kind: 14, Detail: "Immutable variable declaration"},
			{Label: "var", Kind: 14, Detail: "Mutable variable declaration"},
			{Label: "return", Kind: 14, Detail: "Return statement"},
			{Label: "struct", Kind: 14, Detail: "Struct definition"},
			{Label: "enum", Kind: 14, Detail: "Enum definition"},
			{Label: "match", Kind: 14, Detail: "Pattern matching expression"},
			{Label: "async", Kind: 14, Detail: "Async function declaration"},
			{Label: "await", Kind: 14, Detail: "Await coroutine execution"},
			{Label: "macro", Kind: 14, Detail: "Compile-time macro definition"},
			{Label: "Box", Kind: 3, Detail: "Heap allocation Box(val)"},
			{Label: "Rc", Kind: 3, Detail: "Reference counted smart pointer Rc(val)"},
			{Label: "println", Kind: 3, Detail: "Print line to stdout"},
			{Label: "fs.read_file", Kind: 3, Detail: "Read text file contents"},
			{Label: "fs.write_file", Kind: 3, Detail: "Write text file contents"},
			{Label: "sys.now_millis", Kind: 3, Detail: "Get execution timestamp in ms"},
		}
		s.sendResponse(req.ID, items)

	case "textDocument/hover":
		var hover HoverResult
		hover.Contents.Kind = "markdown"
		hover.Contents.Value = "**Cobalt Language Symbol**\n\nHigh-performance Cobalt statically-typed symbol."
		s.sendResponse(req.ID, hover)
	}
}

func (s *Server) diagnoseDocument(uri string, text string) {
	l := lexer.New(text)
	p := parser.New(l)
	p.ParseProgram()

	var diagnostics []Diagnostic
	for idx, parseErr := range p.Errors() {
		diagnostics = append(diagnostics, Diagnostic{
			Range: Range{
				Start: Position{Line: idx, Character: 0},
				End:   Position{Line: idx, Character: 80},
			},
			Severity: 1, // Error
			Message:  parseErr,
		})
	}

	notification := Notification{
		JSONRPC: "2.0",
		Method:  "textDocument/publishDiagnostics",
		Params: PublishDiagnosticsParams{
			URI:         uri,
			Diagnostics: diagnostics,
		},
	}

	data, _ := json.Marshal(notification)
	fmt.Printf("Content-Length: %d\r\n\r\n%s", len(data), data)
}

func (s *Server) sendResponse(id interface{}, result interface{}) {
	resp := Response{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}
	data, _ := json.Marshal(resp)
	fmt.Printf("Content-Length: %d\r\n\r\n%s", len(data), data)
}
