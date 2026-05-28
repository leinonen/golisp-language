// Package ast defines the AST node types for the glisp compiler.
package ast

import "fmt"

// Position tracks source location for error reporting.
type Position struct {
	File   string
	Line   int
	Column int
}

func (p Position) String() string {
	if p.File != "" {
		return fmt.Sprintf("%s:%d:%d", p.File, p.Line, p.Column)
	}
	return fmt.Sprintf("%d:%d", p.Line, p.Column)
}

// Node is implemented by every AST node.
type Node interface {
	nodeMarker()
	Pos() Position
}

// TypeExpr holds a raw Go type string from a ^ annotation.
// Examples: "int", "*http.Request", "[]string", "(chan int)", "[string error]"
type TypeExpr struct {
	Pos_ Position
	Text string
}

func (t *TypeExpr) nodeMarker()    {}
func (t *TypeExpr) Pos() Position  { return t.Pos_ }
func NewTypeExpr(pos Position, text string) *TypeExpr {
	return &TypeExpr{Pos_: pos, Text: text}
}

// Param is a function parameter with optional type annotation.
type Param struct {
	Name      string
	TypeAnnot *TypeExpr
	IsRest    bool // true for & variadic params
}

// MapPair is a key-value pair inside a map literal.
type MapPair struct {
	Key   Node
	Value Node
}

// LetBinding is one name=value binding inside a let form.
// Pattern is either a *Symbol or *VectorLit (for multi-value destructure).
type LetBinding struct {
	Pattern   Node
	TypeAnnot *TypeExpr
	Value     Node
}

// CondClause is one test+body pair inside a cond form.
type CondClause struct {
	Test Node
	Body Node
}

// ImportSpec is one entry in an ns import list.
type ImportSpec struct {
	Alias string // "" = use default package name
	Path  string // e.g. "net/http"
}

// SelectCase is one case inside a select! form.
type SelectCase struct {
	IsDefault bool
	IsSend    bool
	Binding   string // variable name for recv binding (may be "")
	ChanExpr  Node
	SendVal   Node
	Body      []Node
}

// StructField is one field in a defstruct form.
type StructField struct {
	Name      string
	TypeAnnot *TypeExpr
	Tag       string
}

// InterfaceMethod is one method signature in a definterface form.
type InterfaceMethod struct {
	Name       string
	Params     []Param
	ReturnType *TypeExpr
}

// ---------- Literals ----------

type NilLit struct{ Pos_ Position }
type BoolLit struct {
	Pos_  Position
	Value bool
}
type IntLit struct {
	Pos_  Position
	Value int64
}
type FloatLit struct {
	Pos_  Position
	Value float64
}
type StringLit struct {
	Pos_  Position
	Value string
}
type KeywordLit struct {
	Pos_  Position
	Value string // without leading ":"
}

func (n *NilLit) nodeMarker()      {}
func (n *NilLit) Pos() Position    { return n.Pos_ }
func (n *BoolLit) nodeMarker()     {}
func (n *BoolLit) Pos() Position   { return n.Pos_ }
func (n *IntLit) nodeMarker()      {}
func (n *IntLit) Pos() Position    { return n.Pos_ }
func (n *FloatLit) nodeMarker()    {}
func (n *FloatLit) Pos() Position  { return n.Pos_ }
func (n *StringLit) nodeMarker()   {}
func (n *StringLit) Pos() Position { return n.Pos_ }
func (n *KeywordLit) nodeMarker()  {}
func (n *KeywordLit) Pos() Position { return n.Pos_ }

func NewNilLit(pos Position) *NilLit              { return &NilLit{pos} }
func NewBoolLit(pos Position, v bool) *BoolLit    { return &BoolLit{pos, v} }
func NewIntLit(pos Position, v int64) *IntLit     { return &IntLit{pos, v} }
func NewFloatLit(pos Position, v float64) *FloatLit { return &FloatLit{pos, v} }
func NewStringLit(pos Position, v string) *StringLit { return &StringLit{pos, v} }
func NewKeywordLit(pos Position, v string) *KeywordLit { return &KeywordLit{pos, v} }

