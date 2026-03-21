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
	TyTuple // NEW: tuple types (int, str)
	TyFn    // NEW: first-class function types fn(int)->str
)

type ZXType struct {
	Kind    TypeKind
	Elem    *ZXType
	ArrSize int
	Name    string
	// NEW: for TyFn — first-class function types
	Params []*ZXType
	Ret    *ZXType
	// NEW: for TyTuple
	Elems []*ZXType
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
func FnType(params []*ZXType, ret *ZXType) *ZXType {
	return &ZXType{Kind: TyFn, Params: params, Ret: ret}
}
func TupleType(elems []*ZXType) *ZXType {
	return &ZXType{Kind: TyTuple, Elems: elems}
}

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
	case TyFn:
		return "fn(...)"
	case TyTuple:
		return "tuple"
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
	// NEW: fn type coercion — any fn type can coerce to any
	if from.Kind == TyFn || to.Kind == TyFn {
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
	// NEW annotations
	AnnDoc     = "doc"     // @doc="description" — inline documentation
	AnnCold    = "cold"    // @cold — hint: rarely called path
	AnnHot     = "hot"     // @hot — hint: frequently called path
	AnnAlias   = "alias"   // @alias=other_name — alternate name for function
	AnnVersion = "version" // @version=1.2 — version gate
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
//  ModBlock  (EXTENDED)
// ─────────────────────────────────────────────────────────────────────────────

type ModBlock struct {
	Sp   Span
	Name string
	Path string
	// NEW: optional doc comment attached at the mod declaration
	Doc string
	// NEW: visibility — "pub" (default) or "priv"
	Vis     string
	Mods    []*ModBlock
	Structs []*StructDecl
	Methods []*MethodDecl
	Fns     []*FnDecl
	Tests   []*TestDecl
	// NEW: module-level constants — emitted as C file-scope statics
	Consts []*VarDecl
	// NEW: module init function (runs before main, guaranteed once)
	Init *FnDecl
	// NEW: module re-exports from nested mods
	Reexports []string
}

func (n *ModBlock) nodeSpan() Span  { return n.Sp }
func (n *ModBlock) nodeTag() string { return "mod" }

// ─────────────────────────────────────────────────────────────────────────────
//  MacroDecl  (EXTENDED)
// ─────────────────────────────────────────────────────────────────────────────

type MacroDecl struct {
	Sp      Span
	Name    string
	Params  []Param
	RetType *ZXType
	Body    *Block
	Inputs  []string
	Outputs []string
	// NEW: doc comment
	Doc string
	// NEW: marks this macro as always-inline (no C function, always substituted)
	AlwaysInline bool
	// NEW: macro aliases
	Aliases []string
	// NEW: guard condition evaluated before the macro body runs
	Guard Node
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
	// NEW: optional else block for conditional chain steps
	ElseBody *Block
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
	GlobalVars []*VarDecl
	// NEW: module-level init calls collected from mod blocks
	ModInits []*ModBlock
}

// ── Import ────────────────────────────────────────────────────────────────────

type ImportDecl struct {
	Sp        Span
	Path      string
	IsCHeader bool

	Module      string
	IsStdModule bool

	IsFileImport bool
	EnvPrefix    string
	UpsCount     int
	Segments     []string
	ResolvedFile string
	Alias        string
	ImportAll    bool

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
	// NEW: optional doc comment
	Doc string
	// NEW: derived struct — inherits fields from base (emitted as embedded struct)
	Base string
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
	// NEW: optional doc comment parsed from leading ## comment
	Doc string
	// NEW: explicit C name override (for @export="c_name")
	CName string
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

type VarDecl struct {
	Sp           Span
	Name         string
	VarType      *ZXType
	Init         Node
	IsConst      bool
	IsGlobal     bool
	ResolvedType *ZXType
	// NEW: module-scope const (static within a mod block)
	IsModConst bool
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
	// NEW: optional "by" variable to iterate in reverse
	Reverse bool
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
	// NEW: multiple patterns per arm: 1 | 2 | 3 => { }
	Patterns []Node
	IsWild   bool
	Guard    Node
	Body     *Block
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

func (n *BreakStmt) nodeSpan() Span     { return n.Sp }
func (n *BreakStmt) nodeTag() string    { return "break" }
func (n *ContinueStmt) nodeSpan() Span  { return n.Sp }
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

// NEW: repeat N times syntactic sugar — repeat 5 { }
type RepeatStmt struct {
	Sp    Span
	Count Node
	Body  *Block
}

func (n *RepeatStmt) nodeSpan() Span  { return n.Sp }
func (n *RepeatStmt) nodeTag() string { return "repeat" }

// NEW: with statement — scoped alias for a long expression
// with expensive_expr() as x { use x }
type WithStmt struct {
	Sp   Span
	Expr Node
	As   string
	Body *Block
}

func (n *WithStmt) nodeSpan() Span  { return n.Sp }
func (n *WithStmt) nodeTag() string { return "with" }

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
type ModCallExpr struct {
	Sp   Span
	Mod  string
	Fn   string
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

type MultilineStr struct {
	Sp    Span
	Parts []MlsPart
	Typ   *ZXType
}

type MlsPart struct {
	Text   string
	IsExpr bool
	Expr   Node
	IsStmt bool
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

// NEW: LambdaExpr — anonymous inline function: |x int, y int| -> int { return x + y; }
type LambdaExpr struct {
	Sp      Span
	Params  []Param
	RetType *ZXType
	Body    *Block
	Typ     *ZXType
}

func (n *LambdaExpr) nodeSpan() Span  { return n.Sp }
func (n *LambdaExpr) nodeTag() string { return "lambda" }

// NEW: RangeExpr — represents a..b as a value (for future iteration / slice use)
type RangeExpr struct {
	Sp   Span
	From Node
	To   Node
	Typ  *ZXType
}

func (n *RangeExpr) nodeSpan() Span  { return n.Sp }
func (n *RangeExpr) nodeTag() string { return "range" }

// NEW: MacroApplyExpr — apply!(macro_name, value) — apply a named macro as a value
// Allows storing macro application in variables.
type MacroApplyExpr struct {
	Sp    Span
	Macro string
	Value Node
	Args  []Node
	Typ   *ZXType
}

func (n *MacroApplyExpr) nodeSpan() Span  { return n.Sp }
func (n *MacroApplyExpr) nodeTag() string { return "macroapply" }
