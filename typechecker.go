package main

import (
	"fmt"
)

// ─────────────────────────────────────────────────────────────────────────────
//  Type checker
// ─────────────────────────────────────────────────────────────────────────────

type Scope struct {
	vars   map[string]*VarInfo
	parent *Scope
}

type VarInfo struct {
	Type    *ZXType
	IsConst bool
	Sp      Span
}

func newScope(parent *Scope) *Scope {
	return &Scope{vars: make(map[string]*VarInfo), parent: parent}
}

func (s *Scope) define(name string, vi *VarInfo) {
	s.vars[name] = vi
}

func (s *Scope) lookup(name string) *VarInfo {
	if vi, ok := s.vars[name]; ok {
		return vi
	}
	if s.parent != nil {
		return s.parent.lookup(name)
	}
	return nil
}

// ── TypeChecker ───────────────────────────────────────────────────────────────

type TypeChecker struct {
	prog    *Program
	scope   *Scope
	fnStack []*FnDecl // current function context
	structs map[string]*StructDecl
	fns     map[string]*FnDecl
	externs map[string]*ExternDecl
	ok      bool
}

func TypeCheck(prog *Program, src, file string) bool {
	tc := &TypeChecker{
		prog:    prog,
		scope:   newScope(nil),
		structs: make(map[string]*StructDecl),
		fns:     make(map[string]*FnDecl),
		externs: make(map[string]*ExternDecl),
		ok:      true,
	}

	// pre-register structs
	for _, s := range prog.Structs {
		tc.structs[s.Name] = s
	}
	// pre-register functions (forward declarations)
	for _, stmt := range prog.TopStmts {
		if fn, ok := stmt.(*FnDecl); ok {
			tc.fns[fn.Name] = fn
			tc.scope.define(fn.Name, &VarInfo{
				Type: &ZXType{Kind: TyVoid, Name: fn.Name}, // placeholder
				Sp:   fn.Sp,
			})
		}
	}
	// pre-register externs
	for _, e := range prog.Externs {
		tc.externs[e.Name] = e
		tc.scope.define(e.Name, &VarInfo{Type: TypVoid, Sp: e.Sp})
	}

	tc.checkTopLevel()
	return tc.ok
}

func (tc *TypeChecker) checkTopLevel() {
	for _, stmt := range tc.prog.TopStmts {
		switch n := stmt.(type) {
		case *FnDecl:
			tc.checkFn(n)
		default:
			tc.checkStmt(stmt)
		}
	}
}

func (tc *TypeChecker) checkFn(fn *FnDecl) {
	saved := tc.scope
	tc.scope = newScope(saved)
	tc.fnStack = append(tc.fnStack, fn)

	for _, p := range fn.Params {
		tc.scope.define(p.Name, &VarInfo{Type: p.Type, Sp: p.Sp})
	}
	tc.checkBlock(fn.Body)

	tc.fnStack = tc.fnStack[:len(tc.fnStack)-1]
	tc.scope = saved
}

func (tc *TypeChecker) checkBlock(b *Block) {
	saved := tc.scope
	tc.scope = newScope(saved)
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
	case *WhileStmt:
		tc.checkWhile(s)
	case *ForRangeStmt:
		tc.checkForRange(s)
	case *AssignStmt:
		tc.checkAssign(s)
	case *ExprStmt:
		tc.inferExpr(s.Expr)
	case *PrintStmt:
		for _, a := range s.Args {
			tc.inferExpr(a)
		}
	case *ExitStmt:
		t := tc.inferExpr(s.Code)
		if !typeEq(t, TypInt) {
			errAt(s.Sp, fmt.Sprintf("exit() expects int, got %s", t),
				"use an integer exit code like exit(0) or exit(1)")
			tc.ok = false
		}
	case *BreakStmt, *ContinueStmt:
		// ok
	case *FnDecl:
		tc.checkFn(s)
	case *Block:
		tc.checkBlock(s)
	}
}

func (tc *TypeChecker) checkVarDecl(v *VarDecl) {
	var initType *ZXType
	if v.Init != nil {
		initType = tc.inferExpr(v.Init)
	}

	resolvedType := v.VarType
	if resolvedType == nil || resolvedType.Kind == TyUnknown {
		if initType == nil {
			errAt(v.Sp, fmt.Sprintf("cannot infer type of %q without initializer", v.Name),
				"add a type annotation: let "+v.Name+": int = ...")
			tc.ok = false
			resolvedType = TypUnknown
		} else {
			resolvedType = initType
		}
	} else if initType != nil && !coercible(initType, resolvedType) {
		errAt(v.Sp,
			fmt.Sprintf("type mismatch: cannot assign %s to %s", initType, resolvedType),
			fmt.Sprintf("change the type annotation to %s, or cast the value", initType))
		tc.ok = false
	}

	v.ResolvedType = resolvedType
	tc.scope.define(v.Name, &VarInfo{Type: resolvedType, IsConst: v.IsConst, Sp: v.Sp})
}

