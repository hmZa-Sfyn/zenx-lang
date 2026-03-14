package main

import "fmt"

// ─────────────────────────────────────────────────────────────────────────────
//  Type system
// ─────────────────────────────────────────────────────────────────────────────

type TypeKind int

const (
	TyInt    TypeKind = iota
	TyFloat
	TyBool
	TyStr
	TyChar
	TyVoid
	TyPtr    // ptr<T>
	TyArray  // [N]T
	TyStruct
	TyUnknown
)

type ZXType struct {
	Kind    TypeKind
	PtrElem *ZXType
	ArrElem *ZXType
	ArrSize int
	Name    string // TyStruct
}

var (
	TypInt     = &ZXType{Kind: TyInt}
	TypFloat   = &ZXType{Kind: TyFloat}
	TypBool    = &ZXType{Kind: TyBool}
	TypStr     = &ZXType{Kind: TyStr}
	TypChar    = &ZXType{Kind: TyChar}
	TypVoid    = &ZXType{Kind: TyVoid}
	TypUnknown = &ZXType{Kind: TyUnknown}
)

func PtrOf(elem *ZXType) *ZXType       { return &ZXType{Kind: TyPtr, PtrElem: elem} }
func ArrayOf(elem *ZXType, n int) *ZXType { return &ZXType{Kind: TyArray, ArrElem: elem, ArrSize: n} }
func StructType(name string) *ZXType   { return &ZXType{Kind: TyStruct, Name: name} }

func (t *ZXType) String() string {
	if t == nil { return "<nil>" }
	switch t.Kind {
	case TyInt:    return "int"
	case TyFloat:  return "float"
	case TyBool:   return "bool"
	case TyStr:    return "str"
	case TyChar:   return "char"
	case TyVoid:   return "void"
	case TyPtr:
		if t.PtrElem != nil { return fmt.Sprintf("ptr<%s>", t.PtrElem) }
		return "ptr"
	case TyArray:
		if t.ArrElem != nil {
			if t.ArrSize > 0 { return fmt.Sprintf("[%d]%s", t.ArrSize, t.ArrElem) }
			return fmt.Sprintf("[]%s", t.ArrElem)
		}
		return "array"
	case TyStruct: return t.Name
	default:       return "unknown"
	}
}

func typeEq(a, b *ZXType) bool {
	if a == nil || b == nil { return false }
	if a == b { return true }
	if a.Kind != b.Kind { return false }
	switch a.Kind {
	case TyPtr:    return typeEq(a.PtrElem, b.PtrElem)
	case TyArray:  return typeEq(a.ArrElem, b.ArrElem)
	case TyStruct: return a.Name == b.Name
	default:       return true
	}
}

func coercible(from, to *ZXType) bool {
	if typeEq(from, to) { return true }
	if from == nil || to == nil { return false }
	if from.Kind == TyInt   && to.Kind == TyFloat { return true }
	if from.Kind == TyInt   && to.Kind == TyChar  { return true }
	if from.Kind == TyChar  && to.Kind == TyInt   { return true }
	if from.Kind == TyBool  && to.Kind == TyInt   { return true }
	// nil -> any pointer
	if from.Kind == TyPtr && from.PtrElem != nil && from.PtrElem.Kind == TyVoid && to.Kind == TyPtr {
		return true
	}
	// any ptr -> void*
	if from.Kind == TyPtr && to.Kind == TyPtr && to.PtrElem != nil && to.PtrElem.Kind == TyVoid {
		return true
	}
	// str <-> ptr<char>  (for C interop)
	if from.Kind == TyStr && to.Kind == TyPtr && to.PtrElem != nil && to.PtrElem.Kind == TyChar {
		return true
	}
	if from.Kind == TyPtr && from.PtrElem != nil && from.PtrElem.Kind == TyChar && to.Kind == TyStr {
		return true
	}
	return false
}

func isNumeric(t *ZXType) bool {
	return t != nil && (t.Kind == TyInt || t.Kind == TyFloat || t.Kind == TyChar)
}
func isInteger(t *ZXType) bool {
	return t != nil && (t.Kind == TyInt || t.Kind == TyChar || t.Kind == TyBool)
}
func isTruthy(t *ZXType) bool {
	return t != nil && (t.Kind == TyInt || t.Kind == TyBool || t.Kind == TyChar ||
		t.Kind == TyFloat || t.Kind == TyPtr)
}