// ---------- Symbol ----------

type Symbol struct {
	Pos_      Position
	Name      string
	TypeAnnot *TypeExpr // nil if no annotation
}

func (n *Symbol) nodeMarker()    {}
func (n *Symbol) Pos() Position  { return n.Pos_ }
func NewSymbol(pos Position, name string) *Symbol { return &Symbol{Pos_: pos, Name: name} }

// ---------- Collections ----------

type VectorLit struct {
	Pos_      Position
	Elements  []Node
	TypeAnnot *TypeExpr
}

type MapLit struct {
	Pos_      Position
	Pairs     []MapPair
	TypeAnnot *TypeExpr
}

type SetLit struct {
	Pos_     Position
	Elements []Node
}

func (n *VectorLit) nodeMarker()    {}
func (n *VectorLit) Pos() Position  { return n.Pos_ }
func (n *MapLit) nodeMarker()       {}
func (n *MapLit) Pos() Position     { return n.Pos_ }
func (n *SetLit) nodeMarker()       {}
func (n *SetLit) Pos() Position     { return n.Pos_ }

func NewVectorLit(pos Position, elems []Node) *VectorLit {
	return &VectorLit{Pos_: pos, Elements: elems}
}
func NewMapLit(pos Position, pairs []MapPair) *MapLit {
	return &MapLit{Pos_: pos, Pairs: pairs}
}

// ---------- Top-level declarations ----------

// NSDecl: (ns name (:import [...]))
type NSDecl struct {
	Pos_    Position
	Name    string
	Imports []ImportSpec
}

func (n *NSDecl) nodeMarker()    {}
func (n *NSDecl) Pos() Position  { return n.Pos_ }
func NewNSDecl(pos Position, name string, imports []ImportSpec) *NSDecl {
	return &NSDecl{Pos_: pos, Name: name, Imports: imports}
}

// DefDecl: (def ^T name value)
type DefDecl struct {
	Pos_      Position
	Name      string
	TypeAnnot *TypeExpr
	Value     Node
}

func (n *DefDecl) nodeMarker()    {}
func (n *DefDecl) Pos() Position  { return n.Pos_ }
func NewDefDecl(pos Position, name string, annot *TypeExpr, val Node) *DefDecl {
	return &DefDecl{Pos_: pos, Name: name, TypeAnnot: annot, Value: val}
}

// DefnDecl: (defn ^ReturnType name [params...] body...)
type DefnDecl struct {
	Pos_       Position
	Name       string
	Params     []Param
	ReturnType *TypeExpr
	Doc        string
	Body       []Node
}

func (n *DefnDecl) nodeMarker()   {}
func (n *DefnDecl) Pos() Position { return n.Pos_ }
func NewDefnDecl(pos Position, name string, params []Param, ret *TypeExpr, doc string, body []Node) *DefnDecl {
	return &DefnDecl{Pos_: pos, Name: name, Params: params, ReturnType: ret, Doc: doc, Body: body}
}

// StructDecl: (defstruct Name ^T1 field1 ^T2 field2 ...)
type StructDecl struct {
	Pos_   Position
	Name   string
	Fields []StructField
}

func (n *StructDecl) nodeMarker()    {}
func (n *StructDecl) Pos() Position  { return n.Pos_ }
func NewStructDecl(pos Position, name string, fields []StructField) *StructDecl {
	return &StructDecl{Pos_: pos, Name: name, Fields: fields}
}

// InterfaceDecl: (definterface Name (Method [params] ^Ret) ...)
type InterfaceDecl struct {
	Pos_    Position
	Name    string
	Methods []InterfaceMethod
}

func (n *InterfaceDecl) nodeMarker()    {}
func (n *InterfaceDecl) Pos() Position  { return n.Pos_ }
func NewInterfaceDecl(pos Position, name string, methods []InterfaceMethod) *InterfaceDecl {
	return &InterfaceDecl{Pos_: pos, Name: name, Methods: methods}
}

