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
	IsGlobal  bool   // declared with 'our'
	IsModFn   bool   // belongs to a mod block — NOT accessible by plain name
	ModName   string // which mod block owns this fn
	Sp        Span
	UsedCount int
	Defined   bool
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

// depth returns how many scopes are nested (for shadowing analysis)
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
	prog          *Program
	scope         *Scope
	fnStack       []*FnDecl
	methodStack   []*MethodDecl
	macroStack    []*MacroDecl
	structs       map[string]*StructDecl
	fns           map[string]*FnDecl
	methods       map[string]*MethodDecl
	macros        map[string]*MacroDecl
	externs       map[string]*ExternDecl
	stdFns        map[string]*StdFn
	importPaths   map[string]bool
	importMods    map[string]bool
	deferredCalls []Node
	ok            bool

	// modFns maps "modName::fnName" → *FnDecl for namespace-qualified lookup.
	modFns   map[string]*FnDecl
	modNames map[string]bool

	// returnSeen tracks whether the current fn has a reachable return
	// (used for non-void functions that appear to have no return)
	returnSeen bool
}

func TypeCheck(prog *Program, src, file string) bool {
	tc := &TypeChecker{
		prog:    prog,
		structs: make(map[string]*StructDecl), fns: make(map[string]*FnDecl),
		methods: make(map[string]*MethodDecl), macros: make(map[string]*MacroDecl),
		externs: make(map[string]*ExternDecl), stdFns: make(map[string]*StdFn),
		importPaths: make(map[string]bool), importMods: make(map[string]bool),
		modFns: make(map[string]*FnDecl), modNames: make(map[string]bool),
		ok: true,
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

	// ── E01: struct registration ─────────────────────────────────────────────
	for _, s := range prog.Structs {
		if existing, exists := tc.structs[s.Name]; exists {
			errCodeSecondary("E01", s.Sp,
				fmt.Sprintf("struct %q is defined more than once", s.Name),
				"rename one of these struct definitions",
				[]SecondarySpan{{Span: existing.Sp, Label: "first defined here"}})
			tc.ok = false
		}
		tc.structs[s.Name] = s

		// ── E02: duplicate struct fields ─────────────────────────────────────
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

		// ── NEW: warn on empty structs ────────────────────────────────────────
		if len(s.Fields) == 0 {
			warnCode("W20", s.Sp,
				fmt.Sprintf("struct %q has no fields — it carries no data", s.Name),
				"add at least one field, or remove the struct if it is unused")
		}

		// ── NEW: warn on very large structs (> 32 fields) ────────────────────
		if len(s.Fields) > 32 {
			warnCode("W21", s.Sp,
				fmt.Sprintf("struct %q has %d fields — consider splitting it into smaller structs", s.Name, len(s.Fields)),
				"large structs are harder to maintain and may hurt cache performance")
		}
	}

	// ── register externs + std fns ───────────────────────────────────────────
	for _, e := range prog.Externs {
		if existing := tc.scope.lookupLocal(e.Name); existing != nil {
			warnCode("W22", e.Sp,
				fmt.Sprintf("extern %q re-declared — previous declaration will be shadowed", e.Name),
				"remove the duplicate extern declaration")
		}
		tc.externs[e.Name] = e
		tc.scope.define(e.Name, &VarInfo{Type: e.RetType, IsFn: true, IsExtern: true, Sp: e.Sp})
	}
	for name, fn := range tc.stdFns {
		f := fn
		tc.scope.define(name, &VarInfo{Type: f.Ret, IsFn: true, IsStd: true})
	}

	// ── E03: top-level function registration ─────────────────────────────────
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
			tc.scope.define(fn.Name, &VarInfo{Type: fn.RetType, IsFn: true, Sp: fn.Sp})

			// ── NEW: warn on suspiciously long parameter lists ────────────────
			if len(fn.Params) > 8 {
				warnCode("W23", fn.Sp,
					fmt.Sprintf("function %q has %d parameters — consider grouping them into a struct", fn.Name, len(fn.Params)),
					fmt.Sprintf("example: fn %s(cfg Config) { ... }", fn.Name))
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
		tc.scope.define(key, &VarInfo{Type: m.RetType, IsFn: true, IsMethod: true, Sp: m.Sp})
		if _, ok := tc.structs[m.RecvType]; !ok {
			errCode("E57", m.Sp,
				fmt.Sprintf("method %q defined on unknown struct type %q", m.Name, m.RecvType),
				fmt.Sprintf("declare the struct first: type %s struct { ... }", m.RecvType))
			tc.ok = false
		}
	}

	// ── mod block function registration ──────────────────────────────────────
	for _, mb := range prog.ModBlocks {
		tc.registerModFns(mb)
	}

	// ── EM01–EM10: macro registration ────────────────────────────────────────
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
		tc.scope.define(mc.Name, &VarInfo{Type: mc.RetType, IsFn: true, Sp: mc.Sp})
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

	// ── define 'our' globals so non-fn stmts can see them ───────────────────
	for _, vd := range prog.GlobalVars {
		tc.checkVarDecl(vd)
	}

	// ── NEW: unused-variable lint pass ───────────────────────────────────────
	tc.checkUnusedVars()

	return tc.ok
}

// checkUnusedVars walks all registered scopes and warns on unused local vars.
// We only warn — never error — for unused variables.
func (tc *TypeChecker) checkUnusedVars() {
	// The global scope vars are already walked via tc.scope.
	// We rely on UsedCount incremented during inferExpr/checkStmt.
	// Walk the global scope vars only (function-local scopes are ephemeral).
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
		tc.scope.define(p.Name, &VarInfo{Type: t, IsFn: isCallable, Sp: p.Sp})
		if isCallable {
			syntheticFn := &FnDecl{
				Sp:      p.Sp,
				Name:    p.Name,
				Params:  []Param{},
				RetType: TypAny,
				Body:    &Block{},
			}
			tc.fns[p.Name] = syntheticFn
		}
	}

	for _, out := range mc.Outputs {
		tc.scope.define(out, &VarInfo{Type: TypAny, Sp: mc.Sp})
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

	// ── NEW: check for C-keyword function names ───────────────────────────────
	if isCReservedFnTC(fn.Name) {
		warnCode("W10", fn.Sp,
			fmt.Sprintf("function name %q shadows a C keyword — it will be compiled as __zx_%s", fn.Name, fn.Name),
			fmt.Sprintf("rename to avoid confusion: fn my_%s(...) { ... }", fn.Name))
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
		// ── NEW: warn on shadowing of outer names by param ────────────────────
		if outer := saved.lookup(p2.Name); outer != nil && !outer.IsFn && !outer.IsExtern {
			warnCode("W01", p2.Sp,
				fmt.Sprintf("parameter %q shadows an outer variable", p2.Name),
				"rename the parameter to avoid confusion")
		}
		tc.scope.define(p2.Name, &VarInfo{Type: t, Sp: p2.Sp, Defined: true})
	}

	// make 'our' globals visible inside functions
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

	// ── NEW: check non-void function for at least one return ─────────────────
	savedReturnSeen := tc.returnSeen
	tc.returnSeen = false
	tc.checkBlock(fn.Body)
	if fn.RetType != nil && fn.RetType.Kind != TyVoid && fn.RetType.Kind != TyAny {
		if !tc.returnSeen && fn.Name != "main" {
			warnCode("W40", fn.Sp,
				fmt.Sprintf("function %q declares return type %s but may not return a value on all paths", fn.Name, fn.RetType),
				"add a return statement, or change the return type to void")
		}
	}
	tc.returnSeen = savedReturnSeen

	tc.fnStack = tc.fnStack[:len(tc.fnStack)-1]
	tc.scope = saved
}

func (tc *TypeChecker) checkMethod(m *MethodDecl) {
	saved := tc.scope
	tc.scope = newScope(saved, "fn")
	tc.methodStack = append(tc.methodStack, m)
	recvType := StructType(m.RecvType)
	if m.RecvRef {
		recvType = RefOf(StructType(m.RecvType))
	}
	tc.scope.define(m.RecvName, &VarInfo{Type: recvType, Sp: m.Sp, Defined: true})

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
		tc.scope.define(p2.Name, &VarInfo{Type: t, Sp: p2.Sp, Defined: true})
	}

	// globals visible in methods
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
				fmt.Sprintf("method %q.%s declares return type %s but may not return a value on all paths", m.RecvType, m.Name, m.RetType),
				"add a return statement, or change the return type to void")
		}
	}
	tc.returnSeen = savedReturnSeen

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
		// ── NEW: unless with a const-false condition ──────────────────────────
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
		// ── NEW: while(true) without break inside ─────────────────────────────
		if lit, ok := s.Cond.(*BoolLit); ok && lit.Val {
			if !blockHasBreak(s.Body) {
				warnCode("W51", s.Sp,
					"while(true) loop has no break — this may be an infinite loop",
					"add a break statement or a condition that eventually becomes false")
			}
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
		// ── NEW: defer outside fn ────────────────────────────────────────────
		if !tc.scope.inFn() {
			errCode("E60", s.Sp,
				"'defer' used outside a function — defer only runs at function exit",
				"move this defer inside a fn block")
			tc.ok = false
		}
		tc.inferExpr(s.Call)
	case *AssertStmt:
		cond := tc.inferExpr(s.Cond)
		if cond.Kind == TyVoid {
			errCode("E71", s.Sp,
				"assert condition has type void — void expressions cannot be true or false",
				"use a boolean expression or a comparison")
			tc.ok = false
		}
		// ── NEW: assert(true) is always satisfied ─────────────────────────────
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
		// ── NEW: warn on discarded non-void call results ──────────────────────
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
			errCode("E72", s.Sp,
				fmt.Sprintf("exit code must be an integer, got %s", code),
				"use an integer expression as the exit code: exit 0  or  exit 1")
			tc.ok = false
		}
	case *BreakStmt:
		if !tc.scope.inLoop() {
			errCode("E07", s.Sp,
				"'break' used outside a loop",
				"break can only appear inside while / for / until loops")
			tc.ok = false
		}
	case *ContinueStmt:
		if !tc.scope.inLoop() {
			errCode("E08", s.Sp,
				"'continue' used outside a loop",
				"continue can only appear inside while / for / until loops")
			tc.ok = false
		}
	case *FnDecl:
		tc.checkFn(s)
	case *Block:
		tc.checkBlock(s)
	case *PipeExpr:
		tc.inferExpr(s)
	}
}

