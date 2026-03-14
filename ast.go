package main

import "fmt"

// ─────────────────────────────────────────────────────────────────────────────
//  Type system
// ─────────────────────────────────────────────────────────────────────────────

type TypeKind int

const (
	TyInt TypeKind = iota
	TyFloat
	TyBool
	TyStr
	TyChar
	TyVoid
	TyPtr  // ptr<T>
	TyStruct
	TyUnknown // pre-resolve placeholder
)

type ZXType struct {
	Kind    TypeKind
	PtrElem *ZXType // for TyPtr
	Name    string  // for TyStruct
}

var (
	TypInt   = &ZXType{Kind: TyInt}
	TypFloat = &ZXType{Kind: TyFloat}
	TypBool  = &ZXType{Kind: TyBool}
	TypStr   = &ZXType{Kind: TyStr}
	TypChar  = &ZXType{Kind: TyChar}
	TypVoid  = &ZXType{Kind: TyVoid}
	TypUnknown = &ZXType{Kind: TyUnknown}
)

func PtrOf(elem *ZXType) *ZXType { return &ZXType{Kind: TyPtr, PtrElem: elem} }
func StructType(name string) *ZXType { return &ZXType{Kind: TyStruct, Name: name} }

func (t *ZXType) String() string {
	if t == nil {
		return "<nil>"
	}
	switch t.Kind {
	case TyInt:
		return "int"
	case TyFloat:
		return "float"
	case TyBool:
		return "bool"
	case TyStr:
		return "str"
	case TyChar:
		return "char"
	case TyVoid:
		return "void"
	case TyPtr:
		if t.PtrElem != nil {
			return fmt.Sprintf("ptr<%s>", t.PtrElem)
		}
		return "ptr"
	case TyStruct:
		return t.Name
	default:
		return "unknown"
	}
}

func typeEq(a, b *ZXType) bool {
	if a == nil || b == nil {
		return false
	}
	if a.Kind != b.Kind {
		return false
	}
	if a.Kind == TyPtr {
		return typeEq(a.PtrElem, b.PtrElem)
	}
	if a.Kind == TyStruct {
		return a.Name == b.Name
	}
	return true
}

// Can 'from' be coerced to 'to' implicitly?
func coercible(from, to *ZXType) bool {
	if typeEq(from, to) {
		return true
	}
	// int <-> float promotion
	if from.Kind == TyInt && to.Kind == TyFloat {
		return true
	}
	// int <-> char
	if from.Kind == TyInt && to.Kind == TyChar {
		return true
	}
	if from.Kind == TyChar && to.Kind == TyInt {
		return true
	}
	return false
}

// ─────────────────────────────────────────────────────────────────────────────
//  AST nodes
// ─────────────────────────────────────────────────────────────────────────────

type Node interface {
	nodeSpan() Span
	nodeTag() string
}

// ── Program ───────────────────────────────────────────────────────────────────

type Program struct {
	Imports  []*ImportDecl
	Externs  []*ExternDecl
	Structs  []*StructDecl
	TopStmts []Node // functions + top-level stmts
}

// ── Declarations ──────────────────────────────────────────────────────────────

// import "stdio.h"
// import "mylib.h" as mylib
type ImportDecl struct {
	Sp   Span
	Path string
	Alias string // optional
}

func (n *ImportDecl) nodeSpan() Span   { return n.Sp }
func (n *ImportDecl) nodeTag() string  { return "import" }

// extern fn printf(fmt: str, ...) -> int;
type ExternDecl struct {
	Sp      Span
	Name    string
	Params  []Param
	Variadic bool
	RetType *ZXType
}

func (n *ExternDecl) nodeSpan() Span  { return n.Sp }
func (n *ExternDecl) nodeTag() string { return "extern" }

// struct Point { x: float, y: float }
type StructDecl struct {
	Sp     Span
	Name   string
	Fields []Param
}

func (n *StructDecl) nodeSpan() Span  { return n.Sp }
func (n *StructDecl) nodeTag() string { return "struct" }

// fn add(a: int, b: int) -> int { ... }
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

// let x: int = 5;
// const PI: float = 3.14;
type VarDecl struct {
	Sp      Span
	Name    string
	VarType *ZXType // nil = infer
	Init    Node    // nil allowed for let without init
	IsConst bool
	ResolvedType *ZXType // set by type checker
}

func (n *VarDecl) nodeSpan() Span  { return n.Sp }
func (n *VarDecl) nodeTag() string { return "var" }

// return expr;
type ReturnStmt struct {
	Sp    Span
	Value Node // nil for bare return
}

func (n *ReturnStmt) nodeSpan() Span  { return n.Sp }
func (n *ReturnStmt) nodeTag() string { return "return" }

// if cond { } elif cond { } else { }
type IfStmt struct {
	Sp       Span
	Cond     Node
	Then     *Block
	Elifs    []ElifClause
	Else     *Block
}

