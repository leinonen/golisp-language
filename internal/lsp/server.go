package lsp

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strings"

	"golisp/internal/formatter"
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
	case "textDocument/references":
		return s.handleReferences(req), nil
	case "textDocument/documentSymbol":
		return s.handleDocumentSymbol(req), nil
	case "textDocument/completion":
		return s.handleCompletion(req), nil
	case "textDocument/formatting":
		return s.handleFormatting(req), nil
	case "textDocument/rename":
		return s.handleRename(req), nil
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
			TextDocumentSync:           TextDocumentSyncFull,
			HoverProvider:              true,
			DefinitionProvider:         true,
			ReferencesProvider:         true,
			DocumentSymbolProvider:     true,
			CompletionProvider:         &CompletionOptions{},
			DocumentFormattingProvider: true,
			RenameProvider:             true,
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
		// Multi-line docstrings join their lines with "\n"; in Markdown a bare
		// newline collapses, so emit hard line breaks to keep each line.
		value += "\n\n" + strings.ReplaceAll(result.Doc, "\n", "  \n")
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
	if r := FindDefinition(source, p.Position.Line, p.Position.Character); r != nil {
		return s.ok(req, Location{URI: p.TextDocument.URI, Range: *r})
	}
	// Symbol not found in current file — resolve name once, then search other open docs.
	name := symbolAtPosition(source, p.Position.Line, p.Position.Character)
	if name == "" {
		return s.ok(req, nil)
	}
	for uri, src := range s.docs {
		if uri == p.TextDocument.URI {
			continue
		}
		if r := FindDeclByName(src, name); r != nil {
			return s.ok(req, Location{URI: uri, Range: *r})
		}
	}
	// Still not found — scan sibling .glsp files on disk that aren't open.
	if loc := s.searchSiblingFiles(p.TextDocument.URI, name); loc != nil {
		return s.ok(req, *loc)
	}
	return s.ok(req, nil)
}

// searchSiblingFiles reads all .glsp files in the same directory as currentURI
// that are not already tracked in s.docs, and searches each for a declaration of name.
func (s *Server) searchSiblingFiles(currentURI, name string) *Location {
	path := strings.TrimPrefix(currentURI, "file://")
	dir := filepath.Dir(path)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".glsp") {
			continue
		}
		filePath := filepath.Join(dir, e.Name())
		fileURI := "file://" + filePath
		if _, open := s.docs[fileURI]; open {
			continue // already searched above
		}
		src, err := os.ReadFile(filePath)
		if err != nil {
			continue
		}
		if r := FindDeclByName(string(src), name); r != nil {
			return &Location{URI: fileURI, Range: *r}
		}
	}
	return nil
}

func (s *Server) handleReferences(req *Request) *Response {
	var p ReferenceParams
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return s.invalidParams(req, err.Error())
	}
	source, ok := s.docs[p.TextDocument.URI]
	if !ok {
		return s.ok(req, []Location{})
	}
	name := symbolAtPosition(source, p.Position.Line, p.Position.Character)
	if name == "" {
		return s.ok(req, []Location{})
	}

	var locs []Location
	for _, r := range findOccurrences(source, name) {
		locs = append(locs, Location{URI: p.TextDocument.URI, Range: r})
	}
	// Extend the search across the rest of the project so references are
	// project-wide, not just current-file: other open docs, then sibling files
	// on disk that aren't open.
	searched := map[string]bool{p.TextDocument.URI: true}
	for uri, src := range s.docs {
		if searched[uri] {
			continue
		}
		searched[uri] = true
		for _, r := range findOccurrences(src, name) {
			locs = append(locs, Location{URI: uri, Range: r})
		}
	}
	s.appendSiblingReferences(p.TextDocument.URI, name, searched, &locs)

	if locs == nil {
		locs = []Location{}
	}
	return s.ok(req, locs)
}

// appendSiblingReferences scans .glsp files in the same directory that aren't
// already searched and appends every reference to name found in them.
func (s *Server) appendSiblingReferences(currentURI, name string, searched map[string]bool, locs *[]Location) {
	path := strings.TrimPrefix(currentURI, "file://")
	dir := filepath.Dir(path)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".glsp") {
			continue
		}
		fileURI := "file://" + filepath.Join(dir, e.Name())
		if searched[fileURI] {
			continue
		}
		searched[fileURI] = true
		src, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		for _, r := range findOccurrences(string(src), name) {
			*locs = append(*locs, Location{URI: fileURI, Range: r})
		}
	}
}

func (s *Server) handleDocumentSymbol(req *Request) *Response {
	var p DocumentSymbolParams
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return s.invalidParams(req, err.Error())
	}
	source, ok := s.docs[p.TextDocument.URI]
	if !ok {
		return s.ok(req, []DocumentSymbol{})
	}
	syms := DocumentSymbols(source)
	if syms == nil {
		syms = []DocumentSymbol{}
	}
	return s.ok(req, syms)
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

func (s *Server) handleFormatting(req *Request) *Response {
	var p DocumentFormattingParams
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return s.invalidParams(req, err.Error())
	}
	source, ok := s.docs[p.TextDocument.URI]
	if !ok {
		return s.ok(req, []TextEdit{})
	}
	out, err := formatter.Format(source)
	if err != nil {
		// Don't surface a hard error on an unparseable buffer; just no-op.
		return s.ok(req, []TextEdit{})
	}
	if out == source {
		return s.ok(req, []TextEdit{})
	}
	// Replace the whole document. End position is one past the last line.
	lines := strings.Count(source, "\n")
	edit := TextEdit{
		Range: Range{
			Start: Position{Line: 0, Character: 0},
			End:   Position{Line: lines + 1, Character: 0},
		},
		NewText: out,
	}
	return s.ok(req, []TextEdit{edit})
}

func (s *Server) handleRename(req *Request) *Response {
	var p RenameParams
	if err := json.Unmarshal(req.Params, &p); err != nil {
		return s.invalidParams(req, err.Error())
	}
	source, ok := s.docs[p.TextDocument.URI]
	if !ok {
		return s.ok(req, nil)
	}
	edits := FindRenameEdits(source, p.Position.Line, p.Position.Character, p.NewName)
	if edits == nil {
		return s.ok(req, nil)
	}
	return s.ok(req, WorkspaceEdit{
		Changes: map[string][]TextEdit{p.TextDocument.URI: edits},
	})
}

func (s *Server) diagNotif(uri, source string) *Notification {
	filename := strings.TrimPrefix(uri, "file://")
	diags := Diagnostics(source, filename)
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
