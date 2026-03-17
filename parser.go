package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
)

type Parser struct {
	tokens  []Token
	pos     int
	file    string
	ok      bool
	srcDir  string // directory of the source file, for local imports
}

func Parse(tokens []Token, src, file string) *Program {
	srcDir := filepath.Dir(file)
	if srcDir == "" {
		srcDir = "."
	}
	p := &Parser{tokens: tokens, file: file, ok: true, srcDir: srcDir}
	prog := p.parseProgram()
	if !p.ok {
		return nil
	}
	return prog
}

// ── token helpers ─────────────────────────────────────────────────────────────

func (p *Parser) peek() Token {
	if p.pos >= len(p.tokens) {
		return Token{Kind: TK_EOF}
	}
	return p.tokens[p.pos]
}
func (p *Parser) peekN(n int) Token {
	if p.pos+n >= len(p.tokens) {
		return Token{Kind: TK_EOF}
	}
	return p.tokens[p.pos+n]
}
func (p *Parser) at(k TK) bool { return p.peek().Kind == k }
func (p *Parser) atAny(ks ...TK) bool {
	k := p.peek().Kind
	for _, x := range ks {
		if k == x {
			return true
		}
	}
	return false
}
func (p *Parser) advance() Token {
	t := p.peek()
	if t.Kind != TK_EOF {
		p.pos++
	}
	return t
}
func (p *Parser) expect(k TK) Token {
	t := p.peek()
	if t.Kind != k {
		got := t.Value
		if got == "" {
			got = t.Kind.String()
		}
		errAt(t.Span, fmt.Sprintf("expected '%s', got '%s'", k, got),
			fmt.Sprintf("add %s here", k))
		p.ok = false
	}
	return p.advance()
}
func (p *Parser) expectSemi() {
	if p.at(TK_SEMI) {
		p.advance()
		return
	}
	if p.at(TK_RBRACE) || p.at(TK_EOF) {
		return
	}
	t := p.peek()
	errAt(t.Span, "missing ';' after statement",
		"add a semicolon ';' at the end of this statement")
	p.ok = false
}
func (p *Parser) eatSemi() {
	if p.at(TK_SEMI) {
		p.advance()
	}
}

func (p *Parser) isTypeStart() bool {
	switch p.peek().Kind {
	case TK_TYPE_INT, TK_TYPE_FLOAT, TK_TYPE_BOOL, TK_TYPE_STR,
		TK_TYPE_VOID, TK_TYPE_CHAR, TK_TYPE_REF, TK_TYPE_ANY,
		TK_LBRACKET, TK_STAR:
		return true
	}
	return false
}

// ── Annotation helpers ────────────────────────────────────────────────────────

func (p *Parser) parseAnnotations() []Annotation {
	var anns []Annotation
	for p.at(TK_ANNOTATION) {
		t := p.advance()
		anns = append(anns, parseAnnotationValue(t))
	}
	return anns
}

func parseAnnotationValue(t Token) Annotation {
	raw := t.Value
	ann := Annotation{Sp: t.Span}
	eqIdx := strings.Index(raw, "=")
	if eqIdx < 0 {
		ann.Name = raw
		return ann
	}
	ann.Name = raw[:eqIdx]
	rest := raw[eqIdx+1:]
	if strings.HasPrefix(rest, "{") {
		var m map[string]interface{}
		if err := json.Unmarshal([]byte(rest), &m); err == nil {
			ann.Args = make(map[string]string)
			for k, v := range m {
				ann.Args[k] = fmt.Sprintf("%v", v)
			}
		} else {
			warnAt(t.Span, fmt.Sprintf("invalid JSON in @%s: %v", ann.Name, err),
				`use valid JSON: @args={"key": value}`)
			ann.Args = map[string]string{"_raw": rest}
		}
	} else {
		ann.Args = map[string]string{"value": rest}
	}
	return ann
}

// buildTestDecl converts a @test fn into a TestDecl.
func buildTestDecl(fn *FnDecl, modPath string) *TestDecl {
	td := &TestDecl{Fn: fn, ModPath: modPath}
	for _, ann := range fn.Annotations {
		switch ann.Name {
		case AnnIgnore, AnnSkip:
			td.Ignored = true
		case AnnArgs:
			td.Args = ann.Args
		case AnnExpect:
			if ann.Args != nil {
				td.Expected = ann.Args["value"]
			}
		case AnnTimeout:
			if ann.Args != nil {
				if ms, err := strconv.Atoi(ann.Args["value"]); err == nil {
					td.Timeout = ms
				}
			}
		}
	}
	return td
}

// ── Program ───────────────────────────────────────────────────────────────────

func (p *Parser) parseProgram() *Program {
	prog := &Program{}
	// optional: mod name;
	if p.at(TK_MOD) && p.peekN(1).Kind == TK_IDENT && p.peekN(2).Kind != TK_LBRACE {
		p.advance()
		prog.Module = p.expect(TK_IDENT).Value
		p.eatSemi()
	}
	for !p.at(TK_EOF) && p.ok {
		anns := p.parseAnnotations()
		switch p.peek().Kind {
		case TK_IMPORT, TK_USE:
			if len(anns) > 0 {
				warnAt(anns[0].Sp, "annotations on imports are ignored", "")
			}
			prog.Imports = append(prog.Imports, p.parseImport())
		case TK_MOD:
			mb := p.parseModBlock("")
			prog.ModBlocks = append(prog.ModBlocks, mb)
		case TK_EXTERN:
			prog.Externs = append(prog.Externs, p.parseExtern())
		case TK_STRUCT:
			sd := p.parseStruct()
			sd.Annotations = anns
			prog.Structs = append(prog.Structs, sd)
		case TK_TYPE:
			sd := p.parseTypeStruct()
			sd.Annotations = anns
			prog.Structs = append(prog.Structs, sd)
		case TK_MACRO:
			mc := p.parseMacroDecl()
			prog.Macros = append(prog.Macros, mc)
		case TK_FN, TK_SUB:
			if p.isFnMethod() {
				md := p.parseMethod()
				md.Annotations = anns
				prog.Methods = append(prog.Methods, md)
			} else {
				fn := p.parseFnDecl(anns)
				prog.TopStmts = append(prog.TopStmts, fn)
			}
		default:
			if len(anns) > 0 {
				warnAt(anns[0].Sp,
					fmt.Sprintf("@%s cannot be applied here — annotations go before fn or struct", anns[0].Name), "")
			}
			if s := p.parseStmt(); s != nil {
				prog.TopStmts = append(prog.TopStmts, s)
			}
		}
	}
	return prog
}

func (p *Parser) isFnMethod() bool {
	return p.pos+1 < len(p.tokens) && p.tokens[p.pos+1].Kind == TK_LPAREN
}

// ── mod name { ... } ──────────────────────────────────────────────────────────