// ─────────────────────────────────────────────────────────────────────────────
//  AST
// ─────────────────────────────────────────────────────────────────────────────

type Node interface {
	nodeSpan() Span
	nodeTag() string
}

type Program struct {
	Imports  []*ImportDecl
	Externs  []*ExternDecl
	Structs  []*StructDecl
	TopStmts []Node
}

// import "stdio.h"
type ImportDecl struct {
	Sp    Span
	Path  string
	Alias string
}
func (n *ImportDecl) nodeSpan() Span  { return n.Sp }
func (n *ImportDecl) nodeTag() string { return "import" }

// extern fn sqrt(x: float) -> float;
type ExternDecl struct {
	Sp       Span
	Name     string
	Params   []Param
	Variadic bool
	RetType  *ZXType
}
func (n *ExternDecl) nodeSpan() Span  { return n.Sp }
func (n *ExternDecl) nodeTag() string { return "extern" }

// struct Foo { x: int, y: float }
type StructDecl struct {
	Sp     Span
	Name   string
	Fields []Param
}
func (n *StructDecl) nodeSpan() Span  { return n.Sp }
func (n *StructDecl) nodeTag() string { return "struct" }

// fn foo(a: int) -> int { }
type FnDecl struct {
	Sp       Span
	Name     string
	Params   []Param
	Variadic bool
	RetType  *ZXType
	Body     *Block
}
func (n *FnDecl) nodeSpan() Span  { return n.Sp }
func (n *FnDecl) nodeTag() string { return "fn" }

type Param struct {
	Sp   Span
	Name string
	Type *ZXType
}

// ── Statements ────────────────────────────────────────────────────────────────

type Block struct {
	Sp    Span
	Stmts []Node
}
func (n *Block) nodeSpan() Span  { return n.Sp }
func (n *Block) nodeTag() string { return "block" }

type VarDecl struct {
	Sp           Span
	Name         string
	VarType      *ZXType
	Init         Node
	IsConst      bool
	ResolvedType *ZXType
}
func (n *VarDecl) nodeSpan() Span  { return n.Sp }
func (n *VarDecl) nodeTag() string { return "var" }

type ReturnStmt struct {
	Sp    Span
	Value Node
}
func (n *ReturnStmt) nodeSpan() Span  { return n.Sp }
func (n *ReturnStmt) nodeTag() string { return "return" }

type IfStmt struct {
	Sp    Span
	Cond  Node
	Then  *Block
	Elifs []ElifClause
	Else  *Block
}
type ElifClause struct{ Cond Node; Body *Block }
func (n *IfStmt) nodeSpan() Span  { return n.Sp }
func (n *IfStmt) nodeTag() string { return "if" }

type WhileStmt struct {
	Sp   Span
	Cond Node
	Body *Block
}
func (n *WhileStmt) nodeSpan() Span  { return n.Sp }
func (n *WhileStmt) nodeTag() string { return "while" }

type ForRangeStmt struct {
	Sp   Span
	Var  string
	From Node
	To   Node
	Body *Block
}
func (n *ForRangeStmt) nodeSpan() Span  { return n.Sp }
func (n *ForRangeStmt) nodeTag() string { return "for-range" }

type ExprStmt struct {
	Sp   Span
	Expr Node
}
func (n *ExprStmt) nodeSpan() Span  { return n.Sp }
func (n *ExprStmt) nodeTag() string { return "exprstmt" }

type AssignStmt struct {
	Sp    Span
	LHS   Node
	Op    string
	Value Node
}
func (n *AssignStmt) nodeSpan() Span  { return n.Sp }
func (n *AssignStmt) nodeTag() string { return "assign" }

type BreakStmt    struct{ Sp Span }
type ContinueStmt struct{ Sp Span }
func (n *BreakStmt)    nodeSpan() Span  { return n.Sp }
func (n *BreakStmt)    nodeTag() string { return "break" }
func (n *ContinueStmt) nodeSpan() Span  { return n.Sp }
func (n *ContinueStmt) nodeTag() string { return "continue" }