// MethodDecl: (defmethod ^*ReceiverType name [receiver params...] ^RetType body...)
type MethodDecl struct {
	Pos_         Position
	ReceiverType *TypeExpr
	ReceiverName string
	Name         string
	Params       []Param
	ReturnType   *TypeExpr
	Doc          string
	Body         []Node
}

func (n *MethodDecl) nodeMarker()   {}
func (n *MethodDecl) Pos() Position { return n.Pos_ }
func NewMethodDecl(pos Position, recvType *TypeExpr, recvName, name string,
	params []Param, retType *TypeExpr, doc string, body []Node) *MethodDecl {
	return &MethodDecl{Pos_: pos, ReceiverType: recvType, ReceiverName: recvName,
		Name: name, Params: params, ReturnType: retType, Doc: doc, Body: body}
}

// ---------- Expressions ----------

// CallExpr: (f arg1 arg2 ...)
type CallExpr struct {
	Pos_  Position
	Head  Node
	Args  []Node
}

func (n *CallExpr) nodeMarker()    {}
func (n *CallExpr) Pos() Position  { return n.Pos_ }
func NewCallExpr(pos Position, head Node, args []Node) *CallExpr {
	return &CallExpr{Pos_: pos, Head: head, Args: args}
}

// FnExpr: (fn [params] body...)
type FnExpr struct {
	Pos_       Position
	Params     []Param
	ReturnType *TypeExpr
	Body       []Node
}

func (n *FnExpr) nodeMarker()    {}
func (n *FnExpr) Pos() Position  { return n.Pos_ }
func NewFnExpr(pos Position, params []Param, ret *TypeExpr, body []Node) *FnExpr {
	return &FnExpr{Pos_: pos, Params: params, ReturnType: ret, Body: body}
}

// LetExpr: (let [bindings...] body...)
type LetExpr struct {
	Pos_     Position
	Bindings []LetBinding
	Body     []Node
}

func (n *LetExpr) nodeMarker()    {}
func (n *LetExpr) Pos() Position  { return n.Pos_ }
func NewLetExpr(pos Position, bindings []LetBinding, body []Node) *LetExpr {
	return &LetExpr{Pos_: pos, Bindings: bindings, Body: body}
}

// IfExpr: (if cond then else?)
type IfExpr struct {
	Pos_  Position
	Cond  Node
	Then  Node
	Else  Node // nil if absent
}

func (n *IfExpr) nodeMarker()    {}
func (n *IfExpr) Pos() Position  { return n.Pos_ }
func NewIfExpr(pos Position, cond, then, els Node) *IfExpr {
	return &IfExpr{Pos_: pos, Cond: cond, Then: then, Else: els}
}

// WhenExpr: (when cond body...)
type WhenExpr struct {
	Pos_  Position
	Cond  Node
	Body  []Node
}

func (n *WhenExpr) nodeMarker()    {}
func (n *WhenExpr) Pos() Position  { return n.Pos_ }
func NewWhenExpr(pos Position, cond Node, body []Node) *WhenExpr {
	return &WhenExpr{Pos_: pos, Cond: cond, Body: body}
}

// CondExpr: (cond test1 val1 ... :else default)
type CondExpr struct {
	Pos_     Position
	Clauses  []CondClause
	Default  Node
}

func (n *CondExpr) nodeMarker()    {}
func (n *CondExpr) Pos() Position  { return n.Pos_ }
func NewCondExpr(pos Position, clauses []CondClause, def Node) *CondExpr {
	return &CondExpr{Pos_: pos, Clauses: clauses, Default: def}
}

// DoExpr: (do body...)
type DoExpr struct {
	Pos_  Position
	Body  []Node
}

func (n *DoExpr) nodeMarker()    {}
func (n *DoExpr) Pos() Position  { return n.Pos_ }
func NewDoExpr(pos Position, body []Node) *DoExpr {
	return &DoExpr{Pos_: pos, Body: body}
}