func (p *Parser) parseModBlock(parentPath string) *ModBlock {
	sp := p.peek().Span
	p.expect(TK_MOD)
	name := p.expect(TK_IDENT).Value
	p.expect(TK_LBRACE)
	path := name
	if parentPath != "" {
		path = parentPath + "::" + name
	}
	mb := &ModBlock{Sp: sp, Name: name, Path: path}
	for !p.at(TK_RBRACE) && !p.at(TK_EOF) && p.ok {
		anns := p.parseAnnotations()
		switch p.peek().Kind {
		case TK_MOD:
			mb.Mods = append(mb.Mods, p.parseModBlock(path))
		case TK_STRUCT:
			sd := p.parseStruct()
			sd.Annotations = anns
			mb.Structs = append(mb.Structs, sd)
		case TK_TYPE:
			sd := p.parseTypeStruct()
			sd.Annotations = anns
			mb.Structs = append(mb.Structs, sd)
		case TK_FN, TK_SUB:
			fn := p.parseFnDecl(anns)
			fn.ModPath = path
			if fn.HasAnnotation(AnnTest) {
				mb.Tests = append(mb.Tests, buildTestDecl(fn, path))
			} else {
				mb.Fns = append(mb.Fns, fn)
			}
		case TK_SEMI:
			p.advance()
		default:
			t := p.peek()
			if t.Kind != TK_RBRACE && t.Kind != TK_EOF {
				warnAt(t.Span,
					fmt.Sprintf("unexpected %q inside mod block", t.Value),
					"mod blocks can contain: fn, struct, type, or nested mod")
				p.advance()
			}
		}
	}
	p.expect(TK_RBRACE)
	p.eatSemi()
	return mb
}

// ── Import / use ──────────────────────────────────────────────────────────────
//
// Supported syntax:
//   use std::str                      stdlib module
//   use "stdio.h"                     C header
//   import "stdio.h"                  C header
//   use "./"::abc                     local file ./abc.zx
//   import _/mod1/logger (logger_ng)  local path ./mod1/logger.zx, import mod logger_ng
//   import __/__/mod2/abc             ../../mod2/abc.zx, import all

func (p *Parser) parseImport() *ImportDecl {
	sp := p.peek().Span
	p.advance() // import or use
	var imp ImportDecl
	imp.Sp = sp

	if p.at(TK_STRING) {
		pathTok := p.advance()
		rawPath := pathTok.Value

		// Check for: use "./"::modname  (local file import)
		if strings.HasPrefix(rawPath, "./") || strings.HasPrefix(rawPath, "../") || rawPath == "./" {
			if p.at(TK_DCOLON) {
				p.advance()
				modName := p.expect(TK_IDENT).Value
				// resolve file path: "./" + modName + ".zx"
				localPath := filepath.Join(p.srcDir, strings.TrimSuffix(rawPath, "/"), modName+".zx")
				imp.IsLocal = true
				imp.LocalFile = localPath
				imp.Module = modName
			} else {
				// bare path string — treat as C header or relative
				imp.Path = rawPath
			}
		} else {
			imp.Path = rawPath
		}

	} else if p.at(TK_IDENT) || p.at(TK_TYPE_STR) {
		// check for _/path/  or  __/__/path/  style (local file path)
		first := p.peek().Value
		if first == "_" || strings.HasPrefix(first, "_") {
			imp = p.parseLocalPathImport(sp)
			return &imp
		}

		// std::module or user::module
		name := p.advance().Value
		for p.at(TK_DCOLON) {
			p.advance()
			next := p.peek()
			if next.Kind == TK_IDENT || (next.Kind >= TK_TYPE_INT && next.Kind <= TK_TYPE_ANY) {
				name += "::" + p.advance().Value
			} else {
				break
			}
		}
		imp.Module = name
		imp.IsStd = strings.HasPrefix(name, "std::")
		imp.IsUser = !imp.IsStd

		// warn if unknown std module
		if imp.IsStd && LookupStdModule(name) == nil {
			warnAt(sp, fmt.Sprintf("unknown stdlib module %q", name),
				"known: std::str std::io std::math std::sys std::fs std::cmd std::mem std::conv std::time std::net")
		}
	} else {
		errAt(sp, "expected a string path or module name after import/use",
			`examples:  use std::str    use "stdio.h"    use "./"::mymod    import _/utils/logger`)
		p.ok = false
	}

	// optional: (mod_alias) or as alias
	if p.at(TK_LPAREN) {
		p.advance()
		imp.Alias = p.expect(TK_IDENT).Value
		p.expect(TK_RPAREN)
	} else if p.at(TK_AS) {
		p.advance()
		imp.Alias = p.expect(TK_IDENT).Value
	}
	p.eatSemi()
	return &imp
}

// parseLocalPathImport handles:
//   import _/mod1/logger (logger_ng)   → ./mod1/logger.zx, import mod logger_ng
//   import __/__/mod2/abc              → ../../mod2/abc.zx, import all
// '_' = current dir, '__' = parent dir, '___' = grandparent, etc.
func (p *Parser) parseLocalPathImport(sp Span) ImportDecl {
	var imp ImportDecl
	imp.Sp = sp

	// collect path segments separated by /
	var segments []string
	seg := p.advance().Value // first segment like "_", "__", "___"
	// count underscores to determine how many levels up
	ups := len(seg) - 1 // _ = 0 up, __ = 1 up, ___ = 2 up
	for i := 0; i < ups; i++ {
		segments = append(segments, "..")
	}
	// consume remaining path segments separated by /
	// In ZX, we use :: as path separator since / is division
	for p.at(TK_DCOLON) {
		p.advance()
		if p.at(TK_IDENT) {
			segments = append(segments, p.advance().Value)
		} else {
			break
		}
	}

	// build the local file path
	if len(segments) > 0 {
		localPath := filepath.Join(append([]string{p.srcDir}, segments...)...) + ".zx"
		imp.IsLocal = true
		imp.LocalFile = localPath
		if len(segments) > 0 {
			imp.Module = segments[len(segments)-1]
		}
	}

	// optional: (modname) to import a specific mod from the file
	if p.at(TK_LPAREN) {
		p.advance()
		imp.Alias = p.expect(TK_IDENT).Value
		p.expect(TK_RPAREN)
	} else {
		imp.ImportAll = true
	}
	p.eatSemi()
	return imp
}

func (p *Parser) parseExtern() *ExternDecl {
	sp := p.peek().Span
	p.expect(TK_EXTERN)
	if p.atAny(TK_FN, TK_SUB) {
		p.advance()
	}
	name := p.expect(TK_IDENT)
	p.expect(TK_LPAREN)
	params, variadic := p.parseParamList()
	p.expect(TK_RPAREN)
	ret := TypVoid
	if p.at(TK_ARROW) {
		p.advance()
		ret = p.parseType()
	}
	p.eatSemi()
	return &ExternDecl{Sp: sp, Name: name.Value, Params: params, Variadic: variadic, RetType: ret}
}

func (p *Parser) parseStruct() *StructDecl {
	sp := p.peek().Span
	p.expect(TK_STRUCT)
	name := p.expect(TK_IDENT)
	p.expect(TK_LBRACE)
	fields := p.parseStructFields()
	p.expect(TK_RBRACE)
	p.eatSemi()
	return &StructDecl{Sp: sp, Name: name.Value, Fields: fields}
}

