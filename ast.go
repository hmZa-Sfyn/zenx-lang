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
	TyRef
	TyArray
	TySlice
	TyStruct
	TyAny
	TyUnknown
)

type ZXType struct {
	Kind    TypeKind
	Elem    *ZXType
	ArrSize int
	Name    string
}

var (
	TypInt     = &ZXType{Kind: TyInt}
	TypFloat   = &ZXType{Kind: TyFloat}
	TypBool    = &ZXType{Kind: TyBool}
	TypStr     = &ZXType{Kind: TyStr}
	TypChar    = &ZXType{Kind: TyChar}
	TypVoid    = &ZXType{Kind: TyVoid}
	TypAny     = &ZXType{Kind: TyAny}
	TypUnknown = &ZXType{Kind: TyUnknown}
)

func RefOf(elem *ZXType) *ZXType          { return &ZXType{Kind: TyRef, Elem: elem} }
func ArrayOf(elem *ZXType, n int) *ZXType { return &ZXType{Kind: TyArray, Elem: elem, ArrSize: n} }
func SliceOf(elem *ZXType) *ZXType        { return &ZXType{Kind: TySlice, Elem: elem} }
func StructType(name string) *ZXType      { return &ZXType{Kind: TyStruct, Name: name} }
func PtrOf(elem *ZXType) *ZXType          { return RefOf(elem) }

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
	case TyAny:
		return "any"
	case TyRef:
		if t.Elem != nil {
			return "ref " + t.Elem.String()
		}
		return "ref"
	case TyArray:
		if t.Elem != nil {
			if t.ArrSize > 0 {
				return fmt.Sprintf("[%d]%s", t.ArrSize, t.Elem)
			}
			return "[]" + t.Elem.String()
		}
		return "array"
	case TySlice:
		if t.Elem != nil {
			return "[]" + t.Elem.String()
		}
		return "slice"
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
	if a == b {
		return true
	}
	if a.Kind == TyAny || b.Kind == TyAny {
		return true
	}
	if a.Kind != b.Kind {
		return false
	}
	switch a.Kind {
	case TyRef, TyArray, TySlice:
		return typeEq(a.Elem, b.Elem)
	case TyStruct:
		return a.Name == b.Name
	default:
		return true
	}
}

func coercible(from, to *ZXType) bool {
	if typeEq(from, to) {
		return true
	}
	if from == nil || to == nil {
		return false
	}
	if from.Kind == TyAny || to.Kind == TyAny {
		return true
	}
	if from.Kind == TyUnknown || to.Kind == TyUnknown {
		return true
	}
	if from.Kind == TyInt && to.Kind == TyFloat {
		return true
	}
	if from.Kind == TyInt && to.Kind == TyChar {
		return true
	}
	if from.Kind == TyChar && to.Kind == TyInt {
		return true
	}
	if from.Kind == TyBool && to.Kind == TyInt {
		return true
	}
	if from.Kind == TyInt && to.Kind == TyBool {
		return true
	}
	if from.Kind == TyRef && to.Kind == TyRef {
		return true
	}
	if from.Kind == TyStr && to.Kind == TyRef && to.Elem != nil && to.Elem.Kind == TyChar {
		return true
	}
	if from.Kind == TyRef && from.Elem != nil && from.Elem.Kind == TyChar && to.Kind == TyStr {
		return true
	}
	return false
}

func isNumeric(t *ZXType) bool {
	if t == nil {
		return false
	}
	return t.Kind == TyInt || t.Kind == TyFloat || t.Kind == TyChar || t.Kind == TyAny
}
func isInteger(t *ZXType) bool {
	if t == nil {
		return false
	}
	return t.Kind == TyInt || t.Kind == TyChar || t.Kind == TyBool || t.Kind == TyAny
}
func isTruthy(t *ZXType) bool {
	if t == nil {
		return false
	}
	return t.Kind == TyInt || t.Kind == TyBool || t.Kind == TyChar ||
		t.Kind == TyFloat || t.Kind == TyRef || t.Kind == TyAny
}

