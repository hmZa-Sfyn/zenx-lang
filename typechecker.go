package main

import (
	"fmt"
	"os"
	"strings"
)

func modPathToC(path string) string {
	return strings.ReplaceAll(path, "::", "_")
}

// ─────────────────────────────────────────────────────────────────────────────
//  Scope
// ─────────────────────────────────────────────────────────────────────────────

type VarInfo struct {
	Type      *ZXType
	IsConst   bool
	IsFn      bool
	IsMethod  bool
	IsExtern  bool
	IsStd     bool
	IsGlobal  bool
	IsModFn   bool
	ModName   string
	Sp        Span
	UsedCount int
	Defined   bool
	DeclFile  string
	Vis       Vis
}

type Scope struct {
	vars   map[string]*VarInfo
	parent *Scope
	kind   string // "global" "fn" "block" "loop" "try"
}

func newScope(parent *Scope, kind string) *Scope {
	return &Scope{vars: make(map[string]*VarInfo), parent: parent, kind: kind}
}
func (s *Scope) define(name string, vi *VarInfo) { s.vars[name] = vi }
func (s *Scope) lookupLocal(n string) *VarInfo   { return s.vars[n] }
func (s *Scope) lookup(name string) *VarInfo {
	if vi, ok := s.vars[name]; ok {
		return vi
	}
	if s.parent != nil {
		return s.parent.lookup(name)
	}
	return nil
}
func (s *Scope) inLoop() bool {
	if s == nil {
		return false
	}
	if s.kind == "loop" {
		return true
	}
	return s.parent.inLoop()
}
func (s *Scope) inFn() bool {
	if s == nil {
		return false
	}
	if s.kind == "fn" {
		return true
	}
	return s.parent.inFn()
}
func (s *Scope) inTry() bool {
	if s == nil {
		return false
	}
	if s.kind == "try" {
		return true
	}
	return s.parent.inTry()
}

// ─────────────────────────────────────────────────────────────────────────────
//  TypeChecker
// ─────────────────────────────────────────────────────────────────────────────

type TypeChecker struct {
	prog        *Program
	scope       *Scope
	fnStack     []*FnDecl
	methodStack []*MethodDecl
	macroStack  []*MacroDecl
	structs     map[string]*StructDecl
	fns         map[string]*FnDecl
	methods     map[string]*MethodDecl
	macros      map[string]*MacroDecl
	externs     map[string]*ExternDecl
	stdFns      map[string]*StdFn
	importPaths map[string]bool
	importMods  map[string]bool
	ok          bool

	modFns   map[string]*FnDecl
	modNames map[string]bool
	// mod properties: key = "ModPath::propName"
	modProps map[string]*ModProperty

	returnSeen  bool
	currentFile string
	trace       TraceBuilder

	importedProgs map[string]*Program

	privDecls   map[string]bool
	privFns     map[string]string
	privStructs map[string]string
	privMacros  map[string]string

	callChain    map[string]bool
	recursiveFns map[string]bool
}

func TypeCheck(prog *Program, src, file string) bool {
	tc := &TypeChecker{
		prog:          prog,
		structs:       make(map[string]*StructDecl),
		fns:           make(map[string]*FnDecl),
		methods:       make(map[string]*MethodDecl),
		macros:        make(map[string]*MacroDecl),
		externs:       make(map[string]*ExternDecl),
		stdFns:        make(map[string]*StdFn),
		importPaths:   make(map[string]bool),
		importMods:    make(map[string]bool),
		modFns:        make(map[string]*FnDecl),
		modNames:      make(map[string]bool),
		modProps:      make(map[string]*ModProperty),
		importedProgs: make(map[string]*Program),
		privDecls:     make(map[string]bool),
		privFns:       make(map[string]string),
		privStructs:   make(map[string]string),
		privMacros:    make(map[string]string),
		callChain:     make(map[string]bool),
		recursiveFns:  make(map[string]bool),
		ok:            true,
		currentFile:   file,
	}
	tc.scope = newScope(nil, "global")

	for _, imp := range prog.Imports {
		tc.validateImport(imp)
		if imp.IsStdModule {
			tc.importMods[imp.Module] = true
		} else if imp.IsCHeader {
			tc.importPaths[imp.Path] = true
		} else if imp.IsFileImport && imp.IsLocal {
			tc.resolveFileImport(imp, prog)
		}
	}
	tc.stdFns = prog.AllStdFns()

	// ── struct registration ──────────────────────────────────────────────────
	for _, s := range prog.Structs {
		if existing, exists := tc.structs[s.Name]; exists {
			errCodeSecondary("E01", s.Sp,
				fmt.Sprintf("struct %q is defined more than once", s.Name),
				"rename one of these struct definitions",
				[]SecondarySpan{{Span: existing.Sp, Label: "first defined here"}})
			tc.ok = false
		}
		tc.structs[s.Name] = s
		seen := map[string]Span{}
		for _, f := range s.Fields {
			if prev, dup := seen[f.Name]; dup {
				errCodeSecondary("E02", f.Sp,
					fmt.Sprintf("duplicate field %q in struct %q", f.Name, s.Name),
					"remove the duplicate field or rename it",
					[]SecondarySpan{{Span: prev, Label: "first declared here"}})
				tc.ok = false
			}
			seen[f.Name] = f.Sp
			if f.Type != nil && f.Type.Kind == TyStruct {
				tc.validateTypeExists(f.Type, f.Sp)
			}
		}
		if s.Vis == VisPrivate {
			tc.privDecls[file+"::"+s.Name] = true
		}
	}

	// ── externs + std fns ────────────────────────────────────────────────────
	for _, e := range prog.Externs {
		tc.externs[e.Name] = e
		tc.scope.define(e.Name, &VarInfo{
			Type: e.RetType, IsFn: true, IsExtern: true,
			Sp: e.Sp, DeclFile: file,
		})
	}
	for name, fn := range tc.stdFns {
		f := fn
		tc.scope.define(name, &VarInfo{Type: f.Ret, IsFn: true, IsStd: true, DeclFile: "<stdlib>"})
	}

	// ── top-level function registration ──────────────────────────────────────
	for _, stmt := range prog.TopStmts {
		if fn, ok := stmt.(*FnDecl); ok {
			if existing, exists := tc.fns[fn.Name]; exists {
				errCodeSecondary("E03", fn.Sp,
					fmt.Sprintf("function %q is defined more than once", fn.Name),
					"rename one of these function definitions",
					[]SecondarySpan{{Span: existing.Sp, Label: "first defined here"}})
				tc.ok = false
			}
			tc.fns[fn.Name] = fn
			tc.scope.define(fn.Name, &VarInfo{
				Type: fn.RetType, IsFn: true, Sp: fn.Sp,
				DeclFile: file, Vis: fn.Vis,
			})
			if fn.Vis == VisPrivate {
				tc.privDecls[file+"::"+fn.Name] = true
			}
		}
		if vd, ok := stmt.(*VarDecl); ok && vd.IsGlobal {
			prog.GlobalVars = append(prog.GlobalVars, vd)
		}
	}

	// ── methods ──────────────────────────────────────────────────────────────
	for _, m := range prog.Methods {
		key := m.CName()
		if existing, exists := tc.methods[key]; exists {
			errCodeSecondary("E58", m.Sp,
				fmt.Sprintf("method %q on %q is defined more than once", m.Name, m.RecvType),
				"rename or remove one of the duplicate method definitions",
				[]SecondarySpan{{Span: existing.Sp, Label: "first defined here"}})
			tc.ok = false
		}
		tc.methods[key] = m
		tc.scope.define(key, &VarInfo{Type: m.RetType, IsFn: true, IsMethod: true, Sp: m.Sp, DeclFile: file})
		if _, ok := tc.structs[m.RecvType]; !ok {
			errCode("E57", m.Sp,
				fmt.Sprintf("method %q defined on unknown struct type %q", m.Name, m.RecvType),
				fmt.Sprintf("declare the struct first: type %s struct { ... }", m.RecvType))
			tc.ok = false
		}
	}

	// ── mod block registration ────────────────────────────────────────────────
	for _, mb := range prog.ModBlocks {
		tc.registerModFns(mb, file)
	}

	// ── macro registration ────────────────────────────────────────────────────
	for _, mc := range prog.Macros {
		if existing, exists := tc.macros[mc.Name]; exists {
			errCodeSecondary("EM03", mc.Sp,
				fmt.Sprintf("macro %q is defined more than once", mc.Name),
				"rename one of the macro definitions",
				[]SecondarySpan{{Span: existing.Sp, Label: "first defined here"}})
			tc.ok = false
		}
		if _, exists := tc.fns[mc.Name]; exists {
			errCode("EM04", mc.Sp,
				fmt.Sprintf("macro %q has the same name as a function — names must be unique", mc.Name),
				"rename the macro or the function")
			tc.ok = false
		}
		tc.macros[mc.Name] = mc
		tc.scope.define(mc.Name, &VarInfo{
			Type: mc.RetType, IsFn: true, Sp: mc.Sp,
			DeclFile: file, Vis: mc.Vis,
		})
		if mc.Vis == VisPrivate {
			tc.privDecls[file+"::"+mc.Name] = true
		}
	}

	// ── check bodies ─────────────────────────────────────────────────────────
	for _, stmt := range prog.TopStmts {
		if fn, ok := stmt.(*FnDecl); ok {
			tc.checkFn(fn)
		}
	}
	for _, m := range prog.Methods {
		tc.checkMethod(m)
	}
	for _, mc := range prog.Macros {
		tc.checkMacro(mc)
	}
	for _, mb := range prog.ModBlocks {
		tc.checkModFns(mb)
	}
	for _, stmt := range prog.TopStmts {
		if _, ok := stmt.(*FnDecl); !ok {
			tc.checkStmt(stmt)
		}
	}
	for _, vd := range prog.GlobalVars {
		tc.checkVarDecl(vd)
	}

	tc.checkRecursion()
	tc.checkDeadCode()
	tc.checkUnusedVars()

	return tc.ok
}

// ─────────────────────────────────────────────────────────────────────────────
//  Dead-code detection
// ─────────────────────────────────────────────────────────────────────────────

func (tc *TypeChecker) checkDeadCode() {
	called := make(map[string]bool)
	for _, fn := range tc.fns {
		tc.walkBody(fn.Body, called)
	}
	for _, m := range tc.methods {
		tc.walkBody(m.Body, called)
	}
	for _, mb := range tc.prog.ModBlocks {
		tc.walkModBlock(mb, called)
	}
	for _, stmt := range tc.prog.TopStmts {
		if _, isFn := stmt.(*FnDecl); !isFn {
			tc.walkNode(stmt, called)
		}
	}
	for name, fn := range tc.fns {
		if name == "main" {
			continue
		}
		if fn.HasAnnotation(AnnTest) || fn.HasAnnotation(AnnSetup) || fn.HasAnnotation(AnnTeardown) {
			continue
		}
		if fn.Vis == VisPublic {
			continue
		}
		if !called[name] {
			warnCode("W91", fn.Sp,
				fmt.Sprintf("private function %q is defined but never called", name),
				"remove it or make it public if it is meant to be used externally")
		}
	}
}

func (tc *TypeChecker) walkModBlock(mb *ModBlock, called map[string]bool) {
	if mb == nil {
		return
	}
	for _, fn := range mb.Fns {
		tc.walkBody(fn.Body, called)
	}
	for _, td := range mb.Tests {
		tc.walkBody(td.Fn.Body, called)
	}
	for _, nested := range mb.Mods {
		tc.walkModBlock(nested, called)
	}
}

func (tc *TypeChecker) walkBody(b *Block, called map[string]bool) {
	if b == nil {
		return
	}
	for _, s := range b.Stmts {
		tc.walkNode(s, called)
	}
}