func (tc *TypeChecker) checkVarDecl(v *VarDecl) {
	// ── E09: duplicate in same scope ─────────────────────────────────────────
	if existing := tc.scope.lookupLocal(v.Name); existing != nil {
		errCodeSecondary("E09", v.Sp,
			fmt.Sprintf("variable %q is already declared in this scope", v.Name),
			"use a different name, or remove the duplicate declaration",
			[]SecondarySpan{{Span: existing.Sp, Label: "previous declaration here"}})
		tc.ok = false
	}

	// ── NEW: disallow single-underscore as a named variable (it's a discard) ─
	if v.Name == "_" {
		// Allow _ as a discard sink but don't register it
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
				errCode("E11", v.Sp,
					fmt.Sprintf("type mismatch: cannot assign %s to variable %q of type %s", initType, v.Name, resolved),
					fmt.Sprintf("change the declared type to %s, or convert: %s(%s)", initType, resolved, v.Name))
				tc.ok = false
			}
		}
	}

	// ── E12: void variable ────────────────────────────────────────────────────
	if resolved != nil && resolved.Kind == TyVoid {
		errCode("E12", v.Sp,
			fmt.Sprintf("variable %q cannot have type void — void means 'no value'", v.Name),
			"use int, float, str, bool, any, or a struct type instead")
		tc.ok = false
	}

	// ── E13: const without init ───────────────────────────────────────────────
	if v.IsConst && v.Init == nil {
		errCode("E13", v.Sp,
			fmt.Sprintf("const %q must have an initializer — constants must be assigned at declaration", v.Name),
			fmt.Sprintf("add a value: const %s = 42", v.Name))
		tc.ok = false
	}

	// ── W02: const naming convention ─────────────────────────────────────────
	if v.IsConst && !v.IsGlobal && v.Name == strings.ToLower(v.Name) && len(v.Name) > 1 {
		warnCode("W02", v.Sp,
			fmt.Sprintf("const %q should be UPPER_CASE by convention", v.Name),
			fmt.Sprintf("rename to %s", strings.ToUpper(v.Name)))
	}

	// ── W01: shadowing ────────────────────────────────────────────────────────
	if outer := tc.scope.parent; outer != nil {
		if o2 := outer.lookup(v.Name); o2 != nil && !o2.IsFn && !o2.IsExtern {
			warnCode("W01", v.Sp,
				fmt.Sprintf("variable %q shadows an outer variable declared at line %d", v.Name, o2.Sp.Line),
				"rename to avoid confusion")
		}
	}

	// ── NEW: warn if initializing with nil but no ref type ────────────────────
	if v.Init != nil {
		if _, isNil := v.Init.(*NilLit); isNil {
			if resolved != nil && resolved.Kind != TyRef && resolved.Kind != TyAny && resolved.Kind != TyUnknown {
				errCode("E73", v.Sp,
					fmt.Sprintf("cannot assign nil to variable %q of type %s — nil is only valid for ref types", v.Name, resolved),
					"declare it as ref<T> if you need a nullable pointer")
				tc.ok = false
			}
		}
	}

	// ── NEW: warn on very long variable names ─────────────────────────────────
	if len(v.Name) > 50 {
		warnCode("W31", v.Sp,
			fmt.Sprintf("variable name %q is very long (%d characters)", v.Name, len(v.Name)),
			"consider a shorter, more descriptive name")
	}

	v.ResolvedType = resolved
	tc.scope.define(v.Name, &VarInfo{
		Type:     resolved,
		IsConst:  v.IsConst,
		IsGlobal: v.IsGlobal,
		Sp:       v.Sp,
		Defined:  true,
	})
}