func (p *Parser) parseTypeStruct() *StructDecl {
	sp := p.peek().Span
	p.expect(TK_TYPE)
	name := p.expect(TK_IDENT)
	if p.at(TK_STRUCT) {
		p.advance()
	}
	p.expect(TK_LBRACE)
	fields := p.parseStructFields()
	p.expect(TK_RBRACE)
	p.eatSemi()
	return &StructDecl{Sp: sp, Name: name.Value, Fields: fields}
}

func (p *Parser) parseStructFields() []Param {
	var fields []Param
	for !p.at(TK_RBRACE) && !p.at(TK_EOF) && p.ok {
		fsp := p.peek().Span
		fname := p.expect(TK_IDENT)
		var ftype *ZXType
		if p.at(TK_COLON) {
			p.advance()
			ftype = p.parseType()
		} else if p.isTypeStart() {
			ftype = p.parseType()
		} else {
			ftype = TypAny
		}
		fields = append(fields, Param{Sp: fsp, Name: fname.Value, Type: ftype})
		if p.at(TK_COMMA) || p.at(TK_SEMI) {
			p.advance()
		}
	}
	return fields
}

// ─────────────────────────────────────────────────────────────────────────────
//  Macro declarations
//
//  Two forms are accepted:
//
//  Form A — typed (preferred):
//    macro fn double |n: int| -> int {
//        return n * 2;
//    }
//
//  Form B — legacy untyped pipe syntax:
//    macro fn ifTrue |input, doStmt| -> |output| {
//        if input { output = doStmt(); }
//        return output;
//    }
// ─────────────────────────────────────────────────────────────────────────────

func (p *Parser) parseMacroDecl() *MacroDecl {
	sp := p.peek().Span
	p.expect(TK_MACRO)

	// 'fn' is optional after macro
	if p.atAny(TK_FN, TK_SUB) {
		p.advance()
	}

	name := p.expect(TK_IDENT).Value

	// E_M01: macro name cannot be a C reserved word
	if isCReservedFn(name) {
		errCode("EM01", sp,
			fmt.Sprintf("macro name %q is a C keyword and cannot be used", name),
			fmt.Sprintf("rename to: macro fn %s_macro ...", name))
		p.ok = false
		return nil
	}

	var params []Param
	retType := TypVoid
	var inputs, outputs []string

	if p.at(TK_PIPE) {
		// Form B: |input, doStmt| -> |output|
		// NOTE: param names can be keywords like 'input', 'output' — use advance() not expect(TK_IDENT)
		p.advance()
		for !p.at(TK_PIPE) && !p.at(TK_EOF) && p.ok {
			psp := p.peek().Span
			// Accept any token as a param name (keywords like 'input' are valid param names here)
			pnameTok := p.advance()
			pname := pnameTok.Value
			if pname == "" {
				errAt(psp, "expected a parameter name between | |",
					"write: macro fn myMacro |param1, param2| -> |output| { }")
				p.ok = false
				break
			}
			var ptype *ZXType = TypAny
			if p.at(TK_COLON) {
				p.advance()
				ptype = p.parseType()
			}
			inputs = append(inputs, pname)
			params = append(params, Param{Sp: psp, Name: pname, Type: ptype})
			if p.at(TK_COMMA) {
				p.advance()
			}
		}
		if p.at(TK_PIPE) {
			p.advance()
		}
		// optional -> |output| or -> type
		if p.at(TK_ARROW) {
			p.advance()
			if p.at(TK_PIPE) {
				// |output| form — treat as any return
				p.advance()
				for !p.at(TK_PIPE) && !p.at(TK_EOF) {
					onameTok := p.advance()
					oname := onameTok.Value
					outputs = append(outputs, oname)
					if p.at(TK_COMMA) {
						p.advance()
					}
				}
				if p.at(TK_PIPE) {
					p.advance()
				}
				retType = TypAny
			} else {
				retType = p.parseType()
			}
		}
	} else if p.at(TK_LPAREN) {
		// Form A with parens: macro fn double(n: int) -> int { }
		p.advance()
		params, _ = p.parseParamList()
		p.expect(TK_RPAREN)
		if p.at(TK_ARROW) {
			p.advance()
			retType = p.parseType()
		}
	}

	body := p.parseBlock()

	// E_M02: macro body must not be empty
	if body == nil || len(body.Stmts) == 0 {
		warnAt(sp,
			fmt.Sprintf("macro %q has an empty body", name),
			"add at least one statement to the macro body")
	}

	return &MacroDecl{
		Sp: sp, Name: name,
		Params: params, RetType: retType,
		Inputs: inputs, Outputs: outputs,
		Body: body,
	}
}

// parseBangMacroCall parses:  name!(arg1, arg2, ...)
// Called from parsePrimary when TK_BANG_MACRO is seen.
// parseBangMacroCall parses:  name!(arg1, arg2, ...)
// Returns MacroCallExpr for user-defined macros, BangMacroExpr for built-ins.
func (p *Parser) parseBangMacroCall() Node {
	sp := p.peek().Span
	t := p.advance() // consume TK_BANG_MACRO token
	name := strings.TrimSuffix(t.Value, "!")

	var args []Node
	if p.at(TK_LPAREN) {
		p.advance()
		for !p.at(TK_RPAREN) && !p.at(TK_EOF) && p.ok {
			args = append(args, p.parseExpr())
			if p.at(TK_COMMA) {
				p.advance()
			}
		}
		p.expect(TK_RPAREN)
	}

	// Built-in bang macros use BangMacroExpr (no __zx_macro_ prefix needed).
	// User-defined macros use MacroCallExpr (resolved at emit time with prefix).
	switch name {
	case "dbg", "panic", "unreachable", "todo", "env",
		"assert", "ok", "try", "log", "time":
		return &BangMacroExpr{Sp: sp, Name: name, Args: args}
	default:
		return &MacroCallExpr{Sp: sp, Name: name, Args: args}
	}
}

// tryParseMacroChain checks if what follows an expression is a macro chain:
//   value macroName: do { } macroName2: do { }
// Returns nil if no chain follows.
func (p *Parser) tryParseMacroChain(recv Node) Node {
	// A chain step looks like:  IDENT ':'  'do'  block
	// We need 2-token lookahead: IDENT followed by COLON
	if !p.at(TK_IDENT) || p.peekN(1).Kind != TK_COLON {
		return nil
	}

	sp := p.peek().Span
	chain := &MacroCallChain{Sp: sp, Recv: recv}

	for p.at(TK_IDENT) && p.peekN(1).Kind == TK_COLON && p.ok {
		stepSp := p.peek().Span
		macroName := p.advance().Value // consume macro name
		p.expect(TK_COLON)            // consume ':'

		// expect 'do' keyword followed by a block
		if !p.at(TK_DO) {
			errAt(p.peek().Span,
				fmt.Sprintf("expected 'do { }' after '%s:'", macroName),
				fmt.Sprintf("write: %s: do { /* your code */ }", macroName))
			p.ok = false
			break
		}
		p.advance() // consume 'do'
		body := p.parseBlock()
		chain.Steps = append(chain.Steps, MacroChainStep{
			Sp:    stepSp,
			Macro: macroName,
			Body:  body,
		})
	}

	if len(chain.Steps) == 0 {
		return nil
	}
	return chain
}

