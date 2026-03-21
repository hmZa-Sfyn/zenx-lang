package main

import (
	"fmt"
	"os"
	"strings"
)

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
	// NEW: which source file declared this (for priv enforcement)
	DeclFile string
	// NEW: visibility
	Vis Vis
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
func (s *Scope) depth() int {
	if s == nil {
		return 0
	}
	return 1 + s.parent.depth()
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

	returnSeen bool

	// NEW: current source file (set from program's source file path)
	currentFile string

	// NEW: call-stack trace builder
	trace TraceBuilder

	// NEW: map of imported file paths → their Program (for priv checks)
	importedProgs map[string]*Program

	// NEW: set of priv fn/struct/mod names by file, for import enforcement
	// key = "filename::name" → true means it is private
	privDecls map[string]bool

	// NEW: map of fn name → declaring file path, for every priv fn that was
	// skipped during import. Lets inferCall emit "this is private" instead of
	// "undefined function" when the caller tries to use a priv import.
	// key = fn name, value = file that declared it priv
	privFns map[string]string

	// Same for priv structs, macros, and mod-level fns.
	privStructs map[string]string // struct name → file
	privMacros  map[string]string // macro name → file

	// NEW: recursive call detection — tracks fn names in active call chain
	callChain map[string]bool

	// NEW: set of functions that have been seen to recurse (for infinite recursion warning)
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
			// NEW: warn on field named same as struct (common mistake)
			if f.Name == s.Name {
				warnCode("W90", f.Sp,
					fmt.Sprintf("field %q has the same name as its containing struct %q — this is usually a mistake",
						f.Name, s.Name),
					"rename the field to something descriptive")
			}
		}
		if len(s.Fields) == 0 {
			warnCode("W20", s.Sp,
				fmt.Sprintf("struct %q has no fields — it carries no data", s.Name),
				"add at least one field, or remove the struct if it is unused")
		}
		if len(s.Fields) > 32 {
			warnCode("W21", s.Sp,
				fmt.Sprintf("struct %q has %d fields — consider splitting it into smaller structs", s.Name, len(s.Fields)),
				"large structs are harder to maintain and may hurt cache performance")
		}
		// NEW: register priv structs
		if s.Vis == VisPrivate {
			tc.privDecls[file+"::"+s.Name] = true
		}
	}

	// ── externs + std fns ────────────────────────────────────────────────────
	for _, e := range prog.Externs {
		if existing := tc.scope.lookupLocal(e.Name); existing != nil {
			warnCode("W22", e.Sp,
				fmt.Sprintf("extern %q re-declared — previous declaration will be shadowed", e.Name),
				"remove the duplicate extern declaration")
		}
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
			if len(fn.Params) > 8 {
				warnCode("W23", fn.Sp,
					fmt.Sprintf("function %q has %d parameters — consider grouping them into a struct",
						fn.Name, len(fn.Params)),
					fmt.Sprintf("example: fn %s(cfg Config) { ... }", fn.Name))
			}
			// NEW: register priv fns
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

	// ── mod block function registration ──────────────────────────────────────
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

	// NEW: recursion / infinite-loop analysis pass
	tc.checkRecursion()

	// NEW: dead-code detection (functions defined but never called)
	tc.checkDeadCode()

	// unused-variable lint
	tc.checkUnusedVars()

	return tc.ok
}

// ── Privacy enforcement ───────────────────────────────────────────────────────

// isPrivateFrom returns true if the name declared in declFile is private
// and the current file is different.
func (tc *TypeChecker) isPrivateFrom(name, declFile string) bool {
	if declFile == "" || declFile == tc.currentFile {
		return false
	}
	key := declFile + "::" + name
	return tc.privDecls[key]
}

// ── Dead-code / unused-fn detection ──────────────────────────────────────────