func (tc *TypeChecker) checkReturn(r *ReturnStmt) {
	tc.returnSeen = true
	ret := tc.currentRetType()
	if ret == nil {
		errCode("E14", r.Sp,
			"'return' used outside a function",
			"move this return statement inside a fn block")
		tc.ok = false
		return
	}
	if r.Value == nil {
		if ret.Kind != TyVoid && ret.Kind != TyAny {
			errCode("E15", r.Sp,
				fmt.Sprintf("function %q has return type %s but this return has no value", tc.currentName(), ret),
				fmt.Sprintf("return a value: return <expr>  —  or change the return type to void"))
			tc.ok = false
		}
		return
	}
	got := tc.inferExpr(r.Value)
	if ret.Kind == TyVoid {
		errCode("E16", r.Sp,
			fmt.Sprintf("function %q is declared void but returns a %s value", tc.currentName(), got),
			fmt.Sprintf("remove the return value, or change the return type to '-> %s'", got))
		tc.ok = false
		return
	}
	if got.Kind != TyUnknown && got.Kind != TyAny && ret.Kind != TyAny && !coercible(got, ret) {
		errCode("E17", r.Sp,
			fmt.Sprintf("return type mismatch in %q — expected %s but got %s", tc.currentName(), ret, got),
			fmt.Sprintf("cast the value: %s(value)  —  or change the return type to '-> %s'", ret, got))
		tc.ok = false
	}
}