// ── Function declarations ─────────────────────────────────────────────────────

func (p *Parser) parseFnDecl(anns []Annotation) *FnDecl {
	sp := p.peek().Span
	p.advance() // fn or sub
	name := p.expect(TK_IDENT)
	// E90: warn if name conflicts with C keyword
	if isCReservedFn(name.Value) {
		warnCode("W10", name.Span,
			fmt.Sprintf("function name %q is a C keyword — it will be compiled as __zx_%s", name.Value, name.Value),
			fmt.Sprintf("rename to avoid confusion: fn to_%s(...) or fn my_%s(...)", name.Value, name.Value))
	}
	p.expect(TK_LPAREN)
	params, variadic := p.parseParamList()
	p.expect(TK_RPAREN)
	ret := TypVoid
	if p.at(TK_ARROW) {
		p.advance()
		ret = p.parseType()
	}
	body := p.parseBlock()
	fn := &FnDecl{Sp: sp, Name: name.Value, Params: params, Variadic: variadic, RetType: ret, Body: body, Annotations: anns}
	validateFnAnnotations(fn)
	return fn
}

func validateFnAnnotations(fn *FnDecl) {
	for _, ann := range fn.Annotations {
		switch ann.Name {
		case AnnTest, AnnIgnore, AnnSkip, AnnArgs, AnnExpect, AnnTimeout,
			AnnInline, AnnDeprecated, AnnNoReturn, AnnPure, AnnUnsafe,
			AnnExport, AnnBenchmark, AnnSetup, AnnTeardown:
		default:
			warnAt(ann.Sp, fmt.Sprintf("unknown annotation @%s", ann.Name),
				"known: @test @ignore @skip @args @expect @timeout @inline @deprecated @noreturn @pure @unsafe @export @benchmark")
		}
	}
	if fn.HasAnnotation(AnnArgs) && !fn.HasAnnotation(AnnTest) {
		warnAt(fn.Sp, fmt.Sprintf("@args on %q has no effect without @test", fn.Name),
			"add @test to make this a test case")
	}
	if fn.HasAnnotation(AnnExpect) && !fn.HasAnnotation(AnnTest) {
		warnAt(fn.Sp, fmt.Sprintf("@expect on %q has no effect without @test", fn.Name),
			"add @test to make this a test case")
	}
}

func (p *Parser) parseMethod() *MethodDecl {
	sp := p.peek().Span
	p.advance()
	p.expect(TK_LPAREN)
	recvName := p.expect(TK_IDENT).Value
	recvRef := false
	if p.atAny(TK_TYPE_REF, TK_STAR) {
		p.advance()
		recvRef = true
	}
	recvType := p.expect(TK_IDENT).Value
	p.expect(TK_RPAREN)
	methodName := p.expect(TK_IDENT).Value
	p.expect(TK_LPAREN)
	params, variadic := p.parseParamList()
	p.expect(TK_RPAREN)
	ret := TypVoid
	if p.at(TK_ARROW) {
		p.advance()
		ret = p.parseType()
	}
	body := p.parseBlock()
	return &MethodDecl{Sp: sp, RecvName: recvName, RecvType: recvType, RecvRef: recvRef,
		Name: methodName, Params: params, Variadic: variadic, RetType: ret, Body: body}
}

func (p *Parser) parseParamList() ([]Param, bool) {
	var params []Param
	variadic := false
	for !p.at(TK_RPAREN) && !p.at(TK_EOF) && p.ok {
		if p.at(TK_ELLIPSIS) || p.at(TK_DOTDOT) {
			p.advance()
			variadic = true
			break
		}
		psp := p.peek().Span
		pname := p.expect(TK_IDENT)
		var ptype *ZXType
		if p.at(TK_COLON) {
			p.advance()
			ptype = p.parseType()
		} else if p.isTypeStart() && !p.at(TK_COMMA) && !p.at(TK_RPAREN) {
			ptype = p.parseType()
		} else {
			ptype = TypAny
		}
		var def Node
		if p.at(TK_ASSIGN) {
			p.advance()
			def = p.parseExpr()
		}
		params = append(params, Param{Sp: psp, Name: pname.Value, Type: ptype, Default: def})
		if p.at(TK_COMMA) {
			p.advance()
		}
	}
	return params, variadic
}

// ── Types ─────────────────────────────────────────────────────────────────────

func (p *Parser) parseType() *ZXType {
	t := p.peek()
	if t.Kind == TK_STAR {
		p.advance()
		return RefOf(p.parseType())
	}
	if t.Kind == TK_LBRACKET {
		p.advance()
		size := 0
		if p.at(TK_INT) {
			n, _ := strconv.Atoi(p.peek().Value)
			size = n
			p.advance()
		}
		p.expect(TK_RBRACKET)
		elem := p.parseType()
		if size == 0 {
			return SliceOf(elem)
		}
		return ArrayOf(elem, size)
	}
	switch t.Kind {
	case TK_TYPE_INT:
		p.advance()
		return TypInt
	case TK_TYPE_FLOAT:
		p.advance()
		return TypFloat
	case TK_TYPE_BOOL:
		p.advance()
		return TypBool
	case TK_TYPE_STR:
		p.advance()
		return TypStr
	case TK_TYPE_CHAR:
		p.advance()
		return TypChar
	case TK_TYPE_VOID:
		p.advance()
		return TypVoid
	case TK_TYPE_ANY:
		p.advance()
		return TypAny
	case TK_TYPE_REF:
		p.advance()
		if p.at(TK_LT) {
			p.advance()
			elem := p.parseType()
			p.expect(TK_GT)
			return RefOf(elem)
		}
		if p.isTypeStart() {
			return RefOf(p.parseType())
		}
		return RefOf(TypVoid)
	case TK_IDENT:
		p.advance()
		return StructType(t.Value)
	default:
		errAt(t.Span, fmt.Sprintf("expected a type name, got %q", t.Value),
			"valid types: int float bool str char void any ref T [N]T or a struct name")
		p.ok = false
		return TypUnknown
	}
}

// ── Block ─────────────────────────────────────────────────────────────────────

func (p *Parser) parseBlock() *Block {
	sp := p.peek().Span
	p.expect(TK_LBRACE)
	var stmts []Node
	for !p.at(TK_RBRACE) && !p.at(TK_EOF) && p.ok {
		if s := p.parseStmt(); s != nil {
			stmts = append(stmts, s)
		}
	}
	p.expect(TK_RBRACE)
	return &Block{Sp: sp, Stmts: stmts}
}

// ── Statements ────────────────────────────────────────────────────────────────

