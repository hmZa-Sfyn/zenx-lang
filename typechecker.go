package main

import (
	"fmt"
	"strings"
)

// ─────────────────────────────────────────────────────────────────────────────
//  Scope / symbol table
// ─────────────────────────────────────────────────────────────────────────────

type VarInfo struct {
	Type      *ZXType
	IsConst   bool
	IsFn      bool
	IsMethod  bool
	IsExtern  bool
	Sp        Span
	UsedCount int
}

type Scope struct {
	vars   map[string]*VarInfo
	parent *Scope
	kind   string
}

func newScope(parent *Scope, kind string) *Scope {
	return &Scope{vars: make(map[string]*VarInfo), parent: parent, kind: kind}
}
func (s *Scope) define(name string, vi *VarInfo)   { s.vars[name] = vi }
func (s *Scope) lookupLocal(name string) *VarInfo   { return s.vars[name] }
func (s *Scope) lookup(name string) *VarInfo {
	if vi, ok := s.vars[name]; ok { return vi }
	if s.parent != nil { return s.parent.lookup(name) }
	return nil
}
func (s *Scope) inLoop() bool {
	if s == nil { return false }
	if s.kind == "loop" { return true }
	return s.parent.inLoop()
}

// ─────────────────────────────────────────────────────────────────────────────
//  TypeChecker
// ─────────────────────────────────────────────────────────────────────────────

type TypeChecker struct {
	prog        *Program
	scope       *Scope
	fnStack     []*FnDecl
	methodStack []*MethodDecl
	structs     map[string]*StructDecl
	fns         map[string]*FnDecl
	methods     map[string]*MethodDecl // key = "Type_Method"
	externs     map[string]*ExternDecl
	importPaths map[string]bool
	ok          bool
}

func TypeCheck(prog *Program, src, file string) bool {
	tc := &TypeChecker{
		prog:        prog,
		structs:     make(map[string]*StructDecl),
		fns:         make(map[string]*FnDecl),
		methods:     make(map[string]*MethodDecl),
		externs:     make(map[string]*ExternDecl),
		importPaths: make(map[string]bool),
		ok:          true,
	}
	tc.scope = newScope(nil, "global")

	for _, imp := range prog.Imports { tc.importPaths[imp.Path] = true }

	// register structs
	for _, s := range prog.Structs {
		if _, exists := tc.structs[s.Name]; exists {
			errAt(s.Sp, fmt.Sprintf("E01: struct %q defined more than once", s.Name),
				"rename one of the definitions")
			tc.ok = false
		}
		tc.structs[s.Name] = s
	}

	// register externs
	for _, e := range prog.Externs {
		tc.externs[e.Name] = e
		tc.scope.define(e.Name, &VarInfo{Type: e.RetType, IsFn: true, IsExtern: true, Sp: e.Sp})
	}

	// register user functions
	for _, stmt := range prog.TopStmts {
		if fn, ok := stmt.(*FnDecl); ok {
			if _, exists := tc.fns[fn.Name]; exists {
				errAt(fn.Sp, fmt.Sprintf("E03: function %q defined more than once", fn.Name), "")
				tc.ok = false
			}
			tc.fns[fn.Name] = fn
			tc.scope.define(fn.Name, &VarInfo{Type: fn.RetType, IsFn: true, Sp: fn.Sp})
		}
	}

	// register methods
	for _, m := range prog.Methods {
		key := m.CName()
		tc.methods[key] = m
		tc.scope.define(key, &VarInfo{Type: m.RetType, IsFn: true, IsMethod: true, Sp: m.Sp})
	}

	// check function bodies
	for _, stmt := range prog.TopStmts {
		if fn, ok := stmt.(*FnDecl); ok { tc.checkFn(fn) }
	}
	// check method bodies
	for _, m := range prog.Methods { tc.checkMethod(m) }
	// check top-level stmts
	for _, stmt := range prog.TopStmts {
		if _, ok := stmt.(*FnDecl); !ok { tc.checkStmt(stmt) }
	}
	return tc.ok
}

func (tc *TypeChecker) currentFn() *FnDecl {
	if len(tc.fnStack) == 0 { return nil }
	return tc.fnStack[len(tc.fnStack)-1]
}
func (tc *TypeChecker) currentMethod() *MethodDecl {
	if len(tc.methodStack) == 0 { return nil }
	return tc.methodStack[len(tc.methodStack)-1]
}
func (tc *TypeChecker) currentRetType() *ZXType {
	if m := tc.currentMethod(); m != nil { return m.RetType }
	if f := tc.currentFn(); f != nil { return f.RetType }
	return nil
}

