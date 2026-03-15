package main

import (
	"fmt"
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

// ─────────────────────────────────────────────────────────────────────────────
//  TypeChecker
// ─────────────────────────────────────────────────────────────────────────────

type TypeChecker struct {
	prog          *Program
	scope         *Scope
	fnStack       []*FnDecl
	methodStack   []*MethodDecl
	structs       map[string]*StructDecl
	fns           map[string]*FnDecl
	methods       map[string]*MethodDecl
	externs       map[string]*ExternDecl
	stdFns        map[string]*StdFn
	importPaths   map[string]bool
	importMods    map[string]bool
	deferredCalls []Node // for defer tracking
	ok            bool
}

func TypeCheck(prog *Program, src, file string) bool {
	tc := &TypeChecker{
		prog:    prog,
		structs: make(map[string]*StructDecl), fns: make(map[string]*FnDecl),
		methods: make(map[string]*MethodDecl), externs: make(map[string]*ExternDecl),
		stdFns:      make(map[string]*StdFn),
		importPaths: make(map[string]bool), importMods: make(map[string]bool),
		ok: true,
	}
	tc.scope = newScope(nil, "global")

	for _, imp := range prog.Imports {
		if imp.IsStd {
			tc.importMods[imp.Module] = true
		} else {
			tc.importPaths[imp.Path] = true
		}
	}
	tc.stdFns = prog.AllStdFns()

	// E01 structs
	for _, s := range prog.Structs {
		if _, exists := tc.structs[s.Name]; exists {
			errCode("E01", s.Sp, fmt.Sprintf("struct %q defined more than once", s.Name), "rename one")
			tc.ok = false
		}
		tc.structs[s.Name] = s
		seen := map[string]bool{}
		for _, f := range s.Fields {
			if seen[f.Name] {
				errCode("E02", f.Sp, fmt.Sprintf("duplicate field %q in struct %q", f.Name, s.Name), "remove the duplicate")
				tc.ok = false
			}
			seen[f.Name] = true
			if f.Type != nil && f.Type.Kind == TyStruct {
				tc.validateTypeExists(f.Type, f.Sp)
			}
		}
	}

	// register externs + std fns
	for _, e := range prog.Externs {
		tc.externs[e.Name] = e
		tc.scope.define(e.Name, &VarInfo{Type: e.RetType, IsFn: true, IsExtern: true, Sp: e.Sp})
	}
	for name, fn := range tc.stdFns {
		f := fn
		tc.scope.define(name, &VarInfo{Type: f.Ret, IsFn: true, IsStd: true})
	}

	// E03 user functions
	for _, stmt := range prog.TopStmts {
		if fn, ok := stmt.(*FnDecl); ok {
			if _, exists := tc.fns[fn.Name]; exists {
				errCode("E03", fn.Sp, fmt.Sprintf("function %q defined more than once", fn.Name), "rename one")
				tc.ok = false
			}
			tc.fns[fn.Name] = fn
			tc.scope.define(fn.Name, &VarInfo{Type: fn.RetType, IsFn: true, Sp: fn.Sp})
		}
	}

	// methods
	for _, m := range prog.Methods {
		key := m.CName()
		tc.methods[key] = m
		tc.scope.define(key, &VarInfo{Type: m.RetType, IsFn: true, IsMethod: true, Sp: m.Sp})
		if _, ok := tc.structs[m.RecvType]; !ok {
			errCode("E57", m.Sp, fmt.Sprintf("method on undefined struct %q", m.RecvType),
				fmt.Sprintf("declare the struct first: type %s struct { ... }", m.RecvType))
			tc.ok = false
		}
	}

	// register functions + tests from mod blocks (recursively)
	for _, mb := range prog.ModBlocks {
		tc.registerModBlock(mb)
	}

	// check bodies
	for _, stmt := range prog.TopStmts {
		if fn, ok := stmt.(*FnDecl); ok {
			tc.checkFn(fn)
		}
	}
	for _, m := range prog.Methods {
		tc.checkMethod(m)
	}
	for _, stmt := range prog.TopStmts {
		if _, ok := stmt.(*FnDecl); !ok {
			tc.checkStmt(stmt)
		}
	}

	// check mod block function bodies
	for _, mb := range prog.ModBlocks {
		tc.checkModBlock(mb)
	}
	return tc.ok
}

// registerModBlock registers all functions/tests from a mod block and its children.
func (tc *TypeChecker) registerModBlock(mb *ModBlock) {
	for _, s := range mb.Structs {
		if _, exists := tc.structs[s.Name]; !exists {
			tc.structs[s.Name] = s
		}
	}
	for _, fn := range mb.Fns {
		if _, exists := tc.fns[fn.Name]; !exists {
			tc.fns[fn.Name] = fn
			tc.scope.define(fn.Name, &VarInfo{Type: fn.RetType, IsFn: true, Sp: fn.Sp})
		}
	}
	for _, td := range mb.Tests {
		fn := td.Fn
		if _, exists := tc.fns[fn.Name]; !exists {
			tc.fns[fn.Name] = fn
			tc.scope.define(fn.Name, &VarInfo{Type: fn.RetType, IsFn: true, Sp: fn.Sp})
		}
	}
	for _, nested := range mb.Mods {
		tc.registerModBlock(nested)
	}
}

// checkModBlock type-checks all function bodies inside a mod block.
func (tc *TypeChecker) checkModBlock(mb *ModBlock) {
	for _, fn := range mb.Fns {
		tc.checkFn(fn)
	}
	for _, td := range mb.Tests {
		tc.checkFn(td.Fn)
	}
	for _, nested := range mb.Mods {
		tc.checkModBlock(nested)
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
func (tc *TypeChecker) currentRetType() *ZXType {
	if m := tc.currentMethod(); m != nil {
		return m.RetType
	}
	if f := tc.currentFn(); f != nil {
		return f.RetType
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
	return "<top-level>"
}

func (tc *TypeChecker) checkFn(fn *FnDecl) {
	saved := tc.scope
	tc.scope = newScope(saved, "fn")
	tc.fnStack = append(tc.fnStack, fn)
	seen := map[string]bool{}
	for _, p2 := range fn.Params {
		if seen[p2.Name] {
			errCode("E04", p2.Sp, fmt.Sprintf("duplicate param %q in %q", p2.Name, fn.Name), "rename one")
			tc.ok = false
		}
		seen[p2.Name] = true
		t := p2.Type
		if t == nil {
			t = TypAny
		}
		tc.scope.define(p2.Name, &VarInfo{Type: t, Sp: p2.Sp})
	}
	tc.validateTypeExists(fn.RetType, fn.Sp)
	tc.checkBlock(fn.Body)
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
	tc.scope.define(m.RecvName, &VarInfo{Type: recvType, Sp: m.Sp})
	for _, p2 := range m.Params {
		t := p2.Type
		if t == nil {
			t = TypAny
		}
		tc.scope.define(p2.Name, &VarInfo{Type: t, Sp: p2.Sp})
	}
	tc.checkBlock(m.Body)
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
		tc.inferExpr(s.Cond)
		tc.checkBlock(s.Body)
		if s.Else != nil {
			tc.checkBlock(s.Else)
		}
	case *WhileStmt:
		tc.inferExpr(s.Cond)
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
		tc.inferExpr(s.Call)
	case *AssertStmt:
		cond := tc.inferExpr(s.Cond)
		if cond.Kind == TyVoid {
			errCode("E71", s.Sp, "assert condition has type void — it cannot be true or false",
				"use a boolean or comparison expression")
			tc.ok = false
		}
		tc.inferExpr(s.Msg)
	case *SpawnStmt:
		tc.inferExpr(s.Call)
	case *ExprStmt:
		tc.inferExpr(s.Expr)
	case *PrintStmt:
		for _, a := range s.Args {
			tc.inferExpr(a)
		}
	case *ExitStmt:
		tc.inferExpr(s.Code)
	case *BreakStmt:
		if !tc.scope.inLoop() {
			errCode("E07", s.Sp, "'break'/'last' outside of a loop", "only use break inside while / for / until loops")
			tc.ok = false
		}
	case *ContinueStmt:
		if !tc.scope.inLoop() {
			errCode("E08", s.Sp, "'continue'/'next' outside of a loop", "only use continue inside while / for / until loops")
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
	if existing := tc.scope.lookupLocal(v.Name); existing != nil {
		errCode("E09", v.Sp, fmt.Sprintf("variable %q already declared in this scope", v.Name),
			"use a different name, or remove the duplicate declaration")
		noteAt(existing.Sp, "previous declaration here")
		tc.ok = false
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
					fmt.Sprintf("cannot assign %s to %s variable %q", initType, resolved, v.Name),
					fmt.Sprintf("either change the type to %s, or cast: %s(%s)", initType, resolved, v.Name))
				tc.ok = false
			}
		}
	}
	// E12 void variable
	if resolved != nil && resolved.Kind == TyVoid {
		errCode("E12", v.Sp, fmt.Sprintf("variable %q cannot have type void — void means 'no value'", v.Name),
			"use any, int, float, str, bool, or a struct type instead")
		tc.ok = false
	}
	// E13 const without init
	if v.IsConst && v.Init == nil {
		errCode("E13", v.Sp, fmt.Sprintf("const/our %q must have an initializer", v.Name),
			fmt.Sprintf("add = <value>, e.g. const %s = 42", v.Name))
		tc.ok = false
	}
	// E60 all-lowercase const naming warning
	if v.IsConst && v.Name == strings.ToLower(v.Name) && len(v.Name) > 1 {
		warnCode("W02", v.Sp, fmt.Sprintf("const %q should be UPPER_CASE by convention", v.Name),
			fmt.Sprintf("rename to %s", strings.ToUpper(v.Name)))
	}
	// W01 shadowing
	if outer := tc.scope.parent; outer != nil {
		if o2 := outer.lookup(v.Name); o2 != nil && !o2.IsFn && !o2.IsExtern {
			warnCode("W01", v.Sp, fmt.Sprintf("variable %q shadows an outer variable", v.Name),
				"consider using a different name to avoid confusion")
		}
	}
	v.ResolvedType = resolved
	tc.scope.define(v.Name, &VarInfo{Type: resolved, IsConst: v.IsConst, Sp: v.Sp, Defined: true})
}

func (tc *TypeChecker) checkReturn(r *ReturnStmt) {
	ret := tc.currentRetType()
	if ret == nil {
		errCode("E14", r.Sp, "'return' used outside of a function",
			"move this return statement inside a fn block")
		tc.ok = false
		return
	}
	if r.Value == nil {
		if ret.Kind != TyVoid && ret.Kind != TyAny {
			errCode("E15", r.Sp, fmt.Sprintf("function %q must return %s, but has an empty return", tc.currentName(), ret),
				fmt.Sprintf("return a value: return <expr>, or change the return type to void"))
			tc.ok = false
		}
		return
	}
	got := tc.inferExpr(r.Value)
	if ret.Kind == TyVoid {
		errCode("E16", r.Sp, fmt.Sprintf("void function %q is returning a %s value — void means no return value", tc.currentName(), got),
			"remove the return value, or change '-> void' to '-> "+got.String()+"'")
		tc.ok = false
		return
	}
	if got.Kind != TyUnknown && got.Kind != TyAny && ret.Kind != TyAny && !coercible(got, ret) {
		errCode("E17", r.Sp, fmt.Sprintf("return type mismatch in %q — expected %s, got %s", tc.currentName(), ret, got),
			fmt.Sprintf("either cast: %s(value), or change the return type to %s", ret, got))
		tc.ok = false
	}
}

func (tc *TypeChecker) checkIf(s *IfStmt) {
	cond := tc.inferExpr(s.Cond)
	if cond.Kind == TyVoid {
		errCode("E18", s.Cond.nodeSpan(), "if condition has type void — void cannot be truthy or falsy",
			"use a comparison expression that produces a bool or int")
		tc.ok = false
	}
	tc.checkBlock(s.Then)
	for _, el := range s.Elifs {
		tc.inferExpr(el.Cond)
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
		errCode("E20", s.From.nodeSpan(), fmt.Sprintf("for-range start must be int, got %s", fromT),
			"use an integer expression, e.g.:  for i in 0..10 { }")
		tc.ok = false
	}
	if !isInteger(toT) && toT.Kind != TyUnknown && toT.Kind != TyAny {
		errCode("E21", s.To.nodeSpan(), fmt.Sprintf("for-range end must be int, got %s", toT),
			"use an integer expression, e.g.:  for i in 0..len(arr) { }")
		tc.ok = false
	}
	saved := tc.scope
	tc.scope = newScope(saved, "loop")
	tc.scope.define(s.Var, &VarInfo{Type: TypInt, Sp: s.Sp})
	for _, st := range s.Body.Stmts {
		tc.checkStmt(st)
	}
	tc.scope = saved
}

func (tc *TypeChecker) checkMatch(s *MatchStmt) {
	tc.inferExpr(s.Expr)
	for _, arm := range s.Arms {
		if arm.Pattern != nil {
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
			tc.scope.define(s.ErrVar, &VarInfo{Type: TypInt, Sp: s.Sp})
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
			errCode("E22", s.Sp, fmt.Sprintf("cannot assign to const/our %q — it is immutable", id.Name),
				"use let/my instead of const/our if you need a mutable variable")
			tc.ok = false
			return
		}
		if vi != nil && vi.IsFn {
			errCode("E23", s.Sp, fmt.Sprintf("cannot assign to function %q — functions are not variables", id.Name),
				"if you want a function pointer, use: let f = fn_name")
			tc.ok = false
			return
		}
	}
	switch s.LHS.(type) {
	case *IntLit, *FloatLit, *BoolLit, *StrLit:
		errCode("E24", s.Sp, "left side of assignment is a literal — you cannot assign to a value",
			"use a variable name on the left side: let x = ...")
		tc.ok = false
		return
	}
	rhsType := tc.inferExpr(s.Value)
	if lhsType.Kind != TyUnknown && lhsType.Kind != TyAny && rhsType.Kind != TyUnknown && rhsType.Kind != TyAny {
		if !coercible(rhsType, lhsType) {
			errCode("E25", s.Sp, fmt.Sprintf("type mismatch: cannot assign %s to %s variable", rhsType, lhsType),
				fmt.Sprintf("cast the right side: %s(expr)", lhsType))
			tc.ok = false
		}
	}
	if s.Op != "=" && lhsType.Kind != TyAny && !isNumeric(lhsType) {
		errCode("E26", s.Sp, fmt.Sprintf("%s requires a numeric type, but variable is %s", s.Op, lhsType),
			"compound assignment (+=, -=, *=, /=) only works on int, float, and char")
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
	case *CmdExpr:
		e.Typ = TypStr
		tc.inferExpr(e.Command)
		return TypStr
	case *ReadFileExpr:
		e.Typ = TypStr
		tc.inferExpr(e.Path)
		return TypStr
	case *TernaryExpr:
		tc.inferExpr(e.Cond)
		then := tc.inferExpr(e.Then)
		tc.inferExpr(e.Else)
		e.Typ = then
		return then

	case *Ident:
		vi := tc.scope.lookup(e.Name)
		if vi == nil {
			if bd := LookupBuiltin(e.Name); bd != nil {
				e.Typ = bd.Ret
				return bd.Ret
			}
			suggestion := tc.suggestName(e.Name)
			hint := fmt.Sprintf("declare it: let %s = ...", e.Name)
			if suggestion != "" {
				hint = fmt.Sprintf("did you mean %q?", suggestion)
			}
			errCode("E27", e.Sp, fmt.Sprintf("undefined variable or function %q", e.Name), hint)
			tc.ok = false
			e.Typ = TypUnknown
			return TypUnknown
		}
		vi.UsedCount++
		e.Typ = vi.Type
		return vi.Type

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
			errCode("E47", e.Idx.nodeSpan(), fmt.Sprintf("array index must be int, got %s", idx),
				"use an integer expression as the array/string index")
			tc.ok = false
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
			errCode("E46", e.Sp, fmt.Sprintf("cannot index into type %s — only arrays and strings can be indexed", obj),
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
			errCode("E28", e.Sp, fmt.Sprintf("cannot cast %s to %s", from, e.ToType),
				"valid casts: between numeric types (int/float/char/bool), and between str and ref char")
			tc.ok = false
		}
		e.Typ = e.ToType
		return e.ToType

	case *AddrExpr:
		inner := tc.inferExpr(e.Operand)
		if e.Deref {
			if inner.Kind != TyRef {
				errCode("E29", e.Sp, fmt.Sprintf("cannot dereference type %s with ^ — it is not a ref", inner),
					"only ref<T> values can be dereferenced — use ^ on a ref variable")
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
				errCode("E55", el.nodeSpan(), fmt.Sprintf("array element %d has type %s, but first element is %s", i+2, got, first),
					fmt.Sprintf("cast this element to %s to make all elements the same type", first))
				tc.ok = false
			}
		}
		e.Typ = ArrayOf(first, len(e.Elems))
		return e.Typ

	default:
		return TypUnknown
	}
}

func (tc *TypeChecker) inferBin(e *BinExpr) *ZXType {
	lhs := tc.inferExpr(e.LHS)
	rhs := tc.inferExpr(e.RHS)
	switch e.Op {
	case "==", "!=":
		if lhs.Kind != TyUnknown && rhs.Kind != TyUnknown && lhs.Kind != TyAny && rhs.Kind != TyAny {
			if !coercible(lhs, rhs) && !coercible(rhs, lhs) {
				errCode("E30", e.Sp, fmt.Sprintf("cannot compare %s with %s using %s — incompatible types", lhs, rhs, e.Op),
					"both sides of == or != must have the same (or compatible) types")
				tc.ok = false
			}
		}
		e.Typ = TypBool
		return TypBool
	case "<", ">", "<=", ">=":
		if lhs.Kind != TyAny && rhs.Kind != TyAny && lhs.Kind != TyUnknown {
			if !isNumeric(lhs) || !isNumeric(rhs) {
				errCode("E31", e.Sp, fmt.Sprintf("comparison %s requires numeric operands, got %s and %s", e.Op, lhs, rhs),
					"comparisons only work on int, float, and char")
				tc.ok = false
			}
		}
		e.Typ = TypBool
		return TypBool
	case "&&", "||":
		e.Typ = TypBool
		return TypBool
	case "+":
		if lhs.Kind == TyStr || rhs.Kind == TyStr {
			errCode("E32", e.Sp, "'+' cannot concatenate strings — ZX does not support string addition",
				"use str_cat(a, b) from std::str, or use f-strings: f\"{a}{b}\"")
			tc.ok = false
			e.Typ = TypStr
			return TypStr
		}
		fallthrough
	case "-", "*", "/", "%":
		if lhs.Kind != TyAny && lhs.Kind != TyUnknown && !isNumeric(lhs) {
			errCode("E33", e.LHS.nodeSpan(), fmt.Sprintf("operator '%s' requires numeric operands, but left side is %s", e.Op, lhs),
				"arithmetic operators only work on int, float, and char")
			tc.ok = false
		}
		if e.Op == "/" {
			if lit, ok := e.RHS.(*IntLit); ok && lit.Val == 0 {
				errCode("E34", e.RHS.nodeSpan(), "division by literal zero — this will crash at runtime",
					"use a variable as divisor, or check for zero before dividing")
				tc.ok = false
			}
			if lit, ok := e.RHS.(*FloatLit); ok && lit.Val == 0.0 {
				warnCode("W03", e.RHS.nodeSpan(), "division by 0.0 will produce Inf or NaN",
					"check for zero before dividing floating-point values")
			}
		}
		if e.Op == "%" && (lhs.Kind == TyFloat || rhs.Kind == TyFloat) {
			errCode("E35", e.Sp, "modulo '%' cannot be used with float operands",
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
			errCode("E36", e.Sp, fmt.Sprintf("bitwise operator '%s' requires integer operands, got %s", e.Op, lhs),
				"bitwise operators only work on int and char")
			tc.ok = false
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
			errCode("E37", e.Sp, fmt.Sprintf("logical NOT '!' cannot be applied to type %s", inner),
				"'!' works on bool, int, and ref values only")
			tc.ok = false
		}
		e.Typ = TypBool
		return TypBool
	case "-":
		if inner.Kind != TyAny && inner.Kind != TyUnknown && !isNumeric(inner) {
			errCode("E38", e.Sp, fmt.Sprintf("unary '-' cannot be applied to type %s", inner),
				"unary minus only works on numeric types (int, float, char)")
			tc.ok = false
		}
		e.Typ = inner
		return inner
	case "~":
		if inner.Kind != TyAny && inner.Kind != TyUnknown && !isInteger(inner) {
			errCode("E39", e.Sp, fmt.Sprintf("bitwise NOT '~' cannot be applied to type %s", inner),
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

	// builtin
	if bd := LookupBuiltin(fnName); bd != nil {
		for _, a := range e.Args {
			tc.inferExpr(a)
		}
		if bd.Arity >= 0 && len(e.Args) != bd.Arity {
			errCode("E41", e.Sp, fmt.Sprintf("builtin %q expects %d argument(s), got %d", fnName, bd.Arity, len(e.Args)),
				"check the number of arguments")
			tc.ok = false
		}
		if id, ok := e.Func.(*Ident); ok {
			id.Typ = bd.Ret
		}
		e.Typ = bd.Ret
		return bd.Ret
	}

	// std fn
	if sf, ok := tc.stdFns[fnName]; ok {
		for _, a := range e.Args {
			tc.inferExpr(a)
		}
		e.Typ = sf.Ret
		return sf.Ret
	}

	// extern
	if ext, ok := tc.externs[fnName]; ok {
		for i, a := range e.Args {
			got := tc.inferExpr(a)
			if !ext.Variadic && i < len(ext.Params) {
				expected := ext.Params[i].Type
				if expected.Kind != TyAny && got.Kind != TyAny && got.Kind != TyUnknown && !coercible(got, expected) {
					errCode("E40", a.nodeSpan(), fmt.Sprintf("extern %q arg %d: expected %s, got %s", fnName, i+1, expected, got),
						fmt.Sprintf("cast with: %s(value)", expected))
					tc.ok = false
				}
			}
		}
		if !ext.Variadic && len(e.Args) != len(ext.Params) {
			errCode("E41", e.Sp, fmt.Sprintf("extern %q expects %d argument(s), got %d", fnName, len(ext.Params), len(e.Args)),
				"check the extern declaration to see the expected parameters")
			tc.ok = false
		}
		if id, ok := e.Func.(*Ident); ok {
			id.Typ = ext.RetType
		}
		e.Typ = ext.RetType
		return ext.RetType
	}

	// user fn
	if fn, ok := tc.fns[fnName]; ok {
		if !fn.Variadic && len(e.Args) != len(fn.Params) {
			// check for params with defaults
			minArgs := 0
			for _, p2 := range fn.Params {
				if p2.Default == nil {
					minArgs++
				}
			}
			if len(e.Args) < minArgs || (!fn.Variadic && len(e.Args) > len(fn.Params)) {
				errCode("E42", e.Sp, fmt.Sprintf("function %q expects %d argument(s), got %d", fnName, len(fn.Params), len(e.Args)),
					fmt.Sprintf("the function signature is: fn %s(%s)", fnName, listParamTypes(fn.Params)))
				tc.ok = false
			}
		}
		for i, a := range e.Args {
			got := tc.inferExpr(a)
			if i < len(fn.Params) {
				expected := fn.Params[i].Type
				if expected != nil && expected.Kind != TyAny && got.Kind != TyAny && got.Kind != TyUnknown && !coercible(got, expected) {
					errCode("E43", a.nodeSpan(), fmt.Sprintf("function %q arg %d: expected %s, got %s", fnName, i+1, expected, got),
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

	// E44 undefined function
	if fnName != "" {
		vi := tc.scope.lookup(fnName)
		if vi == nil {
			suggestion := tc.suggestName(fnName)
			hint := fmt.Sprintf("declare it: extern fn %s(...) -> int  or  fn %s(...) { }", fnName, fnName)
			if suggestion != "" {
				hint = fmt.Sprintf("did you mean %q? (or declare it as an extern fn)", suggestion)
			}
			errCode("E44", e.Sp, fmt.Sprintf("call to undefined function %q", fnName), hint)
			tc.ok = false
			e.Typ = TypUnknown
			return TypUnknown
		}
		if !vi.IsFn {
			errCode("E45", e.Sp, fmt.Sprintf("%q is a %s variable, not callable", fnName, vi.Type),
				fmt.Sprintf("to call a function, declare: fn %s() { } or extern fn %s()", fnName, fnName))
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
	recvType := tc.inferExpr(e.Recv)
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
		hint := fmt.Sprintf("struct %q has no method %q", structName, e.Method)
		if len(available) > 0 {
			hint += fmt.Sprintf(" — available: %s", strings.Join(available, ", "))
		}
		errCode("E59", e.Sp, hint,
			fmt.Sprintf("define it: fn (s ref %s) %s() { }", structName, e.Method))
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

	// 😄 funny warning for dot on ref
	if e.UsedDot && objType.Kind == TyRef {
		funnyWarn(e.Sp,
			"you used '.' on a ref type — sure, it works, but seriously... grow up and use '->' like a C programmer 😄",
			"replace '.' with '->' for ref/pointer field access")
	}

	if eff.Kind != TyStruct {
		if eff.Kind != TyAny && eff.Kind != TyUnknown {
			errCode("E48", e.Sp, fmt.Sprintf("cannot access field %q on type %s — only structs have fields", e.Field, objType),
				"field access works only on struct types and ref<StructType>")
			tc.ok = false
		}
		e.Typ = TypAny
		return TypAny
	}
	sd, ok := tc.structs[eff.Name]
	if !ok {
		errCode("E49", e.Sp, fmt.Sprintf("struct type %q is not defined", eff.Name),
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
	errCode("E50", e.Sp, fmt.Sprintf("struct %q has no field %q", eff.Name, e.Field),
		fmt.Sprintf("available fields: %s", listFields(sd.Fields)))
	tc.ok = false
	e.Typ = TypAny
	return TypAny
}

func (tc *TypeChecker) inferStructInit(e *StructInit) *ZXType {
	sd, ok := tc.structs[e.Name]
	if !ok {
		errCode("E51", e.Sp, fmt.Sprintf("undefined struct %q in initializer", e.Name),
			fmt.Sprintf("declare it first: type %s struct { ... }", e.Name))
		tc.ok = false
		e.Typ = TypUnknown
		return TypUnknown
	}
	provided := map[string]bool{}
	for _, fi := range e.Fields {
		found := false
		for _, sf := range sd.Fields {
			if sf.Name == fi.Name {
				found = true
				break
			}
		}
		if !found {
			errCode("E52", fi.Sp, fmt.Sprintf("struct %q has no field %q", e.Name, fi.Name),
				fmt.Sprintf("valid fields: %s", listFields(sd.Fields)))
			tc.ok = false
		}
		if provided[fi.Name] {
			errCode("E53", fi.Sp, fmt.Sprintf("field %q set more than once in struct literal", fi.Name),
				"remove the duplicate field assignment")
			tc.ok = false
		}
		provided[fi.Name] = true
		got := tc.inferExpr(fi.Value)
		for _, sf := range sd.Fields {
			if sf.Name == fi.Name && sf.Type != nil && sf.Type.Kind != TyAny &&
				got.Kind != TyAny && got.Kind != TyUnknown && !coercible(got, sf.Type) {
				errCode("E54", fi.Sp, fmt.Sprintf("field %q expects %s but got %s", fi.Name, sf.Type, got),
					fmt.Sprintf("cast with: %s(value)", sf.Type))
				tc.ok = false
				break
			}
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
			errCode("E56", sp, fmt.Sprintf("unknown type %q — no struct with this name is defined", t.Name),
				fmt.Sprintf("declare it: type %s struct { ... }", t.Name))
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