func (p *Parser) parseStmt() Node {
	if p.at(TK_ANNOTATION) {
		anns := p.parseAnnotations()
		if p.atAny(TK_FN, TK_SUB) && !p.isFnMethod() {
			return p.parseFnDecl(anns)
		}
		if len(anns) > 0 {
			warnAt(anns[0].Sp,
				fmt.Sprintf("@%s inside a block is only valid before a fn", anns[0].Name), "")
		}
	}
	t := p.peek()
	switch t.Kind {
	case TK_LET, TK_MY:
		return p.parseVarDecl(false)
	case TK_CONST, TK_OUR:
		return p.parseVarDecl(true)
	case TK_RETURN:
		return p.parseReturn()
	case TK_IF:
		return p.parseIf()
	case TK_UNLESS:
		return p.parseUnless()
	case TK_WHILE:
		return p.parseWhile()
	case TK_UNTIL:
		return p.parseUntil()
	case TK_FOR, TK_FOREACH:
		return p.parseFor()
	case TK_DO:
		p.advance()
		b := p.parseBlock()
		p.eatSemi()
		return b
	case TK_MATCH:
		return p.parseMatch()
	case TK_TRY:
		return p.parseTryCatch()
	case TK_DEFER:
		return p.parseDefer()
	case TK_ASSERT:
		return p.parseAssert()
	case TK_SPAWN:
		return p.parseSpawn()
	case TK_BREAK, TK_LAST:
		p.advance()
		p.expectSemi()
		return &BreakStmt{Sp: t.Span}
	case TK_CONTINUE, TK_NEXT:
		p.advance()
		p.expectSemi()
		return &ContinueStmt{Sp: t.Span}
	case TK_PRINT:
		return p.parsePrint(false, false)
	case TK_PRINTLN, TK_SAY:
		return p.parsePrint(true, false)
	case TK_WARN, TK_EPRINT:
		return p.parsePrint(true, true)
	case TK_EXIT:
		return p.parseExit()
	case TK_DIE, TK_THROW, TK_RAISE:
		return p.parseDie()
	case TK_FN, TK_SUB:
		if p.isFnMethod() {
			p.parseMethod()
			return nil
		}
		return p.parseFnDecl(nil)
	case TK_SEMI:
		p.advance()
		return nil
	default:
		return p.parseExprOrAssign()
	}
}

func (p *Parser) parseVarDecl(isConst bool) *VarDecl {
	sp := p.peek().Span
	p.advance()
	name := p.expect(TK_IDENT)
	var vt *ZXType
	if p.at(TK_COLON) {
		p.advance()
		vt = p.parseType()
	} else if p.isTypeStart() && !p.at(TK_ASSIGN) {
		vt = p.parseType()
	}
	var init Node
	if p.at(TK_ASSIGN) {
		p.advance()
		init = p.parsePipeExpr()
	} else if isConst {
		errAt(name.Span, fmt.Sprintf("const/our %q must have an initializer", name.Value),
			fmt.Sprintf("add = <value>: const %s = 42", name.Value))
		p.ok = false
	}
	p.expectSemi()
	return &VarDecl{Sp: sp, Name: name.Value, VarType: vt, Init: init, IsConst: isConst}
}

func (p *Parser) parseReturn() *ReturnStmt {
	sp := p.peek().Span
	p.expect(TK_RETURN)
	if p.at(TK_SEMI) || p.at(TK_RBRACE) {
		p.eatSemi()
		return &ReturnStmt{Sp: sp}
	}
	val := p.parsePipeExpr()
	p.expectSemi()
	return &ReturnStmt{Sp: sp, Value: val}
}

func (p *Parser) parseIf() *IfStmt {
	sp := p.peek().Span
	p.expect(TK_IF)
	cond := p.parseExpr()
	then := p.parseBlock()
	var elifs []ElifClause
	var els *Block
	for p.atAny(TK_ELIF, TK_ELSE) && p.ok {
		if p.at(TK_ELIF) {
			p.advance()
			ec := p.parseExpr()
			eb := p.parseBlock()
			elifs = append(elifs, ElifClause{Cond: ec, Body: eb})
		} else {
			p.advance()
			els = p.parseBlock()
			break
		}
	}
	return &IfStmt{Sp: sp, Cond: cond, Then: then, Elifs: elifs, Else: els}
}

func (p *Parser) parseUnless() *UnlessStmt {
	sp := p.peek().Span
	p.expect(TK_UNLESS)
	cond := p.parseExpr()
	body := p.parseBlock()
	var els *Block
	if p.at(TK_ELSE) {
		p.advance()
		els = p.parseBlock()
	}
	return &UnlessStmt{Sp: sp, Cond: cond, Body: body, Else: els}
}

func (p *Parser) parseWhile() *WhileStmt {
	sp := p.peek().Span
	p.expect(TK_WHILE)
	return &WhileStmt{Sp: sp, Cond: p.parseExpr(), Body: p.parseBlock()}
}

func (p *Parser) parseUntil() *UntilStmt {
	sp := p.peek().Span
	p.expect(TK_UNTIL)
	return &UntilStmt{Sp: sp, Cond: p.parseExpr(), Body: p.parseBlock()}
}

func (p *Parser) parseFor() Node {
	sp := p.peek().Span
	p.advance()
	varName := p.expect(TK_IDENT)
	p.expect(TK_IN)
	from := p.parseExpr()
	p.expect(TK_DOTDOT)
	to := p.parseExpr()
	var step Node
	if p.at(TK_COLON) {
		p.advance()
		step = p.parseExpr()
	}
	body := p.parseBlock()
	return &ForRangeStmt{Sp: sp, Var: varName.Value, From: from, To: to, Step: step, Body: body}
}

func (p *Parser) parseMatch() *MatchStmt {
	sp := p.peek().Span
	p.expect(TK_MATCH)
	expr := p.parseExpr()
	p.expect(TK_LBRACE)
	var arms []MatchArm
	for !p.at(TK_RBRACE) && !p.at(TK_EOF) && p.ok {
		asp := p.peek().Span
		isWild := false
		var pattern Node
		if p.at(TK_IDENT) && p.peek().Value == "_" {
			p.advance()
			isWild = true
		} else {
			pattern = p.parseExpr()
		}
		var guard Node
		if p.at(TK_IF) {
			p.advance()
			guard = p.parseExpr()
		}
		if p.at(TK_FAT_ARROW) {
			p.advance()
		}
		body := p.parseBlock()
		arms = append(arms, MatchArm{Sp: asp, Pattern: pattern, IsWild: isWild, Guard: guard, Body: body})
		if p.at(TK_COMMA) {
			p.advance()
		}
	}
	p.expect(TK_RBRACE)
	return &MatchStmt{Sp: sp, Expr: expr, Arms: arms}
}

func (p *Parser) parseTryCatch() *TryCatchStmt {
	sp := p.peek().Span
	p.expect(TK_TRY)
	tryBlock := p.parseBlock()
	var errVar string
	var catchBlock, finallyBlock *Block
	if p.at(TK_CATCH) {
		p.advance()
		if p.at(TK_LPAREN) {
			p.advance()
			errVar = p.expect(TK_IDENT).Value
			p.expect(TK_RPAREN)
		}
		catchBlock = p.parseBlock()
	}
	if p.at(TK_FINALLY) {
		p.advance()
		finallyBlock = p.parseBlock()
	}
	return &TryCatchStmt{Sp: sp, Try: tryBlock, ErrVar: errVar, Catch: catchBlock, Finally: finallyBlock}
}

