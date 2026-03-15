package main

import (
	"fmt"
	"strconv"
	"strings"
)

// ─────────────────────────────────────────────────────────────────────────────
//  Parser
// ─────────────────────────────────────────────────────────────────────────────

type Parser struct {
	tokens []Token
	pos    int
	file   string
	ok     bool
}

func Parse(tokens []Token, src, file string) *Program {
	p := &Parser{tokens: tokens, file: file, ok: true}
	prog := p.parseProgram()
	if !p.ok { return nil }
	return prog
}

func (p *Parser) peek() Token {
	if p.pos >= len(p.tokens) { return Token{Kind: TK_EOF} }
	return p.tokens[p.pos]
}
func (p *Parser) peekN(n int) Token {
	if p.pos+n >= len(p.tokens) { return Token{Kind: TK_EOF} }
	return p.tokens[p.pos+n]
}
func (p *Parser) at(k TK) bool { return p.peek().Kind == k }
func (p *Parser) atAny(ks ...TK) bool {
	k := p.peek().Kind
	for _, x := range ks { if k == x { return true } }
	return false
}
func (p *Parser) advance() Token {
	t := p.peek()
	if t.Kind != TK_EOF { p.pos++ }
	return t
}
func (p *Parser) expect(k TK) Token {
	t := p.peek()
	if t.Kind != k {
		got := t.Value; if got == "" { got = t.Kind.String() }
		errAt(t.Span, fmt.Sprintf("expected '%s', got '%s'", k, got), fmt.Sprintf("add %s here", k))
		p.ok = false
	}
	return p.advance()
}
func (p *Parser) expectSemi() {
	if p.at(TK_SEMI)  { p.advance(); return }
	if p.at(TK_RBRACE) || p.at(TK_EOF) { return }
	t := p.peek()
	errAt(t.Span, "missing ';' after statement", "add a semicolon ';' at the end of the statement")
	p.ok = false
}
func (p *Parser) eatSemi() { if p.at(TK_SEMI) { p.advance() } }

// isTypeStart returns true if current token starts a type
func (p *Parser) isTypeStart() bool {
	switch p.peek().Kind {
	case TK_TYPE_INT, TK_TYPE_FLOAT, TK_TYPE_BOOL, TK_TYPE_STR,
		TK_TYPE_VOID, TK_TYPE_CHAR, TK_TYPE_REF, TK_TYPE_ANY,
		TK_LBRACKET, TK_STAR:
		return true
	case TK_IDENT:
		// could be struct name used as type — check peek2 to avoid
		// misidentifying e.g. "x" as type when it's followed by "="
		next := p.peekN(1).Kind
		return next == TK_IDENT || next == TK_COMMA || next == TK_RPAREN ||
			next == TK_LBRACE || next == TK_SEMI || next == TK_RBRACE
	}
	return false
}

// ── Program ───────────────────────────────────────────────────────────────────

