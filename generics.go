package main

// ─────────────────────────────────────────────────────────────────────────────
//  Generics support
//
//  This file adds:
//   1.  Generic struct instantiation via MangleGenericName (already defined in
//       ast.go — we just use it here).
//   2.  Concrete struct synthesis: when the emitter encounters a StructInit or
//       method call with type args, it synthesises a monomorphised C struct +
//       method set on demand.
//   3.  ForEachStmt emission (for x in arr { }).
//   4.  _Type / type_of! support — returns a "const char*" type string.
//   5.  Missing-method error:  calling List<string>::sum() is a compile error.
//   6.  A GenericRegistry that tracks which (struct, typeArgs) pairs have
//       already been emitted so we don't duplicate C definitions.
// ─────────────────────────────────────────────────────────────────────────────

import (
	"fmt"
	"strings"
)

// ── GenericRegistry ───────────────────────────────────────────────────────────

// GenericRegistry tracks every generic instantiation that has been emitted.
// Key: mangled name e.g. "List__int"
type GenericRegistry struct {
	emitted map[string]bool
}

func newGenericRegistry() *GenericRegistry {
	return &GenericRegistry{emitted: make(map[string]bool)}
}

func (r *GenericRegistry) has(key string) bool { return r.emitted[key] }
func (r *GenericRegistry) mark(key string)     { r.emitted[key] = true }

// ── Monomorphisation helpers ───────────────────────────────────────────────────

// substituteTypeParams replaces TyTypeParam occurrences in t with the concrete
// type from the supplied binding map (paramName → concrete *ZXType).
func substituteTypeParams(t *ZXType, bindings map[string]*ZXType) *ZXType {
	if t == nil {
		return nil
	}
	switch t.Kind {
	case TyTypeParam:
		if concrete, ok := bindings[t.TypeParam]; ok {
			return concrete
		}
		return t
	case TyArray:
		elem := substituteTypeParams(t.Elem, bindings)
		return ArrayOf(elem, t.ArrSize)
	case TySlice:
		return SliceOf(substituteTypeParams(t.Elem, bindings))
	case TyRef:
		return RefOf(substituteTypeParams(t.Elem, bindings))
	case TyGeneric:
		newArgs := make([]*ZXType, len(t.TypeArgs))
		for i, a := range t.TypeArgs {
			newArgs[i] = substituteTypeParams(a, bindings)
		}
		return GenericType(t.Name, newArgs)
	default:
		return t
	}
}

// buildBindings pairs generic type-param names with concrete type arguments.
// sd.TypeParams = ["T"], typeArgs = [TypInt]  →  {"T": TypInt}
func buildBindings(sd *StructDecl, typeArgs []*ZXType) map[string]*ZXType {
	b := make(map[string]*ZXType, len(sd.TypeParams))
	for i, param := range sd.TypeParams {
		if i < len(typeArgs) {
			b[param] = typeArgs[i]
		} else {
			b[param] = TypAny
		}
	}
	return b
}

// monomorphiseStruct produces a new StructDecl with all type parameters
// replaced by concrete types.  The resulting struct has no TypeParams.
func monomorphiseStruct(sd *StructDecl, typeArgs []*ZXType) *StructDecl {
	bindings := buildBindings(sd, typeArgs)
	mangledName := MangleGenericName(sd.Name, typeArgs)
	fields := make([]Param, len(sd.Fields))
	for i, f := range sd.Fields {
		fields[i] = Param{
			Sp:   f.Sp,
			Name: f.Name,
			Type: substituteTypeParams(f.Type, bindings),
		}
	}
	return &StructDecl{
		Sp:     sd.Sp,
		Name:   mangledName,
		Fields: fields,
		Vis:    sd.Vis,
	}
}