func (p *Parser) parseDefer() *DeferStmt {
	sp := p.peek().Span
	p.expect(TK_DEFER)
	call := p.parseExpr()
	p.expectSemi()
	return &DeferStmt{Sp: sp, Call: call}
}

func (p *Parser) parseAssert() *AssertStmt {
	sp := p.peek().Span
	p.expect(TK_ASSERT)
	cond := p.parseExpr()
	var msg Node
	if p.at(TK_COMMA) {
		p.advance()
		msg = p.parseExpr()
	} else {
		msg = &StrLit{Sp: sp, Val: "assertion failed"}
	}
	p.expectSemi()
	return &AssertStmt{Sp: sp, Cond: cond, Msg: msg}
}

func (p *Parser) parseSpawn() *SpawnStmt {
	sp := p.peek().Span
	p.expect(TK_SPAWN)
	call := p.parseExpr()
	p.expectSemi()
	return &SpawnStmt{Sp: sp, Call: call}
}

func (p *Parser) parsePrint(newline, toStderr bool) *PrintStmt {
	sp := p.peek().Span
	p.advance()
	if !p.at(TK_LPAREN) {
		var args []Node
		for !p.at(TK_SEMI) && !p.at(TK_RBRACE) && !p.at(TK_EOF) && p.ok {
			args = append(args, p.parseExpr())
			if p.at(TK_COMMA) {
				p.advance()
			}
		}
		p.expectSemi()
		return &PrintStmt{Sp: sp, Args: args, Newline: newline, ToStderr: toStderr}
	}
	p.expect(TK_LPAREN)
	var args []Node
	for !p.at(TK_RPAREN) && !p.at(TK_EOF) && p.ok {
		args = append(args, p.parseExpr())
		if p.at(TK_COMMA) {
			p.advance()
		}
	}
	p.expect(TK_RPAREN)
	p.expectSemi()
	return &PrintStmt{Sp: sp, Args: args, Newline: newline, ToStderr: toStderr}
}

func (p *Parser) parseExit() *ExitStmt {
	sp := p.peek().Span
	p.expect(TK_EXIT)
	if p.at(TK_LPAREN) {
		p.advance()
		code := p.parseExpr()
		p.expect(TK_RPAREN)
		p.expectSemi()
		return &ExitStmt{Sp: sp, Code: code}
	}
	code := p.parseExpr()
	p.expectSemi()
	return &ExitStmt{Sp: sp, Code: code}
}

func (p *Parser) parseDie() Node {
	sp := p.peek().Span
	p.advance()
	var msg Node
	if !p.at(TK_SEMI) && !p.at(TK_RBRACE) && !p.at(TK_EOF) {
		msg = p.parseExpr()
	} else {
		msg = &StrLit{Sp: sp, Val: "program died"}
	}
	p.expectSemi()
	msgPrint := &PrintStmt{Sp: sp, Args: []Node{msg}, Newline: true, ToStderr: true}
	return &Block{Sp: sp, Stmts: []Node{msgPrint, &ExitStmt{Sp: sp, Code: &IntLit{Sp: sp, Val: 1}}}}
}

func (p *Parser) parseExprOrAssign() Node {
	sp := p.peek().Span
	expr := p.parsePipeExpr()
	switch p.peek().Kind {
	case TK_ASSIGN, TK_PLUS_EQ, TK_MINUS_EQ, TK_STAR_EQ, TK_SLASH_EQ, TK_PERCENT_EQ:
		op := p.advance().Value
		val := p.parsePipeExpr()
		p.expectSemi()
		return &AssignStmt{Sp: sp, LHS: expr, Op: op, Value: val}
	}
	// Try macro chain:  expr macroName: do { } ...
	if chain := p.tryParseMacroChain(expr); chain != nil {
		p.expectSemi()
		return &ExprStmt{Sp: sp, Expr: chain}
	}
	p.expectSemi()
	return &ExprStmt{Sp: sp, Expr: expr}
}

// ── Pipe ──────────────────────────────────────────────────────────────────────

func (p *Parser) parsePipeExpr() Node {
	lhs := p.parseTernary()
	if !p.at(TK_PIPE_ARROW) {
		return lhs
	}
	sp := p.peek().Span
	steps := []Node{lhs}
	for p.at(TK_PIPE_ARROW) && p.ok {
		p.advance()
		steps = append(steps, p.parseUnary())
	}
	return &PipeExpr{Sp: sp, Steps: steps}
}

// After parsing a primary expression as a statement, check if a macro chain follows.
// e.g.  whoami() ifTrue: do { say "root"; } ifFalse: do { say "not root"; }
func (p *Parser) parseMaybeChain(expr Node) Node {
	chain := p.tryParseMacroChain(expr)
	if chain != nil {
		return chain
	}
	return expr
}

func (p *Parser) parseTernary() Node {
	cond := p.parseExpr()
	if !p.at(TK_QUESTION) {
		return cond
	}
	sp := p.peek().Span
	p.advance()
	then := p.parseExpr()
	p.expect(TK_COLON)
	els := p.parseExpr()
	return &TernaryExpr{Sp: sp, Cond: cond, Then: then, Else: els}
}

// ── Expressions ───────────────────────────────────────────────────────────────

func (p *Parser) parseExpr() Node { return p.parseOr() }
func (p *Parser) parseOr() Node {
	lhs := p.parseAnd()
	for p.at(TK_OR) && p.ok {
		sp := p.peek().Span
		p.advance()
		lhs = &BinExpr{Sp: sp, Op: "||", LHS: lhs, RHS: p.parseAnd()}
	}
	return lhs
}
func (p *Parser) parseAnd() Node {
	lhs := p.parseBitOr()
	for p.at(TK_AND) && p.ok {
		sp := p.peek().Span
		p.advance()
		lhs = &BinExpr{Sp: sp, Op: "&&", LHS: lhs, RHS: p.parseBitOr()}
	}
	return lhs
}
func (p *Parser) parseBitOr() Node {
	lhs := p.parseBitXor()
	for p.at(TK_PIPE) && p.ok {
		sp := p.peek().Span
		p.advance()
		lhs = &BinExpr{Sp: sp, Op: "|", LHS: lhs, RHS: p.parseBitXor()}
	}
	return lhs
}
func (p *Parser) parseBitXor() Node {
	lhs := p.parseBitAnd()
	for p.at(TK_CARET) && p.ok {
		sp := p.peek().Span
		p.advance()
		lhs = &BinExpr{Sp: sp, Op: "^", LHS: lhs, RHS: p.parseBitAnd()}
	}
	return lhs
}
func (p *Parser) parseBitAnd() Node {
	lhs := p.parseEquality()
	for p.at(TK_AMP) && p.ok {
		sp := p.peek().Span
		p.advance()
		lhs = &BinExpr{Sp: sp, Op: "&", LHS: lhs, RHS: p.parseEquality()}
	}
	return lhs
}
func (p *Parser) parseEquality() Node {
	lhs := p.parseRelational()
	for p.atAny(TK_EQ, TK_NEQ) && p.ok {
		sp := p.peek().Span
		op := p.advance().Value
		lhs = &BinExpr{Sp: sp, Op: op, LHS: lhs, RHS: p.parseRelational()}
	}
	return lhs
}
func (p *Parser) parseRelational() Node {
	lhs := p.parseShift()
	for p.atAny(TK_LT, TK_GT, TK_LTE, TK_GTE) && p.ok {
		sp := p.peek().Span
		op := p.advance().Value
		lhs = &BinExpr{Sp: sp, Op: op, LHS: lhs, RHS: p.parseShift()}
	}
	return lhs
}
func (p *Parser) parseShift() Node {
	lhs := p.parseAddSub()
	for p.atAny(TK_LSHIFT, TK_RSHIFT) && p.ok {
		sp := p.peek().Span
		op := p.advance().Value
		lhs = &BinExpr{Sp: sp, Op: op, LHS: lhs, RHS: p.parseAddSub()}
	}
	return lhs
}
func (p *Parser) parseAddSub() Node {
	lhs := p.parseMulDiv()
	for p.atAny(TK_PLUS, TK_MINUS) && p.ok {
		sp := p.peek().Span
		op := p.advance().Value
		lhs = &BinExpr{Sp: sp, Op: op, LHS: lhs, RHS: p.parseMulDiv()}
	}
	return lhs
}
func (p *Parser) parseMulDiv() Node {
	lhs := p.parseUnary()
	for p.atAny(TK_STAR, TK_SLASH, TK_PERCENT) && p.ok {
		sp := p.peek().Span
		op := p.advance().Value
		lhs = &BinExpr{Sp: sp, Op: op, LHS: lhs, RHS: p.parseUnary()}
	}
	return lhs
}