func (p *Parser) parseProgram() *Program {
	prog := &Program{}
	for !p.at(TK_EOF) && p.ok {
		switch p.peek().Kind {
		case TK_IMPORT, TK_USE:
			imp := p.parseImport()
			prog.Imports = append(prog.Imports, imp)
		case TK_EXTERN:
			prog.Externs = append(prog.Externs, p.parseExtern())
		case TK_STRUCT:
			prog.Structs = append(prog.Structs, p.parseStruct())
		case TK_TYPE:
			prog.Structs = append(prog.Structs, p.parseTypeStruct())
		case TK_FN, TK_SUB:
			if p.isFnMethod() {
				prog.Methods = append(prog.Methods, p.parseMethod())
			} else {
				prog.TopStmts = append(prog.TopStmts, p.parseFn())
			}
		default:
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

// ── Imports ───────────────────────────────────────────────────────────────────
// Supported forms:
//   import "stdio.h"
//   import "stdio.h" as c_stdio
//   use std::str
//   use std::math as math
//   use "mylib.h"

func (p *Parser) parseImport() *ImportDecl {
	sp := p.peek().Span
	p.advance() // import or use

	var imp ImportDecl
	imp.Sp = sp

	if p.at(TK_STRING) {
		// import "header.h"
		imp.Path = p.advance().Value
		imp.IsStd = false
		if p.at(TK_AS) { p.advance(); imp.Alias = p.expect(TK_IDENT).Value }
	} else if p.at(TK_IDENT) {
		// use std::str   or   use mymodule
		name := p.advance().Value
		for p.at(TK_DCOLON) {
			p.advance()
			sub := p.advance().Value
			name += "::" + sub
		}
		imp.Module = name
		imp.IsStd = strings.HasPrefix(name, "std::")
		if p.at(TK_AS) { p.advance(); imp.Alias = p.expect(TK_IDENT).Value }
	} else {
		errAt(sp, "expected a string path or module name after import/use",
			`use "stdio.h"  or  use std::str`)
		p.ok = false
	}
	p.eatSemi()
	return &imp
}

// extern fn name(params) -> ret
func (p *Parser) parseExtern() *ExternDecl {
	sp := p.peek().Span
	p.expect(TK_EXTERN)
	if p.atAny(TK_FN, TK_SUB) { p.advance() }
	name := p.expect(TK_IDENT)
	p.expect(TK_LPAREN)
	params, variadic := p.parseParamList()
	p.expect(TK_RPAREN)
	ret := TypVoid
	if p.at(TK_ARROW) { p.advance(); ret = p.parseType() }
	p.eatSemi()
	return &ExternDecl{Sp: sp, Name: name.Value, Params: params, Variadic: variadic, RetType: ret}
}

// struct Foo { x: int, y: float }
// struct Foo { x int; y float }
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

// type Player struct { ... }
func (p *Parser) parseTypeStruct() *StructDecl {
	sp := p.peek().Span
	p.expect(TK_TYPE)
	name := p.expect(TK_IDENT)
	if p.at(TK_STRUCT) { p.advance() }
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
		if p.at(TK_COLON) { p.advance(); ftype = p.parseType() } else if p.isTypeStart() { ftype = p.parseType() } else { ftype = TypAny }
		fields = append(fields, Param{Sp: fsp, Name: fname.Value, Type: ftype})
		if p.at(TK_COMMA) || p.at(TK_SEMI) { p.advance() }
	}
	return fields
}

// fn name(params) -> ret { body }
func (p *Parser) parseFn() *FnDecl {
	sp := p.peek().Span
	p.advance() // fn or sub
	name := p.expect(TK_IDENT)
	p.expect(TK_LPAREN)
	params, variadic := p.parseParamList()
	p.expect(TK_RPAREN)
	ret := TypVoid
	if p.at(TK_ARROW) { p.advance(); ret = p.parseType() }
	body := p.parseBlock()
	return &FnDecl{Sp: sp, Name: name.Value, Params: params, Variadic: variadic, RetType: ret, Body: body}
}

// fn (recv ref Type) MethodName(params) -> ret { body }
// fn (recv Type) MethodName(...)   ← value receiver
func (p *Parser) parseMethod() *MethodDecl {
	sp := p.peek().Span
	p.advance() // fn
	p.expect(TK_LPAREN)
	recvName := p.expect(TK_IDENT).Value
	recvRef := false
	// accept ref, *, or bare name
	if p.at(TK_TYPE_REF) || p.at(TK_STAR) { p.advance(); recvRef = true }
	recvType := p.expect(TK_IDENT).Value
	p.expect(TK_RPAREN)
	methodName := p.expect(TK_IDENT).Value
	p.expect(TK_LPAREN)
	params, variadic := p.parseParamList()
	p.expect(TK_RPAREN)
	ret := TypVoid
	if p.at(TK_ARROW) { p.advance(); ret = p.parseType() }
	body := p.parseBlock()
	return &MethodDecl{Sp: sp, RecvName: recvName, RecvType: recvType, RecvRef: recvRef,
		Name: methodName, Params: params, Variadic: variadic, RetType: ret, Body: body}
}

// parseParamList — optional types supported
func (p *Parser) parseParamList() ([]Param, bool) {
	var params []Param
	variadic := false
	for !p.at(TK_RPAREN) && !p.at(TK_EOF) && p.ok {
		if p.at(TK_ELLIPSIS) || p.at(TK_DOTDOT) { p.advance(); variadic = true; break }
		psp := p.peek().Span
		pname := p.expect(TK_IDENT)
		var ptype *ZXType
		if p.at(TK_COLON) { p.advance(); ptype = p.parseType() } else if p.isTypeStart() && !p.at(TK_COMMA) && !p.at(TK_RPAREN) { ptype = p.parseType() } else { ptype = TypAny }
		params = append(params, Param{Sp: psp, Name: pname.Value, Type: ptype})
		if p.at(TK_COMMA) { p.advance() }
	}
	return params, variadic
}

// ── Types ─────────────────────────────────────────────────────────────────────
// Supported:
//   int float bool str char void any
//   ref T       (friendly pointer)
//   *T          (raw pointer, also accepted)
//   [N]T        (array)
//   []T         (slice)
//   StructName

func (p *Parser) parseType() *ZXType {
	t := p.peek()
	// *T  (still accepted, just not encouraged)
	if t.Kind == TK_STAR {
		p.advance()
		inner := p.parseType()
		return RefOf(inner)
	}
	// ^T  deref in type position is invalid but handle gracefully
	if t.Kind == TK_LBRACKET {
		p.advance()
		size := 0
		if p.at(TK_INT) { n, _ := strconv.Atoi(p.peek().Value); size = n; p.advance() }
		p.expect(TK_RBRACKET)
		elem := p.parseType()
		if size == 0 { return SliceOf(elem) }
		return ArrayOf(elem, size)
	}
	switch t.Kind {
	case TK_TYPE_INT:   p.advance(); return TypInt
	case TK_TYPE_FLOAT: p.advance(); return TypFloat
	case TK_TYPE_BOOL:  p.advance(); return TypBool
	case TK_TYPE_STR:   p.advance(); return TypStr
	case TK_TYPE_CHAR:  p.advance(); return TypChar
	case TK_TYPE_VOID:  p.advance(); return TypVoid
	case TK_TYPE_ANY:   p.advance(); return TypAny
	case TK_TYPE_REF:
		p.advance()
		if p.at(TK_LT) {
			p.advance(); elem := p.parseType(); p.expect(TK_GT); return RefOf(elem)
		}
		if p.isTypeStart() { return RefOf(p.parseType()) }
		return RefOf(TypVoid)
	case TK_IDENT:
		p.advance()
		return StructType(t.Value)
	default:
		errAt(t.Span, fmt.Sprintf("expected a type, got %q", t.Value),
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
		if s := p.parseStmt(); s != nil { stmts = append(stmts, s) }
	}
	p.expect(TK_RBRACE)
	return &Block{Sp: sp, Stmts: stmts}
}

// ── Statements ────────────────────────────────────────────────────────────────

func (p *Parser) parseStmt() Node {
	t := p.peek()
	switch t.Kind {
	case TK_LET, TK_MY:        return p.parseVarDecl(false)
	case TK_CONST, TK_OUR:     return p.parseVarDecl(true)
	case TK_RETURN:             return p.parseReturn()
	case TK_IF:                 return p.parseIf()
	case TK_UNLESS:             return p.parseUnless()
	case TK_WHILE:              return p.parseWhile()
	case TK_UNTIL:              return p.parseUntil()
	case TK_FOR, TK_FOREACH:   return p.parseFor()
	case TK_DO:                 return p.parseDoBlock()
	case TK_BREAK, TK_LAST:    p.advance(); p.expectSemi(); return &BreakStmt{Sp: t.Span}
	case TK_CONTINUE, TK_NEXT: p.advance(); p.expectSemi(); return &ContinueStmt{Sp: t.Span}
	case TK_PRINT:              return p.parsePrint(false, false)
	case TK_PRINTLN, TK_SAY:   return p.parsePrint(true, false)
	case TK_WARN, TK_EPRINT:   return p.parsePrint(true, true)
	case TK_EXIT:               return p.parseExit()
	case TK_DIE:                return p.parseDie()
	case TK_FN, TK_SUB:
		if p.isFnMethod() {
			p.parseMethod(); return nil
		}
		return p.parseFn()
	case TK_SEMI: p.advance(); return nil
	default:      return p.parseExprOrAssign()
	}
}

func (p *Parser) parseVarDecl(isConst bool) *VarDecl {
	sp := p.peek().Span
	p.advance()
	name := p.expect(TK_IDENT)
	var vt *ZXType
	if p.at(TK_COLON) { p.advance(); vt = p.parseType() } else if p.isTypeStart() && !p.at(TK_ASSIGN) { vt = p.parseType() }
	var init Node
	if p.at(TK_ASSIGN) { p.advance(); init = p.parsePipeExpr() } else if isConst {
		errAt(name.Span, "const/our declaration requires an initializer", "add = <value> after the declaration")
		p.ok = false
	}
	p.expectSemi()
	return &VarDecl{Sp: sp, Name: name.Value, VarType: vt, Init: init, IsConst: isConst}
}

func (p *Parser) parseReturn() *ReturnStmt {
	sp := p.peek().Span; p.expect(TK_RETURN)
	if p.at(TK_SEMI) || p.at(TK_RBRACE) { p.eatSemi(); return &ReturnStmt{Sp: sp} }
	val := p.parsePipeExpr(); p.expectSemi()
	return &ReturnStmt{Sp: sp, Value: val}
}

func (p *Parser) parseIf() *IfStmt {
	sp := p.peek().Span; p.expect(TK_IF)
	cond := p.parseExpr()
	then := p.parseBlock()
	var elifs []ElifClause; var els *Block
	for p.atAny(TK_ELIF, TK_ELSE) && p.ok {
		if p.at(TK_ELIF) { p.advance(); ec := p.parseExpr(); eb := p.parseBlock(); elifs = append(elifs, ElifClause{Cond: ec, Body: eb}) } else { p.advance(); els = p.parseBlock(); break }
	}
	return &IfStmt{Sp: sp, Cond: cond, Then: then, Elifs: elifs, Else: els}
}

func (p *Parser) parseUnless() *UnlessStmt {
	sp := p.peek().Span; p.expect(TK_UNLESS)
	cond := p.parseExpr(); body := p.parseBlock()
	var els *Block
	if p.at(TK_ELSE) { p.advance(); els = p.parseBlock() }
	return &UnlessStmt{Sp: sp, Cond: cond, Body: body, Else: els}
}

func (p *Parser) parseWhile() *WhileStmt {
	sp := p.peek().Span; p.expect(TK_WHILE)
	cond := p.parseExpr(); body := p.parseBlock()
	return &WhileStmt{Sp: sp, Cond: cond, Body: body}
}

func (p *Parser) parseUntil() *UntilStmt {
	sp := p.peek().Span; p.expect(TK_UNTIL)
	cond := p.parseExpr(); body := p.parseBlock()
	return &UntilStmt{Sp: sp, Cond: cond, Body: body}
}

func (p *Parser) parseFor() Node {
	sp := p.peek().Span; p.advance()
	varName := p.expect(TK_IDENT)
	p.expect(TK_IN)
	from := p.parseExpr()
	p.expect(TK_DOTDOT)
	to := p.parseExpr()
	body := p.parseBlock()
	return &ForRangeStmt{Sp: sp, Var: varName.Value, From: from, To: to, Body: body}
}

func (p *Parser) parseDoBlock() Node {
	p.expect(TK_DO)
	b := p.parseBlock(); p.eatSemi(); return b
}

func (p *Parser) parsePrint(newline, toStderr bool) *PrintStmt {
	sp := p.peek().Span; p.advance()
	if !p.at(TK_LPAREN) {
		var args []Node
		for !p.at(TK_SEMI) && !p.at(TK_RBRACE) && !p.at(TK_EOF) && p.ok {
			args = append(args, p.parseExpr()); if p.at(TK_COMMA) { p.advance() }
		}
		p.expectSemi()
		return &PrintStmt{Sp: sp, Args: args, Newline: newline, ToStderr: toStderr}
	}
	p.expect(TK_LPAREN)
	var args []Node
	for !p.at(TK_RPAREN) && !p.at(TK_EOF) && p.ok {
		args = append(args, p.parseExpr()); if p.at(TK_COMMA) { p.advance() }
	}
	p.expect(TK_RPAREN); p.expectSemi()
	return &PrintStmt{Sp: sp, Args: args, Newline: newline, ToStderr: toStderr}
}

func (p *Parser) parseExit() *ExitStmt {
	sp := p.peek().Span; p.expect(TK_EXIT)
	if p.at(TK_LPAREN) { p.advance(); code := p.parseExpr(); p.expect(TK_RPAREN); p.expectSemi(); return &ExitStmt{Sp: sp, Code: code} }
	code := p.parseExpr(); p.expectSemi(); return &ExitStmt{Sp: sp, Code: code}
}

func (p *Parser) parseDie() Node {
	sp := p.peek().Span; p.expect(TK_DIE)
	var msg Node
	if !p.at(TK_SEMI) && !p.at(TK_RBRACE) && !p.at(TK_EOF) { msg = p.parseExpr() } else { msg = &StrLit{Sp: sp, Val: "died"} }
	p.expectSemi()
	msgPrint := &PrintStmt{Sp: sp, Args: []Node{msg}, Newline: true, ToStderr: true}
	return &Block{Sp: sp, Stmts: []Node{msgPrint, &ExitStmt{Sp: sp, Code: &IntLit{Sp: sp, Val: 1}}}}
}

func (p *Parser) parseExprOrAssign() Node {
	sp := p.peek().Span
	expr := p.parsePipeExpr()
	switch p.peek().Kind {
	case TK_ASSIGN, TK_PLUS_EQ, TK_MINUS_EQ, TK_STAR_EQ, TK_SLASH_EQ, TK_PERCENT_EQ:
		op := p.advance().Value; val := p.parsePipeExpr(); p.expectSemi()
		return &AssignStmt{Sp: sp, LHS: expr, Op: op, Value: val}
	}
	p.expectSemi()
	return &ExprStmt{Sp: sp, Expr: expr}
}

// ── Pipe expression: expr |> fn |> fn2 ────────────────────────────────────────

func (p *Parser) parsePipeExpr() Node {
	lhs := p.parseExpr()
	if !p.at(TK_PIPE_ARROW) { return lhs }
	sp := p.peek().Span
	steps := []Node{lhs}
	for p.at(TK_PIPE_ARROW) && p.ok {
		p.advance()
		// right side: can be a function name or a call expression
		step := p.parseUnary()
		steps = append(steps, step)
	}
	return &PipeExpr{Sp: sp, Steps: steps}
}

// ── Expression hierarchy ──────────────────────────────────────────────────────

func (p *Parser) parseExpr() Node { return p.parseOr() }

func (p *Parser) parseOr() Node {
	lhs := p.parseAnd()
	for p.at(TK_OR) && p.ok { sp := p.peek().Span; p.advance(); lhs = &BinExpr{Sp: sp, Op: "||", LHS: lhs, RHS: p.parseAnd()} }
	return lhs
}
func (p *Parser) parseAnd() Node {
	lhs := p.parseBitOr()
	for p.at(TK_AND) && p.ok { sp := p.peek().Span; p.advance(); lhs = &BinExpr{Sp: sp, Op: "&&", LHS: lhs, RHS: p.parseBitOr()} }
	return lhs
}
func (p *Parser) parseBitOr() Node {
	lhs := p.parseBitXor()
	for p.at(TK_PIPE) && p.ok { sp := p.peek().Span; p.advance(); lhs = &BinExpr{Sp: sp, Op: "|", LHS: lhs, RHS: p.parseBitXor()} }
	return lhs
}
func (p *Parser) parseBitXor() Node {
	lhs := p.parseBitAnd()
	for p.at(TK_CARET) && p.ok { sp := p.peek().Span; p.advance(); lhs = &BinExpr{Sp: sp, Op: "^", LHS: lhs, RHS: p.parseBitAnd()} }
	return lhs
}
func (p *Parser) parseBitAnd() Node {
	lhs := p.parseEquality()
	for p.at(TK_AMP) && p.ok { sp := p.peek().Span; p.advance(); lhs = &BinExpr{Sp: sp, Op: "&", LHS: lhs, RHS: p.parseEquality()} }
	return lhs
}
func (p *Parser) parseEquality() Node {
	lhs := p.parseRelational()
	for p.atAny(TK_EQ, TK_NEQ) && p.ok { sp := p.peek().Span; op := p.advance().Value; lhs = &BinExpr{Sp: sp, Op: op, LHS: lhs, RHS: p.parseRelational()} }
	return lhs
}
func (p *Parser) parseRelational() Node {
	lhs := p.parseShift()
	for p.atAny(TK_LT, TK_GT, TK_LTE, TK_GTE) && p.ok { sp := p.peek().Span; op := p.advance().Value; lhs = &BinExpr{Sp: sp, Op: op, LHS: lhs, RHS: p.parseShift()} }
	return lhs
}
func (p *Parser) parseShift() Node {
	lhs := p.parseAddSub()
	for p.atAny(TK_LSHIFT, TK_RSHIFT) && p.ok { sp := p.peek().Span; op := p.advance().Value; lhs = &BinExpr{Sp: sp, Op: op, LHS: lhs, RHS: p.parseAddSub()} }
	return lhs
}
func (p *Parser) parseAddSub() Node {
	lhs := p.parseMulDiv()
	for p.atAny(TK_PLUS, TK_MINUS) && p.ok { sp := p.peek().Span; op := p.advance().Value; lhs = &BinExpr{Sp: sp, Op: op, LHS: lhs, RHS: p.parseMulDiv()} }
	return lhs
}
func (p *Parser) parseMulDiv() Node {
	lhs := p.parseUnary()
	for p.atAny(TK_STAR, TK_SLASH, TK_PERCENT) && p.ok { sp := p.peek().Span; op := p.advance().Value; lhs = &BinExpr{Sp: sp, Op: op, LHS: lhs, RHS: p.parseUnary()} }
	return lhs
}

func (p *Parser) parseUnary() Node {
	sp := p.peek().Span
	switch p.peek().Kind {
	case TK_NOT:   p.advance(); return &UnaryExpr{Sp: sp, Op: "!", Operand: p.parseUnary()}
	case TK_MINUS: p.advance(); return &UnaryExpr{Sp: sp, Op: "-", Operand: p.parseUnary()}
	case TK_TILDE: p.advance(); return &UnaryExpr{Sp: sp, Op: "~", Operand: p.parseUnary()}
	case TK_AMP:   p.advance()
		// &Foo{} heap struct
		if p.at(TK_IDENT) && p.peekN(1).Kind == TK_LBRACE { return p.parseHeapStructInit() }
		return &AddrExpr{Sp: sp, Operand: p.parseUnary(), Deref: false}
	case TK_AT:    p.advance()
		// @Foo{} heap struct (friendly syntax)
		if p.at(TK_IDENT) && p.peekN(1).Kind == TK_LBRACE { return p.parseHeapStructInit() }
		return &AddrExpr{Sp: sp, Operand: p.parseUnary(), Deref: false}
	case TK_STAR:  p.advance(); return &AddrExpr{Sp: sp, Operand: p.parseUnary(), Deref: true}
	case TK_HAT:   p.advance(); return &AddrExpr{Sp: sp, Operand: p.parseUnary(), Deref: true}  // ^ = deref
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
			for !p.at(TK_RPAREN) && !p.at(TK_EOF) && p.ok { args = append(args, p.parseExpr()); if p.at(TK_COMMA) { p.advance() } }
			p.expect(TK_RPAREN)
			expr = &CallExpr{Sp: sp, Func: expr, Args: args}
		case TK_LBRACKET:
			p.advance(); idx := p.parseExpr(); p.expect(TK_RBRACKET)
			expr = &IndexExpr{Sp: sp, Obj: expr, Idx: idx}
		case TK_DOT:
			p.advance()
			field := p.expect(TK_IDENT)
			if p.at(TK_LPAREN) {
				p.advance()
				var args []Node
				for !p.at(TK_RPAREN) && !p.at(TK_EOF) && p.ok { args = append(args, p.parseExpr()); if p.at(TK_COMMA) { p.advance() } }
				p.expect(TK_RPAREN)
				expr = &MethodCallExpr{Sp: sp, Recv: expr, Method: field.Value, Args: args}
			} else {
				// .field on a ref type — mark as UsedDot so we can warn
				expr = &FieldExpr{Sp: sp, Obj: expr, Field: field.Value, UsedDot: true}
			}
		case TK_ARROW:
			p.advance()
			if p.at(TK_IDENT) {
				field := p.advance()
				if p.at(TK_LPAREN) {
					p.advance()
					var args []Node
					for !p.at(TK_RPAREN) && !p.at(TK_EOF) && p.ok { args = append(args, p.parseExpr()); if p.at(TK_COMMA) { p.advance() } }
					p.expect(TK_RPAREN)
					expr = &MethodCallExpr{Sp: sp, Recv: expr, Method: field.Value, Args: args}
				} else {
					expr = &FieldExpr{Sp: sp, Obj: &AddrExpr{Sp: sp, Operand: expr, Deref: true}, Field: field.Value}
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
		p.advance(); v, _ := strconv.ParseInt(t.Value, 0, 64); return &IntLit{Sp: t.Span, Val: v}
	case TK_FLOAT:
		p.advance(); v, _ := strconv.ParseFloat(t.Value, 64); return &FloatLit{Sp: t.Span, Val: v}
	case TK_BOOL:
		p.advance(); return &BoolLit{Sp: t.Span, Val: t.Value == "true"}
	case TK_STRING:
		p.advance(); return &StrLit{Sp: t.Span, Val: t.Value}
	case TK_NIL:
		p.advance(); return &NilLit{Sp: t.Span}
	case TK_SIZEOF:
		return p.parseSizeof()
	case TK_NEW:
		return p.parseNew()
	case TK_LBRACKET:
		return p.parseArrayLit()
	case TK_LPAREN:
		p.advance(); inner := p.parseExpr(); p.expect(TK_RPAREN); return inner
	// cast: int(expr)  float(expr) etc.
	case TK_TYPE_INT, TK_TYPE_FLOAT, TK_TYPE_BOOL, TK_TYPE_CHAR, TK_TYPE_STR:
		typTok := p.advance(); toType := tokenToType(typTok)
		p.expect(TK_LPAREN); operand := p.parseExpr(); p.expect(TK_RPAREN)
		return &CastExpr{Sp: t.Span, ToType: toType, Operand: operand}
	// len(x) / push / pop — parsed as builtins
	case TK_LEN, TK_PUSH, TK_POP:
		name := p.advance().Value
		p.expect(TK_LPAREN)
		var args []Node
		for !p.at(TK_RPAREN) && !p.at(TK_EOF) && p.ok { args = append(args, p.parseExpr()); if p.at(TK_COMMA) { p.advance() } }
		p.expect(TK_RPAREN)
		return &BuiltinExpr{Sp: t.Span, Name: name, Args: args}
	case TK_IDENT:
		p.advance()
		return &Ident{Sp: t.Span, Name: t.Value}
	default:
		errAt(t.Span, fmt.Sprintf("unexpected token '%s' in expression", t.Value),
			"check for missing operator, unmatched parenthesis, or typo")
		p.ok = false; return &NilLit{Sp: t.Span}
	}
}

func (p *Parser) parseSizeof() Node {
	sp := p.peek().Span; p.expect(TK_SIZEOF)
	p.expect(TK_LPAREN); typ := p.parseType(); p.expect(TK_RPAREN)
	return &SizeofExpr{Sp: sp, Of: typ, Typ: TypInt}
}

func (p *Parser) parseNew() Node {
	sp := p.peek().Span; p.expect(TK_NEW)
	name := p.expect(TK_IDENT)
	p.expect(TK_LBRACE); fields := p.parseFieldInits(); p.expect(TK_RBRACE)
	return &StructInit{Sp: sp, Name: name.Value, Fields: fields, HeapAlloc: false}
}

func (p *Parser) parseHeapStructInit() Node {
	sp := p.peek().Span; name := p.advance().Value
	p.expect(TK_LBRACE); fields := p.parseFieldInits(); p.expect(TK_RBRACE)
	return &StructInit{Sp: sp, Name: name, Fields: fields, HeapAlloc: true}
}

func (p *Parser) parseFieldInits() []FieldInit {
	var fields []FieldInit
	for !p.at(TK_RBRACE) && !p.at(TK_EOF) && p.ok {
		fsp := p.peek().Span; fname := p.expect(TK_IDENT)
		if p.at(TK_COLON) || p.at(TK_FAT_ARROW) { p.advance() }
		fval := p.parseExpr()
		fields = append(fields, FieldInit{Sp: fsp, Name: fname.Value, Value: fval})
		if p.at(TK_COMMA) || p.at(TK_SEMI) { p.advance() }
	}
	return fields
}

func (p *Parser) parseArrayLit() Node {
	sp := p.peek().Span; p.expect(TK_LBRACKET)
	var elems []Node
	for !p.at(TK_RBRACKET) && !p.at(TK_EOF) && p.ok { elems = append(elems, p.parseExpr()); if p.at(TK_COMMA) { p.advance() } }
	p.expect(TK_RBRACKET)
	return &ArrayLit{Sp: sp, Elems: elems}
}

func tokenToType(t Token) *ZXType {
	switch t.Kind {
	case TK_TYPE_INT:   return TypInt
	case TK_TYPE_FLOAT: return TypFloat
	case TK_TYPE_BOOL:  return TypBool
	case TK_TYPE_CHAR:  return TypChar
	case TK_TYPE_STR:   return TypStr
	default:            return TypUnknown
	}
}

// suppress unused import warning for strings
var _ = strings.Join