type PrintStmt struct {
	Sp      Span
	Args    []Node
	Newline bool
}
func (n *PrintStmt) nodeSpan() Span  { return n.Sp }
func (n *PrintStmt) nodeTag() string { return "print" }

type ExitStmt struct {
	Sp   Span
	Code Node
}
func (n *ExitStmt) nodeSpan() Span  { return n.Sp }
func (n *ExitStmt) nodeTag() string { return "exit" }

// ── Expressions ───────────────────────────────────────────────────────────────

type IntLit struct   { Sp Span; Val int64 }
type FloatLit struct { Sp Span; Val float64 }
type BoolLit struct  { Sp Span; Val bool }
type StrLit struct   { Sp Span; Val string }
type NilLit struct   { Sp Span }

func (n *IntLit)   nodeSpan() Span  { return n.Sp }
func (n *IntLit)   nodeTag() string { return "int" }
func (n *FloatLit) nodeSpan() Span  { return n.Sp }
func (n *FloatLit) nodeTag() string { return "float" }
func (n *BoolLit)  nodeSpan() Span  { return n.Sp }
func (n *BoolLit)  nodeTag() string { return "bool" }
func (n *StrLit)   nodeSpan() Span  { return n.Sp }
func (n *StrLit)   nodeTag() string { return "str" }
func (n *NilLit)   nodeSpan() Span  { return n.Sp }
func (n *NilLit)   nodeTag() string { return "nil" }

type Ident struct {
	Sp   Span
	Name string
	Typ  *ZXType
}
func (n *Ident) nodeSpan() Span  { return n.Sp }
func (n *Ident) nodeTag() string { return "ident" }

type BinExpr struct {
	Sp  Span
	Op  string
	LHS Node
	RHS Node
	Typ *ZXType
}
func (n *BinExpr) nodeSpan() Span  { return n.Sp }
func (n *BinExpr) nodeTag() string { return "bin" }

type UnaryExpr struct {
	Sp      Span
	Op      string
	Operand Node
	Typ     *ZXType
}
func (n *UnaryExpr) nodeSpan() Span  { return n.Sp }
func (n *UnaryExpr) nodeTag() string { return "unary" }

type CallExpr struct {
	Sp   Span
	Func Node
	Args []Node
	Typ  *ZXType
}
func (n *CallExpr) nodeSpan() Span  { return n.Sp }
func (n *CallExpr) nodeTag() string { return "call" }

type IndexExpr struct {
	Sp  Span
	Obj Node
	Idx Node
	Typ *ZXType
}
func (n *IndexExpr) nodeSpan() Span  { return n.Sp }
func (n *IndexExpr) nodeTag() string { return "index" }

type FieldExpr struct {
	Sp    Span
	Obj   Node
	Field string
	Typ   *ZXType
}
func (n *FieldExpr) nodeSpan() Span  { return n.Sp }
func (n *FieldExpr) nodeTag() string { return "field" }

type CastExpr struct {
	Sp      Span
	ToType  *ZXType
	Operand Node
	Typ     *ZXType
}
func (n *CastExpr) nodeSpan() Span  { return n.Sp }
func (n *CastExpr) nodeTag() string { return "cast" }

type AddrExpr struct {
	Sp      Span
	Operand Node
	Deref   bool
	Typ     *ZXType
}
func (n *AddrExpr) nodeSpan() Span  { return n.Sp }
func (n *AddrExpr) nodeTag() string { return "addr" }

type StructInit struct {
	Sp     Span
	Name   string
	Fields []FieldInit
	Typ    *ZXType
}
type FieldInit struct {
	Sp    Span
	Name  string
	Value Node
}
func (n *StructInit) nodeSpan() Span  { return n.Sp }
func (n *StructInit) nodeTag() string { return "structinit" }

type ArrayLit struct {
	Sp    Span
	Elems []Node
	Typ   *ZXType
}
func (n *ArrayLit) nodeSpan() Span  { return n.Sp }
func (n *ArrayLit) nodeTag() string { return "arraylit" }

type SizeofExpr struct {
	Sp  Span
	Of  *ZXType
	Typ *ZXType
}
func (n *SizeofExpr) nodeSpan() Span  { return n.Sp }
func (n *SizeofExpr) nodeTag() string { return "sizeof" }