// checkDeadCode warns on private top-level functions that are never called.
// Public functions are skipped — they may be called from other files.
func (tc *TypeChecker) checkDeadCode() {
	called := make(map[string]bool)

	// Walk every function body in the whole program.
	for _, fn := range tc.fns {
		tc.walkBody(fn.Body, called)
	}
	for _, m := range tc.methods {
		tc.walkBody(m.Body, called)
	}
	for _, mb := range tc.prog.ModBlocks {
		tc.walkModBlock(mb, called)
	}
	// Also walk top-level statements (script-style code outside any fn).
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
			continue // may be called from another file
		}
		if !called[name] {
			warnCode("W91", fn.Sp,
				fmt.Sprintf("private function %q is defined but never called — it is dead code", name),
				"remove it, or make it public if it is meant to be used from other files")
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

// walkNode is a complete, nil-safe recursive walker over every AST node type.
// It records every function name that appears in a call position into `called`.
func (tc *TypeChecker) walkNode(n Node, called map[string]bool) {
	if n == nil {
		return
	}
	switch s := n.(type) {
	// ── declarations ────────────────────────────────────────────────────────
	case *FnDecl:
		tc.walkBody(s.Body, called)
	case *VarDecl:
		tc.walkNode(s.Init, called)
	case *AssignStmt:
		tc.walkNode(s.LHS, called)
		tc.walkNode(s.Value, called)

	// ── call expressions ────────────────────────────────────────────────────
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

	// ── blocks & control flow ───────────────────────────────────────────────
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

	// ── expressions ─────────────────────────────────────────────────────────
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

	// literals and leaf nodes — nothing to walk
	case *Ident, *IntLit, *FloatLit, *BoolLit, *StrLit, *NilLit,
		*SizeofExpr, *TypeofExpr, *BreakStmt, *ContinueStmt:
		// no children
	}
}

// ── Recursion analysis ────────────────────────────────────────────────────────

// checkRecursion warns on direct recursion without a visible base case.
func (tc *TypeChecker) checkRecursion() {
	for name, fn := range tc.fns {
		if fn.Body == nil {
			continue
		}
		if tc.bodyCallsSelf(fn.Body.Stmts, name) {
			tc.recursiveFns[name] = true
			// Warn only if we cannot find an if/match guard (simple heuristic)
			if !tc.bodyHasGuardedReturn(fn.Body.Stmts) {
				warnCode("W92", fn.Sp,
					fmt.Sprintf("function %q appears to recurse without an obvious base case — potential infinite recursion", name),
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
			// If there's a return directly inside the if body (not recursive), treat as base case
			for _, st := range is.Then.Stmts {
				if _, ok := st.(*ReturnStmt); ok {
					return true
				}
			}
		}
	}
	return false
}

// ── Unused-variable lint ──────────────────────────────────────────────────────

func (tc *TypeChecker) checkUnusedVars() {
	for name, vi := range tc.scope.vars {
		if vi.IsFn || vi.IsExtern || vi.IsStd || vi.IsGlobal {
			continue
		}
		if vi.UsedCount == 0 && vi.Defined {
			warnCode("W30", vi.Sp,
				fmt.Sprintf("variable %q is declared but never used", name),
				"remove the declaration, or use _ as the variable name to suppress this warning")
		}
	}
}

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

// ── Macro checking ────────────────────────────────────────────────────────────

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

func (tc *TypeChecker) checkFn(fn *FnDecl) {
	saved := tc.scope
	tc.scope = newScope(saved, "fn")
	tc.fnStack = append(tc.fnStack, fn)
	tc.trace.Push(fn.Sp, fmt.Sprintf("in function '%s'", fn.Name))

	if isCReservedFnTC(fn.Name) {
		warnCode("W10", fn.Sp,
			fmt.Sprintf("function name %q shadows a C keyword — it will be compiled as __zx_%s", fn.Name, fn.Name),
			fmt.Sprintf("rename to avoid confusion: fn my_%s(...) { ... }", fn.Name))
	}

	// NEW: warn if fn body is empty
	if fn.Body != nil && len(fn.Body.Stmts) == 0 && fn.RetType != nil && fn.RetType.Kind != TyVoid {
		warnCode("W93", fn.Sp,
			fmt.Sprintf("function %q has an empty body but declares return type %s", fn.Name, fn.RetType),
			"add implementation or change return type to void")
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
		// NEW: warn if parameter type is void
		if t.Kind == TyVoid {
			errCode("E94", p2.Sp,
				fmt.Sprintf("parameter %q in function %q has type void — void cannot be used as a parameter type",
					p2.Name, fn.Name),
				"use any, int, str, or a struct type instead")
			tc.ok = false
		}
		if outer := saved.lookup(p2.Name); outer != nil && !outer.IsFn && !outer.IsExtern {
			warnCode("W01", p2.Sp,
				fmt.Sprintf("parameter %q shadows an outer variable", p2.Name),
				"rename the parameter to avoid confusion")
		}
		tc.scope.define(p2.Name, &VarInfo{
			Type: t, Sp: p2.Sp, Defined: true,
			DeclFile: tc.currentFile,
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
		tc.scope.define(p2.Name, &VarInfo{
			Type: t, Sp: p2.Sp, Defined: true, DeclFile: tc.currentFile,
		})
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
		condT := tc.inferExpr(s.Cond)
		if lit, ok := s.Cond.(*BoolLit); ok && !lit.Val {
			warnCode("W50", s.Sp,
				"unless condition is always false — this block will always run",
				"use an if statement or remove the condition")
		}
		_ = condT
		tc.checkBlock(s.Body)
		if s.Else != nil {
			tc.checkBlock(s.Else)
		}
	case *WhileStmt:
		condT := tc.inferExpr(s.Cond)
		if lit, ok := s.Cond.(*BoolLit); ok && lit.Val {
			if !blockHasBreak(s.Body) {
				warnCode("W51", s.Sp,
					"while(true) loop has no break — this may be an infinite loop",
					"add a break statement or a condition that eventually becomes false")
			}
		}
		// NEW: warn on while(false) — dead loop
		if lit, ok := s.Cond.(*BoolLit); ok && !lit.Val {
			warnCode("W95", s.Sp,
				"while(false) loop will never execute — this is dead code",
				"remove the loop or fix the condition")
		}
		_ = condT
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
	case *DeferStmt:
		if !tc.scope.inFn() {
			errCodeTrace("E60", s.Sp,
				"'defer' used outside a function — defer only runs at function exit",
				"move this defer inside a fn block",
				tc.trace.Snapshot())
			tc.ok = false
		}
		tc.inferExpr(s.Call)
	case *AssertStmt:
		cond := tc.inferExpr(s.Cond)
		if cond.Kind == TyVoid {
			errCodeTrace("E71", s.Sp,
				"assert condition has type void — void expressions cannot be true or false",
				"use a boolean expression or a comparison",
				tc.trace.Snapshot())
			tc.ok = false
		}
		if lit, ok := s.Cond.(*BoolLit); ok && lit.Val {
			warnCode("W52", s.Sp,
				"assert condition is always true — this assert does nothing",
				"remove it, or change the condition to something that can fail")
		}
		tc.inferExpr(s.Msg)
	case *SpawnStmt:
		tc.inferExpr(s.Call)
	case *ExprStmt:
		t := tc.inferExpr(s.Expr)
		if call, ok := s.Expr.(*CallExpr); ok {
			if id, ok2 := call.Func.(*Ident); ok2 {
				_ = id
			}
			if t != nil && t.Kind != TyVoid && t.Kind != TyAny {
				warnCode("W60", s.Sp,
					fmt.Sprintf("result of this call (type %s) is discarded", t),
					"assign the result to a variable, or cast to void if intentional: _ = expr")
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
				"use an integer expression as the exit code: exit 0  or  exit 1",
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
		// NEW: warn on repeat(0)
		if lit, ok := s.Count.(*IntLit); ok && lit.Val <= 0 {
			warnCode("W96", s.Sp,
				fmt.Sprintf("repeat count is %d — the block will never execute", lit.Val),
				"use a positive integer for the repeat count")
		}
		tc.checkBlockInLoop(s.Body)
	case *WithStmt:
		tc.inferExpr(s.Expr)
		saved := tc.scope
		tc.scope = newScope(saved, "block")
		tc.scope.define(s.As, &VarInfo{
			Type: tc.inferExpr(s.Expr), Sp: s.Sp,
			Defined: true, DeclFile: tc.currentFile,
		})
		tc.checkBlock(s.Body)
		tc.scope = saved
	}
}

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
					fmt.Sprintf("type mismatch: cannot assign %s to variable %q of type %s", initType, v.Name, resolved),
					fmt.Sprintf("change the declared type to %s, or convert: %s(%s)", initType, resolved, v.Name),
					fmt.Sprintf("let %s %s = %s(...)", v.Name, resolved, v.Name),
					nil, tc.trace.Snapshot())
				tc.ok = false
			}
		}
	}

	if resolved != nil && resolved.Kind == TyVoid {
		errCodeTrace("E12", v.Sp,
			fmt.Sprintf("variable %q cannot have type void — void means 'no value'", v.Name),
			"use int, float, str, bool, any, or a struct type instead",
			tc.trace.Snapshot())
		tc.ok = false
	}

	if v.IsConst && v.Init == nil {
		errCodeTrace("E13", v.Sp,
			fmt.Sprintf("const %q must have an initializer — constants must be assigned at declaration", v.Name),
			fmt.Sprintf("add a value: const %s = 42", v.Name),
			tc.trace.Snapshot())
		tc.ok = false
	}

	if v.IsConst && !v.IsGlobal && v.Name == strings.ToLower(v.Name) && len(v.Name) > 1 {
		warnCode("W02", v.Sp,
			fmt.Sprintf("const %q should be UPPER_CASE by convention", v.Name),
			fmt.Sprintf("rename to %s", strings.ToUpper(v.Name)))
	}

	if outer := tc.scope.parent; outer != nil {
		if o2 := outer.lookup(v.Name); o2 != nil && !o2.IsFn && !o2.IsExtern {
			warnCode("W01", v.Sp,
				fmt.Sprintf("variable %q shadows an outer variable declared at %s:%d",
					v.Name, o2.Sp.File, o2.Sp.Line),
				"rename to avoid confusion")
		}
	}

	if v.Init != nil {
		if _, isNil := v.Init.(*NilLit); isNil {
			if resolved != nil && resolved.Kind != TyRef && resolved.Kind != TyAny && resolved.Kind != TyUnknown {
				errCodeTrace("E73", v.Sp,
					fmt.Sprintf("cannot assign nil to variable %q of type %s — nil is only valid for ref types",
						v.Name, resolved),
					"declare it as ref<T> if you need a nullable pointer",
					tc.trace.Snapshot())
				tc.ok = false
			}
		}
	}

	// NEW: warn on single-char variable names (except i, j, k loop vars)
	if len(v.Name) == 1 && v.Name != "i" && v.Name != "j" && v.Name != "k" &&
		v.Name != "n" && v.Name != "x" && v.Name != "y" && v.Name != "z" {
		warnCode("W97", v.Sp,
			fmt.Sprintf("variable name %q is a single character — prefer descriptive names", v.Name),
			"use a name that describes the variable's purpose")
	}

	if len(v.Name) > 50 {
		warnCode("W31", v.Sp,
			fmt.Sprintf("variable name %q is very long (%d characters)", v.Name, len(v.Name)),
			"consider a shorter, more descriptive name")
	}

	// NEW: warn if initializing a bool with a non-bool literal
	if resolved != nil && resolved.Kind == TyBool && initType != nil {
		if initType.Kind == TyInt {
			warnCode("W98", v.Sp,
				fmt.Sprintf("variable %q is declared bool but initialized with an integer — use true/false", v.Name),
				"replace: true or false")
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
			fmt.Sprintf("function %q is declared void but returns a %s value", tc.currentName(), got),
			fmt.Sprintf("remove the return value, or change the return type to '-> %s'", got),
			tc.trace.Snapshot())
		tc.ok = false
		return
	}
	if got.Kind != TyUnknown && got.Kind != TyAny && ret.Kind != TyAny && !coercible(got, ret) {
		errFull("E17", r.Sp,
			fmt.Sprintf("return type mismatch in %q — expected %s but got %s", tc.currentName(), ret, got),
			fmt.Sprintf("cast the value: %s(value)  —  or change the return type to '-> %s'", ret, got),
			fmt.Sprintf("return %s(value)", ret),
			nil,
			tc.trace.Snapshot())
		tc.ok = false
	}
}

func (tc *TypeChecker) checkIf(s *IfStmt) {
	cond := tc.inferExpr(s.Cond)
	if cond.Kind == TyVoid {
		errCodeTrace("E18", s.Cond.nodeSpan(),
			"if condition has type void — void expressions have no truth value",
			"use a comparison expression that produces a bool or int",
			tc.trace.Snapshot())
		tc.ok = false
	}
	if lit, ok := s.Cond.(*BoolLit); ok {
		if lit.Val {
			warnCode("W50", s.Sp,
				"if condition is always true — the else branch (if any) will never run",
				"remove the condition or replace with unconditional code")
		} else {
			warnCode("W50", s.Sp,
				"if condition is always false — the then branch will never run",
				"remove the if block or fix the condition")
		}
	}
	// NEW: warn if if-body and else-body are identical
	if s.Else != nil && blocksEqual(s.Then, s.Else) {
		warnCode("W99", s.Sp,
			"both branches of this if/else are identical — the condition has no effect",
			"remove the condition or make the branches different")
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
			// NEW: very large range
			if toLit.Val-fromLit.Val > 10_000_000 {
				warnCode("WP1", s.Sp,
					fmt.Sprintf("for-range %d..%d iterates %d times — this may be a performance issue",
						fromLit.Val, toLit.Val, toLit.Val-fromLit.Val),
					"ensure this loop count is intentional")
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
					"duplicate wildcard arm in match — only the first will ever match",
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
	// NEW: warn on match with no wildcard (exhaustiveness hint)
	if !hasWild && len(s.Arms) > 0 {
		warnCode("WE1", s.Sp,
			"match has no wildcard arm — unmatched values will silently fall through",
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
				fmt.Sprintf("cannot assign to function %q — functions are not variables", id.Name),
				"declare a variable to hold the result: let result = "+id.Name+"(...)",
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
				hint,
				fmt.Sprintf("let %s = ...", id.Name),
				nil, tc.trace.Snapshot())
			tc.ok = false
			return
		}
		// NEW: warn on assigning a variable to itself
		if rhs, ok2 := s.Value.(*Ident); ok2 && rhs.Name == id.Name {
			warnCode("W64", s.Sp,
				fmt.Sprintf("assigning variable %q to itself — this has no effect", id.Name),
				"remove the assignment or check the variable names")
		}
	}

	switch s.LHS.(type) {
	case *IntLit, *FloatLit, *BoolLit, *StrLit:
		errCodeTrace("E24", s.Sp,
			"left side of assignment is a literal — you cannot assign to a value",
			"use a variable name on the left side: let x = ...",
			tc.trace.Snapshot())
		tc.ok = false
		return
	}

	if _, ok := s.LHS.(*CallExpr); ok {
		errCodeTrace("E76", s.Sp,
			"left side of assignment is a function call — call results are not assignable",
			"store the result in a variable first: let x = f(); x = ...",
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
				fmt.Sprintf("cast the right-hand side: %s(expr)", lhsType),
				fmt.Sprintf("%s(%s)", lhsType, "expr"),
				nil, tc.trace.Snapshot())
			tc.ok = false
		}
	}
	if s.Op != "=" && lhsType.Kind != TyAny && !isNumeric(lhsType) {
		errCodeTrace("E26", s.Sp,
			fmt.Sprintf("compound operator %s requires a numeric operand, but variable is %s", s.Op, lhsType),
			"compound assignment (+=, -=, *=, /=, %=) only works on int, float, and char",
			tc.trace.Snapshot())
		tc.ok = false
	}
}

// ── Inference ─────────────────────────────────────────────────────────────────

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
		if len(e.Val) > 4096 {
			warnCode("W32", e.Sp,
				fmt.Sprintf("string literal is very long (%d bytes) — consider loading it from a file", len(e.Val)),
				"use readfile!() or a multiline string @`...` for large text")
		}
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
				"ternary condition has type void — void expressions have no truth value",
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
				fmt.Sprintf("cast one branch to match the other, e.g. %s(expr)", then))
		}
		e.Typ = then
		return then

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
			for modKey := range tc.modFns {
				parts := strings.SplitN(modKey, "::", 2)
				if len(parts) == 2 && parts[1] == e.Name {
					errCodeTrace("E81", e.Sp,
						fmt.Sprintf("%q is a mod-private function — it must be called with its module name", e.Name),
						fmt.Sprintf("use: %s->%s()", parts[0], e.Name),
						tc.trace.Snapshot())
					tc.ok = false
					e.Typ = TypUnknown
					return TypUnknown
				}
			}
			suggestion := tc.suggestName(e.Name)
			hint := fmt.Sprintf("declare it: let %s = ...", e.Name)
			if suggestion != "" {
				hint = fmt.Sprintf("did you mean %q?", suggestion)
			}
			// Check if this name is a private declaration from an imported file
			// and give a specific error instead of the generic "undefined name".
			if declFile, isPriv := tc.privFns[e.Name]; isPriv {
				errFull("EP1", e.Sp,
					fmt.Sprintf("cannot use %q — it is declared 'priv' in %q and is not accessible from this file",
						e.Name, declFile),
					fmt.Sprintf("remove 'priv' from fn %q in %q to make it importable", e.Name, declFile),
					"", nil, tc.trace.Snapshot())
				tc.ok = false
				e.Typ = TypUnknown
				return TypUnknown
			}
			if declFile, isPriv := tc.privStructs[e.Name]; isPriv {
				errFull("EP4", e.Sp,
					fmt.Sprintf("cannot use struct %q — it is declared 'priv' in %q and is not accessible from this file",
						e.Name, declFile),
					fmt.Sprintf("remove 'priv' from struct %q in %q to make it importable", e.Name, declFile),
					"", nil, tc.trace.Snapshot())
				tc.ok = false
				e.Typ = TypUnknown
				return TypUnknown
			}
			if declFile, isPriv := tc.privMacros[e.Name]; isPriv {
				errFull("EP3", e.Sp,
					fmt.Sprintf("cannot use macro %q — it is declared 'priv' in %q and is not accessible from this file",
						e.Name, declFile),
					fmt.Sprintf("remove 'priv' from macro %q in %q to make it importable", e.Name, declFile),
					"", nil, tc.trace.Snapshot())
				tc.ok = false
				e.Typ = TypUnknown
				return TypUnknown
			}
			errFull("E27", e.Sp,
				fmt.Sprintf("undefined name %q", e.Name),
				hint, "",
				nil, tc.trace.Snapshot())
			tc.ok = false
			e.Typ = TypUnknown
			return TypUnknown
		}

		// NEW: priv access check
		if vi.DeclFile != "" && vi.DeclFile != tc.currentFile &&
			vi.Vis == VisPrivate {
			errFull("EP1", e.Sp,
				fmt.Sprintf("cannot access private name %q from file %q — it is declared priv in %q",
					e.Name, tc.currentFile, vi.DeclFile),
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
		if lit, ok := e.Idx.(*UnaryExpr); ok && lit.Op == "-" {
			if _, isInt := lit.Operand.(*IntLit); isInt {
				warnCode("W62", e.Idx.nodeSpan(),
					"negative index — this will likely be out of bounds at runtime",
					"use len(arr) - N for reverse indexing")
			}
		}
		// NEW: index out of bounds for array literals with known size
		if obj.Kind == TyArray && obj.ArrSize > 0 {
			if idxLit, ok := e.Idx.(*IntLit); ok {
				if idxLit.Val < 0 || idxLit.Val >= int64(obj.ArrSize) {
					errCodeTrace("EOB", e.Sp,
						fmt.Sprintf("index %d is out of bounds for array of size %d",
							idxLit.Val, obj.ArrSize),
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
				"use an array [1,2,3] or a string \"abc\"",
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
				"valid casts: between numeric types (int/float/char/bool), and between str and ref char",
				tc.trace.Snapshot())
			tc.ok = false
		}
		if typeEq(from, e.ToType) {
			warnCode("W63", e.Sp,
				fmt.Sprintf("redundant cast — expression is already of type %s", e.ToType),
				"remove the cast")
		}
		e.Typ = e.ToType
		return e.ToType

	case *AddrExpr:
		inner := tc.inferExpr(e.Operand)
		if e.Deref {
			if inner.Kind != TyRef {
				errCodeTrace("E29", e.Sp,
					fmt.Sprintf("cannot dereference type %s — only ref<T> values can be dereferenced", inner),
					"use ^ on a ref variable, e.g. ^myPtr",
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
					fmt.Sprintf("array element %d has type %s but the first element is %s — all elements must have the same type",
						i+2, got, first),
					fmt.Sprintf("cast this element: %s(value)", first),
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
		// NEW: type-check lambda body
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

func (tc *TypeChecker) inferModCall(e *ModCallExpr) *ZXType {
	if !tc.modNames[e.Mod] {
		bestMod := tc.suggestModName(e.Mod)
		hint := fmt.Sprintf("declare it: mod %s { fn %s() { ... } }", e.Mod, e.Fn)
		if bestMod != "" {
			hint = fmt.Sprintf("did you mean mod %q?", bestMod)
		}
		errCodeTrace("E82", e.Sp,
			fmt.Sprintf("unknown module %q — no mod block with this name exists", e.Mod),
			hint, tc.trace.Snapshot())
		tc.ok = false
		e.Typ = TypUnknown
		return TypUnknown
	}
	key := e.Mod + "::" + e.Fn
	fn, ok := tc.modFns[key]
	if !ok {
		var available []string
		prefix := e.Mod + "::"
		for k := range tc.modFns {
			if strings.HasPrefix(k, prefix) {
				available = append(available, strings.TrimPrefix(k, prefix))
			}
		}
		hint := fmt.Sprintf("mod %q has no function %q", e.Mod, e.Fn)
		if len(available) > 0 {
			hint += fmt.Sprintf(" — available functions: %s", strings.Join(available, ", "))
		}
		errCodeTrace("E83", e.Sp, hint,
			fmt.Sprintf("define it: mod %s { fn %s() { ... } }", e.Mod, e.Fn),
			tc.trace.Snapshot())
		tc.ok = false
		e.Typ = TypUnknown
		return TypUnknown
	}

	// NEW: priv mod fn access check
	if fn.Vis == VisPrivate && fn.ModPath != tc.currentFile {
		errFull("EP2", e.Sp,
			fmt.Sprintf("cannot call private function %s->%s from file %q — it is declared priv",
				e.Mod, e.Fn, tc.currentFile),
			fmt.Sprintf("remove the 'priv' modifier on fn %s in mod %s to make it callable", e.Fn, e.Mod),
			"",
			[]SecondarySpan{{Span: fn.Sp, Label: "declared priv here"}},
			tc.trace.Snapshot())
		tc.ok = false
		e.Typ = TypUnknown
		return TypUnknown
	}

	if !fn.Variadic && len(e.Args) != len(fn.Params) {
		minArgs := 0
		for _, p := range fn.Params {
			if p.Default == nil {
				minArgs++
			}
		}
		if len(e.Args) < minArgs || (!fn.Variadic && len(e.Args) > len(fn.Params)) {
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
					fmt.Sprintf("cast with: %s(value)", expected),
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

func (tc *TypeChecker) inferMacroCall(e *MacroCallExpr) *ZXType {
	mc, ok := tc.macros[e.Name]
	if !ok {
		// Check if this is a private macro from an imported file.
		if declFile, isPriv := tc.privMacros[e.Name]; isPriv {
			errFull("EP3", e.Sp,
				fmt.Sprintf("cannot call macro %q — it is declared 'priv' in %q and is not accessible from this file",
					e.Name, declFile),
				fmt.Sprintf("remove 'priv' from macro %q in %q to make it importable", e.Name, declFile),
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
	// NEW: priv macro check
	if mc.Vis == VisPrivate && tc.currentFile != "" {
		if vi := tc.scope.lookup(e.Name); vi != nil && vi.DeclFile != tc.currentFile {
			errFull("EP3", e.Sp,
				fmt.Sprintf("cannot call private macro %q from file %q", e.Name, tc.currentFile),
				fmt.Sprintf("remove 'priv' from macro %q to make it accessible", e.Name),
				"", nil, tc.trace.Snapshot())
			tc.ok = false
			e.Typ = TypUnknown
			return TypUnknown
		}
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
			expected := mc.Params[i].Type
			if expected != nil && expected.Kind != TyAny &&
				argType.Kind != TyAny && argType.Kind != TyUnknown &&
				!coercible(argType, expected) {
				errFull("EM09", arg.nodeSpan(),
					fmt.Sprintf("macro %q argument %d: expected %s but got %s",
						e.Name, i+1, expected, argType),
					fmt.Sprintf("cast with: %s(value)", expected),
					fmt.Sprintf("%s(value)", expected),
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
			hint := fmt.Sprintf("declare it: macro fn %s |input, doStmt| -> |output| { output = input; if input { doStmt; } }", step.Macro)
			errCodeTrace("EM07", step.Sp,
				fmt.Sprintf("undefined macro %q in chain", step.Macro),
				hint, tc.trace.Snapshot())
			tc.ok = false
			tc.checkBlock(step.Body)
			continue
		}
		tc.checkBlock(step.Body)
		if mc.RetType != nil && mc.RetType.Kind != TyVoid {
			lastType = mc.RetType
		}
		if len(mc.Outputs) > 0 && mc.RetType != nil {
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
		if isSameIdent(e.LHS, e.RHS) {
			warnCode("W64", e.Sp,
				fmt.Sprintf("comparing a variable to itself with %s — result is always %v", e.Op, e.Op == "=="),
				"this comparison is redundant; check the variable names")
		}
		// NEW: warn on comparing str with == (should use str_eq)
		if lhs.Kind == TyStr || rhs.Kind == TyStr {
			warnCode("WS1", e.Sp,
				"comparing strings with == compares pointers, not contents — use str_eq!(a, b) for value comparison",
				"replace: str_eq!(a, b)")
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
		if isSameIdent(e.LHS, e.RHS) {
			warnCode("W64", e.Sp,
				fmt.Sprintf("comparing a variable to itself with %s — result is always %v",
					e.Op, e.Op == "<=" || e.Op == ">="),
				"this comparison is redundant")
		}
		e.Typ = TypBool
		return TypBool
	case "&&", "||":
		if lhs.Kind != TyAny && lhs.Kind != TyUnknown && !isTruthy(lhs) {
			warnCode("W65", e.LHS.nodeSpan(),
				fmt.Sprintf("left operand of %s has type %s — only bool/int/ref are truthy", e.Op, lhs),
				"cast to bool: bool(expr)")
		}
		if rhs.Kind != TyAny && rhs.Kind != TyUnknown && !isTruthy(rhs) {
			warnCode("W65", e.RHS.nodeSpan(),
				fmt.Sprintf("right operand of %s has type %s — only bool/int/ref are truthy", e.Op, rhs),
				"cast to bool: bool(expr)")
		}
		e.Typ = TypBool
		return TypBool
	case "+":
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
				fmt.Sprintf("bitwise operator %q requires integer operands but left side is %s", e.Op, lhs),
				"bitwise operators only work on int and char",
				tc.trace.Snapshot())
			tc.ok = false
		}
		if rhs.Kind != TyAny && rhs.Kind != TyUnknown && !isInteger(rhs) {
			errCodeTrace("E36", e.Sp,
				fmt.Sprintf("bitwise operator %q requires integer operands but right side is %s", e.Op, rhs),
				"bitwise operators only work on int and char",
				tc.trace.Snapshot())
			tc.ok = false
		}
		if e.Op == "<<" || e.Op == ">>" {
			if lit, ok := e.RHS.(*IntLit); ok {
				if lit.Val < 0 {
					errCodeTrace("E77", e.RHS.nodeSpan(),
						fmt.Sprintf("shift by negative amount %d — this is undefined behaviour", lit.Val),
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
		// NEW: double-negation hint
		if inner.Kind == TyBool {
			if _, isBang := e.Operand.(*UnaryExpr); isBang {
				warnCode("WN1", e.Sp,
					"double negation !! — use the expression directly instead",
					"replace: !!x → x")
			}
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
			minArgs := len(sf.Params)
			if len(e.Args) != minArgs {
				errCodeTrace("E41", e.Sp,
					fmt.Sprintf("std function %q expects %d argument(s) but got %d",
						fnName, minArgs, len(e.Args)),
					fmt.Sprintf("signature: %s(%s)", fnName, listParamTypes(sf.Params)),
					tc.trace.Snapshot())
				tc.ok = false
			}
		} else {
			minArgs := len(sf.Params)
			if len(e.Args) < minArgs {
				errCodeTrace("E41", e.Sp,
					fmt.Sprintf("std function %q expects at least %d argument(s) but got %d",
						fnName, minArgs, len(e.Args)),
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
						fmt.Sprintf("cast with: %s(value)", expected),
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
						fmt.Sprintf("cast with: %s(value)", expected),
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
		// NEW: priv fn access check across files
		if fn.Vis == VisPrivate && fn.ModPath != "" && fn.ModPath != tc.currentFile {
			errFull("EP1", e.Sp,
				fmt.Sprintf("cannot call private function %q from file %q", fnName, tc.currentFile),
				fmt.Sprintf("remove 'priv' from fn %q in %q to allow external calls", fnName, fn.ModPath),
				"",
				[]SecondarySpan{{Span: fn.Sp, Label: "declared priv here"}},
				tc.trace.Snapshot())
			tc.ok = false
			e.Typ = TypUnknown
			return TypUnknown
		}

		if !fn.Variadic && len(e.Args) != len(fn.Params) {
			minArgs := 0
			for _, p2 := range fn.Params {
				if p2.Default == nil {
					minArgs++
				}
			}
			if len(e.Args) < minArgs || (!fn.Variadic && len(e.Args) > len(fn.Params)) {
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
						fmt.Sprintf("cast with: %s(value)", expected),
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
			// Check if this name is a private function from an imported file.
			// Give a specific "it's private" error instead of "undefined".
			if declFile, isPriv := tc.privFns[fnName]; isPriv {
				errFull("EP1", e.Sp,
					fmt.Sprintf("cannot call %q — it is declared 'priv' in %q and is not accessible from this file",
						fnName, declFile),
					fmt.Sprintf("remove 'priv' from fn %q in %q to make it importable", fnName, declFile),
					"",
					nil,
					tc.trace.Snapshot())
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
				hint, "",
				nil, tc.trace.Snapshot())
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
	if recvType.Kind == TyStruct {
		structName = recvType.Name
	}
	if recvType.Kind == TyRef && recvType.Elem != nil && recvType.Elem.Kind == TyStruct {
		structName = recvType.Elem.Name
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

	if e.UsedDot && objType.Kind == TyRef {
		funnyWarn(e.Sp,
			"used '.' on a ref — it works, but use '->' for pointer field access",
			"replace '.' with '->' for consistency with C conventions")
	}

	if eff.Kind != TyStruct {
		if eff.Kind != TyAny && eff.Kind != TyUnknown {
			errCodeTrace("E48", e.Sp,
				fmt.Sprintf("cannot access field %q on type %s — field access is only valid on struct types",
					e.Field, objType),
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
	// NEW: priv struct access check
	if sd.Vis == VisPrivate && tc.privDecls[tc.currentFile+"::"+sd.Name] {
		// Already in same file, allow
	} else if sd.Vis == VisPrivate && sd.Sp.File != "" && sd.Sp.File != tc.currentFile {
		errFull("EP4", e.Sp,
			fmt.Sprintf("cannot access fields of private struct %q from file %q",
				sd.Name, tc.currentFile),
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
		hint, "",
		nil, tc.trace.Snapshot())
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
		// Check if this is a private struct from an imported file.
		if declFile, isPriv := tc.privStructs[e.Name]; isPriv {
			errFull("EP5", e.Sp,
				fmt.Sprintf("cannot construct struct %q — it is declared 'priv' in %q and is not accessible from this file",
					e.Name, declFile),
				fmt.Sprintf("remove 'priv' from struct %q in %q to make it importable", e.Name, declFile),
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
	// NEW: priv struct init from other file
	if sd.Vis == VisPrivate && sd.Sp.File != "" && sd.Sp.File != tc.currentFile {
		errFull("EP5", e.Sp,
			fmt.Sprintf("cannot construct private struct %q from file %q", e.Name, tc.currentFile),
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
					fmt.Sprintf("cast with: %s(value)", sf.Type),
					fmt.Sprintf("%s(value)", sf.Type),
					nil, tc.trace.Snapshot())
				tc.ok = false
				break
			}
		}
	}

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

// ── Type helpers ──────────────────────────────────────────────────────────────

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

// ── AST helpers ───────────────────────────────────────────────────────────────

func blockHasBreak(b *Block) bool {
	if b == nil {
		return false
	}
	for _, s := range b.Stmts {
		if _, ok := s.(*BreakStmt); ok {
			return true
		}
		if is, ok := s.(*IfStmt); ok {
			if blockHasBreak(is.Then) {
				return true
			}
		}
	}
	return false
}

func isSameIdent(a, b Node) bool {
	ia, oka := a.(*Ident)
	ib, okb := b.(*Ident)
	return oka && okb && ia.Name == ib.Name
}

// blocksEqual does a shallow structural equality check on two blocks.
// Used to detect if/else branches that are identical.
func blocksEqual(a, b *Block) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	if len(a.Stmts) != len(b.Stmts) {
		return false
	}
	// simple tag-based check — good enough for the warning
	for i := range a.Stmts {
		if a.Stmts[i] == nil || b.Stmts[i] == nil {
			continue
		}
		if a.Stmts[i].nodeTag() != b.Stmts[i].nodeTag() {
			return false
		}
	}
	return true
}

// ── File import resolution ────────────────────────────────────────────────────

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

	// NEW: register priv declarations from the imported file so we can enforce them
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
		// NEW: block importing a priv mod block
		if found.Vis == VisPrivate {
			errFull("EP6", sp,
				fmt.Sprintf("cannot import private mod %q from file %q — it is declared priv",
					found.Name, filePath),
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
			// priv functions: skip from import but record them so inferCall
			// can give a proper "this function is private" error.
			if fn.Vis == VisPrivate {
				tc.privFns[fn.Name] = filePath
				continue
			}
			if _, exists := tc.fns[fn.Name]; exists {
				warnAt(sp,
					fmt.Sprintf("imported function %q shadows an existing function — rename one to avoid confusion", fn.Name),
					"")
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
	// Record priv structs and macros too so field-access / call errors are precise.
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

// registerPrivDecls walks an imported program and registers all priv names
// so the typechecker can deny access attempts.
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
		if mb.Vis == VisPrivate {
			tc.privDecls[filePath+"::"+mb.Name] = true
		}
		for _, fn := range mb.Fns {
			if fn.Vis == VisPrivate {
				tc.privDecls[filePath+"::"+mb.Name+"::"+fn.Name] = true
			}
		}
	}
}

// ── Filter helpers for import visibility ──────────────────────────────────────

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
			// Also filter priv fns within the mod
			var pubFns []*FnDecl
			for _, fn := range mb.Fns {
				if fn.Vis == VisPublic {
					pubFns = append(pubFns, fn)
				}
			}
			cloned := *mb
			cloned.Fns = pubFns
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

// ── Import validation ─────────────────────────────────────────────────────────

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
				"available modules: std::str  std::io  std::math  std::sys  std::fs  std::cmd  std::mem  std::conv  std::time  std::net")
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
					fmt.Sprintf("invalid path segment %q in import — segments must be valid identifiers", seg),
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

// ── Mod block registration ────────────────────────────────────────────────────

func (tc *TypeChecker) registerModFns(mb *ModBlock, declFile string) {
	tc.modNames[mb.Name] = true
	for _, fn := range mb.Fns {
		fn.ModPath = declFile // track which file owns this fn
		key := mb.Name + "::" + fn.Name
		tc.modFns[key] = fn
		if fn.Vis == VisPrivate {
			tc.privDecls[declFile+"::"+mb.Name+"::"+fn.Name] = true
		}
	}
	for _, td := range mb.Tests {
		fn := td.Fn
		key := mb.Name + "::" + fn.Name
		tc.modFns[key] = fn
	}
	for _, s := range mb.Structs {
		if _, exists := tc.structs[s.Name]; !exists {
			tc.structs[s.Name] = s
		}
		if s.Vis == VisPrivate {
			tc.privDecls[declFile+"::"+s.Name] = true
		}
	}
	for _, nested := range mb.Mods {
		tc.registerModFns(nested, declFile)
	}
}

func (tc *TypeChecker) checkModFns(mb *ModBlock) {
	tc.trace.Push(mb.Sp, fmt.Sprintf("in mod '%s'", mb.Name))
	for _, fn := range mb.Fns {
		tc.checkFn(fn)
	}
	for _, td := range mb.Tests {
		tc.checkFn(td.Fn)
	}
	for _, nested := range mb.Mods {
		tc.checkModFns(nested)
	}
	tc.trace.Pop()
}

// ── Utility ───────────────────────────────────────────────────────────────────

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