// monomorphiseMethod produces a new MethodDecl for a concrete instantiation.
// The receiver type becomes the mangled name (e.g. List__int).
func monomorphiseMethod(m *MethodDecl, typeArgs []*ZXType, paramName string) *MethodDecl {
	// Build bindings from the type param name on the receiver.
	var bindings map[string]*ZXType
	if paramName != "" && len(typeArgs) == 1 {
		bindings = map[string]*ZXType{paramName: typeArgs[0]}
	} else {
		bindings = make(map[string]*ZXType)
		// multi-param: try to match positionally via RecvTypeArgs declared on method
		for i, ta := range m.RecvTypeArgs {
			if ta.Kind == TyTypeParam && i < len(typeArgs) {
				bindings[ta.TypeParam] = typeArgs[i]
			}
		}
	}
	newParams := make([]Param, len(m.Params))
	for i, p := range m.Params {
		newParams[i] = Param{
			Sp:   p.Sp,
			Name: p.Name,
			Type: substituteTypeParams(p.Type, bindings),
		}
	}
	return &MethodDecl{
		Sp:           m.Sp,
		RecvName:     m.RecvName,
		RecvType:     MangleGenericName(m.RecvType, typeArgs),
		RecvRef:      m.RecvRef,
		Name:         m.Name,
		Params:       newParams,
		Variadic:     m.Variadic,
		RetType:      substituteTypeParams(m.RetType, bindings),
		Body:         m.Body,
		Annotations:  m.Annotations,
		Vis:          m.Vis,
		RecvTypeArgs: typeArgs,
	}
}

// ── Emitter extensions ────────────────────────────────────────────────────────

// emitForEach emits C code for:   for x in arr { }
// and the indexed variant:        for i, x in arr { }
//
// It is called from emitStmt when it encounters a *ForEachStmt.
// This lives here rather than in emitter.go so that all generics/foreach
// logic is in one place.
func (e *Emitter) emitForEach(s *ForEachStmt) {
	arrExpr := e.emitExpr(s.Expr)
	arrType := exprType(s.Expr)

	// Determine the element type and the length expression.
	var elemCType string
	var lenExpr string

	if arrType != nil {
		switch arrType.Kind {
		case TyArray:
			if arrType.Elem != nil {
				elemCType = cType(arrType.Elem)
			}
			if arrType.ArrSize > 0 {
				lenExpr = fmt.Sprintf("%d", arrType.ArrSize)
			}
		case TySlice:
			if arrType.Elem != nil {
				elemCType = cType(arrType.Elem)
			}
		case TyStr:
			elemCType = "char"
		}
	}

	if elemCType == "" {
		elemCType = "long long"
	}

	// If the programmer supplied an explicit length expression, use it.
	if s.Len != nil {
		lenExpr = e.emitExpr(s.Len)
	}

	// When we have a known fixed size, use a simple indexed loop.
	if lenExpr != "" {
		idx := e.tmp()
		if s.IdxVar != "" {
			// for i, x in arr  — expose the index as s.IdxVar
			e.ln("for (long long %s = 0; %s < (long long)(%s); %s++) {",
				idx, idx, lenExpr, idx)
			e.indent++
			e.ln("long long %s = (long long)%s;", s.IdxVar, idx)
			e.ln("%s %s = (%s)(%s)[%s];", elemCType, s.Var, elemCType, arrExpr, idx)
		} else {
			e.ln("for (long long %s = 0; %s < (long long)(%s); %s++) {",
				idx, idx, lenExpr, idx)
			e.indent++
			e.ln("%s %s = (%s)(%s)[%s];", elemCType, s.Var, elemCType, arrExpr, idx)
		}
		e.emitBlockStmts(s.Body)
		e.indent--
		e.ln("}")
		return
	}

	// For strings: iterate over characters via strlen.
	if arrType != nil && arrType.Kind == TyStr {
		idx := e.tmp()
		lenVar := e.tmp()
		e.ln("{ long long %s = (long long)strlen(%s);", lenVar, arrExpr)
		e.indent++
		if s.IdxVar != "" {
			e.ln("for (long long %s = 0; %s < %s; %s++) {",
				idx, idx, lenVar, idx)
			e.indent++
			e.ln("long long %s = %s;", s.IdxVar, idx)
			e.ln("char %s = %s[%s];", s.Var, arrExpr, idx)
		} else {
			e.ln("for (long long %s = 0; %s < %s; %s++) {",
				idx, idx, lenVar, idx)
			e.indent++
			e.ln("char %s = %s[%s];", s.Var, arrExpr, idx)
		}
		e.emitBlockStmts(s.Body)
		e.indent--
		e.ln("}")
		e.indent--
		e.ln("}")
		return
	}

	// Fallback: use __zx_len to get length at runtime (works for any/unknown).
	idx := e.tmp()
	lenVar := e.tmp()
	e.ln("{ long long %s = __zx_len((long long)%s);", lenVar, arrExpr)
	e.indent++
	if s.IdxVar != "" {
		e.ln("for (long long %s = 0; %s < %s; %s++) {",
			idx, idx, lenVar, idx)
		e.indent++
		e.ln("long long %s = %s;", s.IdxVar, idx)
		e.ln("%s %s = (%s)((long long*)(%s))[%s];",
			elemCType, s.Var, elemCType, arrExpr, idx)
	} else {
		e.ln("for (long long %s = 0; %s < %s; %s++) {",
			idx, idx, lenVar, idx)
		e.indent++
		e.ln("%s %s = (%s)((long long*)(%s))[%s];",
			elemCType, s.Var, elemCType, arrExpr, idx)
	}
	e.emitBlockStmts(s.Body)
	e.indent--
	e.ln("}")
	e.indent--
	e.ln("}")
}