func (tc *TypeChecker) checkFn(fn *FnDecl) {
	saved := tc.scope
	tc.scope = newScope(saved, "fn")
	tc.fnStack = append(tc.fnStack, fn)
	for _, p := range fn.Params {
		t := p.Type
		if t == nil { t = TypAny }
		tc.scope.define(p.Name, &VarInfo{Type: t, Sp: p.Sp})
	}
	tc.checkBlock(fn.Body)
	tc.fnStack = tc.fnStack[:len(tc.fnStack)-1]
	tc.scope = saved
}

func (tc *TypeChecker) checkMethod(m *MethodDecl) {
	saved := tc.scope
	tc.scope = newScope(saved, "fn")
	tc.methodStack = append(tc.methodStack, m)
	// define receiver
	recvType := PtrOf(StructType(m.RecvType))
	if !m.RecvPtr { recvType = StructType(m.RecvType) }
	tc.scope.define(m.RecvName, &VarInfo{Type: recvType, Sp: m.Sp})
	for _, p := range m.Params {
		t := p.Type
		if t == nil { t = TypAny }
		tc.scope.define(p.Name, &VarInfo{Type: t, Sp: p.Sp})
	}
	tc.checkBlock(m.Body)
	tc.methodStack = tc.methodStack[:len(tc.methodStack)-1]
	tc.scope = saved
}

func (tc *TypeChecker) checkBlock(b *Block) {
	if b == nil { return }
	saved := tc.scope
	tc.scope = newScope(saved, "block")
	for _, s := range b.Stmts { tc.checkStmt(s) }
	tc.scope = saved
}

func (tc *TypeChecker) checkBlockInLoop(b *Block) {
	if b == nil { return }
	saved := tc.scope
	tc.scope = newScope(saved, "loop")
	for _, s := range b.Stmts { tc.checkStmt(s) }
	tc.scope = saved
}

func (tc *TypeChecker) checkStmt(n Node) {
	if n == nil { return }
	switch s := n.(type) {
	case *VarDecl:        tc.checkVarDecl(s)
	case *ReturnStmt:     tc.checkReturn(s)
	case *IfStmt:         tc.checkIf(s)
	case *UnlessStmt:     tc.checkUnless(s)
	case *WhileStmt:      tc.checkWhile(s)
	case *UntilStmt:      tc.inferExpr(s.Cond); tc.checkBlockInLoop(s.Body)
	case *ForRangeStmt:   tc.checkForRange(s)
	case *AssignStmt:     tc.checkAssign(s)
	case *ExprStmt:       tc.inferExpr(s.Expr)
	case *PrintStmt:      for _, a := range s.Args { tc.inferExpr(a) }
	case *ExitStmt:       tc.inferExpr(s.Code)
	case *BreakStmt:
		if !tc.scope.inLoop() {
			errAt(s.Sp, "E07: 'break' used outside of a loop", "break/last can only appear inside while/for")
			tc.ok = false
		}
	case *ContinueStmt:
		if !tc.scope.inLoop() {
			errAt(s.Sp, "E08: 'continue' used outside of a loop", "continue/next can only appear inside while/for")
			tc.ok = false
		}
	case *FnDecl:   tc.checkFn(s)
	case *Block:    tc.checkBlock(s)
	}
}

func (tc *TypeChecker) checkVarDecl(v *VarDecl) {
	if existing := tc.scope.lookupLocal(v.Name); existing != nil {
		errAt(v.Sp, fmt.Sprintf("E09: variable %q already declared in this scope", v.Name),
			"use a different name or remove the duplicate")
		noteAt(existing.Sp, "previous declaration was here")
		tc.ok = false
	}
	var initType *ZXType
	if v.Init != nil { initType = tc.inferExpr(v.Init) }

	resolved := v.VarType
	if resolved == nil || resolved.Kind == TyUnknown {
		if initType == nil || initType.Kind == TyUnknown {
			resolved = TypAny
		} else {
			resolved = initType
		}
	} else if initType != nil && initType.Kind != TyUnknown && initType.Kind != TyAny {
		if !coercible(initType, resolved) {
			errAt(v.Sp,
				fmt.Sprintf("E11: cannot initialize %s variable with %s value", resolved, initType),
				fmt.Sprintf("cast with %s(...) or change the type to %s", resolved, initType))
			tc.ok = false
		}
	}
	if resolved != nil && resolved.Kind == TyVoid {
		errAt(v.Sp, fmt.Sprintf("E12: cannot declare variable %q with type void", v.Name),
			"use a concrete type")
		tc.ok = false
	}
	v.ResolvedType = resolved
	tc.scope.define(v.Name, &VarInfo{Type: resolved, IsConst: v.IsConst, Sp: v.Sp})
}