func (tc *TypeChecker) checkReturn(r *ReturnStmt) {
	if len(tc.fnStack) == 0 {
		errAt(r.Sp, "return outside of function",
			"move this inside a fn block")
		tc.ok = false
		return
	}
	fn := tc.fnStack[len(tc.fnStack)-1]
	if r.Value == nil {
		if !typeEq(fn.RetType, TypVoid) {
			errAt(r.Sp,
				fmt.Sprintf("function %q must return %s, got empty return", fn.Name, fn.RetType),
				fmt.Sprintf("return a %s value", fn.RetType))
			tc.ok = false
		}
		return
	}
	got := tc.inferExpr(r.Value)
	if !coercible(got, fn.RetType) {
		errAt(r.Sp,
			fmt.Sprintf("return type mismatch: function %q returns %s, got %s", fn.Name, fn.RetType, got),
			fmt.Sprintf("cast to %s or change the return type", fn.RetType))
		tc.ok = false
	}
}

func (tc *TypeChecker) checkIf(s *IfStmt) {
	cond := tc.inferExpr(s.Cond)
	if !typeEq(cond, TypBool) && !typeEq(cond, TypInt) {
		errAt(s.Sp,
			fmt.Sprintf("if condition must be bool, got %s", cond),
			"use a comparison operator like == or != to produce a bool")
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

func (tc *TypeChecker) checkWhile(s *WhileStmt) {
	cond := tc.inferExpr(s.Cond)
	if !typeEq(cond, TypBool) && !typeEq(cond, TypInt) {
		errAt(s.Sp,
			fmt.Sprintf("while condition must be bool, got %s", cond),
			"use a comparison or boolean expression")
		tc.ok = false
	}
	tc.checkBlock(s.Body)
}

func (tc *TypeChecker) checkForRange(s *ForRangeStmt) {
	fromT := tc.inferExpr(s.From)
	toT := tc.inferExpr(s.To)
	if !typeEq(fromT, TypInt) {
		errAt(s.From.nodeSpan(), fmt.Sprintf("for range start must be int, got %s", fromT),
			"use an integer expression")
		tc.ok = false
	}
	if !typeEq(toT, TypInt) {
		errAt(s.To.nodeSpan(), fmt.Sprintf("for range end must be int, got %s", toT),
			"use an integer expression")
		tc.ok = false
	}
	saved := tc.scope
	tc.scope = newScope(saved)
	tc.scope.define(s.Var, &VarInfo{Type: TypInt, Sp: s.Sp})
	tc.checkBlock(s.Body)
	tc.scope = saved
}

func (tc *TypeChecker) checkAssign(s *AssignStmt) {
	lhsType := tc.inferExpr(s.LHS)
	// check not const
	if id, ok := s.LHS.(*Ident); ok {
		vi := tc.scope.lookup(id.Name)
		if vi != nil && vi.IsConst {
			errAt(s.Sp,
				fmt.Sprintf("cannot assign to const %q", id.Name),
				"use let instead of const if you need a mutable variable")
			tc.ok = false
			return
		}
	}
	rhsType := tc.inferExpr(s.Value)
	if !coercible(rhsType, lhsType) {
		errAt(s.Sp,
			fmt.Sprintf("assignment type mismatch: %s = %s", lhsType, rhsType),
			fmt.Sprintf("cast the right side to %s", lhsType))
		tc.ok = false
	}
}

// ── Expression type inference ─────────────────────────────────────────────────

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
		return PtrOf(TypVoid)
	case *Ident:
		vi := tc.scope.lookup(e.Name)
		if vi == nil {
			errAt(e.Sp,
				fmt.Sprintf("undefined variable %q", e.Name),
				fmt.Sprintf("declare it with: let %s: <type> = ...", e.Name))
			tc.ok = false
			e.Typ = TypUnknown
			return TypUnknown
		}
		e.Typ = vi.Type
		return vi.Type
	case *BinExpr:
		return tc.inferBin(e)
	case *UnaryExpr:
		return tc.inferUnary(e)
	case *CallExpr:
		return tc.inferCall(e)
	case *IndexExpr:
		tc.inferExpr(e.Obj)
		tc.inferExpr(e.Idx)
		e.Typ = TypInt // best guess without array type info
		return e.Typ
	case *FieldExpr:
		return tc.inferField(e)
	case *CastExpr:
		tc.inferExpr(e.Operand)
		e.Typ = e.ToType
		return e.ToType
	case *AddrExpr:
		inner := tc.inferExpr(e.Operand)
		if e.Deref {
			if inner.Kind == TyPtr {
				e.Typ = inner.PtrElem
				return inner.PtrElem
			}
			warnAt(e.Sp, fmt.Sprintf("dereferencing non-pointer type %s", inner),
				"make sure this is a ptr<T>")
			e.Typ = inner
			return inner
		}
		e.Typ = PtrOf(inner)
		return e.Typ
	case *StructInit:
		return tc.inferStructInit(e)
	default:
		return TypUnknown
	}
}