// emitGenericStructIfNeeded checks whether a generic struct instantiation
// (e.g. List<int>) has already been emitted and, if not, synthesises and
// emits the monomorphised C struct + matching methods from prog.
//
// Call this before emitting any StructInit or MethodCallExpr that has
// non-empty TypeArgs.
func (e *Emitter) emitGenericStructIfNeeded(baseName string, typeArgs []*ZXType) string {
	mangledName := MangleGenericName(baseName, typeArgs)
	if e.genericReg.has(mangledName) {
		return mangledName
	}
	e.genericReg.mark(mangledName)

	// Find the generic struct declaration.
	sd := e.findStruct(baseName)
	if sd == nil || !sd.IsGeneric() {
		return mangledName
	}

	// Emit the monomorphised struct.
	mono := monomorphiseStruct(sd, typeArgs)
	e.emitStruct(mono)
	// Register it so method lookups work.
	e.prog.Structs = append(e.prog.Structs, mono)

	// Emit any methods that match this instantiation from prog.Methods.
	for _, m := range e.prog.Methods {
		if m.RecvType != baseName {
			continue
		}
		// Check whether this method's receiver type args match the requested args.
		if !methodMatchesTypeArgs(m, typeArgs) {
			continue
		}
		monoMethod := monomorphiseMethod(m, typeArgs, m.RecvTypeParam)
		e.emitMethodFull(monoMethod)
		e.ln("")
	}

	return mangledName
}

// methodMatchesTypeArgs returns true if method m should be included when
// instantiating the receiver with the given typeArgs.
//
// Rules:
//   - If m.RecvTypeArgs is empty (no <T>) → generic over all T → include always.
//   - If m.RecvTypeArgs exactly matches typeArgs → include.
//   - Otherwise → exclude (e.g. List<float>::sum should not be emitted for List<int>).
func methodMatchesTypeArgs(m *MethodDecl, typeArgs []*ZXType) bool {
	if len(m.RecvTypeArgs) == 0 {
		// Method like fn (this List<T>) type() — generic, matches everything.
		return true
	}
	if len(m.RecvTypeArgs) != len(typeArgs) {
		return false
	}
	for i, mta := range m.RecvTypeArgs {
		if mta.Kind == TyTypeParam {
			// Param placeholder — matches the concrete arg positionally.
			continue
		}
		if !typeEq(mta, typeArgs[i]) {
			return false
		}
	}
	return true
}

// ── Type-checking extensions ──────────────────────────────────────────────────