func (p *Parser) parseUnary() Node {
	sp := p.peek().Span
	switch p.peek().Kind {
	case TK_NOT:
		p.advance()
		return &UnaryExpr{Sp: sp, Op: "!", Operand: p.parseUnary()}
	case TK_MINUS:
		p.advance()
		return &UnaryExpr{Sp: sp, Op: "-", Operand: p.parseUnary()}
	case TK_TILDE:
		p.advance()
		return &UnaryExpr{Sp: sp, Op: "~", Operand: p.parseUnary()}
	case TK_AMP, TK_AT:
		p.advance()
		if p.at(TK_IDENT) && p.peekN(1).Kind == TK_LBRACE {
			return p.parseHeapStructInit()
		}
		return &AddrExpr{Sp: sp, Operand: p.parseUnary(), Deref: false}
	case TK_STAR, TK_HAT:
		p.advance()
		return &AddrExpr{Sp: sp, Operand: p.parseUnary(), Deref: true}
	}
	return p.parsePostfix()
}

func (p *Parser) parsePostfix() Node {
	expr := p.parsePrimary()
	for p.ok {
		sp := p.peek().Span
		switch p.peek().Kind {
		case TK_LPAREN:
			p.advance()
			var args []Node
			for !p.at(TK_RPAREN) && !p.at(TK_EOF) && p.ok {
				args = append(args, p.parseExpr())
				if p.at(TK_COMMA) {
					p.advance()
				}
			}
			p.expect(TK_RPAREN)
			expr = &CallExpr{Sp: sp, Func: expr, Args: args}
		case TK_LBRACKET:
			p.advance()
			idx := p.parseExpr()
			p.expect(TK_RBRACKET)
			expr = &IndexExpr{Sp: sp, Obj: expr, Idx: idx}
		case TK_DOT:
			p.advance()
			field := p.expect(TK_IDENT)
			if p.at(TK_LPAREN) {
				p.advance()
				var args []Node
				for !p.at(TK_RPAREN) && !p.at(TK_EOF) && p.ok {
					args = append(args, p.parseExpr())
					if p.at(TK_COMMA) {
						p.advance()
					}
				}
				p.expect(TK_RPAREN)
				expr = &MethodCallExpr{Sp: sp, Recv: expr, Method: field.Value, Args: args}
			} else {
				expr = &FieldExpr{Sp: sp, Obj: expr, Field: field.Value, UsedDot: true}
			}
		case TK_ARROW:
			p.advance()
			if p.at(TK_IDENT) {
				field := p.advance()
				if p.at(TK_LPAREN) {
					p.advance()
					var args []Node
					for !p.at(TK_RPAREN) && !p.at(TK_EOF) && p.ok {
						args = append(args, p.parseExpr())
						if p.at(TK_COMMA) {
							p.advance()
						}
					}
					p.expect(TK_RPAREN)
					expr = &MethodCallExpr{Sp: sp, Recv: expr, Method: field.Value, Args: args}
				} else {
					expr = &FieldExpr{Sp: sp,
						Obj:   &AddrExpr{Sp: sp, Operand: expr, Deref: true},
						Field: field.Value,
					}
				}
			}
		default:
			return expr
		}
	}
	return expr
}

func (p *Parser) parsePrimary() Node {
	t := p.peek()
	switch t.Kind {
	case TK_INT:
		p.advance()
		v, _ := strconv.ParseInt(t.Value, 0, 64)
		return &IntLit{Sp: t.Span, Val: v}
	case TK_FLOAT:
		p.advance()
		v, _ := strconv.ParseFloat(t.Value, 64)
		return &FloatLit{Sp: t.Span, Val: v}
	case TK_BOOL:
		p.advance()
		return &BoolLit{Sp: t.Span, Val: t.Value == "true"}
	case TK_STRING:
		p.advance()
		return &StrLit{Sp: t.Span, Val: t.Value}
	case TK_TEMPLATE_STR:
		p.advance()
		return p.parseTemplateStr(t)
	case TK_NIL:
		p.advance()
		return &NilLit{Sp: t.Span}
	case TK_SIZEOF:
		return p.parseSizeof()
	case TK_TYPEOF:
		return p.parseTypeof()
	case TK_NEW:
		return p.parseNew()
	case TK_LBRACKET:
		return p.parseArrayLit()
	case TK_LPAREN:
		p.advance()
		inner := p.parseExpr()
		p.expect(TK_RPAREN)
		return inner
	case TK_TYPE_INT, TK_TYPE_FLOAT, TK_TYPE_BOOL, TK_TYPE_CHAR, TK_TYPE_STR:
		typTok := p.advance()
		toType := tokenToType(typTok)
		p.expect(TK_LPAREN)
		operand := p.parseExpr()
		p.expect(TK_RPAREN)
		return &CastExpr{Sp: t.Span, ToType: toType, Operand: operand}
	case TK_CMD, TK_READFILE, TK_WRITEFILE:
		return p.parseCmdOrFile()
	case TK_BANG_MACRO:
		return p.parseBangMacroCall()
	case TK_INPUT, TK_STDIN:
		return p.parseInput()
	case TK_LEN:
		p.advance()
		p.expect(TK_LPAREN)
		arg := p.parseExpr()
		p.expect(TK_RPAREN)
		return &BuiltinExpr{Sp: t.Span, Name: "len", Args: []Node{arg}}
	case TK_PUSH:
		p.advance()
		p.expect(TK_LPAREN)
		var args []Node
		for !p.at(TK_RPAREN) && !p.at(TK_EOF) && p.ok {
			args = append(args, p.parseExpr())
			if p.at(TK_COMMA) {
				p.advance()
			}
		}
		p.expect(TK_RPAREN)
		return &BuiltinExpr{Sp: t.Span, Name: "push", Args: args}
	case TK_POP:
		p.advance()
		p.expect(TK_LPAREN)
		arg := p.parseExpr()
		p.expect(TK_RPAREN)
		return &BuiltinExpr{Sp: t.Span, Name: "pop", Args: []Node{arg}}
	case TK_IDENT:
		p.advance()
		return &Ident{Sp: t.Span, Name: t.Value}
	default:
		errAt(t.Span, fmt.Sprintf("unexpected token '%s' in expression", t.Value),
			"check for a missing operator, unmatched parenthesis, or a typo")
		p.ok = false
		return &NilLit{Sp: t.Span}
	}
}