func (tc *TypeChecker) checkReturn(r *ReturnStmt) {
	retType := tc.currentRetType()
	if retType == nil {
		errAt(r.Sp, "E14: 'return' used outside of a function", "")
		tc.ok = false; return
	}
	if r.Value == nil {
		if retType.Kind != TyVoid && retType.Kind != TyAny {
			errAt(r.Sp,
				fmt.Sprintf("E15: function must return %s, got empty return", retType),
				fmt.Sprintf("return a %s value", retType))
			tc.ok = false
		}
		return
	}
	got := tc.inferExpr(r.Value)
	if retType.Kind == TyVoid {
		errAt(r.Sp,
			fmt.Sprintf("E16: void function returns a %s value", got),
			"remove the return value or change the return type")
		tc.ok = false; return
	}
	if got.Kind != TyUnknown && got.Kind != TyAny && retType.Kind != TyAny && !coercible(got, retType) {
		errAt(r.Sp,
			fmt.Sprintf("E17: return type mismatch — expected %s, got %s", retType, got),
			fmt.Sprintf("cast with %s(...)", retType))
		tc.ok = false
	}
}

func (tc *TypeChecker) checkIf(s *IfStmt) {
	tc.inferExpr(s.Cond)
	tc.checkBlock(s.Then)
	for _, el := range s.Elifs { tc.inferExpr(el.Cond); tc.checkBlock(el.Body) }
	if s.Else != nil { tc.checkBlock(s.Else) }
}

func (tc *TypeChecker) checkUnless(s *UnlessStmt) {
	tc.inferExpr(s.Cond)
	tc.checkBlock(s.Body)
	if s.Else != nil { tc.checkBlock(s.Else) }
}

func (tc *TypeChecker) checkWhile(s *WhileStmt) {
	tc.inferExpr(s.Cond)
	tc.checkBlockInLoop(s.Body)
}

func (tc *TypeChecker) checkForRange(s *ForRangeStmt) {
	tc.inferExpr(s.From)
	tc.inferExpr(s.To)
	saved := tc.scope
	tc.scope = newScope(saved, "loop")
	tc.scope.define(s.Var, &VarInfo{Type: TypInt, Sp: s.Sp})
	for _, st := range s.Body.Stmts { tc.checkStmt(st) }
	tc.scope = saved
}

func (tc *TypeChecker) checkAssign(s *AssignStmt) {
	lhsType := tc.inferExpr(s.LHS)
	if id, ok := s.LHS.(*Ident); ok {
		vi := tc.scope.lookup(id.Name)
		if vi != nil && vi.IsConst {
			errAt(s.Sp, fmt.Sprintf("E22: cannot assign to const %q", id.Name),
				"use let/my instead of const/our if you need mutation")
			tc.ok = false; return
		}
	}
	rhsType := tc.inferExpr(s.Value)
	if lhsType.Kind != TyUnknown && lhsType.Kind != TyAny &&
		rhsType.Kind != TyUnknown && rhsType.Kind != TyAny {
		if !coercible(rhsType, lhsType) {
			errAt(s.Sp,
				fmt.Sprintf("E25: cannot assign %s to %s variable", rhsType, lhsType),
				fmt.Sprintf("cast the right side with %s(...)", lhsType))
			tc.ok = false
		}
	}
}

// ── Expression inference ──────────────────────────────────────────────────────