func (tc *TypeChecker) inferBin(e *BinExpr) *ZXType {
	lhs := tc.inferExpr(e.LHS)
	rhs := tc.inferExpr(e.RHS)

	switch e.Op {
	case "==", "!=", "<", ">", "<=", ">=":
		if !coercible(lhs, rhs) && !coercible(rhs, lhs) {
			errAt(e.Sp,
				fmt.Sprintf("cannot compare %s with %s", lhs, rhs),
				"both sides of a comparison should have compatible types")
			tc.ok = false
		}
		e.Typ = TypBool
		return TypBool
	case "&&", "||":
		e.Typ = TypBool
		return TypBool
	case "+", "-", "*", "/", "%":
		if lhs.Kind == TyFloat || rhs.Kind == TyFloat {
			e.Typ = TypFloat
			return TypFloat
		}
		if lhs.Kind == TyStr && e.Op == "+" {
			// str concat not supported natively, warn
			warnAt(e.Sp, "string concatenation with + is not supported",
				"use printf or sprintf from the C stdlib instead")
		}
		e.Typ = lhs
		return lhs
	case "|", "&", "^", "<<", ">>":
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
		e.Typ = TypBool
		return TypBool
	case "-":
		e.Typ = inner
		return inner
	case "~":
		e.Typ = TypInt
		return TypInt
	}
	e.Typ = inner
	return inner
}

func (tc *TypeChecker) inferCall(e *CallExpr) *ZXType {
	// resolve function name
	var fnName string
	if id, ok := e.Func.(*Ident); ok {
		fnName = id.Name
	}

	// check externs
	if ext, ok := tc.externs[fnName]; ok {
		for _, a := range e.Args {
			tc.inferExpr(a)
		}
		if !ext.Variadic && len(e.Args) != len(ext.Params) {
			errAt(e.Sp,
				fmt.Sprintf("extern %q expects %d args, got %d", fnName, len(ext.Params), len(e.Args)),
				"check the function signature")
			tc.ok = false
		}
		e.Typ = ext.RetType
		return ext.RetType
	}

	// check user fns
	if fn, ok := tc.fns[fnName]; ok {
		for i, a := range e.Args {
			got := tc.inferExpr(a)
			if i < len(fn.Params) {
				expected := fn.Params[i].Type
				if !coercible(got, expected) {
					errAt(a.nodeSpan(),
						fmt.Sprintf("argument %d type mismatch: expected %s, got %s", i+1, expected, got),
						fmt.Sprintf("cast to %s", expected))
					tc.ok = false
				}
			}
		}
		if !fn.Variadic && len(e.Args) != len(fn.Params) {
			errAt(e.Sp,
				fmt.Sprintf("function %q expects %d args, got %d", fnName, len(fn.Params), len(e.Args)),
				"check the call arguments")
			tc.ok = false
		}
		e.Typ = fn.RetType
		return fn.RetType
	}

	// unknown function — could be a C stdlib fn imported via include
	for _, a := range e.Args {
		tc.inferExpr(a)
	}
	e.Typ = TypInt // assume int return for unknown C fns
	return e.Typ
}

func (tc *TypeChecker) inferField(e *FieldExpr) *ZXType {
	objType := tc.inferExpr(e.Obj)
	var structName string
	if objType.Kind == TyStruct {
		structName = objType.Name
	} else if objType.Kind == TyPtr && objType.PtrElem != nil && objType.PtrElem.Kind == TyStruct {
		structName = objType.PtrElem.Name
	}
	if structName != "" {
		if sd, ok := tc.structs[structName]; ok {
			for _, f := range sd.Fields {
				if f.Name == e.Field {
					e.Typ = f.Type
					return f.Type
				}
			}
			errAt(e.Sp,
				fmt.Sprintf("struct %q has no field %q", structName, e.Field),
				fmt.Sprintf("valid fields: %s", listFields(sd.Fields)))
			tc.ok = false
		}
	}
	e.Typ = TypUnknown
	return TypUnknown
}

func (tc *TypeChecker) inferStructInit(e *StructInit) *ZXType {
	sd, ok := tc.structs[e.Name]
	if !ok {
		errAt(e.Sp,
			fmt.Sprintf("undefined struct %q", e.Name),
			"declare it with: struct "+e.Name+" { ... }")
		tc.ok = false
		e.Typ = TypUnknown
		return TypUnknown
	}
	for _, fi := range e.Fields {
		tc.inferExpr(fi.Value)
		// check field exists
		found := false
		for _, sf := range sd.Fields {
			if sf.Name == fi.Name {
				found = true
				break
			}
		}
		if !found {
			errAt(fi.Sp,
				fmt.Sprintf("struct %q has no field %q", e.Name, fi.Name),
				fmt.Sprintf("valid fields: %s", listFields(sd.Fields)))
			tc.ok = false
		}
	}
	e.Typ = StructType(e.Name)
	return e.Typ
}

func listFields(params []Param) string {
	s := ""
	for i, p := range params {
		if i > 0 {
			s += ", "
		}
		s += p.Name
	}
	return s
}