func (p *Parser) parseTemplateStr(tok Token) *TemplateStr {
	sp := tok.Span
	raw := tok.Value
	ts := &TemplateStr{Sp: sp, Typ: TypStr}
	i := 0
	for i < len(raw) {
		if raw[i] == '{' {
			j := strings.Index(raw[i+1:], "}")
			if j < 0 {
				errAt(sp, "template string: unclosed { in interpolation",
					"add a closing } to the f-string expression")
				p.ok = false
				break
			}
			exprSrc := raw[i+1 : i+1+j]
			subToks := Tokenize(exprSrc, sp.File)
			if subToks != nil {
				subProg := Parse(subToks, exprSrc, sp.File)
				if subProg != nil && len(subProg.TopStmts) > 0 {
					if es, ok := subProg.TopStmts[0].(*ExprStmt); ok {
						ts.Parts = append(ts.Parts, TplPart{IsExpr: true, Expr: es.Expr})
					}
				}
			}
			i = i + 1 + j + 1
		} else {
			j := strings.Index(raw[i:], "{")
			if j < 0 {
				ts.Parts = append(ts.Parts, TplPart{Text: raw[i:]})
				break
			}
			ts.Parts = append(ts.Parts, TplPart{Text: raw[i : i+j]})
			i = i + j
		}
	}
	return ts
}

func (p *Parser) parseCmdOrFile() Node {
	sp := p.peek().Span
	kind := p.advance()
	p.expect(TK_LPAREN)
	arg := p.parseExpr()
	var second Node
	if p.at(TK_COMMA) {
		p.advance()
		second = p.parseExpr()
	}
	p.expect(TK_RPAREN)
	switch kind.Kind {
	case TK_CMD:
		return &CmdExpr{Sp: sp, Command: arg, CaptureOutput: true}
	case TK_READFILE:
		return &ReadFileExpr{Sp: sp, Path: arg}
	case TK_WRITEFILE:
		if second == nil {
			second = &StrLit{Sp: sp, Val: ""}
		}
		return &CallExpr{Sp: sp,
			Func: &Ident{Sp: sp, Name: "__zx_write_file"},
			Args: []Node{arg, second},
		}
	}
	return &NilLit{Sp: sp}
}

func (p *Parser) parseInput() Node {
	sp := p.peek().Span
	p.advance()
	var prompt Node
	if p.at(TK_LPAREN) {
		p.advance()
		if !p.at(TK_RPAREN) {
			prompt = p.parseExpr()
		}
		p.expect(TK_RPAREN)
	}
	args := []Node{}
	if prompt != nil {
		args = []Node{prompt}
	}
	return &BuiltinExpr{Sp: sp, Name: "input", Args: args}
}

func (p *Parser) parseSizeof() Node {
	sp := p.peek().Span
	p.expect(TK_SIZEOF)
	p.expect(TK_LPAREN)
	typ := p.parseType()
	p.expect(TK_RPAREN)
	return &SizeofExpr{Sp: sp, Of: typ, Typ: TypInt}
}

func (p *Parser) parseTypeof() Node {
	sp := p.peek().Span
	p.expect(TK_TYPEOF)
	p.expect(TK_LPAREN)
	arg := p.parseExpr()
	p.expect(TK_RPAREN)
	return &TypeofExpr{Sp: sp, Arg: arg, Typ: TypStr}
}

func (p *Parser) parseNew() Node {
	sp := p.peek().Span
	p.expect(TK_NEW)
	name := p.expect(TK_IDENT)
	p.expect(TK_LBRACE)
	fields := p.parseFieldInits()
	p.expect(TK_RBRACE)
	return &StructInit{Sp: sp, Name: name.Value, Fields: fields, HeapAlloc: false}
}

func (p *Parser) parseHeapStructInit() Node {
	sp := p.peek().Span
	name := p.advance().Value
	p.expect(TK_LBRACE)
	fields := p.parseFieldInits()
	p.expect(TK_RBRACE)
	return &StructInit{Sp: sp, Name: name, Fields: fields, HeapAlloc: true}
}

func (p *Parser) parseFieldInits() []FieldInit {
	var fields []FieldInit
	for !p.at(TK_RBRACE) && !p.at(TK_EOF) && p.ok {
		fsp := p.peek().Span
		fname := p.expect(TK_IDENT)
		if p.at(TK_COLON) || p.at(TK_FAT_ARROW) {
			p.advance()
		}
		fval := p.parseExpr()
		fields = append(fields, FieldInit{Sp: fsp, Name: fname.Value, Value: fval})
		if p.at(TK_COMMA) || p.at(TK_SEMI) {
			p.advance()
		}
	}
	return fields
}

func (p *Parser) parseArrayLit() Node {
	sp := p.peek().Span
	p.expect(TK_LBRACKET)
	var elems []Node
	for !p.at(TK_RBRACKET) && !p.at(TK_EOF) && p.ok {
		elems = append(elems, p.parseExpr())
		if p.at(TK_COMMA) {
			p.advance()
		}
	}
	p.expect(TK_RBRACKET)
	return &ArrayLit{Sp: sp, Elems: elems}
}

func tokenToType(t Token) *ZXType {
	switch t.Kind {
	case TK_TYPE_INT:
		return TypInt
	case TK_TYPE_FLOAT:
		return TypFloat
	case TK_TYPE_BOOL:
		return TypBool
	case TK_TYPE_CHAR:
		return TypChar
	case TK_TYPE_STR:
		return TypStr
	default:
		return TypUnknown
	}
}

// isCReservedFn returns true if the name is a C keyword that would
// break emitted code if used as a function name.
func isCReservedFn(name string) bool {
	switch name {
	case "double", "float", "int", "long", "short", "char", "void",
		"struct", "enum", "union", "typedef", "auto", "register",
		"static", "extern", "const", "volatile", "signed", "unsigned",
		"inline", "return", "if", "else", "for", "while", "do",
		"switch", "case", "break", "continue", "goto", "default",
		"sizeof", "NULL", "true", "false", "bool", "string":
		return true
	}
	return false
}

var _ = strings.Join
var _ = filepath.Join
