package main

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
)

// ─────────────────────────────────────────────────────────────────────────────
//  Parser
// ─────────────────────────────────────────────────────────────────────────────

type Parser struct {
	tokens     []Token
	pos        int
	file       string
	ok         bool
	srcDir     string
	macroScope map[string]bool
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
func (p *Parser) at(k TK) bool    { return p.peek().Kind == k }
func (p *Parser) check(k TK) bool { return p.peek().Kind == k }

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

func (p *Parser) parseVis() Vis {
	if p.check(TK_PRIV) {
		p.advance()
		return VisPrivate
	}
	return VisPublic
}

func (p *Parser) isInMacroScope(name string) bool {
	return p.macroScope != nil && p.macroScope[name]
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

// ─────────────────────────────────────────────────────────────────────────────
//  Program
// ─────────────────────────────────────────────────────────────────────────────

func (p *Parser) parseProgram() *Program {
	prog := &Program{}
	if p.at(TK_MOD) && p.peekN(1).Kind == TK_IDENT && p.peekN(2).Kind != TK_LBRACE {
		p.advance()
		prog.Module = p.expect(TK_IDENT).Value
		p.eatSemi()
	}
	for !p.at(TK_EOF) && p.ok {
		anns := p.parseAnnotations()

		vis := VisPublic
		if p.at(TK_PRIV) {
			vis = p.parseVis()
		}

		switch p.peek().Kind {
		case TK_IMPORT, TK_USE:
			if len(anns) > 0 {
				warnAt(anns[0].Sp, "annotations on imports are ignored", "")
			}
			if vis == VisPrivate {
				warnAt(p.peek().Span, "'priv' on an import has no effect — imports are always file-local", "")
			}
			prog.Imports = append(prog.Imports, p.parseImport())

		case TK_MOD:
			mb := p.parseModBlock("")
			mb.Vis = vis
			prog.ModBlocks = append(prog.ModBlocks, mb)

		case TK_EXTERN:
			ext := p.parseExtern()
			ext.Vis = vis
			prog.Externs = append(prog.Externs, ext)

		case TK_STRUCT:
			sd := p.parseStruct()
			sd.Annotations = anns
			sd.Vis = vis
			prog.Structs = append(prog.Structs, sd)

		case TK_TYPE:
			sd := p.parseTypeStruct()
			sd.Annotations = anns
			sd.Vis = vis
			prog.Structs = append(prog.Structs, sd)

		case TK_MACRO:
			mc := p.parseMacroDecl()
			if mc != nil {
				mc.Vis = vis
				prog.Macros = append(prog.Macros, mc)
			}

		case TK_FN, TK_SUB:
			if p.isFnMethod() {
				md := p.parseMethod()
				md.Annotations = anns
				md.Vis = vis
				prog.Methods = append(prog.Methods, md)
			} else {
				fn := p.parseFnDecl(anns)
				fn.Vis = vis
				prog.TopStmts = append(prog.TopStmts, fn)
			}

		case TK_CONST, TK_OUR:
			vd := p.parseVarDecl(true)
			vd.Vis = vis
			prog.TopStmts = append(prog.TopStmts, vd)

		case TK_LET, TK_MY:
			vd := p.parseVarDecl(false)
			vd.Vis = vis
			prog.TopStmts = append(prog.TopStmts, vd)

		default:
			if vis == VisPrivate {
				errAt(p.peek().Span,
					fmt.Sprintf("'priv' cannot be applied to %q — priv is only valid before fn, struct, macro, mod, extern, or a variable declaration", p.peek().Value),
					"valid: priv fn foo() { }  |  priv struct Bar { }  |  priv macro baz |x| { }")
				p.ok = false
				break
			}
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

// ─────────────────────────────────────────────────────────────────────────────
//  Mod block
//
//  Supports:
//    • nested mod blocks (arbitrary depth)
//    • priv on each item
//    • property declarations (module-level variables with optional get/set)
//
//  Syntax examples inside a mod block:
//    property count int = 0
//    property name str {
//        get { return __name; }
//        set(v) { __name = v; }
//    }
// ─────────────────────────────────────────────────────────────────────────────

func (p *Parser) parseModBlock(parentPath string) *ModBlock {
	sp := p.peek().Span
	p.expect(TK_MOD)
	name := p.expect(TK_IDENT).Value
	p.expect(TK_LBRACE)
	path := name
	if parentPath != "" {
		path = parentPath + "::" + name
	}
	mb := &ModBlock{Sp: sp, Name: name, Path: path, Vis: VisPublic}
	for !p.at(TK_RBRACE) && !p.at(TK_EOF) && p.ok {
		anns := p.parseAnnotations()

		itemVis := VisPublic
		if p.at(TK_PRIV) {
			itemVis = p.parseVis()
		}

		switch p.peek().Kind {
		case TK_MOD:
			nested := p.parseModBlock(path)
			nested.Vis = itemVis
			mb.Mods = append(mb.Mods, nested)

		case TK_PROPERTY:
			prop := p.parseModProperty()
			prop.Vis = itemVis
			mb.Properties = append(mb.Properties, prop)

		case TK_STRUCT:
			sd := p.parseStruct()
			sd.Annotations = anns
			sd.Vis = itemVis
			mb.Structs = append(mb.Structs, sd)

		case TK_TYPE:
			sd := p.parseTypeStruct()
			sd.Annotations = anns
			sd.Vis = itemVis
			mb.Structs = append(mb.Structs, sd)

		case TK_FN, TK_SUB:
			fn := p.parseFnDecl(anns)
			fn.ModPath = path
			fn.Vis = itemVis
			if fn.HasAnnotation(AnnTest) {
				mb.Tests = append(mb.Tests, buildTestDecl(fn, path))
			} else {
				mb.Fns = append(mb.Fns, fn)
			}

		case TK_SEMI:
			p.advance()

		case TK_CONST, TK_OUR, TK_LET, TK_MY:
			isConst := p.peek().Kind == TK_CONST || p.peek().Kind == TK_OUR
			vd := p.parseVarDecl(isConst)
			vd.Vis = itemVis
			mb.Consts = append(mb.Consts, vd)

		default:
			t := p.peek()
			if itemVis == VisPrivate {
				errAt(t.Span,
					fmt.Sprintf("'priv' cannot be applied to %q inside a mod block", t.Value),
					"valid inside mod: priv fn, priv struct, priv type, priv mod, priv let/const, priv property")
				p.ok = false
				break
			}
			if t.Kind != TK_RBRACE && t.Kind != TK_EOF {
				warnAt(t.Span,
					fmt.Sprintf("unexpected %q inside mod block — move this outside the mod, or use a fn", t.Value),
					"mod blocks can contain: fn, struct, type, const, let, property, and nested mod")
				p.advance()
			}
		}
	}
	p.expect(TK_RBRACE)
	p.eatSemi()
	return mb
}

// parseModProperty parses:
//
//	property <name> [type] [= <init>]
//	property <name> [type] {
//	    get { ... }
//	    set[(param)] { ... }
//	}
func (p *Parser) parseModProperty() *ModProperty {
	sp := p.peek().Span
	p.expect(TK_PROPERTY)
	name := p.expect(TK_IDENT).Value

	// optional type
	var typ *ZXType
	if p.isTypeStart() {
		typ = p.parseType()
	} else if !p.at(TK_ASSIGN) && !p.at(TK_LBRACE) && !p.at(TK_SEMI) {
		// try ident as struct type
		if p.at(TK_IDENT) {
			typ = StructType(p.advance().Value)
		}
	}
	if typ == nil {
		typ = TypAny
	}

	prop := &ModProperty{Sp: sp, Name: name, Type: typ, SetParam: "value"}

	// property name int = 42
	if p.at(TK_ASSIGN) {
		p.advance()
		prop.Init = p.parseExpr()
		p.expectSemi()
		// auto get/set
		prop.HasGet = true
		prop.HasSet = true
		return prop
	}

	// property name int { get { ... } set(v) { ... } }
	if p.at(TK_LBRACE) {
		p.advance()
		for !p.at(TK_RBRACE) && !p.at(TK_EOF) && p.ok {
			switch {
			case p.at(TK_GET):
				if prop.HasGet {
					errAt(p.peek().Span,
						fmt.Sprintf("property %q has duplicate 'get' block", name),
						"remove the extra get block")
					p.ok = false
				}
				p.advance()
				prop.HasGet = true
				prop.GetBody = p.parseBlock()
			case p.at(TK_SET):
				if prop.HasSet {
					errAt(p.peek().Span,
						fmt.Sprintf("property %q has duplicate 'set' block", name),
						"remove the extra set block")
					p.ok = false
				}
				p.advance()
				if p.at(TK_LPAREN) {
					p.advance()
					prop.SetParam = p.expect(TK_IDENT).Value
					p.expect(TK_RPAREN)
				}
				prop.HasSet = true
				prop.SetBody = p.parseBlock()
			case p.at(TK_SEMI):
				p.advance()
			default:
				errAt(p.peek().Span,
					fmt.Sprintf("unexpected %q inside property block — expected 'get' or 'set'", p.peek().Value),
					"write: property foo int { get { return __foo; } set(v) { __foo = v; } }")
				p.ok = false
				p.advance()
			}
		}
		p.expect(TK_RBRACE)
		p.eatSemi()
		// if neither get nor set given, default to both auto
		if !prop.HasGet && !prop.HasSet {
			prop.HasGet = true
			prop.HasSet = true
		}
		return prop
	}

	// bare: property count int;  — auto get+set, zero init
	p.expectSemi()
	prop.HasGet = true
	prop.HasSet = true
	return prop
}

// ─────────────────────────────────────────────────────────────────────────────
//  Import parsing
// ─────────────────────────────────────────────────────────────────────────────

func (p *Parser) parseImport() *ImportDecl {
	sp := p.peek().Span
	kw := p.advance()
	_ = kw
	imp := &ImportDecl{Sp: sp}

	if p.at(TK_EOF) {
		errAt(sp, "unexpected end of file after import/use",
			"expected a module name, path, or string literal")
		p.ok = false
		return imp
	}

	switch {
	case p.at(TK_STRING):
		raw := p.advance().Value
		if raw == "" {
			errAt(sp, "import path cannot be empty", `provide a header name: use "stdio.h"`)
			p.ok = false
			return imp
		}
		imp.Path = raw
		imp.IsCHeader = true
		imp.IsLocal = false

	case p.at(TK_IDENT) && len(p.peek().Value) > 0 && p.peek().Value[0] == '_':
		p.parseLocalImport(sp, imp)

	case p.at(TK_IDENT) && (p.peek().Value == "std" || p.peek().Value == "usr") &&
		p.peekN(1).Kind == TK_SLASH:
		p.parseEnvImport(sp, imp)

	case p.at(TK_IDENT) || p.at(TK_TYPE_STR):
		p.parseStdModuleImport(sp, imp)

	default:
		errAt(sp, fmt.Sprintf("unexpected token %q after import/use", p.peek().Value),
			`valid forms:
  import std/net/socket
  import _/a
  import __/a
  use std::str
  use "stdio.h"`)
		p.ok = false
	}

	p.eatSemi()
	return imp
}

func (p *Parser) parseLocalImport(sp Span, imp *ImportDecl) {
	seg := p.advance().Value

	ups := 0
	for _, c := range seg {
		if c == '_' {
			ups++
		}
	}
	ups--

	if ups < 0 {
		errAt(sp, "import path must start with _ (current dir) or __ (parent), ___  (grandparent), etc.",
			"example: import _/utils  or  import __/shared")
		p.ok = false
		return
	}

	if !p.at(TK_SLASH) && !p.at(TK_DCOLON) {
		errAt(sp, fmt.Sprintf("expected '/' or '::' after %q in import path", seg),
			fmt.Sprintf("example: import %s/modulename", seg))
		p.ok = false
		return
	}

	var segments []string
	for (p.at(TK_SLASH) || p.at(TK_DCOLON)) && p.ok {
		sepSp := p.peek().Span
		p.advance()
		if !p.at(TK_IDENT) {
			errAt(sepSp, "expected a module/directory name after '/'",
				"example: import _/utils  or  import __/net/socket")
			p.ok = false
			return
		}
		s := p.advance().Value
		if s == "" {
			errAt(sepSp, "path segment cannot be empty", "")
			p.ok = false
			return
		}
		segments = append(segments, s)
	}

	if len(segments) == 0 {
		errAt(sp, "import path must have at least one segment after the prefix",
			"example: import _/mymodule  or  import __/shared/utils")
		p.ok = false
		return
	}

	var pathParts []string
	for i := 0; i < ups; i++ {
		pathParts = append(pathParts, "..")
	}
	pathParts = append(pathParts, segments...)

	imp.IsFileImport = true
	imp.IsLocal = true
	imp.UpsCount = ups
	imp.Segments = segments
	imp.LocalFile = filepath.Join(append([]string{p.srcDir}, pathParts...)...) + ".zx"
	imp.Module = segments[len(segments)-1]

	p.parseImportModSelector(sp, imp, imp.LocalFile)
}

func (p *Parser) parseEnvImport(sp Span, imp *ImportDecl) {
	prefix := p.advance().Value

	var segments []string
	for p.at(TK_SLASH) && p.ok {
		sepSp := p.peek().Span
		p.advance()
		if !p.at(TK_IDENT) {
			errAt(sepSp, "expected a path segment after '/' in import",
				fmt.Sprintf("example: import %s/net/socket", prefix))
			p.ok = false
			return
		}
		segments = append(segments, p.advance().Value)
	}

	if len(segments) == 0 {
		errAt(sp, fmt.Sprintf("import %q requires at least one path segment", prefix),
			fmt.Sprintf("example: import %s/net/socket", prefix))
		p.ok = false
		return
	}

	var envVar string
	switch prefix {
	case "std":
		envVar = "ZENX_STD_PATH"
	case "usr":
		envVar = "ZENX_USR_PATH"
	default:
		envVar = "ZENX_" + strings.ToUpper(prefix) + "_PATH"
	}

	imp.IsFileImport = true
	imp.IsLocal = true
	imp.IsStd = (prefix == "std")
	imp.IsUser = (prefix != "std")
	imp.EnvPrefix = envVar
	imp.Segments = segments
	imp.Module = segments[len(segments)-1]
	imp.LocalFile = filepath.Join(append([]string{"$" + envVar}, segments...)...) + ".zx"

	p.parseImportModSelector(sp, imp, imp.LocalFile)
}

func (p *Parser) parseStdModuleImport(sp Span, imp *ImportDecl) {
	name := p.advance().Value
	for p.at(TK_DCOLON) {
		p.advance()
		next := p.peek()
		if next.Kind == TK_IDENT || (next.Kind >= TK_TYPE_INT && next.Kind <= TK_TYPE_ANY) {
			name += "::" + p.advance().Value
		} else {
			errAt(p.peek().Span, "expected a module name after '::'",
				"example: use std::str  or  use std::math")
			p.ok = false
			return
		}
	}

	if name == "std" || name == "usr" {
		errAt(sp, fmt.Sprintf("%q alone is not a valid module — add a submodule name", name),
			fmt.Sprintf("example: use %s::str  or  import %s/net/socket", name, name))
		p.ok = false
		return
	}

	imp.Module = name
	imp.IsStdModule = strings.HasPrefix(name, "std::")
	imp.IsStd = imp.IsStdModule

	if imp.IsStdModule && LookupStdModule(name) == nil {
		errAt(sp, fmt.Sprintf("unknown stdlib module %q", name),
			"available modules: std::str std::io std::math std::sys std::fs std::cmd std::mem std::conv std::time std::net")
		p.ok = false
		return
	}

	imp.ImportAll = true
}

func (p *Parser) parseImportModSelector(sp Span, imp *ImportDecl, filePath string) {
	if p.at(TK_LPAREN) {
		p.advance()

		if p.at(TK_RPAREN) {
			errAt(sp, "expected a mod name inside parentheses",
				fmt.Sprintf("example: import _/%s (ModName)", imp.Module))
			p.ok = false
			p.advance()
			return
		}

		if !p.at(TK_IDENT) {
			errAt(p.peek().Span,
				fmt.Sprintf("expected an identifier (mod name) inside parentheses, got %q", p.peek().Value),
				"the name in parentheses must match a mod block in the imported file")
			p.ok = false
			for !p.at(TK_RPAREN) && !p.at(TK_EOF) && !p.at(TK_SEMI) {
				p.advance()
			}
			if p.at(TK_RPAREN) {
				p.advance()
			}
			return
		}

		modName := p.advance().Value

		if len(modName) > 0 && modName[0] >= 'a' && modName[0] <= 'z' {
			warnAt(p.peek().Span,
				fmt.Sprintf("imported mod name %q starts with lowercase — mod blocks are usually PascalCase", modName),
				"example: import _/logger (Logger)")
		}

		if !p.at(TK_RPAREN) {
			errAt(p.peek().Span,
				fmt.Sprintf("expected ')' to close the mod selector, got %q", p.peek().Value),
				"only one mod name can be imported per import statement")
			p.ok = false
			for !p.at(TK_RPAREN) && !p.at(TK_EOF) && !p.at(TK_SEMI) {
				p.advance()
			}
			if p.at(TK_RPAREN) {
				p.advance()
			}
			return
		}
		p.advance()

		imp.Alias = modName
		imp.ImportAll = false
	} else {
		imp.ImportAll = true
		imp.Alias = ""
	}
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

func (p *Parser) parseMacroDecl() *MacroDecl {
	sp := p.peek().Span
	p.expect(TK_MACRO)
	if p.atAny(TK_FN, TK_SUB) {
		p.advance()
	}
	name := p.expect(TK_IDENT).Value
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
		p.advance()
		for !p.at(TK_PIPE) && !p.at(TK_EOF) && p.ok {
			psp := p.peek().Span
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
		if p.at(TK_ARROW) {
			p.advance()
			if p.at(TK_PIPE) {
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
		p.advance()
		params, _ = p.parseParamList()
		p.expect(TK_RPAREN)
		if p.at(TK_ARROW) {
			p.advance()
			retType = p.parseType()
		}
	}

	savedScope := p.macroScope
	p.macroScope = make(map[string]bool)
	for _, param := range params {
		p.macroScope[param.Name] = true
	}
	for _, inp := range inputs {
		p.macroScope[inp] = true
	}
	for _, out := range outputs {
		p.macroScope[out] = true
	}
	body := p.parseBlock()
	p.macroScope = savedScope

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

func (p *Parser) parseBangMacroCall() Node {
	sp := p.peek().Span
	t := p.advance()
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
	switch name {
	case "dbg", "panic", "unreachable", "todo", "env",
		"assert", "ok", "try", "log", "time",
		"type_of", "size_of",
		"max", "min", "abs", "clamp", "swap",
		"len", "str_eq", "str_ne", "str_contains", "str_starts", "str_ends",
		"str_to_int", "str_to_float", "int_to_str", "float_to_str",
		"print", "eprint", "read_line", "exit_ok", "exit_err",
		"alloc", "zalloc", "free", "memcpy", "memset",
		"is_nil", "not_nil", "cast", "count_of", "likely", "unlikely":
		return &BangMacroExpr{Sp: sp, Name: name, Args: args}
	default:
		return &MacroCallExpr{Sp: sp, Name: name, Args: args}
	}
}

func (p *Parser) isChainStepStart() bool {
	if !p.at(TK_IDENT) {
		return false
	}
	if p.peekN(1).Kind == TK_COLON {
		return true
	}
	if p.peekN(1).Kind == TK_LPAREN {
		depth := 0
		for i := 1; p.pos+i < len(p.tokens); i++ {
			k := p.tokens[p.pos+i].Kind
			if k == TK_LPAREN {
				depth++
			} else if k == TK_RPAREN {
				depth--
				if depth == 0 {
					if p.pos+i+1 < len(p.tokens) && p.tokens[p.pos+i+1].Kind == TK_COLON {
						return true
					}
					return false
				}
			} else if k == TK_EOF || k == TK_SEMI || k == TK_LBRACE {
				return false
			}
		}
	}
	return false
}

func (p *Parser) tryParseMacroChain(recv Node) Node {
	if !p.isChainStepStart() {
		return nil
	}
	sp := p.peek().Span
	chain := &MacroCallChain{Sp: sp, Recv: recv}

	for p.isChainStepStart() && p.ok {
		stepSp := p.peek().Span
		macroName := p.advance().Value

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

		p.expect(TK_COLON)

		if p.at(TK_DO) {
			p.advance()
		}

		if !p.at(TK_LBRACE) {
			errAt(p.peek().Span,
				fmt.Sprintf("expected a block '{ }' after '%s:'", macroName),
				fmt.Sprintf("write: %s: { /* your code */ }", macroName))
			p.ok = false
			break
		}
		body := p.parseBlock()

		var elseBody *Block
		if p.at(TK_ELSE) {
			p.advance()
			if p.at(TK_LBRACE) {
				elseBody = p.parseBlock()
			}
		}

		chain.Steps = append(chain.Steps, MacroChainStep{
			Sp:       stepSp,
			Macro:    macroName,
			Args:     args,
			Body:     body,
			ElseBody: elseBody,
		})
	}
	if len(chain.Steps) == 0 {
		return nil
	}
	return chain
}

func (p *Parser) parseFnDecl(anns []Annotation) *FnDecl {
	sp := p.peek().Span
	p.advance()
	name := p.expect(TK_IDENT)
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
		// ref StructName  or  ref int  etc.
		if p.isTypeStart() || p.at(TK_IDENT) {
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

	if p.at(TK_PRIV) {
		errAt(p.peek().Span,
			"'priv' cannot appear inside a function body — use it at top level or inside a mod block",
			"move the priv declaration to the top level or inside a mod block")
		p.ok = false
		return nil
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
	tok := p.advance()
	isGlobal := tok.Kind == TK_OUR
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
	return &VarDecl{Sp: sp, Name: name.Value, VarType: vt, Init: init, IsConst: isConst, IsGlobal: isGlobal}
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

	// Detect mod property set: if expr resolved to a ModPropGetExpr and next is assign op
	if mpg, ok := expr.(*ModPropGetExpr); ok {
		switch p.peek().Kind {
		case TK_ASSIGN, TK_PLUS_EQ, TK_MINUS_EQ, TK_STAR_EQ, TK_SLASH_EQ, TK_PERCENT_EQ:
			op := p.advance().Value
			val := p.parsePipeExpr()
			p.expectSemi()
			return &ModPropSetStmt{Sp: sp, Mod: mpg.Mod, Prop: mpg.Prop, Op: op, Value: val}
		}
	}

	switch p.peek().Kind {
	case TK_ASSIGN, TK_PLUS_EQ, TK_MINUS_EQ, TK_STAR_EQ, TK_SLASH_EQ, TK_PERCENT_EQ:
		op := p.advance().Value
		val := p.parsePipeExpr()
		p.expectSemi()
		return &AssignStmt{Sp: sp, LHS: expr, Op: op, Value: val}
	}
	if chain := p.tryParseMacroChain(expr); chain != nil {
		p.eatSemi()
		return &ExprStmt{Sp: sp, Expr: chain}
	}
	p.expectSemi()
	return &ExprStmt{Sp: sp, Expr: expr}
}

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

		case TK_DCOLON:
			// Handle Mod::prop access or Mod::fn() or nested Mod::Sub::fn()
			p.advance()
			if p.at(TK_IDENT) {
				seg := p.advance()
				recvName := ""
				if id, ok := expr.(*Ident); ok {
					recvName = id.Name
				}
				fullPath := recvName
				if fullPath != "" {
					fullPath = fullPath + "::" + seg.Value
				} else {
					fullPath = seg.Value
				}
				// consume more ::ident path segments (not the final fn call)
				for p.at(TK_DCOLON) && p.peekN(1).Kind == TK_IDENT {
					if p.peekN(2).Kind == TK_LPAREN {
						break
					}
					p.advance()
					nextSeg := p.advance()
					fullPath = fullPath + "::" + nextSeg.Value
				}
				// Mod::nested::fn(...)
				if p.at(TK_DCOLON) && p.peekN(1).Kind == TK_IDENT && p.peekN(2).Kind == TK_LPAREN {
					p.advance()
					fnTok := p.advance()
					p.advance()
					var args []Node
					for !p.at(TK_RPAREN) && !p.at(TK_EOF) && p.ok {
						args = append(args, p.parseExpr())
						if p.at(TK_COMMA) {
							p.advance()
						}
					}
					p.expect(TK_RPAREN)
					expr = &MethodCallExpr{Sp: sp,
						Recv:   &Ident{Sp: seg.Span, Name: fullPath},
						Method: fnTok.Value,
						Args:   args,
					}
				} else if p.at(TK_LPAREN) {
					// direct call: last segment is the fn name
					p.advance()
					var args []Node
					for !p.at(TK_RPAREN) && !p.at(TK_EOF) && p.ok {
						args = append(args, p.parseExpr())
						if p.at(TK_COMMA) {
							p.advance()
						}
					}
					p.expect(TK_RPAREN)
					lastSep := strings.LastIndex(fullPath, "::")
					if lastSep >= 0 {
						modPart := fullPath[:lastSep]
						fnPart := fullPath[lastSep+2:]
						expr = &MethodCallExpr{Sp: sp,
							Recv:   &Ident{Sp: seg.Span, Name: modPart},
							Method: fnPart,
							Args:   args,
						}
					} else {
						expr = &CallExpr{Sp: sp,
							Func: &Ident{Sp: seg.Span, Name: fullPath},
							Args: args,
						}
					}
				} else {
					// Mod::propName — mod property get
					// Split fullPath into mod + prop at the last ::
					lastSep := strings.LastIndex(fullPath, "::")
					if lastSep >= 0 {
						modPart := fullPath[:lastSep]
						propPart := fullPath[lastSep+2:]
						expr = &ModPropGetExpr{Sp: sp, Mod: modPart, Prop: propPart}
					} else {
						expr = &Ident{Sp: seg.Span, Name: fullPath}
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
	if p.isInMacroScope(t.Value) {
		p.advance()
		return &Ident{Sp: t.Span, Name: t.Value}
	}
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
	case TK_MULTILINE_STR:
		p.advance()
		return p.parseMultilineStr(t)
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
		if p.isInMacroScope(t.Value) {
			p.advance()
			return &Ident{Sp: t.Span, Name: t.Value}
		}
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

func (p *Parser) parseMultilineStr(tok Token) *MultilineStr {
	sp := tok.Span
	raw := tok.Value
	ms := &MultilineStr{Sp: sp, Typ: TypStr}
	i := 0
	for i < len(raw) {
		dIdx := strings.Index(raw[i:], "${")
		if dIdx < 0 {
			ms.Parts = append(ms.Parts, MlsPart{Text: raw[i:]})
			break
		}
		if dIdx > 0 {
			ms.Parts = append(ms.Parts, MlsPart{Text: raw[i : i+dIdx]})
		}
		i = i + dIdx + 2

		depth := 1
		j := i
		for j < len(raw) && depth > 0 {
			if raw[j] == '{' {
				depth++
			} else if raw[j] == '}' {
				depth--
			}
			if depth > 0 {
				j++
			}
		}
		if depth != 0 {
			errAt(sp, "multiline string: unclosed ${ in interpolation",
				"add a closing } for the ${ expression")
			p.ok = false
			break
		}
		inner := raw[i:j]
		i = j + 1

		subToks := Tokenize(inner, sp.File)
		if subToks == nil {
			continue
		}
		subProg := Parse(subToks, inner, sp.File)
		if subProg == nil {
			continue
		}
		if len(subProg.TopStmts) == 1 {
			if es, ok := subProg.TopStmts[0].(*ExprStmt); ok {
				ms.Parts = append(ms.Parts, MlsPart{IsExpr: true, Expr: es.Expr})
				continue
			}
		}
		ms.Parts = append(ms.Parts, MlsPart{IsStmt: true, Stmts: subProg.TopStmts})
	}
	return ms
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