func (tc *TypeChecker) checkIf(s *IfStmt) {
	cond := tc.inferExpr(s.Cond)
	if cond.Kind == TyVoid {
		errCode("E18", s.Cond.nodeSpan(),
			"if condition has type void — void expressions have no truth value",
			"use a comparison expression that produces a bool or int")
		tc.ok = false
	}
	// ── NEW: constant condition warnings ─────────────────────────────────────
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
	tc.checkBlock(s.Then)
	for _, el := range s.Elifs {
		condT := tc.inferExpr(el.Cond)
		if condT.Kind == TyVoid {
			errCode("E18", el.Cond.nodeSpan(),
				"elif condition has type void",
				"use a comparison expression")
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
		errCode("E20", s.From.nodeSpan(),
			fmt.Sprintf("for-range start must be an integer, got %s", fromT),
			"use an integer expression, e.g.:  for i in 0..10 { }")
		tc.ok = false
	}
	if !isInteger(toT) && toT.Kind != TyUnknown && toT.Kind != TyAny {
		errCode("E21", s.To.nodeSpan(),
			fmt.Sprintf("for-range end must be an integer, got %s", toT),
			"use an integer expression, e.g.:  for i in 0..len(arr) { }")
		tc.ok = false
	}
	// ── NEW: empty range warning ───────────────────────────────────────────────
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
	tc.scope.define(s.Var, &VarInfo{Type: TypInt, Sp: s.Sp, Defined: true})
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
					errCodeSecondary("E74", arm.Sp,
						fmt.Sprintf("duplicate match arm for value %d", lit.Val),
						"remove the duplicate arm or change its pattern",
						[]SecondarySpan{{Span: prev, Label: "first arm here"}})
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
}

func (tc *TypeChecker) checkTryCatch(s *TryCatchStmt) {
	tc.checkBlockInTry(s.Try)
	if s.Catch != nil {
		saved := tc.scope
		tc.scope = newScope(saved, "block")
		if s.ErrVar != "" {
			tc.scope.define(s.ErrVar, &VarInfo{Type: TypInt, Sp: s.Sp, Defined: true})
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

	// ── check assignability of LHS ────────────────────────────────────────────
	if id, ok := s.LHS.(*Ident); ok {
		vi := tc.scope.lookup(id.Name)
		if vi != nil && vi.IsConst {
			errCode("E22", s.Sp,
				fmt.Sprintf("cannot assign to const %q — constants are immutable", id.Name),
				"use 'let' instead of 'const' if you need a mutable variable")
			tc.ok = false
			return
		}
		if vi != nil && vi.IsFn {
			errCode("E23", s.Sp,
				fmt.Sprintf("cannot assign to function %q — functions are not variables", id.Name),
				"declare a variable to hold the result: let result = "+id.Name+"(...)")
			tc.ok = false
			return
		}
		if vi == nil {
			// Assignment to undeclared variable
			suggestion := tc.suggestName(id.Name)
			hint := fmt.Sprintf("declare it first: let %s = ...", id.Name)
			if suggestion != "" {
				hint = fmt.Sprintf("did you mean %q? — or declare it: let %s = ...", suggestion, id.Name)
			}
			errCode("E75", s.Sp,
				fmt.Sprintf("assignment to undeclared variable %q", id.Name),
				hint)
			tc.ok = false
			return
		}
	}

	// ── disallow assigning to literals ────────────────────────────────────────
	switch s.LHS.(type) {
	case *IntLit, *FloatLit, *BoolLit, *StrLit:
		errCode("E24", s.Sp,
			"left side of assignment is a literal — you cannot assign to a value",
			"use a variable name on the left side: let x = ...")
		tc.ok = false
		return
	}

	// ── NEW: assigning to a call result is nonsensical ────────────────────────
	if _, ok := s.LHS.(*CallExpr); ok {
		errCode("E76", s.Sp,
			"left side of assignment is a function call — call results are not assignable",
			"store the result in a variable first: let x = f(); x = ...")
		tc.ok = false
		return
	}

	rhsType := tc.inferExpr(s.Value)

	// ── NEW: nil to non-ref assignment ────────────────────────────────────────
	if _, isNil := s.Value.(*NilLit); isNil {
		if lhsType != nil && lhsType.Kind != TyRef && lhsType.Kind != TyAny && lhsType.Kind != TyUnknown {
			errCode("E73", s.Sp,
				fmt.Sprintf("cannot assign nil to %s — nil is only valid for ref types", lhsType),
				"declare the variable as ref<T> to allow nil values")
			tc.ok = false
			return
		}
	}

	if lhsType.Kind != TyUnknown && lhsType.Kind != TyAny && rhsType.Kind != TyUnknown && rhsType.Kind != TyAny {
		if !coercible(rhsType, lhsType) {
			errCode("E25", s.Sp,
				fmt.Sprintf("type mismatch: cannot assign %s to %s", rhsType, lhsType),
				fmt.Sprintf("cast the right-hand side: %s(expr)", lhsType))
			tc.ok = false
		}
	}
	if s.Op != "=" && lhsType.Kind != TyAny && !isNumeric(lhsType) {
		errCode("E26", s.Sp,
			fmt.Sprintf("compound operator %s requires a numeric operand, but variable is %s", s.Op, lhsType),
			"compound assignment (+=, -=, *=, /=, %=) only works on int, float, and char")
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
		// ── NEW: warn on very long string literals ────────────────────────────
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
			errCode("E18", e.Cond.nodeSpan(),
				"ternary condition has type void — void expressions have no truth value",
				"use a comparison expression")
			tc.ok = false
		}
		then := tc.inferExpr(e.Then)
		els := tc.inferExpr(e.Else)
		// ── NEW: warn on ternary branch type mismatch ─────────────────────────
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
			// mod block name used as value
			if tc.modNames[e.Name] {
				errCode("E80", e.Sp,
					fmt.Sprintf("%q is a mod block, not a value", e.Name),
					fmt.Sprintf("call a function inside it: %s->myFn()", e.Name))
				tc.ok = false
				e.Typ = TypUnknown
				return TypUnknown
			}
			// mod fn called without namespace
			for modKey := range tc.modFns {
				parts := strings.SplitN(modKey, "::", 2)
				if len(parts) == 2 && parts[1] == e.Name {
					errCode("E81", e.Sp,
						fmt.Sprintf("%q is a mod-private function — it must be called with its module name", e.Name),
						fmt.Sprintf("use: %s->%s()", parts[0], e.Name))
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
			errCode("E27", e.Sp,
				fmt.Sprintf("undefined name %q", e.Name),
				hint)
			tc.ok = false
			e.Typ = TypUnknown
			return TypUnknown
		}
		// mod fn accessed by plain name
		if vi.IsModFn {
			errCode("E81", e.Sp,
				fmt.Sprintf("%q is a mod-private function in mod %q", e.Name, vi.ModName),
				fmt.Sprintf("call it as: %s->%s()", vi.ModName, e.Name))
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
			errCode("E47", e.Idx.nodeSpan(),
				fmt.Sprintf("array index must be an integer, got %s", idx),
				"use an integer expression as the index")
			tc.ok = false
		}
		// ── NEW: negative literal index ────────────────────────────────────────
		if lit, ok := e.Idx.(*UnaryExpr); ok && lit.Op == "-" {
			if _, isInt := lit.Operand.(*IntLit); isInt {
				warnCode("W62", e.Idx.nodeSpan(),
					"negative index — this will likely be out of bounds at runtime",
					"use len(arr) - N for reverse indexing")
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
			errCode("E46", e.Sp,
				fmt.Sprintf("cannot index into type %s — only arrays and strings support indexing", obj),
				"use an array [1,2,3] or a string \"abc\"")
			tc.ok = false
		}
		e.Typ = TypAny
		return TypAny

	case *FieldExpr:
		return tc.inferField(e)

	case *CastExpr:
		from := tc.inferExpr(e.Operand)
		if !canCast(from, e.ToType) {
			errCode("E28", e.Sp,
				fmt.Sprintf("cannot cast %s to %s", from, e.ToType),
				"valid casts: between numeric types (int/float/char/bool), and between str and ref char")
			tc.ok = false
		}
		// ── NEW: warn on redundant cast ────────────────────────────────────────
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
				errCode("E29", e.Sp,
					fmt.Sprintf("cannot dereference type %s — only ref<T> values can be dereferenced", inner),
					"use ^ on a ref variable, e.g. ^myPtr")
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
				errCode("E55", el.nodeSpan(),
					fmt.Sprintf("array element %d has type %s but the first element is %s — all elements must have the same type", i+2, got, first),
					fmt.Sprintf("cast this element: %s(value)", first))
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

	default:
		return TypUnknown
	}
}

// inferModCall type-checks  modName->fn(args)  and  modName::fn(args).
func (tc *TypeChecker) inferModCall(e *ModCallExpr) *ZXType {
	if !tc.modNames[e.Mod] {
		// suggest similar mod name
		bestMod := tc.suggestModName(e.Mod)
		hint := fmt.Sprintf("declare it: mod %s { fn %s() { ... } }", e.Mod, e.Fn)
		if bestMod != "" {
			hint = fmt.Sprintf("did you mean mod %q?", bestMod)
		}
		errCode("E82", e.Sp,
			fmt.Sprintf("unknown module %q — no mod block with this name exists", e.Mod),
			hint)
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
		errCode("E83", e.Sp, hint,
			fmt.Sprintf("define it: mod %s { fn %s() { ... } }", e.Mod, e.Fn))
		tc.ok = false
		e.Typ = TypUnknown
		return TypUnknown
	}
	// type-check arguments
	if !fn.Variadic && len(e.Args) != len(fn.Params) {
		minArgs := 0
		for _, p := range fn.Params {
			if p.Default == nil {
				minArgs++
			}
		}
		if len(e.Args) < minArgs || (!fn.Variadic && len(e.Args) > len(fn.Params)) {
			errCode("E42", e.Sp,
				fmt.Sprintf("%s->%s expects %d argument(s) but got %d", e.Mod, e.Fn, len(fn.Params), len(e.Args)),
				fmt.Sprintf("signature: fn %s(%s)", e.Fn, listParamTypes(fn.Params)))
			tc.ok = false
		}
	}
	for i, arg := range e.Args {
		got := tc.inferExpr(arg)
		if i < len(fn.Params) {
			expected := fn.Params[i].Type
			if expected != nil && expected.Kind != TyAny && got.Kind != TyAny && got.Kind != TyUnknown && !coercible(got, expected) {
				errCode("E43", arg.nodeSpan(),
					fmt.Sprintf("%s->%s argument %d: expected %s but got %s", e.Mod, e.Fn, i+1, expected, got),
					fmt.Sprintf("cast with: %s(value)", expected))
				tc.ok = false
			}
		}
	}
	e.Typ = fn.RetType
	return fn.RetType
}

// suggestModName returns the closest existing mod name to the given name.
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

// ── Macro inference ───────────────────────────────────────────────────────────

func (tc *TypeChecker) inferMacroCall(e *MacroCallExpr) *ZXType {
	mc, ok := tc.macros[e.Name]
	if !ok {
		hint := fmt.Sprintf("declare it: macro fn %s |input| -> |output| { }", e.Name)
		if s := tc.suggestName(e.Name); s != "" {
			hint = fmt.Sprintf("did you mean %q?", s)
		}
		errCode("EM07", e.Sp,
			fmt.Sprintf("call to undefined macro %q", e.Name),
			hint)
		tc.ok = false
		e.Typ = TypUnknown
		return TypUnknown
	}
	expected := len(mc.Params)
	got := len(e.Args)
	if expected != got && !(got == 0 && expected <= 1) {
		errCode("EM08", e.Sp,
			fmt.Sprintf("macro %q expects %d argument(s) but got %d", e.Name, expected, got),
			fmt.Sprintf("signature: macro fn %s |%s|", e.Name, listParamTypes(mc.Params)))
		tc.ok = false
	}
	for i, arg := range e.Args {
		argType := tc.inferExpr(arg)
		if i < len(mc.Params) {
			expected := mc.Params[i].Type
			if expected != nil && expected.Kind != TyAny &&
				argType.Kind != TyAny && argType.Kind != TyUnknown &&
				!coercible(argType, expected) {
				errCode("EM09", arg.nodeSpan(),
					fmt.Sprintf("macro %q argument %d: expected %s but got %s", e.Name, i+1, expected, argType),
					fmt.Sprintf("cast with: %s(value)", expected))
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
			errCode("EM07", step.Sp,
				fmt.Sprintf("undefined macro %q in chain", step.Macro),
				hint)
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
		if lhs.Kind != TyUnknown && rhs.Kind != TyUnknown && lhs.Kind != TyAny && rhs.Kind != TyAny {
			if !coercible(lhs, rhs) && !coercible(rhs, lhs) {
				errCode("E30", e.Sp,
					fmt.Sprintf("cannot compare %s with %s using %s — incompatible types", lhs, rhs, e.Op),
					"both sides of == or != must have the same (or compatible) type")
				tc.ok = false
			}
		}
		// ── NEW: comparing to self is always true/false ────────────────────────
		if isSameIdent(e.LHS, e.RHS) {
			warnCode("W64", e.Sp,
				fmt.Sprintf("comparing a variable to itself with %s — result is always %v", e.Op, e.Op == "=="),
				"this comparison is redundant; check the variable names")
		}
		e.Typ = TypBool
		return TypBool
	case "<", ">", "<=", ">=":
		if lhs.Kind != TyAny && rhs.Kind != TyAny && lhs.Kind != TyUnknown {
			if !isNumeric(lhs) || !isNumeric(rhs) {
				errCode("E31", e.Sp,
					fmt.Sprintf("comparison %s requires numeric operands, got %s and %s", e.Op, lhs, rhs),
					"comparisons only work on int, float, and char")
				tc.ok = false
			}
		}
		// ── NEW: compare same ident ────────────────────────────────────────────
		if isSameIdent(e.LHS, e.RHS) {
			warnCode("W64", e.Sp,
				fmt.Sprintf("comparing a variable to itself with %s — result is always %v", e.Op, e.Op == "<=" || e.Op == ">="),
				"this comparison is redundant")
		}
		e.Typ = TypBool
		return TypBool
	case "&&", "||":
		// ── NEW: warn on non-bool operands of logical operators ───────────────
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
			errCode("E32", e.Sp,
				"'+' cannot concatenate strings in ZX",
				"use str_cat(a, b) from std::str, or an f-string: f\"{a}{b}\"")
			tc.ok = false
			e.Typ = TypStr
			return TypStr
		}
		fallthrough
	case "-", "*", "/", "%":
		if lhs.Kind != TyAny && lhs.Kind != TyUnknown && !isNumeric(lhs) {
			errCode("E33", e.LHS.nodeSpan(),
				fmt.Sprintf("operator %q requires numeric operands but left side is %s", e.Op, lhs),
				"arithmetic operators only work on int, float, and char")
			tc.ok = false
		}
		if rhs.Kind != TyAny && rhs.Kind != TyUnknown && !isNumeric(rhs) {
			errCode("E33", e.RHS.nodeSpan(),
				fmt.Sprintf("operator %q requires numeric operands but right side is %s", e.Op, rhs),
				"arithmetic operators only work on int, float, and char")
			tc.ok = false
		}
		if e.Op == "/" {
			if lit, ok := e.RHS.(*IntLit); ok && lit.Val == 0 {
				errCode("E34", e.RHS.nodeSpan(),
					"division by zero — this will crash at runtime",
					"check the divisor before dividing: if divisor != 0 { ... }")
				tc.ok = false
			}
			if lit, ok := e.RHS.(*FloatLit); ok && lit.Val == 0.0 {
				warnCode("W03", e.RHS.nodeSpan(),
					"division by 0.0 produces Inf or NaN",
					"check for zero before dividing floating-point values")
			}
		}
		if e.Op == "%" && (lhs.Kind == TyFloat || rhs.Kind == TyFloat) {
			errCode("E35", e.Sp,
				"modulo '%' does not work on float operands",
				"use fmod(x, y) from std::math for floating-point modulo")
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
			errCode("E36", e.Sp,
				fmt.Sprintf("bitwise operator %q requires integer operands but left side is %s", e.Op, lhs),
				"bitwise operators only work on int and char")
			tc.ok = false
		}
		if rhs.Kind != TyAny && rhs.Kind != TyUnknown && !isInteger(rhs) {
			errCode("E36", e.Sp,
				fmt.Sprintf("bitwise operator %q requires integer operands but right side is %s", e.Op, rhs),
				"bitwise operators only work on int and char")
			tc.ok = false
		}
		// ── NEW: warn on shift by negative or large constant ─────────────────
		if e.Op == "<<" || e.Op == ">>" {
			if lit, ok := e.RHS.(*IntLit); ok {
				if lit.Val < 0 {
					errCode("E77", e.RHS.nodeSpan(),
						fmt.Sprintf("shift by negative amount %d — this is undefined behaviour", lit.Val),
						"use a non-negative shift amount")
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
			errCode("E37", e.Sp,
				fmt.Sprintf("logical NOT '!' cannot be applied to type %s", inner),
				"'!' works on bool, int, and ref values only")
			tc.ok = false
		}
		e.Typ = TypBool
		return TypBool
	case "-":
		if inner.Kind != TyAny && inner.Kind != TyUnknown && !isNumeric(inner) {
			errCode("E38", e.Sp,
				fmt.Sprintf("unary minus cannot be applied to type %s", inner),
				"unary minus only works on int, float, and char")
			tc.ok = false
		}
		e.Typ = inner
		return inner
	case "~":
		if inner.Kind != TyAny && inner.Kind != TyUnknown && !isInteger(inner) {
			errCode("E39", e.Sp,
				fmt.Sprintf("bitwise NOT '~' cannot be applied to type %s", inner),
				"bitwise NOT requires an integer operand")
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
			errCode("E41", e.Sp,
				fmt.Sprintf("builtin %q expects %d argument(s) but got %d", fnName, bd.Arity, len(e.Args)),
				"check the number of arguments")
			tc.ok = false
		}
		if id, ok := e.Func.(*Ident); ok {
			id.Typ = bd.Ret
		}
		e.Typ = bd.Ret
		return bd.Ret
	}

	if sf, ok := tc.stdFns[fnName]; ok {
		for _, a := range e.Args {
			tc.inferExpr(a)
		}
		e.Typ = sf.Ret
		return sf.Ret
	}

	if ext, ok := tc.externs[fnName]; ok {
		for i, a := range e.Args {
			got := tc.inferExpr(a)
			if !ext.Variadic && i < len(ext.Params) {
				expected := ext.Params[i].Type
				if expected.Kind != TyAny && got.Kind != TyAny && got.Kind != TyUnknown && !coercible(got, expected) {
					errCode("E40", a.nodeSpan(),
						fmt.Sprintf("extern %q argument %d: expected %s but got %s", fnName, i+1, expected, got),
						fmt.Sprintf("cast with: %s(value)", expected))
					tc.ok = false
				}
			}
		}
		if !ext.Variadic && len(e.Args) != len(ext.Params) {
			errCode("E41", e.Sp,
				fmt.Sprintf("extern %q expects %d argument(s) but got %d", fnName, len(ext.Params), len(e.Args)),
				"check the extern declaration for the expected parameters")
			tc.ok = false
		}
		if id, ok := e.Func.(*Ident); ok {
			id.Typ = ext.RetType
		}
		e.Typ = ext.RetType
		return ext.RetType
	}

	if fn, ok := tc.fns[fnName]; ok {
		if !fn.Variadic && len(e.Args) != len(fn.Params) {
			minArgs := 0
			for _, p2 := range fn.Params {
				if p2.Default == nil {
					minArgs++
				}
			}
			if len(e.Args) < minArgs || (!fn.Variadic && len(e.Args) > len(fn.Params)) {
				errCode("E42", e.Sp,
					fmt.Sprintf("function %q expects %d argument(s) but got %d", fnName, len(fn.Params), len(e.Args)),
					fmt.Sprintf("signature: fn %s(%s)", fnName, listParamTypes(fn.Params)))
				tc.ok = false
			}
		}
		for i, a := range e.Args {
			got := tc.inferExpr(a)
			if i < len(fn.Params) {
				expected := fn.Params[i].Type
				if expected != nil && expected.Kind != TyAny && got.Kind != TyAny && got.Kind != TyUnknown && !coercible(got, expected) {
					errCode("E43", a.nodeSpan(),
						fmt.Sprintf("function %q argument %d: expected %s but got %s", fnName, i+1, expected, got),
						fmt.Sprintf("cast with: %s(value)", expected))
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
			suggestion := tc.suggestName(fnName)
			hint := fmt.Sprintf("declare it: extern fn %s(...) -> int  or  fn %s(...) { }", fnName, fnName)
			if suggestion != "" {
				hint = fmt.Sprintf("did you mean %q?", suggestion)
			}
			errCode("E44", e.Sp,
				fmt.Sprintf("call to undefined function %q", fnName),
				hint)
			tc.ok = false
			e.Typ = TypUnknown
			return TypUnknown
		}
		if !vi.IsFn {
			errCode("E45", e.Sp,
				fmt.Sprintf("%q is a %s variable, not a function — it cannot be called", fnName, vi.Type),
				fmt.Sprintf("declare a function: fn %s() { }  or  extern fn %s()", fnName, fnName))
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
			Sp:   e.Sp,
			Mod:  id.Name,
			Fn:   e.Method,
			Args: e.Args,
		}
		t := tc.inferModCall(modCall)
		e.Typ = t
		id.Typ = TypUnknown
		return t
	}

	recvType := tc.inferExpr(e.Recv)

	// ── NEW: method call on nil ────────────────────────────────────────────────
	if _, isNil := e.Recv.(*NilLit); isNil {
		errCode("E78", e.Sp,
			fmt.Sprintf("calling method %q on nil — nil has no methods", e.Method),
			"check for nil before calling methods on ref values")
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
		errCode("E59", e.Sp,
			fmt.Sprintf("struct %q has no method %q", structName, e.Method),
			hint)
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
			errCode("E48", e.Sp,
				fmt.Sprintf("cannot access field %q on type %s — field access is only valid on struct types", e.Field, objType),
				"use a struct type or ref<StructType>")
			tc.ok = false
		}
		e.Typ = TypAny
		return TypAny
	}
	sd, ok := tc.structs[eff.Name]
	if !ok {
		errCode("E49", e.Sp,
			fmt.Sprintf("struct type %q is not defined", eff.Name),
			fmt.Sprintf("declare it: type %s struct { ... }", eff.Name))
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
	// suggest similar field names
	suggestion := tc.suggestField(sd, e.Field)
	hint := fmt.Sprintf("available fields: %s", listFields(sd.Fields))
	if suggestion != "" {
		hint = fmt.Sprintf("did you mean %q?", suggestion)
	}
	errCode("E50", e.Sp,
		fmt.Sprintf("struct %q has no field %q", eff.Name, e.Field),
		hint)
	tc.ok = false
	e.Typ = TypAny
	return TypAny
}

// suggestField returns the closest field name in the struct.
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
		// suggest similar struct name
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
		errCode("E51", e.Sp,
			fmt.Sprintf("undefined struct %q in struct literal", e.Name),
			hint)
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
			errCode("E52", fi.Sp,
				fmt.Sprintf("struct %q has no field %q", e.Name, fi.Name),
				hint)
			tc.ok = false
		}
		if prev, dup := provided[fi.Name]; dup {
			errCodeSecondary("E53", fi.Sp,
				fmt.Sprintf("field %q is set more than once in struct literal for %q", fi.Name, e.Name),
				"remove the duplicate field assignment",
				[]SecondarySpan{{Span: prev, Label: "first set here"}})
			tc.ok = false
		}
		provided[fi.Name] = fi.Sp
		got := tc.inferExpr(fi.Value)
		for _, sf := range sd.Fields {
			if sf.Name == fi.Name && sf.Type != nil && sf.Type.Kind != TyAny &&
				got.Kind != TyAny && got.Kind != TyUnknown && !coercible(got, sf.Type) {
				errCode("E54", fi.Sp,
					fmt.Sprintf("field %q expects type %s but got %s", fi.Name, sf.Type, got),
					fmt.Sprintf("cast with: %s(value)", sf.Type))
				tc.ok = false
				break
			}
		}
	}

	// ── NEW: warn on struct fields that are left uninitialized ───────────────
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

// ── helpers ───────────────────────────────────────────────────────────────────

func (tc *TypeChecker) validateTypeExists(t *ZXType, sp Span) {
	if t == nil || t.Kind == TyAny || t.Kind == TyUnknown {
		return
	}
	if t.Kind == TyStruct {
		if _, ok := tc.structs[t.Name]; !ok {
			// suggest similar type
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
			errCode("E56", sp,
				fmt.Sprintf("unknown type %q — no struct with this name is defined", t.Name),
				hint)
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

// ── Utility: AST helpers ──────────────────────────────────────────────────────

// blockHasBreak returns true if the block contains a BreakStmt at the top level.
func blockHasBreak(b *Block) bool {
	if b == nil {
		return false
	}
	for _, s := range b.Stmts {
		if _, ok := s.(*BreakStmt); ok {
			return true
		}
		// also check nested if/match for break
		if is, ok := s.(*IfStmt); ok {
			if blockHasBreak(is.Then) {
				return true
			}
		}
	}
	return false
}

// isSameIdent returns true if both nodes are Ident nodes with the same name.
func isSameIdent(a, b Node) bool {
	ia, oka := a.(*Ident)
	ib, okb := b.(*Ident)
	return oka && okb && ia.Name == ib.Name
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
		prog.ModBlocks = append(prog.ModBlocks, found)
		prog.Structs = append(prog.Structs, found.Structs...)
		return
	}
	for _, stmt := range imported.TopStmts {
		if fn, ok := stmt.(*FnDecl); ok {
			if _, exists := tc.fns[fn.Name]; exists {
				warnAt(sp,
					fmt.Sprintf("imported function %q shadows an existing function — rename one to avoid confusion", fn.Name),
					"")
			}
			prog.TopStmts = append(prog.TopStmts, fn)
		}
		if vd, ok := stmt.(*VarDecl); ok && vd.IsGlobal {
			prog.GlobalVars = append(prog.GlobalVars, vd)
			prog.TopStmts = append(prog.TopStmts, vd)
		}
	}
	prog.Structs = append(prog.Structs, imported.Structs...)
	prog.Methods = append(prog.Methods, imported.Methods...)
	prog.ModBlocks = append(prog.ModBlocks, imported.ModBlocks...)
	prog.Externs = append(prog.Externs, imported.Externs...)
	prog.Macros = append(prog.Macros, imported.Macros...)
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
		// ── NEW: warn on redundant imports ────────────────────────────────────
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
		if !imp.ImportAll && imp.Alias != "" && !isValidIdent(imp.Alias) {
			errCode("EI06", sp,
				fmt.Sprintf("invalid mod name %q in import selector", imp.Alias),
				"mod names must be valid identifiers: letters, digits, underscores")
			tc.ok = false
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
		if !imp.ImportAll && imp.Alias != "" && !isValidIdent(imp.Alias) {
			errCode("EI09", sp,
				fmt.Sprintf("invalid mod name %q in import selector", imp.Alias),
				"mod names must be valid identifiers")
			tc.ok = false
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

func (tc *TypeChecker) registerModFns(mb *ModBlock) {
	tc.modNames[mb.Name] = true
	for _, fn := range mb.Fns {
		key := mb.Name + "::" + fn.Name
		tc.modFns[key] = fn
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
	}
	for _, nested := range mb.Mods {
		tc.registerModFns(nested)
	}
}

func (tc *TypeChecker) checkModFns(mb *ModBlock) {
	for _, fn := range mb.Fns {
		tc.checkFn(fn)
	}
	for _, td := range mb.Tests {
		tc.checkFn(td.Fn)
	}
	for _, nested := range mb.Mods {
		tc.checkModFns(nested)
	}
}
