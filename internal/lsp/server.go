package lsp

import (
	"encoding/json"
	"log"
)

// Server holds document state and handles JSON-RPC messages.
type Server struct {
	docs     map[string]string
	shutdown bool
}

// NewServer creates an initialised Server.
func NewServer() *Server {
	return &Server{docs: make(map[string]string)}
}

// Handle processes one JSON-RPC message.
// For requests (with ID) it returns a Response; for notifications it returns nil.
// Notifications to push to the client (e.g. publishDiagnostics) are returned separately.
func (s *Server) Handle(req *Request) (*Response, []*Notification) {
	switch req.Method {
	case "initialize":
		return s.handleInitialize(req), nil
	case "initialized":
		return nil, nil
	case "shutdown":
		s.shutdown = true
		return s.ok(req, nil), nil
	case "textDocument/didOpen":
		return nil, s.handleDidOpen(req)
	case "textDocument/didChange":
		return nil, s.handleDidChange(req)
	case "textDocument/didClose":
		return nil, s.handleDidClose(req)
	case "textDocument/hover":
		return s.handleHover(req), nil
	case "textDocument/definition":
		return s.handleDefinition(req), nil
	case "textDocument/completion":
		return s.handleCompletion(req), nil
	default:
		if !req.IsNotification() {
			return s.methodNotFound(req), nil
		}
		return nil, nil
	}
}

func (s *Server) handleInitialize(req *Request) *Response {
	return s.ok(req, InitializeResult{
		Capabilities: ServerCapabilities{
			TextDocumentSync:   TextDocumentSyncFull,
			HoverProvider:      true,
			DefinitionProvider: true,
			CompletionProvider: &CompletionOptions{},
		},
		ServerInfo: ServerInfo{Name: "glisp-lsp", Version: "0.1.0"},
	})
}

func (s *Server) handleDidOpen(req *Request) []*Notification {
	var p DidOpenParams
	if err := json.Unmarshal(req.Params, &p); err != nil {
		log.Printf("didOpen unmarshal: %v", err)
		return nil
	}
	s.docs[p.TextDocument.URI] = p.TextDocument.Text
	return []*Notification{s.diagNotif(p.TextDocument.URI, p.TextDocument.Text)}
}

func (s *Server) handleDidChange(req *Request) []*Notification {
	var p DidChangeParams
	if err := json.Unmarshal(req.Params, &p); err != nil {
		log.Printf("didChange unmarshal: %v", err)
		return nil
	}
	if len(p.ContentChanges) == 0 {
		return nil
	}
	text := p.ContentChanges[len(p.ContentChanges)-1].Text
	s.docs[p.TextDocument.URI] = text
	return []*Notification{s.diagNotif(p.TextDocument.URI, text)}
}

func (s *Server) handleDidClose(req *Request) []*Notification {
	var p DidCloseParams
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return nil
	}
	delete(s.docs, p.TextDocument.URI)
	return []*Notification{s.clearDiagsNotif(p.TextDocument.URI)}
}

func (s *Server) handleHover(req *Request) *Response {
	var p HoverParams
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return s.invalidParams(req, err.Error())
	}
	source, ok := s.docs[p.TextDocument.URI]
	if !ok {
		return s.ok(req, nil)
	}
	result := FindHover(source, p.Position.Line, p.Position.Character)
	if result == nil {
		return s.ok(req, nil)
	}
	value := "```clojure\n" + result.Sig + "\n```"
	if result.Doc != "" {
		value += "\n\n" + result.Doc
	}
	return s.ok(req, Hover{
		Contents: MarkupContent{Kind: "markdown", Value: value},
	})
}

func (s *Server) handleDefinition(req *Request) *Response {
	var p DefinitionParams
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return s.invalidParams(req, err.Error())
	}
	source, ok := s.docs[p.TextDocument.URI]
	if !ok {
		return s.ok(req, nil)
	}
	r := FindDefinition(source, p.Position.Line, p.Position.Character)
	if r == nil {
		return s.ok(req, nil)
	}
	return s.ok(req, Location{URI: p.TextDocument.URI, Range: *r})
}

func (s *Server) handleCompletion(req *Request) *Response {
	var p CompletionParams
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return s.invalidParams(req, err.Error())
	}
	source, ok := s.docs[p.TextDocument.URI]
	if !ok {
		return s.ok(req, []CompletionItem{})
	}
	items := FindCompletions(source, p.Position.Line, p.Position.Character)
	if items == nil {
		items = []CompletionItem{}
	}
	return s.ok(req, items)
}

func (s *Server) diagNotif(uri, source string) *Notification {
	diags := Diagnostics(source)
	if diags == nil {
		diags = []Diagnostic{}
	}
	return &Notification{
		JSONRPC: "2.0",
		Method:  "textDocument/publishDiagnostics",
		Params:  PublishDiagnosticsParams{URI: uri, Diagnostics: diags},
	}
}

func (s *Server) clearDiagsNotif(uri string) *Notification {
	return &Notification{
		JSONRPC: "2.0",
		Method:  "textDocument/publishDiagnostics",
		Params:  PublishDiagnosticsParams{URI: uri, Diagnostics: []Diagnostic{}},
	}
}

func (s *Server) ok(req *Request, result any) *Response {
	return &Response{JSONRPC: "2.0", ID: req.ID, Result: result}
}

func (s *Server) methodNotFound(req *Request) *Response {
	return &Response{
		JSONRPC: "2.0",
		ID:      req.ID,
		Error:   &RPCError{Code: -32601, Message: "method not found: " + req.Method},
	}
}

func (s *Server) invalidParams(req *Request, msg string) *Response {
	return &Response{
		JSONRPC: "2.0",
		ID:      req.ID,
		Error:   &RPCError{Code: -32602, Message: msg},
	}
}