func (tc *TypeChecker) walkNode(n Node, called map[string]bool) {
	if n == nil {
		return
	}
	switch s := n.(type) {
	case *FnDecl:
		tc.walkBody(s.Body, called)
	case *VarDecl:
		tc.walkNode(s.Init, called)
	case *AssignStmt:
		tc.walkNode(s.LHS, called)
		tc.walkNode(s.Value, called)
	case *ModPropSetStmt:
		tc.walkNode(s.Value, called)
	case *CallExpr:
		if id, ok := s.Func.(*Ident); ok && id != nil {
			called[id.Name] = true
		}
		tc.walkNode(s.Func, called)
		for _, a := range s.Args {
			tc.walkNode(a, called)
		}
	case *MethodCallExpr:
		tc.walkNode(s.Recv, called)
		for _, a := range s.Args {
			tc.walkNode(a, called)
		}
	case *ModCallExpr:
		for _, a := range s.Args {
			tc.walkNode(a, called)
		}
	case *MacroCallExpr:
		for _, a := range s.Args {
			tc.walkNode(a, called)
		}
	case *BangMacroExpr:
		for _, a := range s.Args {
			tc.walkNode(a, called)
		}
	case *MacroCallChain:
		tc.walkNode(s.Recv, called)
		for _, step := range s.Steps {
			for _, a := range step.Args {
				tc.walkNode(a, called)
			}
			tc.walkBody(step.Body, called)
			tc.walkBody(step.ElseBody, called)
		}
	case *MacroApplyExpr:
		tc.walkNode(s.Value, called)
		for _, a := range s.Args {
			tc.walkNode(a, called)
		}
	case *Block:
		tc.walkBody(s, called)
	case *ExprStmt:
		tc.walkNode(s.Expr, called)
	case *ReturnStmt:
		tc.walkNode(s.Value, called)
	case *IfStmt:
		tc.walkNode(s.Cond, called)
		tc.walkBody(s.Then, called)
		for _, el := range s.Elifs {
			tc.walkNode(el.Cond, called)
			tc.walkBody(el.Body, called)
		}
		tc.walkBody(s.Else, called)
	case *UnlessStmt:
		tc.walkNode(s.Cond, called)
		tc.walkBody(s.Body, called)
		tc.walkBody(s.Else, called)
	case *WhileStmt:
		tc.walkNode(s.Cond, called)
		tc.walkBody(s.Body, called)
	case *UntilStmt:
		tc.walkNode(s.Cond, called)
		tc.walkBody(s.Body, called)
	case *ForRangeStmt:
		tc.walkNode(s.From, called)
		tc.walkNode(s.To, called)
		tc.walkNode(s.Step, called)
		tc.walkBody(s.Body, called)
	case *RepeatStmt:
		tc.walkNode(s.Count, called)
		tc.walkBody(s.Body, called)
	case *WithStmt:
		tc.walkNode(s.Expr, called)
		tc.walkBody(s.Body, called)
	case *MatchStmt:
		tc.walkNode(s.Expr, called)
		for _, arm := range s.Arms {
			tc.walkNode(arm.Pattern, called)
			for _, p := range arm.Patterns {
				tc.walkNode(p, called)
			}
			tc.walkNode(arm.Guard, called)
			tc.walkBody(arm.Body, called)
		}
	case *TryCatchStmt:
		tc.walkBody(s.Try, called)
		tc.walkBody(s.Catch, called)
		tc.walkBody(s.Finally, called)
	case *DeferStmt:
		tc.walkNode(s.Call, called)
	case *AssertStmt:
		tc.walkNode(s.Cond, called)
		tc.walkNode(s.Msg, called)
	case *PrintStmt:
		for _, a := range s.Args {
			tc.walkNode(a, called)
		}
	case *ExitStmt:
		tc.walkNode(s.Code, called)
	case *SpawnStmt:
		tc.walkNode(s.Call, called)
	case *BinExpr:
		tc.walkNode(s.LHS, called)
		tc.walkNode(s.RHS, called)
	case *UnaryExpr:
		tc.walkNode(s.Operand, called)
	case *TernaryExpr:
		tc.walkNode(s.Cond, called)
		tc.walkNode(s.Then, called)
		tc.walkNode(s.Else, called)
	case *FieldExpr:
		tc.walkNode(s.Obj, called)
	case *IndexExpr:
		tc.walkNode(s.Obj, called)
		tc.walkNode(s.Idx, called)
	case *AddrExpr:
		tc.walkNode(s.Operand, called)
	case *CastExpr:
		tc.walkNode(s.Operand, called)
	case *PipeExpr:
		for _, step := range s.Steps {
			tc.walkNode(step, called)
		}
	case *StructInit:
		for _, f := range s.Fields {
			tc.walkNode(f.Value, called)
		}
	case *ArrayLit:
		for _, el := range s.Elems {
			tc.walkNode(el, called)
		}
	case *LambdaExpr:
		tc.walkBody(s.Body, called)
	case *TemplateStr:
		for _, part := range s.Parts {
			if part.IsExpr {
				tc.walkNode(part.Expr, called)
			}
		}
	case *MultilineStr:
		for _, part := range s.Parts {
			if part.IsExpr {
				tc.walkNode(part.Expr, called)
			}
			for _, st := range part.Stmts {
				tc.walkNode(st, called)
			}
		}
	case *CmdExpr:
		tc.walkNode(s.Command, called)
	case *ReadFileExpr:
		tc.walkNode(s.Path, called)
	case *WriteFileExpr:
		tc.walkNode(s.Path, called)
		tc.walkNode(s.Content, called)
	case *Ident, *IntLit, *FloatLit, *BoolLit, *StrLit, *NilLit,
		*SizeofExpr, *TypeofExpr, *BreakStmt, *ContinueStmt, *ModPropGetExpr:
	}
}

// ─────────────────────────────────────────────────────────────────────────────
//  Recursion analysis
// ─────────────────────────────────────────────────────────────────────────────

func (tc *TypeChecker) checkRecursion() {
	for name, fn := range tc.fns {
		if fn.Body == nil {
			continue
		}
		if tc.bodyCallsSelf(fn.Body.Stmts, name) {
			tc.recursiveFns[name] = true
			if !tc.bodyHasGuardedReturn(fn.Body.Stmts) {
				warnCode("W92", fn.Sp,
					fmt.Sprintf("function %q recurses without an obvious base case — possible infinite recursion", name),
					"add a base-case return before the recursive call")
			}
		}
	}
}

func (tc *TypeChecker) bodyCallsSelf(stmts []Node, fnName string) bool {
	for _, n := range stmts {
		if tc.nodeCallsSelf(n, fnName) {
			return true
		}
	}
	return false
}

func (tc *TypeChecker) nodeCallsSelf(n Node, fnName string) bool {
	if n == nil {
		return false
	}
	switch s := n.(type) {
	case *CallExpr:
		if id, ok := s.Func.(*Ident); ok && id.Name == fnName {
			return true
		}
		for _, a := range s.Args {
			if tc.nodeCallsSelf(a, fnName) {
				return true
			}
		}
	case *ExprStmt:
		return tc.nodeCallsSelf(s.Expr, fnName)
	case *ReturnStmt:
		return tc.nodeCallsSelf(s.Value, fnName)
	case *VarDecl:
		return tc.nodeCallsSelf(s.Init, fnName)
	case *AssignStmt:
		return tc.nodeCallsSelf(s.Value, fnName)
	case *IfStmt:
		for _, st := range s.Then.Stmts {
			if tc.nodeCallsSelf(st, fnName) {
				return true
			}
		}
		if s.Else != nil {
			for _, st := range s.Else.Stmts {
				if tc.nodeCallsSelf(st, fnName) {
					return true
				}
			}
		}
	case *WhileStmt:
		for _, st := range s.Body.Stmts {
			if tc.nodeCallsSelf(st, fnName) {
				return true
			}
		}
	case *Block:
		return tc.bodyCallsSelf(s.Stmts, fnName)
	case *BinExpr:
		return tc.nodeCallsSelf(s.LHS, fnName) || tc.nodeCallsSelf(s.RHS, fnName)
	}
	return false
}

func (tc *TypeChecker) bodyHasGuardedReturn(stmts []Node) bool {
	for _, n := range stmts {
		if is, ok := n.(*IfStmt); ok {
			for _, st := range is.Then.Stmts {
				if _, ok := st.(*ReturnStmt); ok {
					return true
				}
			}
		}
	}
	return false
}

// ─────────────────────────────────────────────────────────────────────────────
//  Unused-variable lint
// ─────────────────────────────────────────────────────────────────────────────