// QuoteExpr: 'x — not transpiled, only for data literals
type QuoteExpr struct {
	Pos_  Position
	Form  Node
}

func (n *QuoteExpr) nodeMarker()    {}
func (n *QuoteExpr) Pos() Position  { return n.Pos_ }

// ReturnExpr: (return a b?) explicit return
type ReturnExpr struct {
	Pos_  Position
	Args  []Node
}

func (n *ReturnExpr) nodeMarker()    {}
func (n *ReturnExpr) Pos() Position  { return n.Pos_ }
func NewReturnExpr(pos Position, args []Node) *ReturnExpr {
	return &ReturnExpr{Pos_: pos, Args: args}
}

// ValuesExpr: (values a b) — multi-return in tail position
type ValuesExpr struct {
	Pos_  Position
	Args  []Node
}

func (n *ValuesExpr) nodeMarker()    {}
func (n *ValuesExpr) Pos() Position  { return n.Pos_ }
func NewValuesExpr(pos Position, args []Node) *ValuesExpr {
	return &ValuesExpr{Pos_: pos, Args: args}
}

// ---------- Go-specific forms ----------

// GoStmt: (go body...) → go func() { body }()
type GoStmt struct {
	Pos_  Position
	Body  []Node
}

func (n *GoStmt) nodeMarker()    {}
func (n *GoStmt) Pos() Position  { return n.Pos_ }
func NewGoStmt(pos Position, body []Node) *GoStmt {
	return &GoStmt{Pos_: pos, Body: body}
}

// DeferStmt: (defer expr)
type DeferStmt struct {
	Pos_  Position
	Expr  Node
}

func (n *DeferStmt) nodeMarker()    {}
func (n *DeferStmt) Pos() Position  { return n.Pos_ }
func NewDeferStmt(pos Position, expr Node) *DeferStmt {
	return &DeferStmt{Pos_: pos, Expr: expr}
}

// ChanExpr: (chan T cap?)
type ChanExpr struct {
	Pos_     Position
	ElemType *TypeExpr
	Cap      Node // nil = unbuffered
}

func (n *ChanExpr) nodeMarker()    {}
func (n *ChanExpr) Pos() Position  { return n.Pos_ }
func NewChanExpr(pos Position, elem *TypeExpr, cap Node) *ChanExpr {
	return &ChanExpr{Pos_: pos, ElemType: elem, Cap: cap}
}

// SendStmt: (send! ch val)
type SendStmt struct {
	Pos_  Position
	Chan  Node
	Val   Node
}

func (n *SendStmt) nodeMarker()    {}
func (n *SendStmt) Pos() Position  { return n.Pos_ }
func NewSendStmt(pos Position, ch, val Node) *SendStmt {
	return &SendStmt{Pos_: pos, Chan: ch, Val: val}
}

// RecvExpr: (recv! ch)
type RecvExpr struct {
	Pos_  Position
	Chan  Node
}

func (n *RecvExpr) nodeMarker()    {}
func (n *RecvExpr) Pos() Position  { return n.Pos_ }
func NewRecvExpr(pos Position, ch Node) *RecvExpr {
	return &RecvExpr{Pos_: pos, Chan: ch}
}

// CloseStmt: (close! ch)
type CloseStmt struct {
	Pos_  Position
	Chan  Node
}

func (n *CloseStmt) nodeMarker()    {}
func (n *CloseStmt) Pos() Position  { return n.Pos_ }
func NewCloseStmt(pos Position, ch Node) *CloseStmt {
	return &CloseStmt{Pos_: pos, Chan: ch}
}

// SelectStmt: (select! cases...)
type SelectStmt struct {
	Pos_  Position
	Cases []SelectCase
}

func (n *SelectStmt) nodeMarker()    {}
func (n *SelectStmt) Pos() Position  { return n.Pos_ }
func NewSelectStmt(pos Position, cases []SelectCase) *SelectStmt {
	return &SelectStmt{Pos_: pos, Cases: cases}
}