func (tc *TypeChecker) inferExpr(n Node) *ZXType {
	if n == nil { return TypVoid }
	switch e := n.(type) {
	case *IntLit:    return TypInt
	case *FloatLit:  return TypFloat
	case *BoolLit:   return TypBool
	case *StrLit:    return TypStr
	case *NilLit:    return PtrOf(TypVoid)
	case *SizeofExpr: e.Typ = TypInt; return TypInt

	case *Ident:
		vi := tc.scope.lookup(e.Name)
		if vi == nil {
			suggestion := tc.suggestName(e.Name)
			hint := fmt.Sprintf("declare it with: let %s = ...", e.Name)
			if suggestion != "" { hint = fmt.Sprintf("did you mean %q?", suggestion) }
			errAt(e.Sp, fmt.Sprintf("E27: undefined variable or function %q", e.Name), hint)
			tc.ok = false; e.Typ = TypUnknown; return TypUnknown
		}
		vi.UsedCount++
		e.Typ = vi.Type
		return vi.Type

	case *BinExpr:   return tc.inferBin(e)
	case *UnaryExpr: return tc.inferUnary(e)
	case *CallExpr:  return tc.inferCall(e)

	case *MethodCallExpr:
		recvType := tc.inferExpr(e.Recv)
		for _, a := range e.Args { tc.inferExpr(a) }
		// look up method
		structName := ""
		if recvType.Kind == TyStruct { structName = recvType.Name }
		if recvType.Kind == TyPtr && recvType.PtrElem != nil && recvType.PtrElem.Kind == TyStruct {
			structName = recvType.PtrElem.Name
		}
		if structName != "" {
			key := structName + "_" + e.Method
			if m, ok := tc.methods[key]; ok {
				e.Typ = m.RetType; return m.RetType
			}
		}
		e.Typ = TypAny; return TypAny

	case *IndexExpr:
		objType := tc.inferExpr(e.Obj)
		tc.inferExpr(e.Idx)
		if objType.Kind == TyArray && objType.ArrElem != nil { e.Typ = objType.ArrElem; return e.Typ }
		if objType.Kind == TyPtr && objType.PtrElem != nil   { e.Typ = objType.PtrElem; return e.Typ }
		if objType.Kind == TyStr                             { e.Typ = TypChar; return TypChar }
		e.Typ = TypAny; return TypAny

	case *FieldExpr: return tc.inferField(e)

	case *CastExpr:
		tc.inferExpr(e.Operand)
		e.Typ = e.ToType; return e.ToType

	case *AddrExpr:
		inner := tc.inferExpr(e.Operand)
		if e.Deref {
			if inner.Kind == TyPtr && inner.PtrElem != nil {
				e.Typ = inner.PtrElem; return inner.PtrElem
			}
			e.Typ = TypAny; return TypAny
		}
		e.Typ = PtrOf(inner); return e.Typ

	case *StructInit: return tc.inferStructInit(e)

	case *ArrayLit:
		if len(e.Elems) == 0 { e.Typ = ArrayOf(TypAny, 0); return e.Typ }
		first := tc.inferExpr(e.Elems[0])
		for _, el := range e.Elems[1:] { tc.inferExpr(el) }
		e.Typ = ArrayOf(first, len(e.Elems)); return e.Typ

	default: return TypUnknown
	}
}

func (tc *TypeChecker) inferBin(e *BinExpr) *ZXType {
	lhs := tc.inferExpr(e.LHS)
	rhs := tc.inferExpr(e.RHS)
	switch e.Op {
	case "==", "!=", "<", ">", "<=", ">=", "&&", "||":
		e.Typ = TypBool; return TypBool
	case "+", "-", "*", "/", "%":
		if e.Op == "+" && (lhs.Kind == TyStr || rhs.Kind == TyStr) {
			errAt(e.Sp, "E32: '+' cannot concatenate strings",
				"use printf/sprintf for string formatting")
			tc.ok = false; e.Typ = TypStr; return TypStr
		}
		if e.Op == "/" {
			if lit, ok := e.RHS.(*IntLit); ok && lit.Val == 0 {
				errAt(e.RHS.nodeSpan(), "E34: division by zero", "the divisor is a literal 0")
				tc.ok = false
			}
		}
		if lhs.Kind == TyFloat || rhs.Kind == TyFloat { e.Typ = TypFloat; return TypFloat }
		e.Typ = lhs; return lhs
	case "|", "&", "^", "<<", ">>":
		e.Typ = TypInt; return TypInt
	default:
		e.Typ = lhs; return lhs
	}
}

func (tc *TypeChecker) inferUnary(e *UnaryExpr) *ZXType {
	inner := tc.inferExpr(e.Operand)
	switch e.Op {
	case "!": e.Typ = TypBool; return TypBool
	case "~": e.Typ = TypInt; return TypInt
	default:  e.Typ = inner; return inner
	}
}