// ─────────────────────────────────────────────────────────────────────────────
//  Annotations
// ─────────────────────────────────────────────────────────────────────────────

type Annotation struct {
	Sp   Span
	Name string
	Args map[string]string
}

const (
	AnnTest       = "test"
	AnnIgnore     = "ignore"
	AnnSkip       = "skip"
	AnnArgs       = "args"
	AnnExpect     = "expect"
	AnnTimeout    = "timeout"
	AnnInline     = "inline"
	AnnDeprecated = "deprecated"
	AnnNoReturn   = "noreturn"
	AnnPure       = "pure"
	AnnUnsafe     = "unsafe"
	AnnExport     = "export"
	AnnBenchmark  = "benchmark"
	AnnSetup      = "setup"
	AnnTeardown   = "teardown"
)

// ─────────────────────────────────────────────────────────────────────────────
//  TestDecl
// ─────────────────────────────────────────────────────────────────────────────

type TestDecl struct {
	Fn       *FnDecl
	Ignored  bool
	Args     map[string]string
	Expected string
	Timeout  int
	ModPath  string
}

// ─────────────────────────────────────────────────────────────────────────────
//  ModBlock
// ─────────────────────────────────────────────────────────────────────────────

type ModBlock struct {
	Sp      Span
	Name    string
	Path    string
	Mods    []*ModBlock
	Structs []*StructDecl
	Methods []*MethodDecl
	Fns     []*FnDecl
	Tests   []*TestDecl
}

func (n *ModBlock) nodeSpan() Span  { return n.Sp }
func (n *ModBlock) nodeTag() string { return "mod" }

// ─────────────────────────────────────────────────────────────────────────────
//  MacroDecl
// ─────────────────────────────────────────────────────────────────────────────

type MacroDecl struct {
	Sp      Span
	Name    string
	Params  []Param
	RetType *ZXType
	Body    *Block
	Inputs  []string
	Outputs []string
}

func (n *MacroDecl) nodeSpan() Span  { return n.Sp }
func (n *MacroDecl) nodeTag() string { return "macro" }

type MacroCallExpr struct {
	Sp   Span
	Name string
	Args []Node
	Typ  *ZXType
}

func (n *MacroCallExpr) nodeSpan() Span  { return n.Sp }
func (n *MacroCallExpr) nodeTag() string { return "macrocall" }

type MacroCallChain struct {
	Sp    Span
	Recv  Node
	Steps []MacroChainStep
	Typ   *ZXType
}
type MacroChainStep struct {
	Sp    Span
	Macro string
	Args  []Node
	Body  *Block
}

func (n *MacroCallChain) nodeSpan() Span  { return n.Sp }
func (n *MacroCallChain) nodeTag() string { return "macrochain" }

// ─────────────────────────────────────────────────────────────────────────────
//  AST nodes
// ─────────────────────────────────────────────────────────────────────────────

type Node interface {
	nodeSpan() Span
	nodeTag() string
}

// ── Program ───────────────────────────────────────────────────────────────────

type Program struct {
	Module    string
	Imports   []*ImportDecl
	Externs   []*ExternDecl
	Structs   []*StructDecl
	Methods   []*MethodDecl
	ModBlocks []*ModBlock
	Macros    []*MacroDecl
	TopStmts  []Node
	// GlobalVars holds 'our' declarations hoisted to C file scope.
	// Populated during type-check, consumed by emitter.
	GlobalVars []*VarDecl
}

// ── Import ────────────────────────────────────────────────────────────────────