// LoopExpr: (loop [bindings...] body...)
type LoopExpr struct {
	Pos_     Position
	Bindings []LetBinding
	Body     []Node
}

func (n *LoopExpr) nodeMarker()    {}
func (n *LoopExpr) Pos() Position  { return n.Pos_ }
func NewLoopExpr(pos Position, bindings []LetBinding, body []Node) *LoopExpr {
	return &LoopExpr{Pos_: pos, Bindings: bindings, Body: body}
}

// RecurExpr: (recur args...)
type RecurExpr struct {
	Pos_  Position
	Args  []Node
}

func (n *RecurExpr) nodeMarker()    {}
func (n *RecurExpr) Pos() Position  { return n.Pos_ }
func NewRecurExpr(pos Position, args []Node) *RecurExpr {
	return &RecurExpr{Pos_: pos, Args: args}
}

// ---------- Interop ----------

// MethodCallExpr: (.Method obj args...)
type MethodCallExpr struct {
	Pos_    Position
	Method  string
	Object  Node
	Args    []Node
}

func (n *MethodCallExpr) nodeMarker()    {}
func (n *MethodCallExpr) Pos() Position  { return n.Pos_ }
func NewMethodCallExpr(pos Position, method string, obj Node, args []Node) *MethodCallExpr {
	return &MethodCallExpr{Pos_: pos, Method: method, Object: obj, Args: args}
}

// FieldAccessExpr: (.-Field obj)
type FieldAccessExpr struct {
	Pos_    Position
	Field   string
	Object  Node
}

func (n *FieldAccessExpr) nodeMarker()    {}
func (n *FieldAccessExpr) Pos() Position  { return n.Pos_ }
func NewFieldAccessExpr(pos Position, field string, obj Node) *FieldAccessExpr {
	return &FieldAccessExpr{Pos_: pos, Field: field, Object: obj}
}

// StructLitExpr: (TypeName. {:field val ...})
type StructLitExpr struct {
	Pos_     Position
	TypeName string
	Fields   []MapPair
}

func (n *StructLitExpr) nodeMarker()    {}
func (n *StructLitExpr) Pos() Position  { return n.Pos_ }
func NewStructLitExpr(pos Position, typeName string, fields []MapPair) *StructLitExpr {
	return &StructLitExpr{Pos_: pos, TypeName: typeName, Fields: fields}
}

// TypeAssertExpr: (as ^T val)
type TypeAssertExpr struct {
	Pos_  Position
	Type  *TypeExpr
	Value Node
}

func (n *TypeAssertExpr) nodeMarker()    {}
func (n *TypeAssertExpr) Pos() Position  { return n.Pos_ }
func NewTypeAssertExpr(pos Position, ty *TypeExpr, val Node) *TypeAssertExpr {
	return &TypeAssertExpr{Pos_: pos, Type: ty, Value: val}
}

// ---------- Test declarations ----------

// DefTestDecl: (deftest name body...)
// Body contains assert= / assert-true / assert-false / assert-nil / assert-err forms.
type DefTestDecl struct {
	Pos_  Position
	Name  string
	Body  []Node
}

func (n *DefTestDecl) nodeMarker()    {}
func (n *DefTestDecl) Pos() Position  { return n.Pos_ }
func NewDefTestDecl(pos Position, name string, body []Node) *DefTestDecl {
	return &DefTestDecl{Pos_: pos, Name: name, Body: body}
}

// ---------- Error handling ----------

// IfErrExpr: (if-err [val err] expr on-err on-ok)
type IfErrExpr struct {
	Pos_    Position
	ValName string
	ErrName string
	Expr    Node
	OnErr   Node
	OnOk    Node
}

func (n *IfErrExpr) nodeMarker()    {}
func (n *IfErrExpr) Pos() Position  { return n.Pos_ }
func NewIfErrExpr(pos Position, val, errName string, expr, onErr, onOk Node) *IfErrExpr {
	return &IfErrExpr{Pos_: pos, ValName: val, ErrName: errName, Expr: expr, OnErr: onErr, OnOk: onOk}
}