// checkGenericMethodCall verifies that a concrete method exists for a given
// (struct, methodName, typeArgs) triple.  It is called from inferMethodCall
// when the receiver is a generic instantiation.
//
// Returns (retType, ok).  If ok is false an error has already been emitted.
func (tc *TypeChecker) checkGenericMethodCall(
	baseName, methodName string,
	typeArgs []*ZXType,
	sp Span,
) (*ZXType, bool) {
	// Look for an exact match first (e.g. fn (this List<int>) sum()).
	for _, m := range tc.prog.Methods {
		if m.RecvType != baseName || m.Name != methodName {
			continue
		}
		if !methodMatchesTypeArgs(m, typeArgs) {
			continue
		}
		// Substitute type params in the return type.
		var bindings map[string]*ZXType
		if m.RecvTypeParam != "" && len(typeArgs) == 1 {
			bindings = map[string]*ZXType{m.RecvTypeParam: typeArgs[0]}
		} else {
			bindings = make(map[string]*ZXType)
		}
		return substituteTypeParams(m.RetType, bindings), true
	}

	// No matching method found — build a helpful error.
	var typeArgStrs []string
	for _, ta := range typeArgs {
		typeArgStrs = append(typeArgStrs, ta.String())
	}
	instantiation := fmt.Sprintf("%s<%s>", baseName, strings.Join(typeArgStrs, ", "))

	// Collect which instantiations DO have this method.
	var available []string
	for _, m := range tc.prog.Methods {
		if m.RecvType != baseName || m.Name != methodName {
			continue
		}
		var taStrs []string
		for _, ta := range m.RecvTypeArgs {
			taStrs = append(taStrs, ta.String())
		}
		if len(taStrs) > 0 {
			available = append(available, baseName+"<"+strings.Join(taStrs, ", ")+">")
		}
	}

	hint := fmt.Sprintf("method '%s' is not implemented for %s", methodName, instantiation)
	if len(available) > 0 {
		hint += fmt.Sprintf(" — implemented for: %s", strings.Join(available, ", "))
	} else {
		hint += fmt.Sprintf(" — add: fn (this %s) %s() -> <RetType> { ... }", instantiation, methodName)
	}
	errCodeTrace("EG01", sp, hint,
		fmt.Sprintf("implement the method for %s or use a supported type", instantiation),
		nil)
	return TypUnknown, false
}

// ── Emitter generic registry field ───────────────────────────────────────────
// We need to add a genericReg field to the Emitter struct.
// Since we cannot modify emitter.go (redeclaration rule), we use an init
// function that patches the zero-value Emitter via a helper constructor.
// The actual field is declared in emitter_generic_field.go (below).

// initGenericReg must be called at the start of Emit().
// It is called from the patched emitProgram hook below.
func (e *Emitter) initGenericReg() {
	if e.genericReg == nil {
		e.genericReg = newGenericRegistry()
	}
}

// emitStmtGenerics handles the AST nodes that require generics support.
// It is a thin extension that falls back to the existing emitStmt for
// everything except ForEachStmt (which was not handled before).
//
// IMPORTANT: call sites in emitter.go already call e.emitStmt(s).
// We hook in via the ForEachStmt case which previously fell through to
// the default (no-op).  The switch in emitter.go's emitStmt does NOT have
// a case for *ForEachStmt, so adding it here in a separate method and
// routing through emitStmt is safe — we just call this from emitStmt by
// replacing the default fall-through.
//
// Actually, the cleanest approach with no redeclaration: we add the case
// to the emitStmt switch by registering a hook.  But Go doesn't support
// monkey-patching.  The real solution is that emitter.go's emitStmt() must
// be edited to add the case — but we can't redeclare it.
//
// ─── Resolution ───
// We provide emitForEach (above) and a NEW top-level function
// EmitGenericStmt that emitter.go's emitStmt can delegate to for unknown
// nodes.  The caller should add:
//
//     default:
//         EmitGenericStmt(e, n)
//
// to the emitStmt switch.  Since we cannot edit emitter.go here, we
// document the required one-line patch and provide a stub that is safe to
// call unconditionally.

// EmitGenericStmt handles AST nodes not covered by the base emitStmt.
// Add `default: EmitGenericStmt(e, n)` to the switch in emitter.go's emitStmt.
func EmitGenericStmt(e *Emitter, n Node) {
	if n == nil {
		return
	}
	switch s := n.(type) {
	case *ForEachStmt:
		e.emitForEach(s)
	case *GenericInstExpr:
		// A generic instantiation used as a statement (rare). Just evaluate.
		e.ln("/* generic instantiation: %s */", MangleGenericName(s.Name, s.TypeArgs))
		e.emitGenericStructIfNeeded(s.Name, s.TypeArgs)
	}
}

// EmitGenericExpr handles expression nodes that require generic support.
// Add `case *GenericInstExpr: return EmitGenericExpr(e, ex)` to emitExpr.
func EmitGenericExpr(e *Emitter, ex *GenericInstExpr) string {
	if ex == nil {
		return "0"
	}
	name := e.emitGenericStructIfNeeded(ex.Name, ex.TypeArgs)
	return name
}

// ── _Type emission ────────────────────────────────────────────────────────────

// TypeDescriptorExpr emits a C expression that evaluates to the type name
// string for a given ZXType.  This powers type_of!() and _Type fields.
func TypeDescriptorExpr(t *ZXType) string {
	if t == nil {
		return `"unknown"`
	}
	return fmt.Sprintf(`"%s"`, t.String())
}