func (tc *TypeChecker) checkUnusedVars() {
	for name, vi := range tc.scope.vars {
		if vi.IsFn || vi.IsExtern || vi.IsStd || vi.IsGlobal {
			continue
		}
		if vi.UsedCount == 0 && vi.Defined {
			warnCode("W30", vi.Sp,
				fmt.Sprintf("variable %q is declared but never used", name),
				"remove it, or prefix with _ to suppress: _"+name)
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
//  Fn/Method/Macro stack helpers
// ─────────────────────────────────────────────────────────────────────────────

func (tc *TypeChecker) currentFn() *FnDecl {
	if len(tc.fnStack) == 0 {
		return nil
	}
	return tc.fnStack[len(tc.fnStack)-1]
}
func (tc *TypeChecker) currentMethod() *MethodDecl {
	if len(tc.methodStack) == 0 {
		return nil
	}
	return tc.methodStack[len(tc.methodStack)-1]
}
func (tc *TypeChecker) currentMacro() *MacroDecl {
	if len(tc.macroStack) == 0 {
		return nil
	}
	return tc.macroStack[len(tc.macroStack)-1]
}
func (tc *TypeChecker) currentRetType() *ZXType {
	if m := tc.currentMethod(); m != nil {
		return m.RetType
	}
	if f := tc.currentFn(); f != nil {
		return f.RetType
	}
	if mc := tc.currentMacro(); mc != nil {
		return mc.RetType
	}
	return nil
}
func (tc *TypeChecker) currentName() string {
	if m := tc.currentMethod(); m != nil {
		return m.RecvType + "." + m.Name
	}
	if f := tc.currentFn(); f != nil {
		return f.Name
	}
	if mc := tc.currentMacro(); mc != nil {
		return "macro:" + mc.Name
	}
	return "<top-level>"
}

// ─────────────────────────────────────────────────────────────────────────────
//  Macro checking
// ─────────────────────────────────────────────────────────────────────────────

func (tc *TypeChecker) checkMacro(mc *MacroDecl) {
	if mc == nil {
		return
	}
	saved := tc.scope
	tc.scope = newScope(saved, "fn")
	tc.macroStack = append(tc.macroStack, mc)
	tc.trace.Push(mc.Sp, fmt.Sprintf("in macro '%s'", mc.Name))

	seen := map[string]Span{}
	for _, p := range mc.Params {
		if prev, dup := seen[p.Name]; dup {
			errCodeSecondary("EM05", p.Sp,
				fmt.Sprintf("duplicate parameter %q in macro %q", p.Name, mc.Name),
				"rename one of the duplicate parameters",
				[]SecondarySpan{{Span: prev, Label: "first declared here"}})
			tc.ok = false
		}
		seen[p.Name] = p.Sp
		t := p.Type
		if t == nil {
			t = TypAny
		}
		isCallable := isBlockParam(p.Name, t)
		tc.scope.define(p.Name, &VarInfo{Type: t, IsFn: isCallable, Sp: p.Sp, DeclFile: tc.currentFile})
		if isCallable {
			syntheticFn := &FnDecl{
				Sp: p.Sp, Name: p.Name,
				Params: []Param{}, RetType: TypAny, Body: &Block{},
			}
			tc.fns[p.Name] = syntheticFn
		}
	}
	for _, out := range mc.Outputs {
		tc.scope.define(out, &VarInfo{Type: TypAny, Sp: mc.Sp, DeclFile: tc.currentFile})
	}
	tc.validateTypeExists(mc.RetType, mc.Sp)
	tc.checkBlock(mc.Body)

	for _, p := range mc.Params {
		t := p.Type
		if t == nil {
			t = TypAny
		}
		if isBlockParam(p.Name, t) {
			delete(tc.fns, p.Name)
		}
	}

	tc.trace.Pop()
	tc.macroStack = tc.macroStack[:len(tc.macroStack)-1]
	tc.scope = saved
}

// ─────────────────────────────────────────────────────────────────────────────
//  Function checking
// ─────────────────────────────────────────────────────────────────────────────

func (tc *TypeChecker) checkFn(fn *FnDecl) {
	saved := tc.scope
	tc.scope = newScope(saved, "fn")
	tc.fnStack = append(tc.fnStack, fn)
	tc.trace.Push(fn.Sp, fmt.Sprintf("in function '%s'", fn.Name))

	if fn.Body != nil && len(fn.Body.Stmts) == 0 && fn.RetType != nil && fn.RetType.Kind != TyVoid {
		warnCode("W93", fn.Sp,
			fmt.Sprintf("function %q has an empty body but declares return type %s", fn.Name, fn.RetType),
			"add a return statement or change return type to void")
	}

	seen := map[string]Span{}
	for _, p2 := range fn.Params {
		if prev, dup := seen[p2.Name]; dup {
			errCodeSecondary("E04", p2.Sp,
				fmt.Sprintf("duplicate parameter %q in function %q", p2.Name, fn.Name),
				"rename one of the duplicate parameters",
				[]SecondarySpan{{Span: prev, Label: "first declared here"}})
			tc.ok = false
		}
		seen[p2.Name] = p2.Sp
		t := p2.Type
		if t == nil {
			t = TypAny
		}
		if t.Kind == TyVoid {
			errCode("E94", p2.Sp,
				fmt.Sprintf("parameter %q in function %q has type void — void cannot be a parameter type", p2.Name, fn.Name),
				"use any, int, str, or a struct type instead")
			tc.ok = false
		}
		tc.scope.define(p2.Name, &VarInfo{
			Type: t, Sp: p2.Sp, Defined: true, DeclFile: tc.currentFile,
		})
	}

	for _, vd := range tc.prog.GlobalVars {
		if vd.ResolvedType != nil {
			tc.scope.define(vd.Name, &VarInfo{Type: vd.ResolvedType, IsGlobal: true, Sp: vd.Sp})
		} else if vd.VarType != nil {
			tc.scope.define(vd.Name, &VarInfo{Type: vd.VarType, IsGlobal: true, Sp: vd.Sp})
		} else {
			tc.scope.define(vd.Name, &VarInfo{Type: TypAny, IsGlobal: true, Sp: vd.Sp})
		}
	}

	tc.validateTypeExists(fn.RetType, fn.Sp)

	savedReturnSeen := tc.returnSeen
	tc.returnSeen = false
	tc.checkBlock(fn.Body)
	if fn.RetType != nil && fn.RetType.Kind != TyVoid && fn.RetType.Kind != TyAny {
		if !tc.returnSeen && fn.Name != "main" {
			warnCode("W40", fn.Sp,
				fmt.Sprintf("function %q declares return type %s but may not return a value on all paths",
					fn.Name, fn.RetType),
				"add a return statement, or change the return type to void")
		}
	}
	tc.returnSeen = savedReturnSeen

	tc.trace.Pop()
	tc.fnStack = tc.fnStack[:len(tc.fnStack)-1]
	tc.scope = saved
}

func (tc *TypeChecker) checkMethod(m *MethodDecl) {
	saved := tc.scope
	tc.scope = newScope(saved, "fn")
	tc.methodStack = append(tc.methodStack, m)
	tc.trace.Push(m.Sp, fmt.Sprintf("in method '%s.%s'", m.RecvType, m.Name))

	recvType := StructType(m.RecvType)
	if m.RecvRef {
		recvType = RefOf(StructType(m.RecvType))
	}
	tc.scope.define(m.RecvName, &VarInfo{
		Type: recvType, Sp: m.Sp, Defined: true, DeclFile: tc.currentFile,
	})

	seen := map[string]Span{}
	for _, p2 := range m.Params {
		if prev, dup := seen[p2.Name]; dup {
			errCodeSecondary("E04", p2.Sp,
				fmt.Sprintf("duplicate parameter %q in method %q", p2.Name, m.Name),
				"rename one of the duplicate parameters",
				[]SecondarySpan{{Span: prev, Label: "first declared here"}})
			tc.ok = false
		}
		seen[p2.Name] = p2.Sp
		t := p2.Type
		if t == nil {
			t = TypAny
		}
		tc.scope.define(p2.Name, &VarInfo{Type: t, Sp: p2.Sp, Defined: true, DeclFile: tc.currentFile})
	}

	for _, vd := range tc.prog.GlobalVars {
		typ := vd.ResolvedType
		if typ == nil {
			typ = vd.VarType
		}
		if typ == nil {
			typ = TypAny
		}
		tc.scope.define(vd.Name, &VarInfo{Type: typ, IsGlobal: true, Sp: vd.Sp})
	}

	savedReturnSeen := tc.returnSeen
	tc.returnSeen = false
	tc.checkBlock(m.Body)
	if m.RetType != nil && m.RetType.Kind != TyVoid && m.RetType.Kind != TyAny {
		if !tc.returnSeen {
			warnCode("W40", m.Sp,
				fmt.Sprintf("method %q.%s declares return type %s but may not return a value on all paths",
					m.RecvType, m.Name, m.RetType),
				"add a return statement, or change the return type to void")
		}
	}
	tc.returnSeen = savedReturnSeen

	tc.trace.Pop()
	tc.methodStack = tc.methodStack[:len(tc.methodStack)-1]
	tc.scope = saved
}

func (tc *TypeChecker) checkBlock(b *Block) {
	if b == nil {
		return
	}
	saved := tc.scope
	tc.scope = newScope(saved, "block")
	for _, s := range b.Stmts {
		tc.checkStmt(s)
	}
	tc.scope = saved
}

func (tc *TypeChecker) checkBlockInLoop(b *Block) {
	if b == nil {
		return
	}
	saved := tc.scope
	tc.scope = newScope(saved, "loop")
	for _, s := range b.Stmts {
		tc.checkStmt(s)
	}
	tc.scope = saved
}

func (tc *TypeChecker) checkBlockInTry(b *Block) {
	if b == nil {
		return
	}
	saved := tc.scope
	tc.scope = newScope(saved, "try")
	for _, s := range b.Stmts {
		tc.checkStmt(s)
	}
	tc.scope = saved
}

// ─────────────────────────────────────────────────────────────────────────────
//  Statement checking
// ─────────────────────────────────────────────────────────────────────────────

func (tc *TypeChecker) checkStmt(n Node) {
	if n == nil {
		return
	}
	switch s := n.(type) {
	case *VarDecl:
		tc.checkVarDecl(s)
	case *ReturnStmt:
		tc.checkReturn(s)
	case *IfStmt:
		tc.checkIf(s)
	case *UnlessStmt:
		tc.inferExpr(s.Cond)
		tc.checkBlock(s.Body)
		if s.Else != nil {
			tc.checkBlock(s.Else)
		}
	case *WhileStmt:
		tc.inferExpr(s.Cond)
		if lit, ok := s.Cond.(*BoolLit); ok && !lit.Val {
			warnCode("W95", s.Sp,
				"while(false) loop will never execute",
				"remove the loop or fix the condition")
		}
		tc.checkBlockInLoop(s.Body)
	case *UntilStmt:
		tc.inferExpr(s.Cond)
		tc.checkBlockInLoop(s.Body)
	case *ForRangeStmt:
		tc.checkForRange(s)
	case *MatchStmt:
		tc.checkMatch(s)
	case *TryCatchStmt:
		tc.checkTryCatch(s)
	case *AssignStmt:
		tc.checkAssign(s)
	case *ModPropSetStmt:
		tc.checkModPropSet(s)
	case *DeferStmt:
		if !tc.scope.inFn() {
			errCodeTrace("E60", s.Sp,
				"'defer' used outside a function",
				"move this defer inside a fn block",
				tc.trace.Snapshot())
			tc.ok = false
		}
		tc.inferExpr(s.Call)
	case *AssertStmt:
		cond := tc.inferExpr(s.Cond)
		if cond.Kind == TyVoid {
			errCodeTrace("E71", s.Sp,
				"assert condition has type void",
				"use a boolean expression or a comparison",
				tc.trace.Snapshot())
			tc.ok = false
		}
		tc.inferExpr(s.Msg)
	case *SpawnStmt:
		tc.inferExpr(s.Call)
	case *ExprStmt:
		t := tc.inferExpr(s.Expr)
		// Only warn on plain function calls, not method/mod calls — those are often fire-and-forget
		if _, ok := s.Expr.(*CallExpr); ok {
			if t != nil && t.Kind != TyVoid && t.Kind != TyAny {
				warnCode("W60", s.Sp,
					fmt.Sprintf("return value of type %s is discarded", t),
					"assign to _ if intentional: _ = expr")
			}
		}
	case *PrintStmt:
		for _, a := range s.Args {
			tc.inferExpr(a)
		}
	case *ExitStmt:
		code := tc.inferExpr(s.Code)
		if code != nil && code.Kind != TyInt && code.Kind != TyAny && code.Kind != TyUnknown {
			errCodeTrace("E72", s.Sp,
				fmt.Sprintf("exit code must be an integer, got %s", code),
				"use an integer expression: exit 0  or  exit 1",
				tc.trace.Snapshot())
			tc.ok = false
		}
	case *BreakStmt:
		if !tc.scope.inLoop() {
			errCodeTrace("E07", s.Sp,
				"'break' used outside a loop",
				"break can only appear inside while / for / until loops",
				tc.trace.Snapshot())
			tc.ok = false
		}
	case *ContinueStmt:
		if !tc.scope.inLoop() {
			errCodeTrace("E08", s.Sp,
				"'continue' used outside a loop",
				"continue can only appear inside while / for / until loops",
				tc.trace.Snapshot())
			tc.ok = false
		}
	case *FnDecl:
		tc.checkFn(s)
	case *Block:
		tc.checkBlock(s)
	case *PipeExpr:
		tc.inferExpr(s)
	case *RepeatStmt:
		tc.inferExpr(s.Count)
		if lit, ok := s.Count.(*IntLit); ok && lit.Val <= 0 {
			warnCode("W96", s.Sp,
				fmt.Sprintf("repeat count is %d — the block will never execute", lit.Val),
				"use a positive integer for the repeat count")
		}
		tc.checkBlockInLoop(s.Body)
	case *WithStmt:
		exprTyp := tc.inferExpr(s.Expr)
		saved := tc.scope
		tc.scope = newScope(saved, "block")
		tc.scope.define(s.As, &VarInfo{
			Type: exprTyp, Sp: s.Sp,
			Defined: true, DeclFile: tc.currentFile,
		})
		tc.checkBlock(s.Body)
		tc.scope = saved
	}
}

// ─────────────────────────────────────────────────────────────────────────────
//  Mod property set/get checking
// ─────────────────────────────────────────────────────────────────────────────

func (tc *TypeChecker) checkModPropSet(s *ModPropSetStmt) {
	key := s.Mod + "::" + s.Prop
	prop, ok := tc.modProps[key]
	if !ok {
		if !tc.modNames[s.Mod] {
			errCodeTrace("E82", s.Sp,
				fmt.Sprintf("unknown module %q", s.Mod),
				fmt.Sprintf("declare: mod %s { property %s int }", s.Mod, s.Prop),
				tc.trace.Snapshot())
		} else {
			// helpful: list available properties
			var avail []string
			prefix := s.Mod + "::"
			for k, p := range tc.modProps {
				if strings.HasPrefix(k, prefix) && p.HasSet {
					avail = append(avail, strings.TrimPrefix(k, prefix))
				}
			}
			hint := fmt.Sprintf("mod %q has no property %q", s.Mod, s.Prop)
			if len(avail) > 0 {
				hint += fmt.Sprintf(" — available settable properties: %s", strings.Join(avail, ", "))
			}
			errCodeTrace("EP10", s.Sp, hint,
				fmt.Sprintf("add to mod %s: property %s int", s.Mod, s.Prop),
				tc.trace.Snapshot())
		}
		tc.ok = false
		return
	}
	if !prop.HasSet {
		errCodeTrace("EP11", s.Sp,
			fmt.Sprintf("property %s::%s is read-only (no set block)", s.Mod, s.Prop),
			"add a set block: set(v) { __"+s.Prop+" = v; }",
			tc.trace.Snapshot())
		tc.ok = false
		return
	}
	valType := tc.inferExpr(s.Value)
	if prop.Type != nil && prop.Type.Kind != TyAny &&
		valType.Kind != TyAny && valType.Kind != TyUnknown &&
		!coercible(valType, prop.Type) {
		errFull("EP12", s.Sp,
			fmt.Sprintf("property %s::%s has type %s but assigned %s",
				s.Mod, s.Prop, prop.Type, valType),
			fmt.Sprintf("cast the value: %s(expr)", prop.Type),
			"",
			nil, tc.trace.Snapshot())
		tc.ok = false
	}
	// compound operators require numeric type
	if s.Op != "=" && prop.Type != nil && prop.Type.Kind != TyAny && !isNumeric(prop.Type) {
		errCodeTrace("E26", s.Sp,
			fmt.Sprintf("compound operator %s requires a numeric property, got %s", s.Op, prop.Type),
			"compound assignment (+=, -=, *=) only works on int and float properties",
			tc.trace.Snapshot())
		tc.ok = false
	}
}

func (tc *TypeChecker) inferModPropGet(e *ModPropGetExpr) *ZXType {
	if !tc.modNames[e.Mod] {
		errCodeTrace("E82", e.Sp,
			fmt.Sprintf("unknown module %q", e.Mod),
			fmt.Sprintf("declare: mod %s { ... }", e.Mod),
			tc.trace.Snapshot())
		tc.ok = false
		e.Typ = TypUnknown
		return TypUnknown
	}
	key := e.Mod + "::" + e.Prop
	prop, ok := tc.modProps[key]
	if !ok {
		// check if it's a fn instead — give a targeted error
		fnKey := e.Mod + "::" + e.Prop
		if _, isFn := tc.modFns[fnKey]; isFn {
			errCodeTrace("E84", e.Sp,
				fmt.Sprintf("%s::%s is a function — call it with parentheses", e.Mod, e.Prop),
				fmt.Sprintf("use: %s->%s()", e.Mod, e.Prop),
				tc.trace.Snapshot())
		} else {
			var avail []string
			prefix := e.Mod + "::"
			for k, p := range tc.modProps {
				if strings.HasPrefix(k, prefix) && p.HasGet {
					avail = append(avail, strings.TrimPrefix(k, prefix))
				}
			}
			hint := fmt.Sprintf("mod %q has no property %q", e.Mod, e.Prop)
			if len(avail) > 0 {
				hint += fmt.Sprintf(" — available properties: %s", strings.Join(avail, ", "))
			}
			errCodeTrace("EP10", e.Sp, hint,
				fmt.Sprintf("add to mod %s: property %s int", e.Mod, e.Prop),
				tc.trace.Snapshot())
		}
		tc.ok = false
		e.Typ = TypUnknown
		return TypUnknown
	}
	if !prop.HasGet {
		errCodeTrace("EP13", e.Sp,
			fmt.Sprintf("property %s::%s is write-only (no get block)", e.Mod, e.Prop),
			"add a get block: get { return __"+e.Prop+"; }",
			tc.trace.Snapshot())
		tc.ok = false
		e.Typ = TypUnknown
		return TypUnknown
	}
	e.Typ = prop.Type
	return prop.Type
}

// ─────────────────────────────────────────────────────────────────────────────
//  VarDecl checking
// ─────────────────────────────────────────────────────────────────────────────

func (tc *TypeChecker) checkVarDecl(v *VarDecl) {
	if existing := tc.scope.lookupLocal(v.Name); existing != nil {
		errFull("E09", v.Sp,
			fmt.Sprintf("variable %q is already declared in this scope", v.Name),
			"use a different name, or remove the duplicate declaration",
			"",
			[]SecondarySpan{{Span: existing.Sp, Label: "previous declaration here"}},
			tc.trace.Snapshot())
		tc.ok = false
	}

	if v.Name == "_" {
		if v.Init != nil {
			tc.inferExpr(v.Init)
		}
		return
	}

	var initType *ZXType
	if v.Init != nil {
		initType = tc.inferExpr(v.Init)
	}

	resolved := v.VarType
	if resolved == nil || resolved.Kind == TyUnknown {
		if initType == nil || initType.Kind == TyUnknown {
			resolved = TypAny
		} else {
			resolved = initType
		}
	} else {
		tc.validateTypeExists(resolved, v.Sp)
		if initType != nil && initType.Kind != TyUnknown && initType.Kind != TyAny {
			if !coercible(initType, resolved) {
				errFull("E11", v.Sp,
					fmt.Sprintf("type mismatch: cannot assign %s to %q (type %s)", initType, v.Name, resolved),
					fmt.Sprintf("change the declared type to %s, or convert: %s(%s)", initType, resolved, v.Name),
					fmt.Sprintf("let %s %s = %s(...)", v.Name, resolved, v.Name),
					nil, tc.trace.Snapshot())
				tc.ok = false
			}
		}
	}

	if resolved != nil && resolved.Kind == TyVoid {
		errCodeTrace("E12", v.Sp,
			fmt.Sprintf("variable %q cannot have type void", v.Name),
			"use int, float, str, bool, any, or a struct type instead",
			tc.trace.Snapshot())
		tc.ok = false
	}

	if v.IsConst && v.Init == nil {
		errCodeTrace("E13", v.Sp,
			fmt.Sprintf("const %q must have an initializer", v.Name),
			fmt.Sprintf("add a value: const %s = 42", v.Name),
			tc.trace.Snapshot())
		tc.ok = false
	}

	if v.Init != nil {
		if _, isNil := v.Init.(*NilLit); isNil {
			if resolved != nil && resolved.Kind != TyRef && resolved.Kind != TyAny && resolved.Kind != TyUnknown {
				errCodeTrace("E73", v.Sp,
					fmt.Sprintf("cannot assign nil to variable %q of type %s", v.Name, resolved),
					"declare it as ref<T> if you need a nullable pointer",
					tc.trace.Snapshot())
				tc.ok = false
			}
		}
	}

	v.ResolvedType = resolved
	tc.scope.define(v.Name, &VarInfo{
		Type:     resolved,
		IsConst:  v.IsConst,
		IsGlobal: v.IsGlobal,
		Sp:       v.Sp,
		Defined:  true,
		DeclFile: tc.currentFile,
		Vis:      v.Vis,
	})
}

func (tc *TypeChecker) checkReturn(r *ReturnStmt) {
	tc.returnSeen = true
	ret := tc.currentRetType()
	if ret == nil {
		errCodeTrace("E14", r.Sp,
			"'return' used outside a function",
			"move this return statement inside a fn block",
			tc.trace.Snapshot())
		tc.ok = false
		return
	}
	if r.Value == nil {
		if ret.Kind != TyVoid && ret.Kind != TyAny {
			errCodeTrace("E15", r.Sp,
				fmt.Sprintf("function %q has return type %s but this return has no value", tc.currentName(), ret),
				"return a value: return <expr>  —  or change the return type to void",
				tc.trace.Snapshot())
			tc.ok = false
		}
		return
	}
	got := tc.inferExpr(r.Value)
	if ret.Kind == TyVoid {
		errCodeTrace("E16", r.Sp,
			fmt.Sprintf("function %q is void but returns a %s value", tc.currentName(), got),
			fmt.Sprintf("remove the return value, or change the return type to '-> %s'", got),
			tc.trace.Snapshot())
		tc.ok = false
		return
	}
	if got.Kind != TyUnknown && got.Kind != TyAny && ret.Kind != TyAny && !coercible(got, ret) {
		errFull("E17", r.Sp,
			fmt.Sprintf("return type mismatch in %q — expected %s but got %s", tc.currentName(), ret, got),
			fmt.Sprintf("cast: %s(value)  —  or change the return type to '-> %s'", ret, got),
			fmt.Sprintf("return %s(value)", ret),
			nil, tc.trace.Snapshot())
		tc.ok = false
	}
}

func (tc *TypeChecker) checkIf(s *IfStmt) {
	cond := tc.inferExpr(s.Cond)
	if cond.Kind == TyVoid {
		errCodeTrace("E18", s.Cond.nodeSpan(),
			"if condition has type void",
			"use a comparison expression that produces a bool or int",
			tc.trace.Snapshot())
		tc.ok = false
	}
	tc.checkBlock(s.Then)
	for _, el := range s.Elifs {
		condT := tc.inferExpr(el.Cond)
		if condT.Kind == TyVoid {
			errCodeTrace("E18", el.Cond.nodeSpan(),
				"elif condition has type void",
				"use a comparison expression",
				tc.trace.Snapshot())
			tc.ok = false
		}
		tc.checkBlock(el.Body)
	}
	if s.Else != nil {
		tc.checkBlock(s.Else)
	}
}

func (tc *TypeChecker) checkForRange(s *ForRangeStmt) {
	fromT := tc.inferExpr(s.From)
	toT := tc.inferExpr(s.To)
	if !isInteger(fromT) && fromT.Kind != TyUnknown && fromT.Kind != TyAny {
		errCodeTrace("E20", s.From.nodeSpan(),
			fmt.Sprintf("for-range start must be an integer, got %s", fromT),
			"use an integer expression, e.g.:  for i in 0..10 { }",
			tc.trace.Snapshot())
		tc.ok = false
	}
	if !isInteger(toT) && toT.Kind != TyUnknown && toT.Kind != TyAny {
		errCodeTrace("E21", s.To.nodeSpan(),
			fmt.Sprintf("for-range end must be an integer, got %s", toT),
			"use an integer expression, e.g.:  for i in 0..len(arr) { }",
			tc.trace.Snapshot())
		tc.ok = false
	}
	if fromLit, ok1 := s.From.(*IntLit); ok1 {
		if toLit, ok2 := s.To.(*IntLit); ok2 {
			if fromLit.Val >= toLit.Val {
				warnCode("W53", s.Sp,
					fmt.Sprintf("for-range %d..%d will never execute — start >= end", fromLit.Val, toLit.Val),
					"check that the range is correct")
			}
		}
	}
	saved := tc.scope
	tc.scope = newScope(saved, "loop")
	tc.scope.define(s.Var, &VarInfo{
		Type: TypInt, Sp: s.Sp, Defined: true, DeclFile: tc.currentFile,
	})
	for _, st := range s.Body.Stmts {
		tc.checkStmt(st)
	}
	tc.scope = saved
}

func (tc *TypeChecker) checkMatch(s *MatchStmt) {
	tc.inferExpr(s.Expr)
	hasWild := false
	seenPatterns := map[int64]Span{}
	for _, arm := range s.Arms {
		if arm.IsWild {
			if hasWild {
				warnCode("W54", arm.Sp,
					"duplicate wildcard arm in match",
					"remove the extra wildcard arm")
			}
			hasWild = true
		} else if arm.Pattern != nil {
			if lit, ok := arm.Pattern.(*IntLit); ok {
				if prev, dup := seenPatterns[lit.Val]; dup {
					errFull("E74", arm.Sp,
						fmt.Sprintf("duplicate match arm for value %d", lit.Val),
						"remove the duplicate arm or change its pattern",
						"",
						[]SecondarySpan{{Span: prev, Label: "first arm here"}},
						tc.trace.Snapshot())
					tc.ok = false
				}
				seenPatterns[lit.Val] = arm.Sp
			}
			tc.inferExpr(arm.Pattern)
		}
		if arm.Guard != nil {
			tc.inferExpr(arm.Guard)
		}
		tc.checkBlock(arm.Body)
	}
	if !hasWild && len(s.Arms) > 0 {
		warnCode("WE1", s.Sp,
			"match has no wildcard arm — unmatched values fall through silently",
			"add a wildcard arm: _ => { ... }")
	}
}

func (tc *TypeChecker) checkTryCatch(s *TryCatchStmt) {
	tc.checkBlockInTry(s.Try)
	if s.Catch != nil {
		saved := tc.scope
		tc.scope = newScope(saved, "block")
		if s.ErrVar != "" {
			tc.scope.define(s.ErrVar, &VarInfo{
				Type: TypInt, Sp: s.Sp, Defined: true, DeclFile: tc.currentFile,
			})
		}
		for _, st := range s.Catch.Stmts {
			tc.checkStmt(st)
		}
		tc.scope = saved
	}
	if s.Finally != nil {
		tc.checkBlock(s.Finally)
	}
}

func (tc *TypeChecker) checkAssign(s *AssignStmt) {
	lhsType := tc.inferExpr(s.LHS)

	if id, ok := s.LHS.(*Ident); ok {
		vi := tc.scope.lookup(id.Name)
		if vi != nil && vi.IsConst {
			errFull("E22", s.Sp,
				fmt.Sprintf("cannot assign to const %q — constants are immutable", id.Name),
				"use 'let' instead of 'const' if you need a mutable variable",
				fmt.Sprintf("let %s = ...", id.Name),
				nil, tc.trace.Snapshot())
			tc.ok = false
			return
		}
		if vi != nil && vi.IsFn {
			errCodeTrace("E23", s.Sp,
				fmt.Sprintf("cannot assign to function %q", id.Name),
				"declare a variable: let result = "+id.Name+"(...)",
				tc.trace.Snapshot())
			tc.ok = false
			return
		}
		if vi == nil {
			suggestion := tc.suggestName(id.Name)
			hint := fmt.Sprintf("declare it first: let %s = ...", id.Name)
			if suggestion != "" {
				hint = fmt.Sprintf("did you mean %q? — or declare it: let %s = ...", suggestion, id.Name)
			}
			errFull("E75", s.Sp,
				fmt.Sprintf("assignment to undeclared variable %q", id.Name),
				hint, fmt.Sprintf("let %s = ...", id.Name),
				nil, tc.trace.Snapshot())
			tc.ok = false
			return
		}
	}

	switch s.LHS.(type) {
	case *IntLit, *FloatLit, *BoolLit, *StrLit:
		errCodeTrace("E24", s.Sp,
			"left side of assignment is a literal — you cannot assign to a value",
			"use a variable name on the left side",
			tc.trace.Snapshot())
		tc.ok = false
		return
	}

	if _, ok := s.LHS.(*CallExpr); ok {
		errCodeTrace("E76", s.Sp,
			"left side of assignment is a function call — call results are not assignable",
			"store the result first: let x = f(); x = ...",
			tc.trace.Snapshot())
		tc.ok = false
		return
	}

	rhsType := tc.inferExpr(s.Value)

	if _, isNil := s.Value.(*NilLit); isNil {
		if lhsType != nil && lhsType.Kind != TyRef && lhsType.Kind != TyAny && lhsType.Kind != TyUnknown {
			errCodeTrace("E73", s.Sp,
				fmt.Sprintf("cannot assign nil to %s — nil is only valid for ref types", lhsType),
				"declare the variable as ref<T> to allow nil values",
				tc.trace.Snapshot())
			tc.ok = false
			return
		}
	}

	if lhsType.Kind != TyUnknown && lhsType.Kind != TyAny &&
		rhsType.Kind != TyUnknown && rhsType.Kind != TyAny {
		if !coercible(rhsType, lhsType) {
			errFull("E25", s.Sp,
				fmt.Sprintf("type mismatch: cannot assign %s to %s", rhsType, lhsType),
				fmt.Sprintf("cast: %s(expr)", lhsType),
				fmt.Sprintf("%s(%s)", lhsType, "expr"),
				nil, tc.trace.Snapshot())
			tc.ok = false
		}
	}
	if s.Op != "=" && lhsType.Kind != TyAny && !isNumeric(lhsType) {
		errCodeTrace("E26", s.Sp,
			fmt.Sprintf("compound operator %s requires a numeric operand, got %s", s.Op, lhsType),
			"compound assignment (+=, -=, *=, /=, %=) only works on int, float, and char",
			tc.trace.Snapshot())
		tc.ok = false
	}
}

// ─────────────────────────────────────────────────────────────────────────────
//  Type inference
// ─────────────────────────────────────────────────────────────────────────────

func (tc *TypeChecker) inferExpr(n Node) *ZXType {
	if n == nil {
		return TypVoid
	}
	switch e := n.(type) {
	case *IntLit:
		return TypInt
	case *FloatLit:
		return TypFloat
	case *BoolLit:
		return TypBool
	case *StrLit:
		return TypStr
	case *NilLit:
		return RefOf(TypVoid)
	case *SizeofExpr:
		e.Typ = TypInt
		return TypInt
	case *TemplateStr:
		e.Typ = TypStr
		for _, part := range e.Parts {
			if part.IsExpr && part.Expr != nil {
				tc.inferExpr(part.Expr)
			}
		}
		return TypStr
	case *MultilineStr:
		e.Typ = TypStr
		for _, part := range e.Parts {
			if part.IsExpr && part.Expr != nil {
				tc.inferExpr(part.Expr)
			}
			if part.IsStmt {
				for _, s := range part.Stmts {
					tc.checkStmt(s)
				}
			}
		}
		return TypStr
	case *CmdExpr:
		e.Typ = TypStr
		tc.inferExpr(e.Command)
		return TypStr
	case *ReadFileExpr:
		e.Typ = TypStr
		tc.inferExpr(e.Path)
		return TypStr
	case *TernaryExpr:
		condT := tc.inferExpr(e.Cond)
		if condT.Kind == TyVoid {
			errCodeTrace("E18", e.Cond.nodeSpan(),
				"ternary condition has type void",
				"use a comparison expression",
				tc.trace.Snapshot())
			tc.ok = false
		}
		then := tc.inferExpr(e.Then)
		els := tc.inferExpr(e.Else)
		if then.Kind != TyAny && then.Kind != TyUnknown &&
			els.Kind != TyAny && els.Kind != TyUnknown &&
			!coercible(then, els) && !coercible(els, then) {
			warnCode("W61", e.Sp,
				fmt.Sprintf("ternary branches have different types: %s vs %s", then, els),
				fmt.Sprintf("cast one branch: %s(expr)", then))
		}
		e.Typ = then
		return then

	case *ModPropGetExpr:
		return tc.inferModPropGet(e)

	case *Ident:
		vi := tc.scope.lookup(e.Name)
		if vi == nil {
			if bd := LookupBuiltin(e.Name); bd != nil {
				e.Typ = bd.Ret
				return bd.Ret
			}
			if tc.modNames[e.Name] {
				errCodeTrace("E80", e.Sp,
					fmt.Sprintf("%q is a mod block, not a value", e.Name),
					fmt.Sprintf("call a function inside it: %s->myFn()", e.Name),
					tc.trace.Snapshot())
				tc.ok = false
				e.Typ = TypUnknown
				return TypUnknown
			}
			suggestion := tc.suggestName(e.Name)
			hint := fmt.Sprintf("declare it: let %s = ...", e.Name)
			if suggestion != "" {
				hint = fmt.Sprintf("did you mean %q?", suggestion)
			}
			if declFile, isPriv := tc.privFns[e.Name]; isPriv {
				errFull("EP1", e.Sp,
					fmt.Sprintf("cannot use %q — declared 'priv' in %q", e.Name, declFile),
					fmt.Sprintf("remove 'priv' from fn %q in %q to make it importable", e.Name, declFile),
					"", nil, tc.trace.Snapshot())
				tc.ok = false
				e.Typ = TypUnknown
				return TypUnknown
			}
			errFull("E27", e.Sp,
				fmt.Sprintf("undefined name %q", e.Name),
				hint, "", nil, tc.trace.Snapshot())
			tc.ok = false
			e.Typ = TypUnknown
			return TypUnknown
		}

		if vi.DeclFile != "" && vi.DeclFile != tc.currentFile && vi.Vis == VisPrivate {
			errFull("EP1", e.Sp,
				fmt.Sprintf("cannot access private name %q declared priv in %q", e.Name, vi.DeclFile),
				fmt.Sprintf("remove the 'priv' modifier in %q to make it importable", vi.DeclFile),
				"",
				[]SecondarySpan{{Span: vi.Sp, Label: "declared priv here"}},
				tc.trace.Snapshot())
			tc.ok = false
			e.Typ = TypUnknown
			return TypUnknown
		}

		if vi.IsModFn {
			errCodeTrace("E81", e.Sp,
				fmt.Sprintf("%q is a mod-private function in mod %q", e.Name, vi.ModName),
				fmt.Sprintf("call it as: %s->%s()", vi.ModName, e.Name),
				tc.trace.Snapshot())
			tc.ok = false
			e.Typ = TypUnknown
			return TypUnknown
		}
		vi.UsedCount++
		e.Typ = vi.Type
		return vi.Type

	case *ModCallExpr:
		return tc.inferModCall(e)
	case *BinExpr:
		return tc.inferBin(e)
	case *UnaryExpr:
		return tc.inferUnary(e)
	case *CallExpr:
		return tc.inferCall(e)
	case *BuiltinExpr:
		return tc.inferBuiltin(e)
	case *MethodCallExpr:
		return tc.inferMethodCall(e)
	case *PipeExpr:
		return tc.inferPipe(e)

	case *IndexExpr:
		obj := tc.inferExpr(e.Obj)
		idx := tc.inferExpr(e.Idx)
		if !isInteger(idx) && idx.Kind != TyUnknown && idx.Kind != TyAny {
			errCodeTrace("E47", e.Idx.nodeSpan(),
				fmt.Sprintf("array index must be an integer, got %s", idx),
				"use an integer expression as the index",
				tc.trace.Snapshot())
			tc.ok = false
		}
		// Static bounds check
		if obj.Kind == TyArray && obj.ArrSize > 0 {
			if idxLit, ok := e.Idx.(*IntLit); ok {
				if idxLit.Val < 0 || idxLit.Val >= int64(obj.ArrSize) {
					errCodeTrace("EOB", e.Sp,
						fmt.Sprintf("index %d is out of bounds for array of size %d", idxLit.Val, obj.ArrSize),
						"use an index in the range 0 to N-1",
						tc.trace.Snapshot())
					tc.ok = false
				}
			}
		}
		if obj.Kind == TyArray && obj.Elem != nil {
			e.Typ = obj.Elem
			return e.Typ
		}
		if obj.Kind == TySlice && obj.Elem != nil {
			e.Typ = obj.Elem
			return e.Typ
		}
		if obj.Kind == TyRef && obj.Elem != nil {
			e.Typ = obj.Elem
			return e.Typ
		}
		if obj.Kind == TyStr {
			e.Typ = TypChar
			return TypChar
		}
		if obj.Kind != TyAny && obj.Kind != TyUnknown {
			errCodeTrace("E46", e.Sp,
				fmt.Sprintf("cannot index into type %s — only arrays and strings support indexing", obj),
				"use an array [1,2,3] or a string",
				tc.trace.Snapshot())
			tc.ok = false
		}
		e.Typ = TypAny
		return TypAny

	case *FieldExpr:
		return tc.inferField(e)

	case *CastExpr:
		from := tc.inferExpr(e.Operand)
		if !canCast(from, e.ToType) {
			errCodeTrace("E28", e.Sp,
				fmt.Sprintf("cannot cast %s to %s", from, e.ToType),
				"valid casts: between numeric types (int/float/char/bool) and str↔ref char",
				tc.trace.Snapshot())
			tc.ok = false
		}
		e.Typ = e.ToType
		return e.ToType

	case *AddrExpr:
		inner := tc.inferExpr(e.Operand)
		if e.Deref {
			if inner.Kind != TyRef {
				errCodeTrace("E29", e.Sp,
					fmt.Sprintf("cannot dereference type %s — only ref<T> values can be dereferenced", inner),
					"use ^ on a ref variable",
					tc.trace.Snapshot())
				tc.ok = false
				e.Typ = TypAny
				return TypAny
			}
			if inner.Elem == nil {
				e.Typ = TypVoid
				return TypVoid
			}
			e.Typ = inner.Elem
			return inner.Elem
		}
		e.Typ = RefOf(inner)
		return e.Typ

	case *StructInit:
		return tc.inferStructInit(e)

	case *ArrayLit:
		if len(e.Elems) == 0 {
			e.Typ = ArrayOf(TypAny, 0)
			return e.Typ
		}
		first := tc.inferExpr(e.Elems[0])
		for i, el := range e.Elems[1:] {
			got := tc.inferExpr(el)
			if got.Kind != TyAny && got.Kind != TyUnknown && !coercible(got, first) {
				errFull("E55", el.nodeSpan(),
					fmt.Sprintf("array element %d has type %s but first element is %s", i+2, got, first),
					fmt.Sprintf("cast: %s(value)", first),
					fmt.Sprintf("%s(value)", first),
					nil, tc.trace.Snapshot())
				tc.ok = false
			}
		}
		e.Typ = ArrayOf(first, len(e.Elems))
		return e.Typ

	case *MacroCallExpr:
		return tc.inferMacroCall(e)
	case *MacroCallChain:
		return tc.inferMacroChain(e)
	case *TypeofExpr:
		tc.inferExpr(e.Arg)
		e.Typ = TypStr
		return TypStr
	case *BangMacroExpr:
		for _, a := range e.Args {
			tc.inferExpr(a)
		}
		e.Typ = TypAny
		return TypAny
	case *LambdaExpr:
		saved := tc.scope
		tc.scope = newScope(saved, "fn")
		for _, p := range e.Params {
			t := p.Type
			if t == nil {
				t = TypAny
			}
			tc.scope.define(p.Name, &VarInfo{
				Type: t, Sp: p.Sp, Defined: true, DeclFile: tc.currentFile,
			})
		}
		tc.checkBlock(e.Body)
		tc.scope = saved
		if e.Typ == nil {
			e.Typ = FnType(nil, e.RetType)
		}
		return e.Typ

	default:
		return TypUnknown
	}
}

// ─────────────────────────────────────────────────────────────────────────────
//  inferModCall
// ─────────────────────────────────────────────────────────────────────────────

func (tc *TypeChecker) inferModCall(e *ModCallExpr) *ZXType {
	if !tc.modNames[e.Mod] {
		bestMod := tc.suggestModName(e.Mod)
		hint := fmt.Sprintf("declare it: mod %s { fn %s() { ... } }", e.Mod, e.Fn)
		if bestMod != "" {
			hint = fmt.Sprintf("did you mean mod %q?", bestMod)
		}
		errCodeTrace("E82", e.Sp,
			fmt.Sprintf("unknown module %q", e.Mod),
			hint, tc.trace.Snapshot())
		tc.ok = false
		e.Typ = TypUnknown
		return TypUnknown
	}
	key := e.Mod + "::" + e.Fn

	// Check if it's a property being called like a function
	if _, isProp := tc.modProps[key]; isProp {
		errCodeTrace("E84", e.Sp,
			fmt.Sprintf("%s::%s is a property, not a function — read it without parentheses", e.Mod, e.Fn),
			fmt.Sprintf("use: %s::%s  (no parentheses)", e.Mod, e.Fn),
			tc.trace.Snapshot())
		tc.ok = false
		e.Typ = TypUnknown
		return TypUnknown
	}

	fn, ok := tc.modFns[key]
	if !ok {
		var available []string
		prefix := e.Mod + "::"
		for k := range tc.modFns {
			if strings.HasPrefix(k, prefix) {
				rest := strings.TrimPrefix(k, prefix)
				if !strings.Contains(rest, "::") {
					available = append(available, rest)
				}
			}
		}
		hint := fmt.Sprintf("mod %q has no function %q", e.Mod, e.Fn)
		if len(available) > 0 {
			hint += fmt.Sprintf(" — available: %s", strings.Join(available, ", "))
		}
		errCodeTrace("E83", e.Sp, hint,
			fmt.Sprintf("define it: mod %s { fn %s() { ... } }", e.Mod, e.Fn),
			tc.trace.Snapshot())
		tc.ok = false
		e.Typ = TypUnknown
		return TypUnknown
	}

	if fn.Vis == VisPrivate && fn.ModPath != tc.currentFile {
		errFull("EP2", e.Sp,
			fmt.Sprintf("cannot call private function %s->%s from this file", e.Mod, e.Fn),
			fmt.Sprintf("remove 'priv' from fn %s in mod %s to allow external calls", e.Fn, e.Mod),
			"",
			[]SecondarySpan{{Span: fn.Sp, Label: "declared priv here"}},
			tc.trace.Snapshot())
		tc.ok = false
		e.Typ = TypUnknown
		return TypUnknown
	}

	if !fn.Variadic {
		minArgs := 0
		for _, p := range fn.Params {
			if p.Default == nil {
				minArgs++
			}
		}
		if len(e.Args) < minArgs || len(e.Args) > len(fn.Params) {
			errCodeTrace("E42", e.Sp,
				fmt.Sprintf("%s->%s expects %d argument(s) but got %d",
					e.Mod, e.Fn, len(fn.Params), len(e.Args)),
				fmt.Sprintf("signature: fn %s(%s)", e.Fn, listParamTypes(fn.Params)),
				tc.trace.Snapshot())
			tc.ok = false
		}
	}
	for i, arg := range e.Args {
		got := tc.inferExpr(arg)
		if i < len(fn.Params) {
			expected := fn.Params[i].Type
			if expected != nil && expected.Kind != TyAny &&
				got.Kind != TyAny && got.Kind != TyUnknown &&
				!coercible(got, expected) {
				errFull("E43", arg.nodeSpan(),
					fmt.Sprintf("%s->%s argument %d: expected %s but got %s",
						e.Mod, e.Fn, i+1, expected, got),
					fmt.Sprintf("cast: %s(value)", expected),
					fmt.Sprintf("%s(value)", expected),
					nil, tc.trace.Snapshot())
				tc.ok = false
			}
		}
	}
	e.Typ = fn.RetType
	return fn.RetType
}

func (tc *TypeChecker) suggestModName(name string) string {
	best := ""
	bestDist := 3
	for n := range tc.modNames {
		if d := editDistance(name, n); d < bestDist {
			bestDist = d
			best = n
		}
	}
	return best
}

// ─────────────────────────────────────────────────────────────────────────────
//  inferMacroCall / inferMacroChain
// ─────────────────────────────────────────────────────────────────────────────

func (tc *TypeChecker) inferMacroCall(e *MacroCallExpr) *ZXType {
	mc, ok := tc.macros[e.Name]
	if !ok {
		if declFile, isPriv := tc.privMacros[e.Name]; isPriv {
			errFull("EP3", e.Sp,
				fmt.Sprintf("cannot call macro %q — declared 'priv' in %q", e.Name, declFile),
				fmt.Sprintf("remove 'priv' from macro %q to make it importable", e.Name),
				"", nil, tc.trace.Snapshot())
			tc.ok = false
			e.Typ = TypUnknown
			return TypUnknown
		}
		hint := fmt.Sprintf("declare it: macro fn %s |input| -> |output| { }", e.Name)
		if s := tc.suggestName(e.Name); s != "" {
			hint = fmt.Sprintf("did you mean %q?", s)
		}
		errCodeTrace("EM07", e.Sp,
			fmt.Sprintf("call to undefined macro %q", e.Name),
			hint, tc.trace.Snapshot())
		tc.ok = false
		e.Typ = TypUnknown
		return TypUnknown
	}
	expected := len(mc.Params)
	got := len(e.Args)
	if expected != got && !(got == 0 && expected <= 1) {
		errCodeTrace("EM08", e.Sp,
			fmt.Sprintf("macro %q expects %d argument(s) but got %d", e.Name, expected, got),
			fmt.Sprintf("signature: macro fn %s |%s|", e.Name, listParamTypes(mc.Params)),
			tc.trace.Snapshot())
		tc.ok = false
	}
	for i, arg := range e.Args {
		argType := tc.inferExpr(arg)
		if i < len(mc.Params) {
			exp := mc.Params[i].Type
			if exp != nil && exp.Kind != TyAny &&
				argType.Kind != TyAny && argType.Kind != TyUnknown &&
				!coercible(argType, exp) {
				errFull("EM09", arg.nodeSpan(),
					fmt.Sprintf("macro %q argument %d: expected %s but got %s", e.Name, i+1, exp, argType),
					fmt.Sprintf("cast: %s(value)", exp),
					fmt.Sprintf("%s(value)", exp),
					nil, tc.trace.Snapshot())
				tc.ok = false
			}
		}
	}
	e.Typ = mc.RetType
	if e.Typ == nil {
		e.Typ = TypVoid
	}
	return e.Typ
}

func (tc *TypeChecker) inferMacroChain(e *MacroCallChain) *ZXType {
	recvType := tc.inferExpr(e.Recv)
	lastType := recvType
	for _, step := range e.Steps {
		for _, a := range step.Args {
			tc.inferExpr(a)
		}
		mc, ok := tc.macros[step.Macro]
		if !ok {
			if isBuiltinChainMacro(step.Macro) {
				tc.checkBlock(step.Body)
				continue
			}
			errCodeTrace("EM07", step.Sp,
				fmt.Sprintf("undefined macro %q in chain", step.Macro),
				fmt.Sprintf("declare it: macro fn %s |input, doStmt| -> |output| { }", step.Macro),
				tc.trace.Snapshot())
			tc.ok = false
			tc.checkBlock(step.Body)
			continue
		}
		tc.checkBlock(step.Body)
		if mc.RetType != nil && mc.RetType.Kind != TyVoid {
			lastType = mc.RetType
		}
	}
	e.Typ = lastType
	return lastType
}

func isBuiltinChainMacro(name string) bool {
	switch name {
	case "ifTrue", "if_true", "whenTrue", "onTrue",
		"ifFalse", "if_false", "whenFalse", "onFalse", "unless",
		"ifNil", "if_nil", "whenNil", "onNil",
		"ifNotNil", "if_not_nil", "whenNotNil", "onNotNil", "ifExists",
		"ifZero", "if_zero", "whenZero",
		"ifNotZero", "if_not_zero", "whenNotZero",
		"ifPositive", "if_positive",
		"ifNegative", "if_negative",
		"then", "always", "do_always", "andThen",
		"whileTrue", "while_true",
		"repeat", "times":
		return true
	}
	return false
}

// ─────────────────────────────────────────────────────────────────────────────
//  inferBin / inferUnary
// ─────────────────────────────────────────────────────────────────────────────

func (tc *TypeChecker) inferBin(e *BinExpr) *ZXType {
	lhs := tc.inferExpr(e.LHS)
	rhs := tc.inferExpr(e.RHS)
	switch e.Op {
	case "==", "!=":
		if lhs.Kind != TyUnknown && rhs.Kind != TyUnknown &&
			lhs.Kind != TyAny && rhs.Kind != TyAny {
			if !coercible(lhs, rhs) && !coercible(rhs, lhs) {
				errCodeTrace("E30", e.Sp,
					fmt.Sprintf("cannot compare %s with %s using %s — incompatible types", lhs, rhs, e.Op),
					"both sides of == or != must have the same (or compatible) type",
					tc.trace.Snapshot())
				tc.ok = false
			}
		}
		// REAL check: string pointer comparison is almost always a bug
		if lhs.Kind == TyStr || rhs.Kind == TyStr {
			warnCode("WS1", e.Sp,
				"comparing strings with == compares pointers, not contents — this is almost always a bug",
				"use str_eq!(a, b) for content comparison")
		}
		e.Typ = TypBool
		return TypBool

	case "<", ">", "<=", ">=":
		if lhs.Kind != TyAny && rhs.Kind != TyAny && lhs.Kind != TyUnknown {
			if !isNumeric(lhs) || !isNumeric(rhs) {
				errCodeTrace("E31", e.Sp,
					fmt.Sprintf("comparison %s requires numeric operands, got %s and %s", e.Op, lhs, rhs),
					"comparisons only work on int, float, and char",
					tc.trace.Snapshot())
				tc.ok = false
			}
		}
		e.Typ = TypBool
		return TypBool

	case "&&", "||":
		e.Typ = TypBool
		return TypBool

	case "+":
		// REAL check: string concatenation via + is a common mistake
		if lhs.Kind == TyStr || rhs.Kind == TyStr {
			errCodeTrace("E32", e.Sp,
				"'+' cannot concatenate strings in ZX",
				"use str_cat(a, b) from std::str, or an f-string: f\"{a}{b}\"",
				tc.trace.Snapshot())
			tc.ok = false
			e.Typ = TypStr
			return TypStr
		}
		fallthrough
	case "-", "*", "/", "%":
		if lhs.Kind != TyAny && lhs.Kind != TyUnknown && !isNumeric(lhs) {
			errCodeTrace("E33", e.LHS.nodeSpan(),
				fmt.Sprintf("operator %q requires numeric operands but left side is %s", e.Op, lhs),
				"arithmetic operators only work on int, float, and char",
				tc.trace.Snapshot())
			tc.ok = false
		}
		if rhs.Kind != TyAny && rhs.Kind != TyUnknown && !isNumeric(rhs) {
			errCodeTrace("E33", e.RHS.nodeSpan(),
				fmt.Sprintf("operator %q requires numeric operands but right side is %s", e.Op, rhs),
				"arithmetic operators only work on int, float, and char",
				tc.trace.Snapshot())
			tc.ok = false
		}
		// REAL check: division by zero literal
		if e.Op == "/" {
			if lit, ok := e.RHS.(*IntLit); ok && lit.Val == 0 {
				errCodeTrace("E34", e.RHS.nodeSpan(),
					"division by zero — this will crash at runtime",
					"check the divisor before dividing: if divisor != 0 { ... }",
					tc.trace.Snapshot())
				tc.ok = false
			}
			if lit, ok := e.RHS.(*FloatLit); ok && lit.Val == 0.0 {
				warnCode("W03", e.RHS.nodeSpan(),
					"division by 0.0 produces Inf or NaN",
					"check for zero before dividing floating-point values")
			}
		}
		if e.Op == "%" && (lhs.Kind == TyFloat || rhs.Kind == TyFloat) {
			errCodeTrace("E35", e.Sp,
				"modulo '%' does not work on float operands",
				"use fmod(x, y) from std::math for floating-point modulo",
				tc.trace.Snapshot())
			tc.ok = false
		}
		if lhs.Kind == TyFloat || rhs.Kind == TyFloat {
			e.Typ = TypFloat
			return TypFloat
		}
		e.Typ = lhs
		if e.Typ == nil || e.Typ.Kind == TyUnknown {
			e.Typ = TypAny
		}
		return e.Typ

	case "|", "&", "^", "<<", ">>":
		if lhs.Kind != TyAny && lhs.Kind != TyUnknown && !isInteger(lhs) {
			errCodeTrace("E36", e.Sp,
				fmt.Sprintf("bitwise %q requires integer operands but left side is %s", e.Op, lhs),
				"bitwise operators only work on int and char",
				tc.trace.Snapshot())
			tc.ok = false
		}
		if rhs.Kind != TyAny && rhs.Kind != TyUnknown && !isInteger(rhs) {
			errCodeTrace("E36", e.Sp,
				fmt.Sprintf("bitwise %q requires integer operands but right side is %s", e.Op, rhs),
				"bitwise operators only work on int and char",
				tc.trace.Snapshot())
			tc.ok = false
		}
		// REAL check: invalid shift amount
		if e.Op == "<<" || e.Op == ">>" {
			if lit, ok := e.RHS.(*IntLit); ok {
				if lit.Val < 0 {
					errCodeTrace("E77", e.RHS.nodeSpan(),
						fmt.Sprintf("shift by negative amount %d — undefined behaviour", lit.Val),
						"use a non-negative shift amount",
						tc.trace.Snapshot())
					tc.ok = false
				} else if lit.Val >= 64 {
					warnCode("W66", e.RHS.nodeSpan(),
						fmt.Sprintf("shift by %d — shifting a 64-bit integer by >= 64 bits is undefined behaviour", lit.Val),
						"use a shift amount between 0 and 63")
				}
			}
		}
		e.Typ = TypInt
		return TypInt

	default:
		e.Typ = lhs
		return lhs
	}
}

func (tc *TypeChecker) inferUnary(e *UnaryExpr) *ZXType {
	inner := tc.inferExpr(e.Operand)
	switch e.Op {
	case "!":
		if inner.Kind != TyAny && inner.Kind != TyUnknown && !isTruthy(inner) {
			errCodeTrace("E37", e.Sp,
				fmt.Sprintf("logical NOT '!' cannot be applied to type %s", inner),
				"'!' works on bool, int, and ref values only",
				tc.trace.Snapshot())
			tc.ok = false
		}
		e.Typ = TypBool
		return TypBool
	case "-":
		if inner.Kind != TyAny && inner.Kind != TyUnknown && !isNumeric(inner) {
			errCodeTrace("E38", e.Sp,
				fmt.Sprintf("unary minus cannot be applied to type %s", inner),
				"unary minus only works on int, float, and char",
				tc.trace.Snapshot())
			tc.ok = false
		}
		e.Typ = inner
		return inner
	case "~":
		if inner.Kind != TyAny && inner.Kind != TyUnknown && !isInteger(inner) {
			errCodeTrace("E39", e.Sp,
				fmt.Sprintf("bitwise NOT '~' cannot be applied to type %s", inner),
				"bitwise NOT requires an integer operand",
				tc.trace.Snapshot())
			tc.ok = false
		}
		e.Typ = TypInt
		return TypInt
	}
	e.Typ = inner
	return inner
}

// ─────────────────────────────────────────────────────────────────────────────
//  inferCall
// ─────────────────────────────────────────────────────────────────────────────

func (tc *TypeChecker) inferCall(e *CallExpr) *ZXType {
	var fnName string
	if id, ok := e.Func.(*Ident); ok {
		fnName = id.Name
	}

	if bd := LookupBuiltin(fnName); bd != nil {
		for _, a := range e.Args {
			tc.inferExpr(a)
		}
		if bd.Arity >= 0 && len(e.Args) != bd.Arity {
			errCodeTrace("E41", e.Sp,
				fmt.Sprintf("builtin %q expects %d argument(s) but got %d", fnName, bd.Arity, len(e.Args)),
				"check the number of arguments",
				tc.trace.Snapshot())
			tc.ok = false
		}
		if id, ok := e.Func.(*Ident); ok {
			id.Typ = bd.Ret
		}
		e.Typ = bd.Ret
		return bd.Ret
	}

	if sf, ok := tc.stdFns[fnName]; ok {
		if !sf.Variadic {
			if len(e.Args) != len(sf.Params) {
				errCodeTrace("E41", e.Sp,
					fmt.Sprintf("std function %q expects %d argument(s) but got %d",
						fnName, len(sf.Params), len(e.Args)),
					fmt.Sprintf("signature: %s(%s)", fnName, listParamTypes(sf.Params)),
					tc.trace.Snapshot())
				tc.ok = false
			}
		} else {
			if len(e.Args) < len(sf.Params) {
				errCodeTrace("E41", e.Sp,
					fmt.Sprintf("std function %q expects at least %d argument(s) but got %d",
						fnName, len(sf.Params), len(e.Args)),
					fmt.Sprintf("signature: %s(%s, ...)", fnName, listParamTypes(sf.Params)),
					tc.trace.Snapshot())
				tc.ok = false
			}
		}
		for i, a := range e.Args {
			got := tc.inferExpr(a)
			if i < len(sf.Params) {
				expected := sf.Params[i].Type
				if expected != nil && expected.Kind != TyAny &&
					got.Kind != TyAny && got.Kind != TyUnknown &&
					!coercible(got, expected) {
					errFull("E43", a.nodeSpan(),
						fmt.Sprintf("std function %q argument %d: expected %s but got %s",
							fnName, i+1, expected, got),
						fmt.Sprintf("cast: %s(value)", expected),
						fmt.Sprintf("%s(value)", expected),
						nil, tc.trace.Snapshot())
					tc.ok = false
				}
			}
		}
		if id, ok := e.Func.(*Ident); ok {
			id.Typ = sf.Ret
		}
		e.Typ = sf.Ret
		return sf.Ret
	}

	if ext, ok := tc.externs[fnName]; ok {
		for i, a := range e.Args {
			got := tc.inferExpr(a)
			if !ext.Variadic && i < len(ext.Params) {
				expected := ext.Params[i].Type
				if expected.Kind != TyAny && got.Kind != TyAny &&
					got.Kind != TyUnknown && !coercible(got, expected) {
					errFull("E40", a.nodeSpan(),
						fmt.Sprintf("extern %q argument %d: expected %s but got %s",
							fnName, i+1, expected, got),
						fmt.Sprintf("cast: %s(value)", expected),
						fmt.Sprintf("%s(value)", expected),
						nil, tc.trace.Snapshot())
					tc.ok = false
				}
			}
		}
		if !ext.Variadic && len(e.Args) != len(ext.Params) {
			errCodeTrace("E41", e.Sp,
				fmt.Sprintf("extern %q expects %d argument(s) but got %d",
					fnName, len(ext.Params), len(e.Args)),
				"check the extern declaration for the expected parameters",
				tc.trace.Snapshot())
			tc.ok = false
		}
		if id, ok := e.Func.(*Ident); ok {
			id.Typ = ext.RetType
		}
		e.Typ = ext.RetType
		return ext.RetType
	}

	if fn, ok := tc.fns[fnName]; ok {
		if fn.Vis == VisPrivate && fn.ModPath != "" && fn.ModPath != tc.currentFile {
			errFull("EP1", e.Sp,
				fmt.Sprintf("cannot call private function %q from this file", fnName),
				fmt.Sprintf("remove 'priv' from fn %q to allow external calls", fnName),
				"",
				[]SecondarySpan{{Span: fn.Sp, Label: "declared priv here"}},
				tc.trace.Snapshot())
			tc.ok = false
			e.Typ = TypUnknown
			return TypUnknown
		}

		if !fn.Variadic {
			minArgs := 0
			for _, p2 := range fn.Params {
				if p2.Default == nil {
					minArgs++
				}
			}
			if len(e.Args) < minArgs || len(e.Args) > len(fn.Params) {
				errCodeTrace("E42", e.Sp,
					fmt.Sprintf("function %q expects %d argument(s) but got %d",
						fnName, len(fn.Params), len(e.Args)),
					fmt.Sprintf("signature: fn %s(%s)", fnName, listParamTypes(fn.Params)),
					tc.trace.Snapshot())
				tc.ok = false
			}
		}
		for i, a := range e.Args {
			got := tc.inferExpr(a)
			if i < len(fn.Params) {
				expected := fn.Params[i].Type
				if expected != nil && expected.Kind != TyAny &&
					got.Kind != TyAny && got.Kind != TyUnknown &&
					!coercible(got, expected) {
					errFull("E43", a.nodeSpan(),
						fmt.Sprintf("function %q argument %d: expected %s but got %s",
							fnName, i+1, expected, got),
						fmt.Sprintf("cast: %s(value)", expected),
						fmt.Sprintf("%s(value)", expected),
						nil, tc.trace.Snapshot())
					tc.ok = false
				}
			}
		}
		if id, ok := e.Func.(*Ident); ok {
			id.Typ = fn.RetType
		}
		e.Typ = fn.RetType
		return fn.RetType
	}

	if fnName != "" {
		vi := tc.scope.lookup(fnName)
		if vi == nil {
			if declFile, isPriv := tc.privFns[fnName]; isPriv {
				errFull("EP1", e.Sp,
					fmt.Sprintf("cannot call %q — declared 'priv' in %q", fnName, declFile),
					fmt.Sprintf("remove 'priv' from fn %q in %q to make it importable", fnName, declFile),
					"", nil, tc.trace.Snapshot())
				tc.ok = false
				e.Typ = TypUnknown
				return TypUnknown
			}
			suggestion := tc.suggestName(fnName)
			hint := fmt.Sprintf("declare it: extern fn %s(...) -> int  or  fn %s(...) { }", fnName, fnName)
			if suggestion != "" {
				hint = fmt.Sprintf("did you mean %q?", suggestion)
			}
			errFull("E44", e.Sp,
				fmt.Sprintf("call to undefined function %q", fnName),
				hint, "", nil, tc.trace.Snapshot())
			tc.ok = false
			e.Typ = TypUnknown
			return TypUnknown
		}
		if !vi.IsFn {
			errCodeTrace("E45", e.Sp,
				fmt.Sprintf("%q is a %s variable, not a function — it cannot be called", fnName, vi.Type),
				fmt.Sprintf("declare a function: fn %s() { }  or  extern fn %s()", fnName, fnName),
				tc.trace.Snapshot())
			tc.ok = false
			e.Typ = TypUnknown
			return TypUnknown
		}
	}
	for _, a := range e.Args {
		tc.inferExpr(a)
	}
	e.Typ = TypAny
	return TypAny
}

func (tc *TypeChecker) inferBuiltin(e *BuiltinExpr) *ZXType {
	for _, a := range e.Args {
		tc.inferExpr(a)
	}
	if e.Name == "input" {
		e.Typ = TypStr
		return TypStr
	}
	if bd := LookupBuiltin(e.Name); bd != nil {
		e.Typ = bd.Ret
		return bd.Ret
	}
	e.Typ = TypAny
	return TypAny
}

func (tc *TypeChecker) inferMethodCall(e *MethodCallExpr) *ZXType {
	// mod call via :: syntax — receiver ident is mod path
	if id, ok := e.Recv.(*Ident); ok && tc.modNames[id.Name] {
		modCall := &ModCallExpr{
			Sp: e.Sp, Mod: id.Name, Fn: e.Method, Args: e.Args,
		}
		t := tc.inferModCall(modCall)
		e.Typ = t
		id.Typ = TypUnknown
		return t
	}

	recvType := tc.inferExpr(e.Recv)

	if _, isNil := e.Recv.(*NilLit); isNil {
		errCodeTrace("E78", e.Sp,
			fmt.Sprintf("calling method %q on nil — nil has no methods", e.Method),
			"check for nil before calling methods on ref values",
			tc.trace.Snapshot())
		tc.ok = false
		e.Typ = TypAny
		return TypAny
	}

	for _, a := range e.Args {
		tc.inferExpr(a)
	}
	structName := ""
	if recvType != nil {
		if recvType.Kind == TyStruct {
			structName = recvType.Name
		}
		if recvType.Kind == TyRef && recvType.Elem != nil && recvType.Elem.Kind == TyStruct {
			structName = recvType.Elem.Name
		}
	}
	if structName != "" {
		key := structName + "_" + e.Method
		if m, ok := tc.methods[key]; ok {
			e.Typ = m.RetType
			return m.RetType
		}
		available := tc.methodsFor(structName)
		hint := fmt.Sprintf("define it: fn (s ref %s) %s() { }", structName, e.Method)
		if len(available) > 0 {
			hint = fmt.Sprintf("available methods on %s: %s", structName, strings.Join(available, ", "))
		}
		errCodeTrace("E59", e.Sp,
			fmt.Sprintf("struct %q has no method %q", structName, e.Method),
			hint, tc.trace.Snapshot())
		tc.ok = false
	}
	e.Typ = TypAny
	return TypAny
}

func (tc *TypeChecker) inferPipe(e *PipeExpr) *ZXType {
	if len(e.Steps) == 0 {
		e.Typ = TypVoid
		return TypVoid
	}
	t := tc.inferExpr(e.Steps[0])
	for _, step := range e.Steps[1:] {
		t = tc.inferExpr(step)
	}
	e.Typ = t
	return t
}

func (tc *TypeChecker) inferField(e *FieldExpr) *ZXType {
	objType := tc.inferExpr(e.Obj)
	eff := objType
	if objType.Kind == TyRef && objType.Elem != nil {
		eff = objType.Elem
	}

	if eff.Kind != TyStruct {
		if eff.Kind != TyAny && eff.Kind != TyUnknown {
			errCodeTrace("E48", e.Sp,
				fmt.Sprintf("cannot access field %q on type %s — field access is only valid on struct types", e.Field, objType),
				"use a struct type or ref<StructType>",
				tc.trace.Snapshot())
			tc.ok = false
		}
		e.Typ = TypAny
		return TypAny
	}
	sd, ok := tc.structs[eff.Name]
	if !ok {
		errCodeTrace("E49", e.Sp,
			fmt.Sprintf("struct type %q is not defined", eff.Name),
			fmt.Sprintf("declare it: type %s struct { ... }", eff.Name),
			tc.trace.Snapshot())
		tc.ok = false
		e.Typ = TypAny
		return TypAny
	}
	if sd.Vis == VisPrivate && sd.Sp.File != "" && sd.Sp.File != tc.currentFile {
		errFull("EP4", e.Sp,
			fmt.Sprintf("cannot access fields of private struct %q from this file", sd.Name),
			fmt.Sprintf("remove 'priv' from struct %q to allow external access", sd.Name),
			"",
			[]SecondarySpan{{Span: sd.Sp, Label: "declared priv here"}},
			tc.trace.Snapshot())
		tc.ok = false
		e.Typ = TypAny
		return TypAny
	}
	for _, f := range sd.Fields {
		if f.Name == e.Field {
			e.Typ = f.Type
			return f.Type
		}
	}
	suggestion := tc.suggestField(sd, e.Field)
	hint := fmt.Sprintf("available fields: %s", listFields(sd.Fields))
	if suggestion != "" {
		hint = fmt.Sprintf("did you mean %q?", suggestion)
	}
	errFull("E50", e.Sp,
		fmt.Sprintf("struct %q has no field %q", eff.Name, e.Field),
		hint, "", nil, tc.trace.Snapshot())
	tc.ok = false
	e.Typ = TypAny
	return TypAny
}

func (tc *TypeChecker) suggestField(sd *StructDecl, name string) string {
	best := ""
	bestDist := 3
	for _, f := range sd.Fields {
		if d := editDistance(name, f.Name); d < bestDist {
			bestDist = d
			best = f.Name
		}
	}
	return best
}

func (tc *TypeChecker) inferStructInit(e *StructInit) *ZXType {
	sd, ok := tc.structs[e.Name]
	if !ok {
		if declFile, isPriv := tc.privStructs[e.Name]; isPriv {
			errFull("EP5", e.Sp,
				fmt.Sprintf("cannot construct struct %q — declared 'priv' in %q", e.Name, declFile),
				fmt.Sprintf("remove 'priv' from struct %q to make it importable", e.Name),
				"", nil, tc.trace.Snapshot())
			tc.ok = false
			e.Typ = TypUnknown
			return TypUnknown
		}
		best := ""
		bestDist := 3
		for sName := range tc.structs {
			if d := editDistance(e.Name, sName); d < bestDist {
				bestDist = d
				best = sName
			}
		}
		hint := fmt.Sprintf("declare it first: type %s struct { ... }", e.Name)
		if best != "" {
			hint = fmt.Sprintf("did you mean %q?", best)
		}
		errCodeTrace("E51", e.Sp,
			fmt.Sprintf("undefined struct %q in struct literal", e.Name),
			hint, tc.trace.Snapshot())
		tc.ok = false
		e.Typ = TypUnknown
		return TypUnknown
	}
	if sd.Vis == VisPrivate && sd.Sp.File != "" && sd.Sp.File != tc.currentFile {
		errFull("EP5", e.Sp,
			fmt.Sprintf("cannot construct private struct %q from this file", e.Name),
			fmt.Sprintf("remove 'priv' from struct %q to allow external construction", e.Name),
			"",
			[]SecondarySpan{{Span: sd.Sp, Label: "declared priv here"}},
			tc.trace.Snapshot())
		tc.ok = false
		e.Typ = TypUnknown
		return TypUnknown
	}

	provided := map[string]Span{}
	for _, fi := range e.Fields {
		found := false
		for _, sf := range sd.Fields {
			if sf.Name == fi.Name {
				found = true
				break
			}
		}
		if !found {
			suggestion := tc.suggestField(sd, fi.Name)
			hint := fmt.Sprintf("valid fields: %s", listFields(sd.Fields))
			if suggestion != "" {
				hint = fmt.Sprintf("did you mean %q?", suggestion)
			}
			errCodeTrace("E52", fi.Sp,
				fmt.Sprintf("struct %q has no field %q", e.Name, fi.Name),
				hint, tc.trace.Snapshot())
			tc.ok = false
		}
		if prev, dup := provided[fi.Name]; dup {
			errFull("E53", fi.Sp,
				fmt.Sprintf("field %q is set more than once in struct literal for %q", fi.Name, e.Name),
				"remove the duplicate field assignment",
				"",
				[]SecondarySpan{{Span: prev, Label: "first set here"}},
				tc.trace.Snapshot())
			tc.ok = false
		}
		provided[fi.Name] = fi.Sp
		got := tc.inferExpr(fi.Value)
		for _, sf := range sd.Fields {
			if sf.Name == fi.Name && sf.Type != nil && sf.Type.Kind != TyAny &&
				got.Kind != TyAny && got.Kind != TyUnknown && !coercible(got, sf.Type) {
				errFull("E54", fi.Sp,
					fmt.Sprintf("field %q expects type %s but got %s", fi.Name, sf.Type, got),
					fmt.Sprintf("cast: %s(value)", sf.Type),
					fmt.Sprintf("%s(value)", sf.Type),
					nil, tc.trace.Snapshot())
				tc.ok = false
				break
			}
		}
	}

	// Warn about unset fields — they'll be zero-initialized
	for _, sf := range sd.Fields {
		if _, set := provided[sf.Name]; !set {
			warnCode("W70", e.Sp,
				fmt.Sprintf("field %q of struct %q is not set — it will be zero-initialized", sf.Name, e.Name),
				fmt.Sprintf("add: %s: <value>", sf.Name))
		}
	}

	if e.HeapAlloc {
		e.Typ = RefOf(StructType(e.Name))
	} else {
		e.Typ = StructType(e.Name)
	}
	return e.Typ
}

// ─────────────────────────────────────────────────────────────────────────────
//  Type helpers
// ─────────────────────────────────────────────────────────────────────────────

func (tc *TypeChecker) validateTypeExists(t *ZXType, sp Span) {
	if t == nil || t.Kind == TyAny || t.Kind == TyUnknown {
		return
	}
	if t.Kind == TyStruct {
		if _, ok := tc.structs[t.Name]; !ok {
			best := ""
			bestDist := 3
			for sName := range tc.structs {
				if d := editDistance(t.Name, sName); d < bestDist {
					bestDist = d
					best = sName
				}
			}
			hint := fmt.Sprintf("declare it: type %s struct { ... }", t.Name)
			if best != "" {
				hint = fmt.Sprintf("did you mean %q?", best)
			}
			errCodeTrace("E56", sp,
				fmt.Sprintf("unknown type %q — no struct with this name is defined", t.Name),
				hint, tc.trace.Snapshot())
			tc.ok = false
		}
	}
	if t.Kind == TyRef || t.Kind == TyArray || t.Kind == TySlice {
		tc.validateTypeExists(t.Elem, sp)
	}
}

func canCast(from, to *ZXType) bool {
	if from == nil || to == nil {
		return true
	}
	if from.Kind == TyAny || to.Kind == TyAny {
		return true
	}
	if from.Kind == TyUnknown || to.Kind == TyUnknown {
		return true
	}
	if isNumeric(from) && isNumeric(to) {
		return true
	}
	if from.Kind == TyBool && isNumeric(to) {
		return true
	}
	if isNumeric(from) && to.Kind == TyBool {
		return true
	}
	if from.Kind == TyRef && to.Kind == TyRef {
		return true
	}
	if from.Kind == TyRef && isInteger(to) {
		return true
	}
	if isInteger(from) && to.Kind == TyRef {
		return true
	}
	return false
}

func (tc *TypeChecker) suggestName(name string) string {
	best := ""
	bestDist := 3
	var cands []string
	for n := range tc.fns {
		cands = append(cands, n)
	}
	for n := range tc.externs {
		cands = append(cands, n)
	}
	for n := range tc.methods {
		cands = append(cands, n)
	}
	for n := range builtinFns {
		cands = append(cands, n)
	}
	s := tc.scope
	for s != nil {
		for n := range s.vars {
			cands = append(cands, n)
		}
		s = s.parent
	}
	for _, c := range cands {
		if d := editDistance(name, c); d < bestDist {
			bestDist = d
			best = c
		}
	}
	return best
}

func (tc *TypeChecker) methodsFor(sn string) []string {
	var out []string
	prefix := sn + "_"
	for k := range tc.methods {
		if strings.HasPrefix(k, prefix) {
			out = append(out, strings.TrimPrefix(k, prefix))
		}
	}
	return out
}

// ─────────────────────────────────────────────────────────────────────────────
//  File import resolution
// ─────────────────────────────────────────────────────────────────────────────

func (tc *TypeChecker) resolveFileImport(imp *ImportDecl, prog *Program) {
	sp := imp.Sp
	filePath := imp.LocalFile
	if filePath == "" {
		errCode("EI20", sp,
			"import has no resolved file path — this is a compiler bug",
			"report this issue")
		tc.ok = false
		return
	}
	raw, err := os.ReadFile(filePath)
	if err != nil {
		errCode("EI21", sp,
			fmt.Sprintf("cannot read imported file %q: %v", filePath, err),
			"check that the file exists at the expected path")
		tc.ok = false
		return
	}
	src := string(raw)
	tokens := Tokenize(src, filePath)
	if tokens == nil {
		errCode("EI22", sp,
			fmt.Sprintf("tokenization failed for imported file %q", filePath),
			"fix the syntax errors in the imported file first")
		tc.ok = false
		return
	}
	imported := Parse(tokens, src, filePath)
	if imported == nil {
		errCode("EI23", sp,
			fmt.Sprintf("parse errors in imported file %q", filePath),
			"fix the errors in the imported file first")
		tc.ok = false
		return
	}

	tc.registerPrivDecls(imported, filePath)

	alias := imp.Alias
	if alias != "" {
		var found *ModBlock
		for _, mb := range imported.ModBlocks {
			if mb.Name == alias {
				found = mb
				break
			}
		}
		if found == nil {
			var avail []string
			for _, mb := range imported.ModBlocks {
				avail = append(avail, mb.Name)
			}
			hint := fmt.Sprintf("file %q has no mod named %q", filePath, alias)
			if len(avail) > 0 {
				hint += fmt.Sprintf(" — available mods: %s", strings.Join(avail, ", "))
			} else {
				hint += " — the file contains no mod blocks"
			}
			errCode("EI24", sp, hint,
				"remove the (ModName) selector to import everything, or fix the mod name")
			tc.ok = false
			return
		}
		if found.Vis == VisPrivate {
			errFull("EP6", sp,
				fmt.Sprintf("cannot import private mod %q from file %q — declared priv", found.Name, filePath),
				fmt.Sprintf("remove 'priv' from mod %q to allow importing", found.Name),
				"",
				[]SecondarySpan{{Span: found.Sp, Label: "declared priv here"}},
				nil)
			tc.ok = false
			return
		}
		prog.ModBlocks = append(prog.ModBlocks, found)
		prog.Structs = append(prog.Structs, filterPublicStructs(found.Structs)...)
		return
	}

	for _, stmt := range imported.TopStmts {
		if fn, ok := stmt.(*FnDecl); ok {
			if fn.Vis == VisPrivate {
				tc.privFns[fn.Name] = filePath
				continue
			}
			prog.TopStmts = append(prog.TopStmts, fn)
		}
		if vd, ok := stmt.(*VarDecl); ok && vd.IsGlobal {
			if vd.Vis == VisPrivate {
				continue
			}
			prog.GlobalVars = append(prog.GlobalVars, vd)
			prog.TopStmts = append(prog.TopStmts, vd)
		}
	}
	for _, s := range imported.Structs {
		if s.Vis == VisPrivate {
			tc.privStructs[s.Name] = filePath
		}
	}
	for _, mc := range imported.Macros {
		if mc.Vis == VisPrivate {
			tc.privMacros[mc.Name] = filePath
		}
	}
	prog.Structs = append(prog.Structs, filterPublicStructs(imported.Structs)...)
	prog.Methods = append(prog.Methods, filterPublicMethods(imported.Methods)...)
	prog.ModBlocks = append(prog.ModBlocks, filterPublicMods(imported.ModBlocks)...)
	prog.Externs = append(prog.Externs, imported.Externs...)
	prog.Macros = append(prog.Macros, filterPublicMacros(imported.Macros)...)
}

func (tc *TypeChecker) registerPrivDecls(prog *Program, filePath string) {
	for _, stmt := range prog.TopStmts {
		if fn, ok := stmt.(*FnDecl); ok && fn.Vis == VisPrivate {
			tc.privDecls[filePath+"::"+fn.Name] = true
		}
	}
	for _, s := range prog.Structs {
		if s.Vis == VisPrivate {
			tc.privDecls[filePath+"::"+s.Name] = true
		}
	}
	for _, mc := range prog.Macros {
		if mc.Vis == VisPrivate {
			tc.privDecls[filePath+"::"+mc.Name] = true
		}
	}
	for _, mb := range prog.ModBlocks {
		tc.registerPrivDeclsInMod(mb, filePath)
	}
}

func (tc *TypeChecker) registerPrivDeclsInMod(mb *ModBlock, filePath string) {
	if mb.Vis == VisPrivate {
		tc.privDecls[filePath+"::"+mb.Path] = true
	}
	for _, fn := range mb.Fns {
		if fn.Vis == VisPrivate {
			tc.privDecls[filePath+"::"+mb.Path+"::"+fn.Name] = true
		}
	}
	for _, nested := range mb.Mods {
		tc.registerPrivDeclsInMod(nested, filePath)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
//  Import visibility filters
// ─────────────────────────────────────────────────────────────────────────────

func filterPublicStructs(ss []*StructDecl) []*StructDecl {
	var out []*StructDecl
	for _, s := range ss {
		if s.Vis == VisPublic {
			out = append(out, s)
		}
	}
	return out
}

func filterPublicMethods(ms []*MethodDecl) []*MethodDecl {
	var out []*MethodDecl
	for _, m := range ms {
		if m.Vis == VisPublic {
			out = append(out, m)
		}
	}
	return out
}

func filterPublicMods(mbs []*ModBlock) []*ModBlock {
	var out []*ModBlock
	for _, mb := range mbs {
		if mb.Vis == VisPublic {
			var pubFns []*FnDecl
			for _, fn := range mb.Fns {
				if fn.Vis == VisPublic {
					pubFns = append(pubFns, fn)
				}
			}
			var pubProps []*ModProperty
			for _, p := range mb.Properties {
				if p.Vis == VisPublic {
					pubProps = append(pubProps, p)
				}
			}
			cloned := *mb
			cloned.Fns = pubFns
			cloned.Properties = pubProps
			cloned.Mods = filterPublicMods(mb.Mods)
			out = append(out, &cloned)
		}
	}
	return out
}

func filterPublicMacros(mcs []*MacroDecl) []*MacroDecl {
	var out []*MacroDecl
	for _, mc := range mcs {
		if mc.Vis == VisPublic {
			out = append(out, mc)
		}
	}
	return out
}

// ─────────────────────────────────────────────────────────────────────────────
//  Import validation
// ─────────────────────────────────────────────────────────────────────────────

func (tc *TypeChecker) validateImport(imp *ImportDecl) {
	sp := imp.Sp
	switch {
	case imp.IsCHeader:
		if imp.Path == "" {
			errCode("EI01", sp,
				"import path is empty",
				`provide a header name: use "stdio.h"`)
			tc.ok = false
		}
		if tc.importPaths[imp.Path] {
			warnCode("W80", sp,
				fmt.Sprintf("header %q is imported more than once", imp.Path),
				"remove the duplicate import")
		}
	case imp.IsStdModule:
		if LookupStdModule(imp.Module) == nil {
			errCode("EI02", sp,
				fmt.Sprintf("unknown stdlib module %q", imp.Module),
				"available: std::str  std::io  std::math  std::sys  std::fs  std::cmd  std::mem  std::conv  std::time  std::net")
			tc.ok = false
		}
		if tc.importMods[imp.Module] {
			warnCode("W80", sp,
				fmt.Sprintf("module %q is imported more than once", imp.Module),
				"remove the duplicate import")
		}
	case imp.IsFileImport && imp.IsStd:
		if imp.EnvPrefix == "" {
			errCode("EI03", sp,
				"stdlib file import is missing an env prefix — this is a compiler bug",
				"report this issue")
			tc.ok = false
			return
		}
		if len(imp.Segments) == 0 {
			errCode("EI04", sp,
				"import requires at least one path segment after the prefix",
				"example: import std/net/socket")
			tc.ok = false
			return
		}
		for _, seg := range imp.Segments {
			if seg == "" || !isValidIdent(seg) {
				errCode("EI05", sp,
					fmt.Sprintf("invalid path segment %q in import", seg),
					"path segments can only contain letters, digits, and underscores")
				tc.ok = false
				return
			}
		}
	case imp.IsFileImport && imp.IsLocal:
		if len(imp.Segments) == 0 {
			errCode("EI07", sp,
				"local import requires at least one path segment",
				"example: import _/utils  or  import __/shared/types")
			tc.ok = false
			return
		}
		for _, seg := range imp.Segments {
			if seg == "" || !isValidIdent(seg) {
				errCode("EI08", sp,
					fmt.Sprintf("invalid path segment %q in local import", seg),
					"path segments can only contain letters, digits, and underscores")
				tc.ok = false
				return
			}
		}
		if imp.LocalFile == "" {
			errCode("EI10", sp,
				"could not resolve local import path",
				"check that the path segments are correct relative to the source file")
			tc.ok = false
		}
	}
}

func isValidIdent(s string) bool {
	if s == "" {
		return false
	}
	for i, c := range s {
		if i == 0 {
			if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_') {
				return false
			}
		} else {
			if !((c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') || c == '_') {
				return false
			}
		}
	}
	return true
}

// ─────────────────────────────────────────────────────────────────────────────
//  Mod block registration — fns + properties
// ─────────────────────────────────────────────────────────────────────────────

func (tc *TypeChecker) registerModFns(mb *ModBlock, declFile string) {
	tc.modNames[mb.Path] = true
	if mb.Name != mb.Path {
		if !tc.modNames[mb.Name] {
			tc.modNames[mb.Name] = true
		}
	}

	// functions
	for _, fn := range mb.Fns {
		fn.ModPath = declFile
		key := mb.Path + "::" + fn.Name
		tc.modFns[key] = fn
		if fn.Vis == VisPrivate {
			tc.privDecls[declFile+"::"+mb.Path+"::"+fn.Name] = true
		}
	}
	for _, td := range mb.Tests {
		fn := td.Fn
		key := mb.Path + "::" + fn.Name
		tc.modFns[key] = fn
	}

	// properties
	for _, prop := range mb.Properties {
		key := mb.Path + "::" + prop.Name
		tc.modProps[key] = prop
		if prop.Vis == VisPrivate {
			tc.privDecls[declFile+"::"+key] = true
		}
	}

	// structs inside mod
	for _, s := range mb.Structs {
		if _, exists := tc.structs[s.Name]; !exists {
			tc.structs[s.Name] = s
		}
		if s.Vis == VisPrivate {
			tc.privDecls[declFile+"::"+s.Name] = true
		}
	}

	// recurse into nested mods
	for _, nested := range mb.Mods {
		tc.registerModFns(nested, declFile)
	}
}

func (tc *TypeChecker) checkModFns(mb *ModBlock) {
	tc.trace.Push(mb.Sp, fmt.Sprintf("in mod '%s'", mb.Path))
	for _, fn := range mb.Fns {
		tc.checkFn(fn)
	}
	for _, td := range mb.Tests {
		tc.checkFn(td.Fn)
	}
	// check property get/set bodies if custom
	for _, prop := range mb.Properties {
		tc.checkModPropertyBodies(prop, mb.Path)
	}
	for _, nested := range mb.Mods {
		tc.checkModFns(nested)
	}
	tc.trace.Pop()
}

// checkModPropertyBodies checks user-supplied get/set bodies for a property.
func (tc *TypeChecker) checkModPropertyBodies(prop *ModProperty, modPath string) {
	// The C backing field name (e.g. "Counter__value" for mod Counter, property value).
	cFieldName := modPathToC(modPath) + "__" + prop.Name

	// The user-facing alias inside get/set bodies is "__propName"
	// (double underscore + property name). Much friendlier than the full C name.
	userAlias := "__" + prop.Name

	// Determine the property's C return type for the synthetic getter fn.
	propType := prop.Type
	if propType == nil {
		propType = TypAny
	}

	if prop.GetBody != nil {
		// Push a synthetic FnDecl so that checkReturn() knows we're inside a fn
		// and accepts `return` statements without firing E14.
		syntheticGet := &FnDecl{
			Sp:      prop.Sp,
			Name:    "__get_" + prop.Name,
			RetType: propType,
			Body:    prop.GetBody,
		}
		tc.fnStack = append(tc.fnStack, syntheticGet)
		tc.trace.Push(prop.Sp, fmt.Sprintf("in property '%s' getter", prop.Name))

		savedScope := tc.scope
		tc.scope = newScope(savedScope, "fn")
		// Expose both the C backing field name and the friendly __propName alias.
		tc.scope.define(cFieldName, &VarInfo{
			Type: propType, Sp: prop.Sp, Defined: true, DeclFile: tc.currentFile,
		})
		tc.scope.define(userAlias, &VarInfo{
			Type: propType, Sp: prop.Sp, Defined: true, DeclFile: tc.currentFile,
		})
		tc.checkBlock(prop.GetBody)
		tc.scope = savedScope

		tc.trace.Pop()
		tc.fnStack = tc.fnStack[:len(tc.fnStack)-1]
	}

	if prop.SetBody != nil {
		// Push a synthetic FnDecl so return statements work inside set bodies.
		syntheticSet := &FnDecl{
			Sp:      prop.Sp,
			Name:    "__set_" + prop.Name,
			RetType: TypVoid,
			Body:    prop.SetBody,
		}
		tc.fnStack = append(tc.fnStack, syntheticSet)
		tc.trace.Push(prop.Sp, fmt.Sprintf("in property '%s' setter", prop.Name))

		savedScope := tc.scope
		tc.scope = newScope(savedScope, "fn")
		// Expose the setter parameter (e.g. "v" or "value").
		param := prop.SetParam
		if param == "" {
			param = "value"
		}
		tc.scope.define(param, &VarInfo{
			Type: propType, Sp: prop.Sp, Defined: true, DeclFile: tc.currentFile,
		})
		// Expose both the C backing field name and the friendly __propName alias.
		tc.scope.define(cFieldName, &VarInfo{
			Type: propType, Sp: prop.Sp, Defined: true, DeclFile: tc.currentFile,
		})
		tc.scope.define(userAlias, &VarInfo{
			Type: propType, Sp: prop.Sp, Defined: true, DeclFile: tc.currentFile,
		})
		tc.checkBlock(prop.SetBody)
		tc.scope = savedScope

		tc.trace.Pop()
		tc.fnStack = tc.fnStack[:len(tc.fnStack)-1]
	}

	// Store the generated C field name on the property for the emitter.
	prop.CFieldName = cFieldName
}

// ─────────────────────────────────────────────────────────────────────────────
//  Utilities
// ─────────────────────────────────────────────────────────────────────────────

func editDistance(a, b string) int {
	la, lb := len(a), len(b)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}
	prev, curr := make([]int, lb+1), make([]int, lb+1)
	for j := 0; j <= lb; j++ {
		prev[j] = j
	}
	for i := 1; i <= la; i++ {
		curr[0] = i
		for j := 1; j <= lb; j++ {
			if a[i-1] == b[j-1] {
				curr[j] = prev[j-1]
			} else {
				curr[j] = 1 + min3(prev[j], curr[j-1], prev[j-1])
			}
		}
		prev, curr = curr, prev
	}
	return prev[lb]
}

func min3(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}

func listFields(params []Param) string {
	parts := make([]string, len(params))
	for i, p := range params {
		t := "any"
		if p.Type != nil {
			t = p.Type.String()
		}
		parts[i] = p.Name + ": " + t
	}
	return strings.Join(parts, ", ")
}

func listParamTypes(params []Param) string {
	parts := make([]string, len(params))
	for i, p := range params {
		t := "any"
		if p.Type != nil {
			t = p.Type.String()
		}
		parts[i] = p.Name + ": " + t
	}
	return strings.Join(parts, ", ")
}

func isBlockParam(name string, t *ZXType) bool {
	lower := strings.ToLower(name)
	switch lower {
	case "dostmt", "block", "body", "action", "fn", "callback",
		"handler", "stmt", "run", "exec", "proc", "closure", "do",
		"then", "ontrue", "onfalse", "iftrue", "iffalse":
		return true
	}
	if strings.HasSuffix(lower, "stmt") || strings.HasSuffix(lower, "block") ||
		strings.HasSuffix(lower, "fn") || strings.HasSuffix(lower, "func") ||
		strings.HasPrefix(lower, "do") || strings.HasPrefix(lower, "on") {
		return true
	}
	return false
}

func isCReservedFnTC(name string) bool {
	switch name {
	case "double", "float", "int", "char", "long", "short",
		"unsigned", "signed", "void", "struct", "union", "enum",
		"static", "extern", "const", "inline", "register", "auto",
		"volatile", "typedef", "return", "if", "else", "while",
		"for", "do", "switch", "case", "break", "continue",
		"goto", "default", "sizeof", "bool", "string":
		return true
	}
	return false
}
