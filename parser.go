package main

import (
	"fmt"
	"strconv"
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
		return t
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
		"add a semicolon ';' at the end of the statement")
	p.ok = false
}
func (p *Parser) eatSemi() {
	if p.at(TK_SEMI) {
		p.advance()
	}
}

// isTypeStart returns true if the current token can begin a type annotation
func (p *Parser) isTypeStart() bool {
	switch p.peek().Kind {
	case TK_TYPE_INT, TK_TYPE_FLOAT, TK_TYPE_BOOL, TK_TYPE_STR,
		TK_TYPE_VOID, TK_TYPE_CHAR, TK_TYPE_PTR, TK_TYPE_ANY,
		TK_LBRACKET, TK_STAR:
		return true
	case TK_IDENT:
		return true // could be a struct name
	}
	return false
}

// ── program ───────────────────────────────────────────────────────────────────

func (p *Parser) parseProgram() *Program {
	prog := &Program{}
	for !p.at(TK_EOF) && p.ok {
		switch p.peek().Kind {
		case TK_IMPORT, TK_USE:
			prog.Imports = append(prog.Imports, p.parseImport())
		case TK_EXTERN:
			prog.Externs = append(prog.Externs, p.parseExtern())
		case TK_STRUCT:
			prog.Structs = append(prog.Structs, p.parseStruct())
		case TK_TYPE:
			// type Name struct { ... }
			prog.Structs = append(prog.Structs, p.parseTypeStruct())
		case TK_FN, TK_SUB:
			// check for method: fn (recv *Type) Name(...)
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

// isFnMethod checks if we have fn (recv Type) ...
func (p *Parser) isFnMethod() bool {
	// fn ( ...
	return p.pos+1 < len(p.tokens) && p.tokens[p.pos+1].Kind == TK_LPAREN
}

// import "stdio.h"  /  use "stdio.h"
func (p *Parser) parseImport() *ImportDecl {
	sp := p.peek().Span
	p.advance() // consume import or use
	path := p.expect(TK_STRING)
	alias := ""
	if p.at(TK_AS) {
		p.advance()
		alias = p.expect(TK_IDENT).Value
	}
	p.eatSemi()
	return &ImportDecl{Sp: sp, Path: path.Value, Alias: alias}
}

// extern fn name(params) -> ret
func (p *Parser) parseExtern() *ExternDecl {
	sp := p.peek().Span
	p.expect(TK_EXTERN)
	p.advance() // consume fn/sub
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

// struct Foo { x: int, y: float }
// struct Foo { x int; y float }     ← Go-style (no colon, semicolons)
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

// type Player struct { name string; ... }
func (p *Parser) parseTypeStruct() *StructDecl {
	sp := p.peek().Span
	p.expect(TK_TYPE)
	name := p.expect(TK_IDENT)
	// must be followed by struct
	if p.at(TK_STRUCT) {
		p.advance()
	}
	p.expect(TK_LBRACE)
	fields := p.parseStructFields()
	p.expect(TK_RBRACE)
	p.eatSemi()
	return &StructDecl{Sp: sp, Name: name.Value, Fields: fields}
}

// parseStructFields handles both styles:
//   name: type ,     ← ZX style
//   name  type ;     ← Go style
//   name  type \n    ← Go style (newline as separator)
func (p *Parser) parseStructFields() []Param {
	var fields []Param
	for !p.at(TK_RBRACE) && !p.at(TK_EOF) && p.ok {
		fsp := p.peek().Span
		fname := p.expect(TK_IDENT)
		var ftype *ZXType
		if p.at(TK_COLON) {
			p.advance() // ZX style: name: type
			ftype = p.parseType()
		} else if p.isTypeStart() {
			ftype = p.parseType() // Go style: name type
		} else {
			ftype = TypAny
		}
		fields = append(fields, Param{Sp: fsp, Name: fname.Value, Type: ftype})
		// separator: comma, semicolon, or just continue
		if p.at(TK_COMMA) || p.at(TK_SEMI) {
			p.advance()
		}
	}
	return fields
}

// fn Name(params) -> ret { body }
// sub Name(params) { body }
func (p *Parser) parseFn() *FnDecl {
	sp := p.peek().Span
	p.advance() // consume fn or sub
	name := p.expect(TK_IDENT)
	p.expect(TK_LPAREN)
	params, variadic := p.parseParamList()
	p.expect(TK_RPAREN)
	ret := TypVoid
	if p.at(TK_ARROW) {
		p.advance()
		ret = p.parseType()
	}
	body := p.parseBlock()
	return &FnDecl{Sp: sp, Name: name.Value, Params: params, Variadic: variadic, RetType: ret, Body: body}
}

// fn (recv *Type) MethodName(params) -> ret { body }
func (p *Parser) parseMethod() *MethodDecl {
	sp := p.peek().Span
	p.advance() // fn or sub
	p.expect(TK_LPAREN)
	recvName := p.expect(TK_IDENT).Value
	recvPtr := false
	if p.at(TK_STAR) {
		p.advance()
		recvPtr = true
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
	return &MethodDecl{
		Sp: sp, RecvName: recvName, RecvType: recvType, RecvPtr: recvPtr,
		Name: methodName, Params: params, Variadic: variadic, RetType: ret, Body: body,
	}
}

// parseParamList: optional types, variadic, both ZX and Go style
// ZX:  name: type, name: type
// Go:  name type, name type
// no-type:  name, name, name
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
		// handle *Type receiver-style or typed param
		pname := p.expect(TK_IDENT)
		var ptype *ZXType
		if p.at(TK_COLON) {
			// ZX style: name: type
			p.advance()
			ptype = p.parseType()
		} else if p.isTypeStart() && !p.at(TK_COMMA) && !p.at(TK_RPAREN) {
			// Go style: name type
			ptype = p.parseType()
		} else {
			// no type — untyped / any
			ptype = TypAny
		}
		params = append(params, Param{Sp: psp, Name: pname.Value, Type: ptype})
		if p.at(TK_COMMA) {
			p.advance()
		}
	}
	return params, variadic
}

// ── types ─────────────────────────────────────────────────────────────────────

func (p *Parser) parseType() *ZXType {
	t := p.peek()

	// *Type  pointer (Go style)
	if t.Kind == TK_STAR {
		p.advance()
		inner := p.parseType()
		return PtrOf(inner)
	}
	// [N]T or []T  array
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
	case TK_TYPE_PTR:
		p.advance()
		if p.at(TK_LT) {
			p.advance()
			elem := p.parseType()
			p.expect(TK_GT)
			return PtrOf(elem)
		}
		return PtrOf(TypVoid)
	case TK_IDENT:
		p.advance()
		return StructType(t.Value)
	default:
		errAt(t.Span,
			fmt.Sprintf("expected a type, got %q", t.Value),
			"valid types: int, float, bool, str, char, void, ptr<T>, *T, [N]T, or a struct name")
		p.ok = false
		return TypUnknown
	}
}

// ── block ─────────────────────────────────────────────────────────────────────

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

// ── statements ────────────────────────────────────────────────────────────────

func (p *Parser) parseStmt() Node {
	t := p.peek()
	switch t.Kind {
	case TK_LET, TK_MY, TK_LOCAL:
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
		return p.parseDoBlock()
	case TK_BREAK, TK_LAST:
		p.advance(); p.expectSemi(); return &BreakStmt{Sp: t.Span}
	case TK_CONTINUE, TK_NEXT:
		p.advance(); p.expectSemi(); return &ContinueStmt{Sp: t.Span}
	case TK_PRINT:
		return p.parsePrint(false)
	case TK_PRINTLN, TK_SAY:
		return p.parsePrint(true)
	case TK_EXIT:
		return p.parseExit()
	case TK_DIE:
		return p.parseDie()
	case TK_FN, TK_SUB:
		if p.isFnMethod() {
			// inline method definition inside a block — unusual but support it
			md := p.parseMethod()
			return &ExprStmt{Sp: md.Sp, Expr: &NilLit{Sp: md.Sp}} // just ignore, methods handled globally
		}
		return p.parseFn()
	case TK_TYPE:
		// type X struct { } inside a function — treat as no-op statement (parsed at top level)
		p.parseTypeStruct()
		return nil
	case TK_STRUCT:
		p.parseStruct()
		return nil
	case TK_SEMI:
		p.advance(); return nil
	default:
		return p.parseExprOrAssign()
	}
}

const TK_LOCAL = TK_MY // alias

// let / my / our / const
func (p *Parser) parseVarDecl(isConst bool) *VarDecl {
	sp := p.peek().Span
	p.advance() // consume keyword
	name := p.expect(TK_IDENT)

	var vt *ZXType
	// optional type annotation
	if p.at(TK_COLON) {
		p.advance()
		vt = p.parseType()
	} else if p.isTypeStart() && !p.at(TK_ASSIGN) {
		// Go-style: let x int = 5
		vt = p.parseType()
	}

	var init Node
	if p.at(TK_ASSIGN) {
		p.advance()
		init = p.parseExpr()
	} else if isConst {
		errAt(name.Span, "const declaration requires an initializer",
			"add = <value> after the declaration")
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
	val := p.parseExpr()
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

// unless cond { } else { }
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
	cond := p.parseExpr()
	body := p.parseBlock()
	return &WhileStmt{Sp: sp, Cond: cond, Body: body}
}

// until cond { } → while !cond { }
func (p *Parser) parseUntil() *UntilStmt {
	sp := p.peek().Span
	p.expect(TK_UNTIL)
	cond := p.parseExpr()
	body := p.parseBlock()
	return &UntilStmt{Sp: sp, Cond: cond, Body: body}
}

// for i in 0..10 { }
// foreach item in list { }
func (p *Parser) parseFor() Node {
	sp := p.peek().Span
	p.advance() // for or foreach
	varName := p.expect(TK_IDENT)
	p.expect(TK_IN)
	from := p.parseExpr()
	p.expect(TK_DOTDOT)
	to := p.parseExpr()
	body := p.parseBlock()
	return &ForRangeStmt{Sp: sp, Var: varName.Value, From: from, To: to, Body: body}
}

// do { } — anonymous block
func (p *Parser) parseDoBlock() Node {
	sp := p.peek().Span
	p.expect(TK_DO)
	b := p.parseBlock()
	p.eatSemi()
	return b
	_ = sp
}

// print(args) / println(args) / say(args)
func (p *Parser) parsePrint(newline bool) *PrintStmt {
	sp := p.peek().Span
	p.advance()
	// support: println "foo"  (no parens, Perl style)
	if !p.at(TK_LPAREN) {
		// Perl-style: say "hello";
		var args []Node
		for !p.at(TK_SEMI) && !p.at(TK_RBRACE) && !p.at(TK_EOF) && p.ok {
			args = append(args, p.parseExpr())
			if p.at(TK_COMMA) {
				p.advance()
			}
		}
		p.expectSemi()
		return &PrintStmt{Sp: sp, Args: args, Newline: newline}
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
	return &PrintStmt{Sp: sp, Args: args, Newline: newline}
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
	// exit 0;
	code := p.parseExpr()
	p.expectSemi()
	return &ExitStmt{Sp: sp, Code: code}
}

// die "message";  → fprintf(stderr, msg); exit(1);
func (p *Parser) parseDie() Node {
	sp := p.peek().Span
	p.expect(TK_DIE)
	var msg Node
	if !p.at(TK_SEMI) && !p.at(TK_RBRACE) && !p.at(TK_EOF) {
		msg = p.parseExpr()
	} else {
		msg = &StrLit{Sp: sp, Val: "died"}
	}
	p.expectSemi()
	// Emit as: { fprintf(stderr, "%s\n", msg); exit(1); }
	msgPrint := &PrintStmt{Sp: sp, Args: []Node{msg}, Newline: true, FmtStr: "stderr"}
	exitOne := &ExitStmt{Sp: sp, Code: &IntLit{Sp: sp, Val: 1}}
	return &Block{Sp: sp, Stmts: []Node{msgPrint, exitOne}}
}

func (p *Parser) parseExprOrAssign() Node {
	sp := p.peek().Span
	expr := p.parseExpr()
	switch p.peek().Kind {
	case TK_ASSIGN, TK_PLUS_EQ, TK_MINUS_EQ, TK_STAR_EQ, TK_SLASH_EQ, TK_PERCENT_EQ:
		op := p.advance().Value
		val := p.parseExpr()
		p.expectSemi()
		return &AssignStmt{Sp: sp, LHS: expr, Op: op, Value: val}
	}
	p.expectSemi()
	return &ExprStmt{Sp: sp, Expr: expr}
}

// ── expressions ───────────────────────────────────────────────────────────────

func (p *Parser) parseExpr() Node { return p.parseOr() }

func (p *Parser) parseOr() Node {
	lhs := p.parseAnd()
	for p.at(TK_OR) && p.ok {
		sp := p.peek().Span; p.advance()
		lhs = &BinExpr{Sp: sp, Op: "||", LHS: lhs, RHS: p.parseAnd()}
	}
	return lhs
}
func (p *Parser) parseAnd() Node {
	lhs := p.parseBitOr()
	for p.at(TK_AND) && p.ok {
		sp := p.peek().Span; p.advance()
		lhs = &BinExpr{Sp: sp, Op: "&&", LHS: lhs, RHS: p.parseBitOr()}
	}
	return lhs
}
func (p *Parser) parseBitOr() Node {
	lhs := p.parseBitXor()
	for p.at(TK_PIPE) && p.ok {
		sp := p.peek().Span; p.advance()
		lhs = &BinExpr{Sp: sp, Op: "|", LHS: lhs, RHS: p.parseBitXor()}
	}
	return lhs
}
func (p *Parser) parseBitXor() Node {
	lhs := p.parseBitAnd()
	for p.at(TK_CARET) && p.ok {
		sp := p.peek().Span; p.advance()
		lhs = &BinExpr{Sp: sp, Op: "^", LHS: lhs, RHS: p.parseBitAnd()}
	}
	return lhs
}
func (p *Parser) parseBitAnd() Node {
	lhs := p.parseEquality()
	for p.at(TK_AMP) && p.ok {
		sp := p.peek().Span; p.advance()
		lhs = &BinExpr{Sp: sp, Op: "&", LHS: lhs, RHS: p.parseEquality()}
	}
	return lhs
}
func (p *Parser) parseEquality() Node {
	lhs := p.parseRelational()
	for p.atAny(TK_EQ, TK_NEQ) && p.ok {
		sp := p.peek().Span; op := p.advance().Value
		lhs = &BinExpr{Sp: sp, Op: op, LHS: lhs, RHS: p.parseRelational()}
	}
	return lhs
}
func (p *Parser) parseRelational() Node {
	lhs := p.parseShift()
	for p.atAny(TK_LT, TK_GT, TK_LTE, TK_GTE) && p.ok {
		sp := p.peek().Span; op := p.advance().Value
		lhs = &BinExpr{Sp: sp, Op: op, LHS: lhs, RHS: p.parseShift()}
	}
	return lhs
}
func (p *Parser) parseShift() Node {
	lhs := p.parseAddSub()
	for p.atAny(TK_LSHIFT, TK_RSHIFT) && p.ok {
		sp := p.peek().Span; op := p.advance().Value
		lhs = &BinExpr{Sp: sp, Op: op, LHS: lhs, RHS: p.parseAddSub()}
	}
	return lhs
}
func (p *Parser) parseAddSub() Node {
	lhs := p.parseMulDiv()
	for p.atAny(TK_PLUS, TK_MINUS) && p.ok {
		sp := p.peek().Span; op := p.advance().Value
		lhs = &BinExpr{Sp: sp, Op: op, LHS: lhs, RHS: p.parseMulDiv()}
	}
	return lhs
}
func (p *Parser) parseMulDiv() Node {
	lhs := p.parseUnary()
	for p.atAny(TK_STAR, TK_SLASH, TK_PERCENT) && p.ok {
		sp := p.peek().Span; op := p.advance().Value
		lhs = &BinExpr{Sp: sp, Op: op, LHS: lhs, RHS: p.parseUnary()}
	}
	return lhs
}

func (p *Parser) parseUnary() Node {
	sp := p.peek().Span
	switch p.peek().Kind {
	case TK_NOT:   p.advance(); return &UnaryExpr{Sp: sp, Op: "!", Operand: p.parseUnary()}
	case TK_MINUS: p.advance(); return &UnaryExpr{Sp: sp, Op: "-", Operand: p.parseUnary()}
	case TK_TILDE: p.advance(); return &UnaryExpr{Sp: sp, Op: "~", Operand: p.parseUnary()}
	case TK_AMP:
		p.advance()
		// &StructName{ ... }  heap-allocated struct init
		if p.at(TK_IDENT) && p.peekN(1).Kind == TK_LBRACE {
			return p.parseHeapStructInit()
		}
		return &AddrExpr{Sp: sp, Operand: p.parseUnary(), Deref: false}
	case TK_STAR:
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
			// check if method call: expr.Method(args)
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
				expr = &FieldExpr{Sp: sp, Obj: expr, Field: field.Value}
			}
		case TK_ARROW:
			p.advance()
			if p.at(TK_IDENT) {
				field := p.advance()
				if p.at(TK_LPAREN) {
					// ptr->Method(args) method call
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
					// ptr->field access
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
	case TK_NIL:
		p.advance()
		return &NilLit{Sp: t.Span}
	case TK_SIZEOF:
		return p.parseSizeof()
	case TK_NEW:
		return p.parseNew()
	case TK_LBRACKET:
		return p.parseArrayLit()
	case TK_LPAREN:
		p.advance()
		inner := p.parseExpr()
		p.expect(TK_RPAREN)
		return inner
	// cast: int(expr)  float(expr)  etc.
	case TK_TYPE_INT, TK_TYPE_FLOAT, TK_TYPE_BOOL, TK_TYPE_CHAR, TK_TYPE_STR:
		typTok := p.advance()
		toType := tokenToType(typTok)
		p.expect(TK_LPAREN)
		operand := p.parseExpr()
		p.expect(TK_RPAREN)
		return &CastExpr{Sp: t.Span, ToType: toType, Operand: operand}
	case TK_IDENT:
		p.advance()
		return &Ident{Sp: t.Span, Name: t.Value}
	default:
		errAt(t.Span,
			fmt.Sprintf("unexpected token '%s' in expression", t.Value),
			"check for missing operator, unmatched parenthesis, or typo")
		p.ok = false
		return &NilLit{Sp: t.Span}
	}
}

func (p *Parser) parseSizeof() Node {
	sp := p.peek().Span
	p.expect(TK_SIZEOF)
	p.expect(TK_LPAREN)
	typ := p.parseType()
	p.expect(TK_RPAREN)
	return &SizeofExpr{Sp: sp, Of: typ, Typ: TypInt}
}

// new Foo { field: val, ... }
// new Foo { field => val, ... }   ← Perl fat-arrow style
func (p *Parser) parseNew() Node {
	sp := p.peek().Span
	p.expect(TK_NEW)
	name := p.expect(TK_IDENT)
	p.expect(TK_LBRACE)
	fields := p.parseFieldInits()
	p.expect(TK_RBRACE)
	return &StructInit{Sp: sp, Name: name.Value, Fields: fields, HeapAlloc: false}
}

// &Foo{ field: val, ... }   ← Go-style heap alloc
func (p *Parser) parseHeapStructInit() Node {
	sp := p.peek().Span
	name := p.advance().Value // consume struct name ident
	p.expect(TK_LBRACE)
	fields := p.parseFieldInits()
	p.expect(TK_RBRACE)
	return &StructInit{Sp: sp, Name: name, Fields: fields, HeapAlloc: true}
}

// field: val,   or   field => val,   (both accepted)
func (p *Parser) parseFieldInits() []FieldInit {
	var fields []FieldInit
	for !p.at(TK_RBRACE) && !p.at(TK_EOF) && p.ok {
		fsp := p.peek().Span
		fname := p.expect(TK_IDENT)
		// accept : or =>
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
	case TK_TYPE_INT:   return TypInt
	case TK_TYPE_FLOAT: return TypFloat
	case TK_TYPE_BOOL:  return TypBool
	case TK_TYPE_CHAR:  return TypChar
	case TK_TYPE_STR:   return TypStr
	default:            return TypUnknown
	}
}
