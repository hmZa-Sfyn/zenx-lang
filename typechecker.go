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
	IsExtern  bool
	Sp        Span
	UsedCount int
}

type Scope struct {
	vars   map[string]*VarInfo
	parent *Scope
	kind   string // "global" "fn" "block" "loop"
}

func newScope(parent *Scope, kind string) *Scope {
	return &Scope{vars: make(map[string]*VarInfo), parent: parent, kind: kind}
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

func (s *Scope) lookupLocal(name string) *VarInfo {
	return s.vars[name]
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

// ─────────────────────────────────────────────────────────────────────────────
//  TypeChecker
// ─────────────────────────────────────────────────────────────────────────────

type TypeChecker struct {
	prog        *Program
	scope       *Scope
	fnStack     []*FnDecl
	structs     map[string]*StructDecl
	fns         map[string]*FnDecl
	externs     map[string]*ExternDecl
	importPaths map[string]bool // headers that were imported
	ok          bool
}

func TypeCheck(prog *Program, src, file string) bool {
	tc := &TypeChecker{
		prog:        prog,
		structs:     make(map[string]*StructDecl),
		fns:         make(map[string]*FnDecl),
		externs:     make(map[string]*ExternDecl),
		importPaths: make(map[string]bool),
		ok:          true,
	}
	tc.scope = newScope(nil, "global")

	// collect import paths
	for _, imp := range prog.Imports {
		tc.importPaths[imp.Path] = true
	}

	// pre-register structs
	for _, s := range prog.Structs {
		if _, exists := tc.structs[s.Name]; exists {
			// E01: duplicate struct
			errAt(s.Sp,
				fmt.Sprintf("E01: struct %q is defined more than once", s.Name),
				fmt.Sprintf("rename one of the struct definitions"))
			tc.ok = false
		}
		tc.structs[s.Name] = s
		// check duplicate fields
		seen := map[string]bool{}
		for _, f := range s.Fields {
			if seen[f.Name] {
				// E02: duplicate struct field
				errAt(f.Sp,
					fmt.Sprintf("E02: field %q is defined more than once in struct %q", f.Name, s.Name),
					"remove or rename the duplicate field")
				tc.ok = false
			}
			seen[f.Name] = true
		}
	}

	// pre-register externs
	for _, e := range prog.Externs {
		tc.externs[e.Name] = e
		tc.scope.define(e.Name, &VarInfo{Type: e.RetType, IsFn: true, IsExtern: true, Sp: e.Sp})
	}

	// pre-register user functions (forward decls)
	for _, stmt := range prog.TopStmts {
		if fn, ok := stmt.(*FnDecl); ok {
			if _, exists := tc.fns[fn.Name]; exists {
				// E03: duplicate function
				errAt(fn.Sp,
					fmt.Sprintf("E03: function %q is defined more than once", fn.Name),
					"rename one of the function definitions")
				tc.ok = false
			}
			tc.fns[fn.Name] = fn
			tc.scope.define(fn.Name, &VarInfo{Type: fn.RetType, IsFn: true, Sp: fn.Sp})
		}
	}

	// check function bodies first so top-level stmts can call them
	for _, stmt := range prog.TopStmts {
		if fn, ok := stmt.(*FnDecl); ok {
			tc.checkFn(fn)
		}
	}

	// check top-level statements
	for _, stmt := range prog.TopStmts {
		if _, ok := stmt.(*FnDecl); !ok {
			tc.checkStmt(stmt)
		}
	}

	return tc.ok
}

func (tc *TypeChecker) currentFn() *FnDecl {
	if len(tc.fnStack) == 0 {
		return nil
	}
	return tc.fnStack[len(tc.fnStack)-1]
}

func (tc *TypeChecker) checkFn(fn *FnDecl) {
	saved := tc.scope
	tc.scope = newScope(saved, "fn")
	tc.fnStack = append(tc.fnStack, fn)

	// check for duplicate param names
	seenParams := map[string]bool{}
	for _, p := range fn.Params {
		if seenParams[p.Name] {
			// E04: duplicate param
			errAt(p.Sp,
				fmt.Sprintf("E04: parameter %q is listed more than once in function %q", p.Name, fn.Name),
				"rename one of the parameters")
			tc.ok = false
		}
		seenParams[p.Name] = true
		tc.scope.define(p.Name, &VarInfo{Type: p.Type, Sp: p.Sp})
	}

	// check param types exist
	for _, p := range fn.Params {
		tc.checkTypeExists(p.Type, p.Sp)
	}
	tc.checkTypeExists(fn.RetType, fn.Sp)

	tc.checkBlock(fn.Body)

	tc.fnStack = tc.fnStack[:len(tc.fnStack)-1]
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
		t := tc.inferExpr(s.Expr)
		// E05: discarding non-void call is fine; warn on discarding values
		_ = t
	case *PrintStmt:
		for _, a := range s.Args {
			tc.inferExpr(a)
		}
	case *ExitStmt:
		t := tc.inferExpr(s.Code)
		if !isInteger(t) {
			// E06: exit requires int
			errAt(s.Sp,
				fmt.Sprintf("E06: exit() requires an int argument, got %s", t),
				"use an integer exit code: exit(0) or exit(1)")
			tc.ok = false
		}
	case *BreakStmt:
		if !tc.scope.inLoop() {
			// E07: break outside loop
			errAt(s.Sp, "E07: 'break' used outside of a loop",
				"break can only be used inside a while or for loop")
			tc.ok = false
		}
	case *ContinueStmt:
		if !tc.scope.inLoop() {
			// E08: continue outside loop
			errAt(s.Sp, "E08: 'continue' used outside of a loop",
				"continue can only be used inside a while or for loop")
			tc.ok = false
		}
	case *FnDecl:
		tc.checkFn(s)
	case *Block:
		tc.checkBlock(s)
	}
}

func (tc *TypeChecker) checkVarDecl(v *VarDecl) {
	// E09: redeclaration in same scope
	if existing := tc.scope.lookupLocal(v.Name); existing != nil {
		errAt(v.Sp,
			fmt.Sprintf("E09: variable %q is already declared in this scope", v.Name),
			fmt.Sprintf("use a different name, or remove the duplicate declaration"))
		noteAt(existing.Sp, "previous declaration was here")
		tc.ok = false
	}

	var initType *ZXType
	if v.Init != nil {
		initType = tc.inferExpr(v.Init)
	}

	resolved := v.VarType
	if resolved == nil || resolved.Kind == TyUnknown {
		if initType == nil {
			// E10: cannot infer type
			errAt(v.Sp,
				fmt.Sprintf("E10: cannot infer the type of %q — no initializer provided", v.Name),
				fmt.Sprintf("add a type annotation: let %s: int = ...", v.Name))
			tc.ok = false
			resolved = TypUnknown
		} else if initType.Kind == TyUnknown {
			resolved = TypUnknown
		} else {
			resolved = initType
		}
	} else {
		// check declared type exists
		tc.checkTypeExists(resolved, v.Sp)

		if initType != nil && initType.Kind != TyUnknown {
			if !coercible(initType, resolved) {
				// E11: type mismatch in assignment
				errAt(v.Sp,
					fmt.Sprintf("E11: type mismatch — cannot initialize %s variable with %s value", resolved, initType),
					fmt.Sprintf("cast with %s(...) or change the type to %s", resolved, initType))
				tc.ok = false
			}
		}
	}

	// E12: void variable
	if resolved != nil && resolved.Kind == TyVoid {
		errAt(v.Sp,
			fmt.Sprintf("E12: cannot declare variable %q with type void", v.Name),
			"use a concrete type like int, float, str, etc.")
		tc.ok = false
	}

	// E13: const without init (already caught in parser but double-check)
	if v.IsConst && v.Init == nil {
		errAt(v.Sp,
			fmt.Sprintf("E13: const %q must have an initializer", v.Name),
			fmt.Sprintf("add = <value> after the type, e.g. const %s: int = 42", v.Name))
		tc.ok = false
	}

	v.ResolvedType = resolved
	tc.scope.define(v.Name, &VarInfo{Type: resolved, IsConst: v.IsConst, Sp: v.Sp})
}

func (tc *TypeChecker) checkReturn(r *ReturnStmt) {
	fn := tc.currentFn()
	if fn == nil {
		// E14: return outside function
		errAt(r.Sp, "E14: 'return' used outside of a function",
			"move this statement inside a fn block")
		tc.ok = false
		return
	}
	if r.Value == nil {
		if fn.RetType != nil && fn.RetType.Kind != TyVoid {
			// E15: missing return value
			errAt(r.Sp,
				fmt.Sprintf("E15: function %q must return %s, but got an empty return", fn.Name, fn.RetType),
				fmt.Sprintf("return a %s value", fn.RetType))
			tc.ok = false
		}
		return
	}
	got := tc.inferExpr(r.Value)
	if fn.RetType != nil && fn.RetType.Kind == TyVoid {
		// E16: returning value from void function
		errAt(r.Sp,
			fmt.Sprintf("E16: function %q has return type void but returns a %s value", fn.Name, got),
			"remove the return value, or change the return type")
		tc.ok = false
		return
	}
	if fn.RetType != nil && got != nil && got.Kind != TyUnknown && fn.RetType.Kind != TyUnknown {
		if !coercible(got, fn.RetType) {
			// E17: wrong return type
			errAt(r.Sp,
				fmt.Sprintf("E17: return type mismatch — function %q returns %s, but found %s", fn.Name, fn.RetType, got),
				fmt.Sprintf("cast with %s(...) or change the function return type", fn.RetType))
			tc.ok = false
		}
	}
}

func (tc *TypeChecker) checkIf(s *IfStmt) {
	cond := tc.inferExpr(s.Cond)
	if !isTruthy(cond) && cond.Kind != TyUnknown {
		// E18: non-truthy condition
		errAt(s.Cond.nodeSpan(),
			fmt.Sprintf("E18: if condition has type %s, which cannot be used as a boolean", cond),
			"use a comparison operator (==, !=, <, >) or cast to int/bool")
		tc.ok = false
	}
	tc.checkBlock(s.Then)
	for _, el := range s.Elifs {
		ec := tc.inferExpr(el.Cond)
		if !isTruthy(ec) && ec.Kind != TyUnknown {
			errAt(el.Cond.nodeSpan(),
				fmt.Sprintf("E18: elif condition has type %s, which cannot be used as a boolean", ec),
				"use a comparison operator")
			tc.ok = false
		}
		tc.checkBlock(el.Body)
	}
	if s.Else != nil {
		tc.checkBlock(s.Else)
	}
}

func (tc *TypeChecker) checkWhile(s *WhileStmt) {
	cond := tc.inferExpr(s.Cond)
	if !isTruthy(cond) && cond.Kind != TyUnknown {
		// E19: non-truthy while condition
		errAt(s.Cond.nodeSpan(),
			fmt.Sprintf("E19: while condition has type %s, which cannot be used as a boolean", cond),
			"use a comparison expression that evaluates to bool or int")
		tc.ok = false
	}
	tc.checkBlockInLoop(s.Body)
}

func (tc *TypeChecker) checkForRange(s *ForRangeStmt) {
	fromT := tc.inferExpr(s.From)
	toT := tc.inferExpr(s.To)
	if !isInteger(fromT) && fromT.Kind != TyUnknown {
		// E20: non-integer range start
		errAt(s.From.nodeSpan(),
			fmt.Sprintf("E20: for-range start must be int, got %s", fromT),
			"use an integer expression for the range start")
		tc.ok = false
	}
	if !isInteger(toT) && toT.Kind != TyUnknown {
		// E21: non-integer range end
		errAt(s.To.nodeSpan(),
			fmt.Sprintf("E21: for-range end must be int, got %s", toT),
			"use an integer expression for the range end")
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

func (tc *TypeChecker) checkAssign(s *AssignStmt) {
	lhsType := tc.inferExpr(s.LHS)

	// E22: cannot assign to a const
	if id, ok := s.LHS.(*Ident); ok {
		vi := tc.scope.lookup(id.Name)
		if vi != nil && vi.IsConst {
			errAt(s.Sp,
				fmt.Sprintf("E22: cannot assign to const %q", id.Name),
				fmt.Sprintf("declare with 'let' instead of 'const' if you need mutation"))
			tc.ok = false
			return
		}
	}

	// E23: cannot assign to a function name
	if id, ok := s.LHS.(*Ident); ok {
		vi := tc.scope.lookup(id.Name)
		if vi != nil && vi.IsFn {
			errAt(s.Sp,
				fmt.Sprintf("E23: cannot assign to function %q — it is not a variable", id.Name),
				"use a variable to store the result of calling the function")
			tc.ok = false
			return
		}
	}

	// E24: cannot assign to a literal
	switch s.LHS.(type) {
	case *IntLit, *FloatLit, *BoolLit, *StrLit:
		errAt(s.Sp, "E24: left-hand side of assignment must be a variable or field, not a literal",
			"use a variable name on the left side")
		tc.ok = false
		return
	}

	rhsType := tc.inferExpr(s.Value)
	if lhsType.Kind != TyUnknown && rhsType.Kind != TyUnknown {
		if !coercible(rhsType, lhsType) {
			// E25: assignment type mismatch
			errAt(s.Sp,
				fmt.Sprintf("E25: cannot assign %s to variable of type %s", rhsType, lhsType),
				fmt.Sprintf("cast the right-hand side with %s(...)", lhsType))
			tc.ok = false
		}
	}

	// compound assignment type checks
	if s.Op != "=" && !isNumeric(lhsType) {
		// E26: compound op on non-numeric
		errAt(s.Sp,
			fmt.Sprintf("E26: operator %s requires a numeric type, but variable is %s", s.Op, lhsType),
			"use += -= *= /= only with int or float variables")
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
	case *SizeofExpr:
		e.Typ = TypInt
		return TypInt

	case *Ident:
		vi := tc.scope.lookup(e.Name)
		if vi == nil {
			// E27: undefined variable
			suggestion := tc.suggestName(e.Name)
			hint := "declare it with: let " + e.Name + ": <type> = ..."
			if suggestion != "" {
				hint = fmt.Sprintf("did you mean %q?", suggestion)
			}
			errAt(e.Sp,
				fmt.Sprintf("E27: undefined variable or function %q", e.Name),
				hint)
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
	case *IndexExpr:
		return tc.inferIndex(e)
	case *FieldExpr:
		return tc.inferField(e)
	case *CastExpr:
		from := tc.inferExpr(e.Operand)
		// E28: invalid cast
		if !canCast(from, e.ToType) {
			errAt(e.Sp,
				fmt.Sprintf("E28: cannot cast %s to %s", from, e.ToType),
				"casts are only valid between numeric types (int, float, char, bool)")
			tc.ok = false
		}
		e.Typ = e.ToType
		return e.ToType

	case *AddrExpr:
		inner := tc.inferExpr(e.Operand)
		if e.Deref {
			if inner.Kind != TyPtr {
				// E29: deref non-pointer
				errAt(e.Sp,
					fmt.Sprintf("E29: cannot dereference a non-pointer type %s", inner),
					"only ptr<T> values can be dereferenced with *")
				tc.ok = false
				e.Typ = inner
				return inner
			}
			if inner.PtrElem == nil {
				e.Typ = TypVoid
				return TypVoid
			}
			e.Typ = inner.PtrElem
			return inner.PtrElem
		}
		e.Typ = PtrOf(inner)
		return e.Typ

	case *StructInit:
		return tc.inferStructInit(e)
	case *ArrayLit:
		return tc.inferArrayLit(e)
	default:
		return TypUnknown
	}
}

func (tc *TypeChecker) inferBin(e *BinExpr) *ZXType {
	lhs := tc.inferExpr(e.LHS)
	rhs := tc.inferExpr(e.RHS)

	switch e.Op {
	case "==", "!=":
		if lhs.Kind != TyUnknown && rhs.Kind != TyUnknown {
			if !coercible(lhs, rhs) && !coercible(rhs, lhs) {
				// E30: incompatible comparison types
				errAt(e.Sp,
					fmt.Sprintf("E30: cannot compare %s with %s using %s", lhs, rhs, e.Op),
					"both sides of == or != must have compatible types")
				tc.ok = false
			}
		}
		e.Typ = TypBool
		return TypBool

	case "<", ">", "<=", ">=":
		if lhs.Kind != TyUnknown && rhs.Kind != TyUnknown {
			if !isNumeric(lhs) || !isNumeric(rhs) {
				// E31: comparison on non-numeric
				errAt(e.Sp,
					fmt.Sprintf("E31: operator %s requires numeric operands, got %s and %s", e.Op, lhs, rhs),
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
		// E32: string + string not supported
		if lhs.Kind == TyStr || rhs.Kind == TyStr {
			errAt(e.Sp,
				"E32: '+' cannot be used for string concatenation in ZX",
				"use printf/sprintf for string formatting, or import string.h for strcat")
			tc.ok = false
			e.Typ = TypStr
			return TypStr
		}
		fallthrough
	case "-", "*", "/", "%":
		if lhs.Kind != TyUnknown && rhs.Kind != TyUnknown {
			if !isNumeric(lhs) {
				// E33: arithmetic on non-numeric left
				errAt(e.LHS.nodeSpan(),
					fmt.Sprintf("E33: operator '%s' cannot be applied to type %s", e.Op, lhs),
					"arithmetic operators require int or float operands")
				tc.ok = false
			}
			if !isNumeric(rhs) {
				// E33: arithmetic on non-numeric right
				errAt(e.RHS.nodeSpan(),
					fmt.Sprintf("E33: operator '%s' cannot be applied to type %s", e.Op, rhs),
					"arithmetic operators require int or float operands")
				tc.ok = false
			}
			// E34: integer division by zero literal
			if e.Op == "/" {
				if lit, ok := e.RHS.(*IntLit); ok && lit.Val == 0 {
					errAt(e.RHS.nodeSpan(),
						"E34: division by zero literal",
						"the divisor is a literal 0 — this will crash at runtime")
					tc.ok = false
				}
				if lit, ok := e.RHS.(*FloatLit); ok && lit.Val == 0.0 {
					warnAt(e.RHS.nodeSpan(),
						"E34: division by zero literal (float)",
						"dividing by 0.0 will produce Inf or NaN")
				}
			}
			// E35: modulo on float
			if e.Op == "%" && (lhs.Kind == TyFloat || rhs.Kind == TyFloat) {
				errAt(e.Sp,
					"E35: modulo '%' cannot be used with float operands",
					"use fmod() from math.h for floating-point modulo")
				tc.ok = false
			}
		}
		if lhs.Kind == TyFloat || rhs.Kind == TyFloat {
			e.Typ = TypFloat
			return TypFloat
		}
		e.Typ = lhs
		return lhs

	case "|", "&", "^", "<<", ">>":
		if lhs.Kind != TyUnknown && !isInteger(lhs) {
			// E36: bitwise on non-integer
			errAt(e.Sp,
				fmt.Sprintf("E36: bitwise operator '%s' requires integer operands, got %s", e.Op, lhs),
				"bitwise ops only work on int and char")
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
		// E37: logical not on non-boolean
		if inner.Kind != TyUnknown && !isTruthy(inner) {
			errAt(e.Sp,
				fmt.Sprintf("E37: '!' operator cannot be applied to type %s", inner),
				"use ! only with bool or int expressions")
			tc.ok = false
		}
		e.Typ = TypBool
		return TypBool
	case "-":
		if inner.Kind != TyUnknown && !isNumeric(inner) {
			// E38: negate non-numeric
			errAt(e.Sp,
				fmt.Sprintf("E38: unary '-' cannot be applied to type %s", inner),
				"unary minus requires a numeric operand (int or float)")
			tc.ok = false
		}
		e.Typ = inner
		return inner
	case "~":
		if inner.Kind != TyUnknown && !isInteger(inner) {
			// E39: bitwise not on non-integer
			errAt(e.Sp,
				fmt.Sprintf("E39: bitwise '~' cannot be applied to type %s", inner),
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
	// resolve callee name
	var fnName string
	if id, ok := e.Func.(*Ident); ok {
		fnName = id.Name
	}

	// check externs
	if ext, ok := tc.externs[fnName]; ok {
		for i, a := range e.Args {
			got := tc.inferExpr(a)
			if !ext.Variadic && i < len(ext.Params) {
				expected := ext.Params[i].Type
				if got.Kind != TyUnknown && !coercible(got, expected) {
					// E40: extern arg type mismatch
					errAt(a.nodeSpan(),
						fmt.Sprintf("E40: extern %q argument %d: expected %s, got %s", fnName, i+1, expected, got),
						fmt.Sprintf("cast to %s", expected))
					tc.ok = false
				}
			}
		}
		if !ext.Variadic && len(e.Args) != len(ext.Params) {
			// E41: wrong arg count extern
			errAt(e.Sp,
				fmt.Sprintf("E41: extern %q expects %d argument(s), got %d", fnName, len(ext.Params), len(e.Args)),
				"check the extern declaration and adjust the call")
			tc.ok = false
		}
		if id, ok := e.Func.(*Ident); ok {
			id.Typ = ext.RetType
		}
		e.Typ = ext.RetType
		return ext.RetType
	}

	// check user functions
	if fn, ok := tc.fns[fnName]; ok {
		if !fn.Variadic && len(e.Args) != len(fn.Params) {
			// E42: wrong arg count user fn
			errAt(e.Sp,
				fmt.Sprintf("E42: function %q expects %d argument(s), got %d", fnName, len(fn.Params), len(e.Args)),
				"check the function signature and adjust the call")
			tc.ok = false
		}
		for i, a := range e.Args {
			got := tc.inferExpr(a)
			if i < len(fn.Params) {
				expected := fn.Params[i].Type
				if got.Kind != TyUnknown && expected.Kind != TyUnknown && !coercible(got, expected) {
					// E43: user fn arg type mismatch
					errAt(a.nodeSpan(),
						fmt.Sprintf("E43: function %q argument %d: expected %s, got %s", fnName, i+1, expected, got),
						fmt.Sprintf("cast the argument with %s(...)", expected))
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

	// E44: calling undefined function / value
	if fnName != "" {
		vi := tc.scope.lookup(fnName)
		if vi == nil {
			suggestion := tc.suggestName(fnName)
			hint := fmt.Sprintf("declare it with: extern fn %s(...) -> int;  or  fn %s(...) { }", fnName, fnName)
			if suggestion != "" {
				hint = fmt.Sprintf("did you mean %q?", suggestion)
			}
			errAt(e.Sp,
				fmt.Sprintf("E44: call to undefined function %q", fnName),
				hint)
			tc.ok = false
			e.Typ = TypUnknown
			return TypUnknown
		}
		if !vi.IsFn {
			// E45: calling a non-function
			errAt(e.Sp,
				fmt.Sprintf("E45: %q is a variable of type %s, not a function — cannot call it", fnName, vi.Type),
				"check that you're calling the right name")
			tc.ok = false
			e.Typ = TypUnknown
			return TypUnknown
		}
	}

	// unknown C function from import — infer args, return int
	for _, a := range e.Args {
		tc.inferExpr(a)
	}
	e.Typ = TypInt
	return TypInt
}

func (tc *TypeChecker) inferIndex(e *IndexExpr) *ZXType {
	objType := tc.inferExpr(e.Obj)
	idxType := tc.inferExpr(e.Idx)

	// E46: index non-array/pointer
	if objType.Kind != TyArray && objType.Kind != TyPtr && objType.Kind != TyStr && objType.Kind != TyUnknown {
		errAt(e.Sp,
			fmt.Sprintf("E46: cannot index into type %s", objType),
			"indexing is only valid on arrays, pointers, and str")
		tc.ok = false
		e.Typ = TypUnknown
		return TypUnknown
	}

	// E47: non-integer index
	if !isInteger(idxType) && idxType.Kind != TyUnknown {
		errAt(e.Idx.nodeSpan(),
			fmt.Sprintf("E47: array index must be int, got %s", idxType),
			"use an integer expression as the array index")
		tc.ok = false
	}

	if objType.Kind == TyArray && objType.ArrElem != nil {
		e.Typ = objType.ArrElem
		return objType.ArrElem
	}
	if objType.Kind == TyPtr && objType.PtrElem != nil {
		e.Typ = objType.PtrElem
		return objType.PtrElem
	}
	if objType.Kind == TyStr {
		e.Typ = TypChar
		return TypChar
	}
	e.Typ = TypUnknown
	return TypUnknown
}

func (tc *TypeChecker) inferField(e *FieldExpr) *ZXType {
	objType := tc.inferExpr(e.Obj)

	// unwrap pointer-to-struct
	effectiveType := objType
	if objType.Kind == TyPtr && objType.PtrElem != nil {
		effectiveType = objType.PtrElem
	}

	if effectiveType.Kind != TyStruct {
		if effectiveType.Kind != TyUnknown {
			// E48: field access on non-struct
			errAt(e.Sp,
				fmt.Sprintf("E48: cannot access field %q on type %s — it is not a struct", e.Field, objType),
				"field access with '.' is only valid on struct types")
			tc.ok = false
		}
		e.Typ = TypUnknown
		return TypUnknown
	}

	sd, ok := tc.structs[effectiveType.Name]
	if !ok {
		// E49: unknown struct type
		errAt(e.Sp,
			fmt.Sprintf("E49: struct type %q is not defined", effectiveType.Name),
			fmt.Sprintf("declare it with: struct %s { ... }", effectiveType.Name))
		tc.ok = false
		e.Typ = TypUnknown
		return TypUnknown
	}

	for _, f := range sd.Fields {
		if f.Name == e.Field {
			e.Typ = f.Type
			return f.Type
		}
	}

	// E50: unknown struct field
	errAt(e.Sp,
		fmt.Sprintf("E50: struct %q has no field %q", effectiveType.Name, e.Field),
		fmt.Sprintf("valid fields are: %s", listFields(sd.Fields)))
	tc.ok = false
	e.Typ = TypUnknown
	return TypUnknown
}

func (tc *TypeChecker) inferStructInit(e *StructInit) *ZXType {
	sd, ok := tc.structs[e.Name]
	if !ok {
		// E51: unknown struct in initializer
		errAt(e.Sp,
			fmt.Sprintf("E51: undefined struct %q", e.Name),
			fmt.Sprintf("declare it with: struct %s { ... }", e.Name))
		tc.ok = false
		e.Typ = TypUnknown
		return TypUnknown
	}

	// check all provided field names are valid
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
			// E52: unknown field in struct init
			errAt(fi.Sp,
				fmt.Sprintf("E52: struct %q has no field %q", e.Name, fi.Name),
				fmt.Sprintf("valid fields are: %s", listFields(sd.Fields)))
			tc.ok = false
		}
		if provided[fi.Name] {
			// E53: duplicate field in struct init
			errAt(fi.Sp,
				fmt.Sprintf("E53: field %q is set more than once in struct initializer", fi.Name),
				"remove the duplicate field assignment")
			tc.ok = false
		}
		provided[fi.Name] = true

		got := tc.inferExpr(fi.Value)
		// type check the field value
		for _, sf := range sd.Fields {
			if sf.Name == fi.Name && got.Kind != TyUnknown && !coercible(got, sf.Type) {
				// E54: field type mismatch
				errAt(fi.Sp,
					fmt.Sprintf("E54: field %q expects %s, got %s", fi.Name, sf.Type, got),
					fmt.Sprintf("cast with %s(...)", sf.Type))
				tc.ok = false
			}
		}
	}

	e.Typ = StructType(e.Name)
	return e.Typ
}

func (tc *TypeChecker) inferArrayLit(e *ArrayLit) *ZXType {
	if len(e.Elems) == 0 {
		e.Typ = ArrayOf(TypUnknown, 0)
		return e.Typ
	}
	first := tc.inferExpr(e.Elems[0])
	for i, el := range e.Elems[1:] {
		got := tc.inferExpr(el)
		if got.Kind != TyUnknown && !typeEq(got, first) && !coercible(got, first) {
			// E55: inconsistent array literal types
			errAt(el.nodeSpan(),
				fmt.Sprintf("E55: array element %d has type %s, expected %s (from first element)", i+2, got, first),
				fmt.Sprintf("cast element to %s to make types consistent", first))
			tc.ok = false
		}
	}
	e.Typ = ArrayOf(first, len(e.Elems))
	return e.Typ
}

// ── helpers ───────────────────────────────────────────────────────────────────

// checkTypeExists verifies struct type names exist
func (tc *TypeChecker) checkTypeExists(t *ZXType, sp Span) {
	if t == nil {
		return
	}
	switch t.Kind {
	case TyStruct:
		if _, ok := tc.structs[t.Name]; !ok {
			errAt(sp,
				fmt.Sprintf("E56: unknown type %q — no struct with this name exists", t.Name),
				fmt.Sprintf("declare it with: struct %s { ... }", t.Name))
			tc.ok = false
		}
	case TyPtr:
		tc.checkTypeExists(t.PtrElem, sp)
	case TyArray:
		tc.checkTypeExists(t.ArrElem, sp)
	}
}

func canCast(from, to *ZXType) bool {
	if from == nil || to == nil {
		return true
	}
	if from.Kind == TyUnknown || to.Kind == TyUnknown {
		return true
	}
	// numeric <-> numeric always ok
	if isNumeric(from) && isNumeric(to) {
		return true
	}
	if from.Kind == TyBool && isNumeric(to) {
		return true
	}
	if isNumeric(from) && to.Kind == TyBool {
		return true
	}
	// pointer casts
	if from.Kind == TyPtr && to.Kind == TyPtr {
		return true
	}
	if from.Kind == TyPtr && isInteger(to) {
		return true
	}
	if isInteger(from) && to.Kind == TyPtr {
		return true
	}
	return false
}

// suggestName does simple edit-distance suggestion
func (tc *TypeChecker) suggestName(name string) string {
	best := ""
	bestDist := 3 // only suggest if distance <= 2
	// collect all known names
	var candidates []string
	for n := range tc.fns {
		candidates = append(candidates, n)
	}
	for n := range tc.externs {
		candidates = append(candidates, n)
	}
	s := tc.scope
	for s != nil {
		for n := range s.vars {
			candidates = append(candidates, n)
		}
		s = s.parent
	}
	for _, c := range candidates {
		d := editDistance(name, c)
		if d < bestDist {
			bestDist = d
			best = c
		}
	}
	return best
}

func editDistance(a, b string) int {
	la, lb := len(a), len(b)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}
	prev := make([]int, lb+1)
	curr := make([]int, lb+1)
	for j := 0; j <= lb; j++ {
		prev[j] = j
	}
	for i := 1; i <= la; i++ {
		curr[0] = i
		for j := 1; j <= lb; j++ {
			if a[i-1] == b[j-1] {
				curr[j] = prev[j-1]
			} else {
				curr[j] = 1 + minOf3(prev[j], curr[j-1], prev[j-1])
			}
		}
		prev, curr = curr, prev
	}
	return prev[lb]
}

func minOf3(a, b, c int) int {
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
		parts[i] = p.Name + ": " + p.Type.String()
	}
	return strings.Join(parts, ", ")
}