func (tc *TypeChecker) inferCall(e *CallExpr) *ZXType {
	var fnName string
	if id, ok := e.Func.(*Ident); ok { fnName = id.Name }

	if ext, ok := tc.externs[fnName]; ok {
		for _, a := range e.Args { tc.inferExpr(a) }
		if id, ok := e.Func.(*Ident); ok { id.Typ = ext.RetType }
		e.Typ = ext.RetType; return ext.RetType
	}
	if fn, ok := tc.fns[fnName]; ok {
		for i, a := range e.Args {
			got := tc.inferExpr(a)
			if i < len(fn.Params) {
				expected := fn.Params[i].Type
				if expected != nil && expected.Kind != TyAny &&
					got.Kind != TyUnknown && got.Kind != TyAny && !coercible(got, expected) {
					errAt(a.nodeSpan(),
						fmt.Sprintf("E43: arg %d to %q: expected %s, got %s", i+1, fnName, expected, got),
						fmt.Sprintf("cast with %s(...)", expected))
					tc.ok = false
				}
			}
		}
		if id, ok := e.Func.(*Ident); ok { id.Typ = fn.RetType }
		e.Typ = fn.RetType; return fn.RetType
	}
	// unknown — could be a C function from import
	for _, a := range e.Args { tc.inferExpr(a) }
	e.Typ = TypAny; return TypAny
}

func (tc *TypeChecker) inferField(e *FieldExpr) *ZXType {
	objType := tc.inferExpr(e.Obj)
	eff := objType
	if objType.Kind == TyPtr && objType.PtrElem != nil { eff = objType.PtrElem }
	if eff.Kind != TyStruct {
		e.Typ = TypAny; return TypAny
	}
	if sd, ok := tc.structs[eff.Name]; ok {
		for _, f := range sd.Fields {
			if f.Name == e.Field { e.Typ = f.Type; return f.Type }
		}
		errAt(e.Sp,
			fmt.Sprintf("E50: struct %q has no field %q", eff.Name, e.Field),
			fmt.Sprintf("valid fields: %s", listFields(sd.Fields)))
		tc.ok = false
	}
	e.Typ = TypAny; return TypAny
}

func (tc *TypeChecker) inferStructInit(e *StructInit) *ZXType {
	sd, ok := tc.structs[e.Name]
	if !ok {
		errAt(e.Sp, fmt.Sprintf("E51: undefined struct %q", e.Name),
			fmt.Sprintf("declare it with: struct %s { ... }", e.Name))
		tc.ok = false; e.Typ = TypUnknown; return TypUnknown
	}
	for _, fi := range e.Fields {
		tc.inferExpr(fi.Value)
		found := false
		for _, sf := range sd.Fields { if sf.Name == fi.Name { found = true; break } }
		if !found {
			errAt(fi.Sp, fmt.Sprintf("E52: struct %q has no field %q", e.Name, fi.Name),
				fmt.Sprintf("valid fields: %s", listFields(sd.Fields)))
			tc.ok = false
		}
	}
	if e.HeapAlloc {
		e.Typ = PtrOf(StructType(e.Name)); return e.Typ
	}
	e.Typ = StructType(e.Name); return e.Typ
}

func (tc *TypeChecker) suggestName(name string) string {
	best := ""; bestDist := 3
	var cands []string
	for n := range tc.fns     { cands = append(cands, n) }
	for n := range tc.externs { cands = append(cands, n) }
	for n := range tc.methods { cands = append(cands, n) }
	s := tc.scope
	for s != nil { for n := range s.vars { cands = append(cands, n) }; s = s.parent }
	for _, c := range cands {
		if d := editDistance(name, c); d < bestDist { bestDist = d; best = c }
	}
	return best
}

func editDistance(a, b string) int {
	la, lb := len(a), len(b)
	if la == 0 { return lb }
	if lb == 0 { return la }
	prev, curr := make([]int, lb+1), make([]int, lb+1)
	for j := 0; j <= lb; j++ { prev[j] = j }
	for i := 1; i <= la; i++ {
		curr[0] = i
		for j := 1; j <= lb; j++ {
			if a[i-1] == b[j-1] { curr[j] = prev[j-1]
			} else { curr[j] = 1 + min3(prev[j], curr[j-1], prev[j-1]) }
		}
		prev, curr = curr, prev
	}
	return prev[lb]
}

func min3(a, b, c int) int {
	if a < b { if a < c { return a }; return c }
	if b < c { return b }; return c
}

func listFields(params []Param) string {
	parts := make([]string, len(params))
	for i, p := range params {
		t := "any"
		if p.Type != nil { t = p.Type.String() }
		parts[i] = p.Name + ": " + t
	}
	return strings.Join(parts, ", ")
}