type ElifClause struct {
	Cond Node
	Body *Block
}

func (n *IfStmt) nodeSpan() Span  { return n.Sp }
func (n *IfStmt) nodeTag() string { return "if" }

// while cond { }
type WhileStmt struct {
	Sp   Span
	Cond Node
	Body *Block
}

func (n *WhileStmt) nodeSpan() Span  { return n.Sp }
func (n *WhileStmt) nodeTag() string { return "while" }

// for i in 0..10 { }
type ForRangeStmt struct {
	Sp    Span
	Var   string
	From  Node
	To    Node
	Body  *Block
}

func (n *ForRangeStmt) nodeSpan() Span  { return n.Sp }
func (n *ForRangeStmt) nodeTag() string { return "for-range" }

// expr;  (expression statement)
type ExprStmt struct {
	Sp   Span
	Expr Node
}

func (n *ExprStmt) nodeSpan() Span  { return n.Sp }
func (n *ExprStmt) nodeTag() string { return "exprstmt" }

// x = expr  or  x += expr etc.
type AssignStmt struct {
	Sp    Span
	LHS   Node
	Op    string // = += -= *= /=
	Value Node
}

func (n *AssignStmt) nodeSpan() Span  { return n.Sp }
func (n *AssignStmt) nodeTag() string { return "assign" }

type BreakStmt struct{ Sp Span }

func (n *BreakStmt) nodeSpan() Span  { return n.Sp }
func (n *BreakStmt) nodeTag() string { return "break" }

type ContinueStmt struct{ Sp Span }

func (n *ContinueStmt) nodeSpan() Span  { return n.Sp }
func (n *ContinueStmt) nodeTag() string { return "continue" }

// print(expr, expr, ...);  println(...)
type PrintStmt struct {
	Sp      Span
	Args    []Node
	Newline bool
}

func (n *PrintStmt) nodeSpan() Span  { return n.Sp }
func (n *PrintStmt) nodeTag() string { return "print" }

// exit(code)
type ExitStmt struct {
	Sp   Span
	Code Node
}

func (n *ExitStmt) nodeSpan() Span  { return n.Sp }
func (n *ExitStmt) nodeTag() string { return "exit" }

// ── Expressions ───────────────────────────────────────────────────────────────

type IntLit struct {
	Sp  Span
	Val int64
}

func (n *IntLit) nodeSpan() Span  { return n.Sp }
func (n *IntLit) nodeTag() string { return "int" }

type FloatLit struct {
	Sp  Span
	Val float64
}

func (n *FloatLit) nodeSpan() Span  { return n.Sp }
func (n *FloatLit) nodeTag() string { return "float" }

type BoolLit struct {
	Sp  Span
	Val bool
}

func (n *BoolLit) nodeSpan() Span  { return n.Sp }
func (n *BoolLit) nodeTag() string { return "bool" }

type StrLit struct {
	Sp  Span
	Val string
}

func (n *StrLit) nodeSpan() Span  { return n.Sp }
func (n *StrLit) nodeTag() string { return "str" }

type NilLit struct{ Sp Span }

func (n *NilLit) nodeSpan() Span  { return n.Sp }
func (n *NilLit) nodeTag() string { return "nil" }

type Ident struct {
	Sp   Span
	Name string
	Typ  *ZXType // resolved by type checker
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

// f(args...)
type CallExpr struct {
	Sp   Span
	Func Node // Ident or FieldAccess
	Args []Node
	Typ  *ZXType
}

func (n *CallExpr) nodeSpan() Span  { return n.Sp }
func (n *CallExpr) nodeTag() string { return "call" }

// a[i]
type IndexExpr struct {
	Sp  Span
	Obj Node
	Idx Node
	Typ *ZXType
}

func (n *IndexExpr) nodeSpan() Span  { return n.Sp }
func (n *IndexExpr) nodeTag() string { return "index" }

// a.b
type FieldExpr struct {
	Sp    Span
	Obj   Node
	Field string
	Typ   *ZXType
}

func (n *FieldExpr) nodeSpan() Span  { return n.Sp }
func (n *FieldExpr) nodeTag() string { return "field" }

// cast<int>(expr)
type CastExpr struct {
	Sp      Span
	ToType  *ZXType
	Operand Node
	Typ     *ZXType
}

func (n *CastExpr) nodeSpan() Span  { return n.Sp }
func (n *CastExpr) nodeTag() string { return "cast" }

// &expr  *expr
type AddrExpr struct {
	Sp      Span
	Operand Node
	Deref   bool // true = *expr
	Typ     *ZXType
}

func (n *AddrExpr) nodeSpan() Span  { return n.Sp }
func (n *AddrExpr) nodeTag() string { return "addr" }

// new Point { x: 1.0, y: 2.0 }
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
