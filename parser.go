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

func (p *Parser) peek2() Token {
	if p.pos+1 >= len(p.tokens) {
		return Token{Kind: TK_EOF}
	}
	return p.tokens[p.pos+1]
}

func (p *Parser) at(k TK) bool { return p.peek().Kind == k }

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
		hint := fmt.Sprintf("expected %s here", k)
		errAt(t.Span, fmt.Sprintf("expected %s, got %s", k, t.Kind), hint)
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
	// allow implicit semicolons before } and EOF
	if p.at(TK_RBRACE) || p.at(TK_EOF) {
		return
	}
	t := p.peek()
	errAt(t.Span, "expected ';' after statement", "add a semicolon here")
	p.ok = false
}

// ── top level ─────────────────────────────────────────────────────────────────

func (p *Parser) parseProgram() *Program {
	prog := &Program{}
	for !p.at(TK_EOF) && p.ok {
		switch p.peek().Kind {
		case TK_IMPORT:
			prog.Imports = append(prog.Imports, p.parseImport())
		case TK_EXTERN:
			prog.Externs = append(prog.Externs, p.parseExtern())
		case TK_STRUCT:
			prog.Structs = append(prog.Structs, p.parseStruct())
		case TK_FN:
			prog.TopStmts = append(prog.TopStmts, p.parseFn())
		default:
			stmt := p.parseStmt()
			if stmt != nil {
				prog.TopStmts = append(prog.TopStmts, stmt)
			}
		}
	}
	return prog
}

// import "stdio.h"
// import "mylib.h" as ml
func (p *Parser) parseImport() *ImportDecl {
	sp := p.peek().Span
	p.expect(TK_IMPORT)
	path := p.expect(TK_STRING)
	alias := ""
	if p.at(TK_AS) {
		p.advance()
		alias = p.expect(TK_IDENT).Value
	}
	p.expectSemi()
	return &ImportDecl{Sp: sp, Path: path.Value, Alias: alias}
}

// extern fn printf(fmt: str, ...) -> int;
func (p *Parser) parseExtern() *ExternDecl {
	sp := p.peek().Span
	p.expect(TK_EXTERN)
	p.expect(TK_FN)
	name := p.expect(TK_IDENT)
	p.expect(TK_LPAREN)
	params, variadic := p.parseParamList()
	p.expect(TK_RPAREN)
	ret := TypVoid
	if p.at(TK_ARROW) {
		p.advance()
		ret = p.parseType()
	}
	p.expectSemi()
	return &ExternDecl{Sp: sp, Name: name.Value, Params: params, Variadic: variadic, RetType: ret}
}

// struct Point { x: float, y: float }
func (p *Parser) parseStruct() *StructDecl {
	sp := p.peek().Span
	p.expect(TK_STRUCT)
	name := p.expect(TK_IDENT)
	p.expect(TK_LBRACE)
	var fields []Param
	for !p.at(TK_RBRACE) && !p.at(TK_EOF) && p.ok {
		fsp := p.peek().Span
		fname := p.expect(TK_IDENT)
		p.expect(TK_COLON)
		ftype := p.parseType()
		fields = append(fields, Param{Sp: fsp, Name: fname.Value, Type: ftype})
		if p.at(TK_COMMA) {
			p.advance()
		}
	}
	p.expect(TK_RBRACE)
	return &StructDecl{Sp: sp, Name: name.Value, Fields: fields}
}