// ImportDecl represents a single import/use statement.
//
// Forms and their fields:
//   use "stdio.h"                   → Path="stdio.h", IsCHeader=true
//   use std::str                    → Module="std::str", IsStdModule=true
//   import std/net/socket           → EnvPrefix="ZENX_STD_PATH", Segments=["net","socket"], ImportAll=true
//   import std/net/socket (socServ) → EnvPrefix="ZENX_STD_PATH", Segments=["net","socket"], Alias="socServ"
//   import _/a                      → UpsCount=0, Segments=["a"], ImportAll=true
//   import __/a (Mod)              → UpsCount=1, Segments=["a"], Alias="Mod"
type ImportDecl struct {
	Sp   Span
	// Raw C header path — set only for: use "stdio.h"
	Path      string
	IsCHeader bool

	// ZX stdlib module — set only for: use std::str
	Module      string
	IsStdModule bool

	// File-path import — set for std/x/y and _/x forms
	IsFileImport bool
	EnvPrefix    string // e.g. "ZENX_STD_PATH" or "ZENX_USR_PATH" or "" for local
	UpsCount     int    // how many levels up (0=same dir, 1=.., 2=../..)
	Segments     []string // path components after the prefix/ups
	ResolvedFile string   // populated by typechecker/resolver: absolute .zx path
	Alias        string   // (ModName) — specific mod to import; empty = import all
	ImportAll    bool     // true when no (ModName) specified

	// Legacy fields kept for compatibility with emitter/typechecker that read them
	IsStd     bool
	IsUser    bool
	IsLocal   bool
	LocalFile string
}

func (n *ImportDecl) nodeSpan() Span  { return n.Sp }
func (n *ImportDecl) nodeTag() string { return "import" }

type ExternDecl struct {
	Sp       Span
	Name     string
	Params   []Param
	Variadic bool
	RetType  *ZXType
}

func (n *ExternDecl) nodeSpan() Span  { return n.Sp }
func (n *ExternDecl) nodeTag() string { return "extern" }

type StructDecl struct {
	Sp          Span
	Name        string
	Fields      []Param
	Annotations []Annotation
}

func (n *StructDecl) nodeSpan() Span  { return n.Sp }
func (n *StructDecl) nodeTag() string { return "struct" }

type FnDecl struct {
	Sp          Span
	Name        string
	Params      []Param
	Variadic    bool
	RetType     *ZXType
	Body        *Block
	Annotations []Annotation
	ModPath     string
}

func (n *FnDecl) nodeSpan() Span  { return n.Sp }
func (n *FnDecl) nodeTag() string { return "fn" }

func (n *FnDecl) HasAnnotation(name string) bool {
	for _, a := range n.Annotations {
		if a.Name == name {
			return true
		}
	}
	return false
}
func (n *FnDecl) GetAnnotation(name string) *Annotation {
	for i := range n.Annotations {
		if n.Annotations[i].Name == name {
			return &n.Annotations[i]
		}
	}
	return nil
}

type MethodDecl struct {
	Sp          Span
	RecvName    string
	RecvType    string
	RecvRef     bool
	Name        string
	Params      []Param
	Variadic    bool
	RetType     *ZXType
	Body        *Block
	Annotations []Annotation
}

func (n *MethodDecl) nodeSpan() Span  { return n.Sp }
func (n *MethodDecl) nodeTag() string { return "method" }
func (n *MethodDecl) CName() string   { return n.RecvType + "_" + n.Name }

type Param struct {
	Sp      Span
	Name    string
	Type    *ZXType
	Default Node
}

// ── Statements ────────────────────────────────────────────────────────────────

type Block struct {
	Sp    Span
	Stmts []Node
}

func (n *Block) nodeSpan() Span  { return n.Sp }
func (n *Block) nodeTag() string { return "block" }

// VarDecl — IsGlobal is set when declared with 'our' at top level.
// The emitter hoists IsGlobal vars to C file scope before main().
type VarDecl struct {
	Sp           Span
	Name         string
	VarType      *ZXType
	Init         Node
	IsConst      bool
	IsGlobal     bool // true for 'our' top-level declarations
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

type ElifClause struct {
	Cond Node
	Body *Block
}

func (n *IfStmt) nodeSpan() Span  { return n.Sp }
func (n *IfStmt) nodeTag() string { return "if" }

type UnlessStmt struct {
	Sp   Span
	Cond Node
	Body *Block
	Else *Block
}

func (n *UnlessStmt) nodeSpan() Span  { return n.Sp }
func (n *UnlessStmt) nodeTag() string { return "unless" }

type WhileStmt struct {
	Sp   Span
	Cond Node
	Body *Block
}

func (n *WhileStmt) nodeSpan() Span  { return n.Sp }
func (n *WhileStmt) nodeTag() string { return "while" }

type UntilStmt struct {
	Sp   Span
	Cond Node
	Body *Block
}

func (n *UntilStmt) nodeSpan() Span  { return n.Sp }
func (n *UntilStmt) nodeTag() string { return "until" }

type ForRangeStmt struct {
	Sp   Span
	Var  string
	From Node
	To   Node
	Step Node
	Body *Block
}

func (n *ForRangeStmt) nodeSpan() Span  { return n.Sp }
func (n *ForRangeStmt) nodeTag() string { return "for-range" }

type MatchStmt struct {
	Sp   Span
	Expr Node
	Arms []MatchArm
}

type MatchArm struct {
	Sp      Span
	Pattern Node
	IsWild  bool
	Guard   Node
	Body    *Block
}

func (n *MatchStmt) nodeSpan() Span  { return n.Sp }
func (n *MatchStmt) nodeTag() string { return "match" }

type TryCatchStmt struct {
	Sp      Span
	Try     *Block
	ErrVar  string
	Catch   *Block
	Finally *Block
}

func (n *TryCatchStmt) nodeSpan() Span  { return n.Sp }
func (n *TryCatchStmt) nodeTag() string { return "try" }

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

type BreakStmt struct{ Sp Span }
type ContinueStmt struct{ Sp Span }

func (n *BreakStmt) nodeSpan() Span    { return n.Sp }
func (n *BreakStmt) nodeTag() string   { return "break" }
func (n *ContinueStmt) nodeSpan() Span { return n.Sp }
func (n *ContinueStmt) nodeTag() string { return "continue" }

type PrintStmt struct {
	Sp       Span
	Args     []Node
	Newline  bool
	ToStderr bool
}

func (n *PrintStmt) nodeSpan() Span  { return n.Sp }
func (n *PrintStmt) nodeTag() string { return "print" }

type ExitStmt struct {
	Sp   Span
	Code Node
}

func (n *ExitStmt) nodeSpan() Span  { return n.Sp }
func (n *ExitStmt) nodeTag() string { return "exit" }

type DeferStmt struct {
	Sp   Span
	Call Node
}

func (n *DeferStmt) nodeSpan() Span  { return n.Sp }
func (n *DeferStmt) nodeTag() string { return "defer" }

type AssertStmt struct {
	Sp   Span
	Cond Node
	Msg  Node
}

func (n *AssertStmt) nodeSpan() Span  { return n.Sp }
func (n *AssertStmt) nodeTag() string { return "assert" }

type SpawnStmt struct {
	Sp   Span
	Call Node
}

func (n *SpawnStmt) nodeSpan() Span  { return n.Sp }
func (n *SpawnStmt) nodeTag() string { return "spawn" }

// ── Expressions ───────────────────────────────────────────────────────────────

type IntLit struct {
	Sp  Span
	Val int64
}
type FloatLit struct {
	Sp  Span
	Val float64
}
type BoolLit struct {
	Sp  Span
	Val bool
}
type StrLit struct {
	Sp  Span
	Val string
}
type NilLit struct{ Sp Span }

func (n *IntLit) nodeSpan() Span    { return n.Sp }
func (n *IntLit) nodeTag() string   { return "int" }
func (n *FloatLit) nodeSpan() Span  { return n.Sp }
func (n *FloatLit) nodeTag() string { return "float" }
func (n *BoolLit) nodeSpan() Span   { return n.Sp }
func (n *BoolLit) nodeTag() string  { return "bool" }
func (n *StrLit) nodeSpan() Span    { return n.Sp }
func (n *StrLit) nodeTag() string   { return "str" }
func (n *NilLit) nodeSpan() Span    { return n.Sp }
func (n *NilLit) nodeTag() string   { return "nil" }

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
	Sp      Span
	Obj     Node
	Field   string
	UsedDot bool
	Typ     *ZXType
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
	Sp        Span
	Name      string
	Fields    []FieldInit
	HeapAlloc bool
	Typ       *ZXType
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

type MethodCallExpr struct {
	Sp     Span
	Recv   Node
	Method string
	Args   []Node
	Typ    *ZXType
}

func (n *MethodCallExpr) nodeSpan() Span  { return n.Sp }
func (n *MethodCallExpr) nodeTag() string { return "methodcall" }

// ModCallExpr — modName->fn(args) or modName::fn(args)
// Enforces namespace: only callable via the mod name, not by plain fn name.
type ModCallExpr struct {
	Sp   Span
	Mod  string // mod block name
	Fn   string // function name within mod
	Args []Node
	Typ  *ZXType
}

func (n *ModCallExpr) nodeSpan() Span  { return n.Sp }
func (n *ModCallExpr) nodeTag() string { return "modcall" }

type BuiltinExpr struct {
	Sp   Span
	Name string
	Args []Node
	Typ  *ZXType
}

func (n *BuiltinExpr) nodeSpan() Span  { return n.Sp }
func (n *BuiltinExpr) nodeTag() string { return "builtin" }

type PipeExpr struct {
	Sp    Span
	Steps []Node
	Typ   *ZXType
}

func (n *PipeExpr) nodeSpan() Span  { return n.Sp }
func (n *PipeExpr) nodeTag() string { return "pipe" }

type TemplateStr struct {
	Sp    Span
	Parts []TplPart
	Typ   *ZXType
}

type TplPart struct {
	IsExpr bool
	Text   string
	Expr   Node
}

func (n *TemplateStr) nodeSpan() Span  { return n.Sp }
func (n *TemplateStr) nodeTag() string { return "tplstr" }

// MultilineStr — @`...` multiline strings with ${expr} and ${stmts} interpolation.
// Text parts are literal. IsExpr parts are value-producing expressions.
// IsStmt parts are statement blocks whose say/print output is captured.
type MultilineStr struct {
	Sp    Span
	Parts []MlsPart
	Typ   *ZXType
}

type MlsPart struct {
	Text   string // literal text
	IsExpr bool   // ${expr} — interpolate expression value
	Expr   Node
	IsStmt bool   // ${stmts} — run statements, capture their stdout
	Stmts  []Node
}

func (n *MultilineStr) nodeSpan() Span  { return n.Sp }
func (n *MultilineStr) nodeTag() string { return "multilinestr" }

type CmdExpr struct {
	Sp            Span
	Command       Node
	CaptureOutput bool
	Typ           *ZXType
}

func (n *CmdExpr) nodeSpan() Span  { return n.Sp }
func (n *CmdExpr) nodeTag() string { return "cmd" }

type ReadFileExpr struct {
	Sp   Span
	Path Node
	Typ  *ZXType
}

func (n *ReadFileExpr) nodeSpan() Span  { return n.Sp }
func (n *ReadFileExpr) nodeTag() string { return "readfile" }

type TernaryExpr struct {
	Sp   Span
	Cond Node
	Then Node
	Else Node
	Typ  *ZXType
}

func (n *TernaryExpr) nodeSpan() Span  { return n.Sp }
func (n *TernaryExpr) nodeTag() string { return "ternary" }

type TypeofExpr struct {
	Sp  Span
	Arg Node
	Typ *ZXType
}

func (n *TypeofExpr) nodeSpan() Span  { return n.Sp }
func (n *TypeofExpr) nodeTag() string { return "typeof" }

type BangMacroExpr struct {
	Sp   Span
	Name string
	Args []Node
	Typ  *ZXType
}

func (n *BangMacroExpr) nodeSpan() Span  { return n.Sp }
func (n *BangMacroExpr) nodeTag() string { return "bangmacro" }

type WriteFileExpr struct {
	Sp      Span
	Path    Node
	Content Node
	Typ     *ZXType
}

func (n *WriteFileExpr) nodeSpan() Span  { return n.Sp }
func (n *WriteFileExpr) nodeTag() string { return "writefile" }