// fn name(params) -> rettype { body }
func (p *Parser) parseFn() *FnDecl {
	sp := p.peek().Span
	p.expect(TK_FN)
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

func (p *Parser) parseParamList() ([]Param, bool) {
	var params []Param
	variadic := false
	for !p.at(TK_RPAREN) && !p.at(TK_EOF) && p.ok {
		// ... variadic
		if p.at(TK_DOTDOT) || (p.peek().Value == "." && p.peek2().Value == ".") {
			p.advance()
			// consume extra dot if needed
			if p.at(TK_DOT) {
				p.advance()
			}
			variadic = true
			break
		}
		// check for "..." as single token (some editors might produce this)
		if p.peek().Value == "..." {
			p.advance()
			variadic = true
			break
		}
		psp := p.peek().Span
		pname := p.expect(TK_IDENT)
		p.expect(TK_COLON)
		ptype := p.parseType()
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
	switch t.Kind {
	case TK_TYPE_INT:
		p.advance(); return TypInt
	case TK_TYPE_FLOAT:
		p.advance(); return TypFloat
	case TK_TYPE_BOOL:
		p.advance(); return TypBool
	case TK_TYPE_STR:
		p.advance(); return TypStr
	case TK_TYPE_CHAR:
		p.advance(); return TypChar
	case TK_TYPE_VOID:
		p.advance(); return TypVoid
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
		errAt(t.Span, fmt.Sprintf("expected a type, got %q", t.Value),
			"valid types: int, float, bool, str, char, void, ptr<T>, or a struct name")
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
		stmt := p.parseStmt()
		if stmt != nil {
			stmts = append(stmts, stmt)
		}
	}
	p.expect(TK_RBRACE)
	return &Block{Sp: sp, Stmts: stmts}
}

// ── statements ────────────────────────────────────────────────────────────────

func (p *Parser) parseStmt() Node {
	t := p.peek()
	switch t.Kind {
	case TK_LET, TK_CONST:
		return p.parseVarDecl()
	case TK_RETURN:
		return p.parseReturn()
	case TK_IF:
		return p.parseIf()
	case TK_WHILE:
		return p.parseWhile()
	case TK_FOR:
		return p.parseFor()
	case TK_BREAK:
		p.advance()
		p.expectSemi()
		return &BreakStmt{Sp: t.Span}
	case TK_CONTINUE:
		p.advance()
		p.expectSemi()
		return &ContinueStmt{Sp: t.Span}
	case TK_PRINT, TK_PRINTLN:
		return p.parsePrint()
	case TK_EXIT:
		return p.parseExit()
	case TK_FN:
		return p.parseFn()
	case TK_SEMI:
		p.advance()
		return nil
	default:
		return p.parseExprOrAssign()
	}
}

// let x: int = 5;
// let x = 5;
// const PI: float = 3.14;
func (p *Parser) parseVarDecl() *VarDecl {
	sp := p.peek().Span
	isConst := p.peek().Kind == TK_CONST
	p.advance()
	name := p.expect(TK_IDENT)
	var varType *ZXType
	if p.at(TK_COLON) {
		p.advance()
		varType = p.parseType()
	}
	var init Node
	if p.at(TK_ASSIGN) {
		p.advance()
		init = p.parseExpr()
	} else if isConst {
		errAt(name.Span, "const declaration requires an initializer",
			"add = <value> after the type")
		p.ok = false
	}
	p.expectSemi()
	return &VarDecl{Sp: sp, Name: name.Value, VarType: varType, Init: init, IsConst: isConst}
}

func (p *Parser) parseReturn() *ReturnStmt {
	sp := p.peek().Span
	p.expect(TK_RETURN)
	if p.at(TK_SEMI) || p.at(TK_RBRACE) {
		p.expectSemi()
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
	for p.at(TK_ELIF) && p.ok {
		p.advance()
		ec := p.parseExpr()
		eb := p.parseBlock()
		elifs = append(elifs, ElifClause{Cond: ec, Body: eb})
	}
	if p.at(TK_ELSE) {
		p.advance()
		els = p.parseBlock()
	}
	return &IfStmt{Sp: sp, Cond: cond, Then: then, Elifs: elifs, Else: els}
}

func (p *Parser) parseWhile() *WhileStmt {
	sp := p.peek().Span
	p.expect(TK_WHILE)
	cond := p.parseExpr()
	body := p.parseBlock()
	return &WhileStmt{Sp: sp, Cond: cond, Body: body}
}

// for i in 0..10 { }
func (p *Parser) parseFor() Node {
	sp := p.peek().Span
	p.expect(TK_FOR)
	varName := p.expect(TK_IDENT)
	p.expect(TK_IN)
	from := p.parseExpr()
	p.expect(TK_DOTDOT)
	to := p.parseExpr()
	body := p.parseBlock()
	return &ForRangeStmt{Sp: sp, Var: varName.Value, From: from, To: to, Body: body}
}

func (p *Parser) parsePrint() *PrintStmt {
	sp := p.peek().Span
	newline := p.peek().Kind == TK_PRINTLN
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
	p.expectSemi()
	return &PrintStmt{Sp: sp, Args: args, Newline: newline}
}

func (p *Parser) parseExit() *ExitStmt {
	sp := p.peek().Span
	p.expect(TK_EXIT)
	p.expect(TK_LPAREN)
	code := p.parseExpr()
	p.expect(TK_RPAREN)
	p.expectSemi()
	return &ExitStmt{Sp: sp, Code: code}
}

// expression or assignment statement
func (p *Parser) parseExprOrAssign() Node {
	sp := p.peek().Span
	expr := p.parseExpr()
	// assignment?
	switch p.peek().Kind {
	case TK_ASSIGN, TK_PLUS_EQ, TK_MINUS_EQ, TK_STAR_EQ, TK_SLASH_EQ:
		op := p.advance().Value
		val := p.parseExpr()
		p.expectSemi()
		return &AssignStmt{Sp: sp, LHS: expr, Op: op, Value: val}
	}
	p.expectSemi()
	return &ExprStmt{Sp: sp, Expr: expr}
}

// ── expressions (Pratt/precedence climbing) ───────────────────────────────────

func (p *Parser) parseExpr() Node { return p.parseOr() }

func (p *Parser) parseOr() Node {
	lhs := p.parseAnd()
	for p.at(TK_OR) && p.ok {
		sp := p.peek().Span
		p.advance()
		rhs := p.parseAnd()
		lhs = &BinExpr{Sp: sp, Op: "||", LHS: lhs, RHS: rhs}
	}
	return lhs
}

func (p *Parser) parseAnd() Node {
	lhs := p.parseBitOr()
	for p.at(TK_AND) && p.ok {
		sp := p.peek().Span
		p.advance()
		rhs := p.parseBitOr()
		lhs = &BinExpr{Sp: sp, Op: "&&", LHS: lhs, RHS: rhs}
	}
	return lhs
}

func (p *Parser) parseBitOr() Node {
	lhs := p.parseBitXor()
	for p.at(TK_PIPE) && p.ok {
		sp := p.peek().Span
		p.advance()
		rhs := p.parseBitXor()
		lhs = &BinExpr{Sp: sp, Op: "|", LHS: lhs, RHS: rhs}
	}
	return lhs
}

func (p *Parser) parseBitXor() Node {
	lhs := p.parseBitAnd()
	for p.at(TK_CARET) && p.ok {
		sp := p.peek().Span
		p.advance()
		rhs := p.parseBitAnd()
		lhs = &BinExpr{Sp: sp, Op: "^", LHS: lhs, RHS: rhs}
	}
	return lhs
}

func (p *Parser) parseBitAnd() Node {
	lhs := p.parseEquality()
	for p.at(TK_AMP) && p.ok {
		sp := p.peek().Span
		p.advance()
		rhs := p.parseEquality()
		lhs = &BinExpr{Sp: sp, Op: "&", LHS: lhs, RHS: rhs}
	}
	return lhs
}

func (p *Parser) parseEquality() Node {
	lhs := p.parseRelational()
	for (p.at(TK_EQ) || p.at(TK_NEQ)) && p.ok {
		sp := p.peek().Span
		op := p.advance().Value
		rhs := p.parseRelational()
		lhs = &BinExpr{Sp: sp, Op: op, LHS: lhs, RHS: rhs}
	}
	return lhs
}

func (p *Parser) parseRelational() Node {
	lhs := p.parseShift()
	for (p.at(TK_LT) || p.at(TK_GT) || p.at(TK_LTE) || p.at(TK_GTE)) && p.ok {
		sp := p.peek().Span
		op := p.advance().Value
		rhs := p.parseShift()
		lhs = &BinExpr{Sp: sp, Op: op, LHS: lhs, RHS: rhs}
	}
	return lhs
}

func (p *Parser) parseShift() Node {
	lhs := p.parseAddSub()
	for (p.at(TK_LSHIFT) || p.at(TK_RSHIFT)) && p.ok {
		sp := p.peek().Span
		op := p.advance().Value
		rhs := p.parseAddSub()
		lhs = &BinExpr{Sp: sp, Op: op, LHS: lhs, RHS: rhs}
	}
	return lhs
}

func (p *Parser) parseAddSub() Node {
	lhs := p.parseMulDiv()
	for (p.at(TK_PLUS) || p.at(TK_MINUS)) && p.ok {
		sp := p.peek().Span
		op := p.advance().Value
		rhs := p.parseMulDiv()
		lhs = &BinExpr{Sp: sp, Op: op, LHS: lhs, RHS: rhs}
	}
	return lhs
}

func (p *Parser) parseMulDiv() Node {
	lhs := p.parseUnary()
	for (p.at(TK_STAR) || p.at(TK_SLASH) || p.at(TK_PERCENT)) && p.ok {
		sp := p.peek().Span
		op := p.advance().Value
		rhs := p.parseUnary()
		lhs = &BinExpr{Sp: sp, Op: op, LHS: lhs, RHS: rhs}
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
	case TK_AMP:
		p.advance()
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
			expr = &FieldExpr{Sp: sp, Obj: expr, Field: field.Value}
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
		v, _ := strconv.ParseInt(t.Value, 10, 64)
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
	case TK_IDENT:
		// struct init: Name { field: val, ... }
		if p.peek2().Kind == TK_LBRACE {
			return p.parseStructInit()
		}
		p.advance()
		return &Ident{Sp: t.Span, Name: t.Name()}
	case TK_NEW:
		return p.parseNew()
	case TK_LPAREN:
		p.advance()
		// cast<type>(expr) — written as (type) expr  OR cast<type>(expr)
		inner := p.parseExpr()
		p.expect(TK_RPAREN)
		return inner
	case TK_TYPE_INT, TK_TYPE_FLOAT, TK_TYPE_BOOL, TK_TYPE_CHAR, TK_TYPE_STR:
		// cast: int(expr)  float(expr)
		typTok := p.advance()
		toType := tokenToType(typTok)
		p.expect(TK_LPAREN)
		operand := p.parseExpr()
		p.expect(TK_RPAREN)
		return &CastExpr{Sp: t.Span, ToType: toType, Operand: operand}
	default:
		errAt(t.Span, fmt.Sprintf("unexpected token %q in expression", t.Value),
			"check the expression syntax")
		p.ok = false
		return &NilLit{Sp: t.Span}
	}
}

func (t Token) Name() string { return t.Value }

func (p *Parser) parseNew() Node {
	sp := p.peek().Span
	p.expect(TK_NEW)
	name := p.expect(TK_IDENT)
	p.expect(TK_LBRACE)
	var fields []FieldInit
	for !p.at(TK_RBRACE) && !p.at(TK_EOF) && p.ok {
		fsp := p.peek().Span
		fname := p.expect(TK_IDENT)
		p.expect(TK_COLON)
		fval := p.parseExpr()
		fields = append(fields, FieldInit{Sp: fsp, Name: fname.Value, Value: fval})
		if p.at(TK_COMMA) {
			p.advance()
		}
	}
	p.expect(TK_RBRACE)
	return &StructInit{Sp: sp, Name: name.Value, Fields: fields}
}

func (p *Parser) parseStructInit() Node {
	sp := p.peek().Span
	name := p.advance().Value
	p.expect(TK_LBRACE)
	var fields []FieldInit
	for !p.at(TK_RBRACE) && !p.at(TK_EOF) && p.ok {
		fsp := p.peek().Span
		fname := p.expect(TK_IDENT)
		p.expect(TK_COLON)
		fval := p.parseExpr()
		fields = append(fields, FieldInit{Sp: fsp, Name: fname.Value, Value: fval})
		if p.at(TK_COMMA) {
			p.advance()
		}
	}
	p.expect(TK_RBRACE)
	return &StructInit{Sp: sp, Name: name, Fields: fields}
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
