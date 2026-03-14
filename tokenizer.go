package main

import (
	"fmt"
	"os"
	"strings"
)

// ─────────────────────────────────────────────────────────────────────────────
//  Token kinds
// ─────────────────────────────────────────────────────────────────────────────

type TK int

const (
	// literals
	TK_INT    TK = iota // 42
	TK_FLOAT            // 3.14
	TK_STRING           // "hello"
	TK_BOOL             // true / false
	TK_NIL              // nil

	// identifiers & keywords
	TK_IDENT

	// keywords
	TK_LET    // let
	TK_CONST  // const
	TK_FN     // fn
	TK_RETURN // return
	TK_IF     // if
	TK_ELIF   // elif
	TK_ELSE   // else
	TK_WHILE  // while
	TK_FOR    // for
	TK_IN     // in
	TK_IMPORT // import
	TK_AS     // as
	TK_EXTERN // extern  (declare C function signatures)
	TK_STRUCT // struct
	TK_NEW    // new
	TK_BREAK  // break
	TK_CONTINUE // continue
	TK_PRINT  // print  (built-in)
	TK_PRINTLN // println
	TK_EXIT   // exit

	// types
	TK_TYPE_INT    // int
	TK_TYPE_FLOAT  // float
	TK_TYPE_BOOL   // bool
	TK_TYPE_STR    // str
	TK_TYPE_VOID   // void
	TK_TYPE_CHAR   // char
	TK_TYPE_PTR    // ptr

	// operators
	TK_PLUS   // +
	TK_MINUS  // -
	TK_STAR   // *
	TK_SLASH  // /
	TK_PERCENT// %
	TK_AMP    // &
	TK_PIPE   // |
	TK_CARET  // ^
	TK_TILDE  // ~
	TK_LSHIFT // <<
	TK_RSHIFT // >>

	TK_EQ     // ==
	TK_NEQ    // !=
	TK_LT     // <
	TK_GT     // >
	TK_LTE    // <=
	TK_GTE    // >=

	TK_AND    // &&
	TK_OR     // ||
	TK_NOT    // !

	TK_ASSIGN // =
	TK_PLUS_EQ  // +=
	TK_MINUS_EQ // -=
	TK_STAR_EQ  // *=
	TK_SLASH_EQ // /=

	TK_ARROW  // ->
	TK_DOT    // .
	TK_DOTDOT // ..  (range)

	// delimiters
	TK_LPAREN   // (
	TK_RPAREN   // )
	TK_LBRACE   // {
	TK_RBRACE   // }
	TK_LBRACKET // [
	TK_RBRACKET // ]
	TK_COMMA    // ,
	TK_SEMI     // ;
	TK_COLON    // :
	TK_HASH     // #  (unused, reserved)

	TK_EOF
)

var tkNames = map[TK]string{
	TK_INT: "int-literal", TK_FLOAT: "float-literal", TK_STRING: "string-literal",
	TK_BOOL: "bool-literal", TK_NIL: "nil",
	TK_IDENT: "ident",
	TK_LET: "let", TK_CONST: "const", TK_FN: "fn", TK_RETURN: "return",
	TK_IF: "if", TK_ELIF: "elif", TK_ELSE: "else",
	TK_WHILE: "while", TK_FOR: "for", TK_IN: "in",
	TK_IMPORT: "import", TK_AS: "as", TK_EXTERN: "extern",
	TK_STRUCT: "struct", TK_NEW: "new",
	TK_BREAK: "break", TK_CONTINUE: "continue",
	TK_PRINT: "print", TK_PRINTLN: "println", TK_EXIT: "exit",
	TK_TYPE_INT: "int", TK_TYPE_FLOAT: "float", TK_TYPE_BOOL: "bool",
	TK_TYPE_STR: "str", TK_TYPE_VOID: "void", TK_TYPE_CHAR: "char", TK_TYPE_PTR: "ptr",
	TK_PLUS: "+", TK_MINUS: "-", TK_STAR: "*", TK_SLASH: "/", TK_PERCENT: "%",
	TK_AMP: "&", TK_PIPE: "|", TK_CARET: "^", TK_TILDE: "~",
	TK_LSHIFT: "<<", TK_RSHIFT: ">>",
	TK_EQ: "==", TK_NEQ: "!=", TK_LT: "<", TK_GT: ">", TK_LTE: "<=", TK_GTE: ">=",
	TK_AND: "&&", TK_OR: "||", TK_NOT: "!",
	TK_ASSIGN: "=", TK_PLUS_EQ: "+=", TK_MINUS_EQ: "-=", TK_STAR_EQ: "*=", TK_SLASH_EQ: "/=",
	TK_ARROW: "->", TK_DOT: ".", TK_DOTDOT: "..",
	TK_LPAREN: "(", TK_RPAREN: ")", TK_LBRACE: "{", TK_RBRACE: "}",
	TK_LBRACKET: "[", TK_RBRACKET: "]",
	TK_COMMA: ",", TK_SEMI: ";", TK_COLON: ":",
	TK_EOF: "<EOF>",
}

func (t TK) String() string {
	if s, ok := tkNames[t]; ok {
		return s
	}
	return fmt.Sprintf("tk(%d)", int(t))
}

var keywords = map[string]TK{
	"let": TK_LET, "const": TK_CONST, "fn": TK_FN, "return": TK_RETURN,
	"if": TK_IF, "elif": TK_ELIF, "else": TK_ELSE,
	"while": TK_WHILE, "for": TK_FOR, "in": TK_IN,
	"import": TK_IMPORT, "as": TK_AS, "extern": TK_EXTERN,
	"struct": TK_STRUCT, "new": TK_NEW,
	"break": TK_BREAK, "continue": TK_CONTINUE,
	"true": TK_BOOL, "false": TK_BOOL, "nil": TK_NIL,
	"print": TK_PRINT, "println": TK_PRINTLN, "exit": TK_EXIT,
	"int": TK_TYPE_INT, "float": TK_TYPE_FLOAT, "bool": TK_TYPE_BOOL,
	"str": TK_TYPE_STR, "void": TK_TYPE_VOID, "char": TK_TYPE_CHAR, "ptr": TK_TYPE_PTR,
}

// ─────────────────────────────────────────────────────────────────────────────
//  Token
// ─────────────────────────────────────────────────────────────────────────────

type Token struct {
	Kind  TK
	Value string
	Span  Span
}

func (t Token) String() string {
	return fmt.Sprintf("Token(%s, %q, %d:%d)", t.Kind, t.Value, t.Span.Line, t.Span.Col)
}

// ─────────────────────────────────────────────────────────────────────────────
//  Tokenizer
// ─────────────────────────────────────────────────────────────────────────────

type Tokenizer struct {
	src    []rune
	file   string
	pos    int
	line   int
	col    int
	tokens []Token
	ok     bool
}

func Tokenize(src, file string) []Token {
	registerSource(file, src)
	t := &Tokenizer{
		src:  []rune(src),
		file: file,
		pos:  0,
		line: 1,
		col:  1,
		ok:   true,
	}
	t.run()
	if !t.ok {
		return nil
	}
	return t.tokens
}

func (t *Tokenizer) run() {
	for !t.eof() {
		t.skipWhitespaceAndComments()
		if t.eof() {
			break
		}
		t.nextToken()
	}
	t.push(TK_EOF, "", t.here(1))
}

func (t *Tokenizer) nextToken() {
	ch := t.peek(0)

	// numbers
	if isDigit(ch) {
		t.lexNumber()
		return
	}

	// identifiers / keywords
	if isAlpha(ch) || ch == '_' {
		t.lexIdent()
		return
	}

	// strings
	if ch == '"' {
		t.lexString()
		return
	}

	// char literal
	if ch == '\'' {
		t.lexCharLit()
		return
	}

	sp := t.here(1)
	t.advance()

	switch ch {
	case '+':
		if t.peekIs('=') {
			t.advance(); t.push(TK_PLUS_EQ, "+=", sp)
		} else {
			t.push(TK_PLUS, "+", sp)
		}
	case '-':
		if t.peekIs('>') {
			t.advance(); t.push(TK_ARROW, "->", sp)
		} else if t.peekIs('=') {
			t.advance(); t.push(TK_MINUS_EQ, "-=", sp)
		} else {
			t.push(TK_MINUS, "-", sp)
		}
	case '*':
		if t.peekIs('=') {
			t.advance(); t.push(TK_STAR_EQ, "*=", sp)
		} else {
			t.push(TK_STAR, "*", sp)
		}
	case '/':
		if t.peekIs('=') {
			t.advance(); t.push(TK_SLASH_EQ, "/=", sp)
		} else {
			t.push(TK_SLASH, "/", sp)
		}
	case '%':
		t.push(TK_PERCENT, "%", sp)
	case '&':
		if t.peekIs('&') {
			t.advance(); t.push(TK_AND, "&&", sp)
		} else {
			t.push(TK_AMP, "&", sp)
		}
	case '|':
		if t.peekIs('|') {
			t.advance(); t.push(TK_OR, "||", sp)
		} else {
			t.push(TK_PIPE, "|", sp)
		}
	case '^':
		t.push(TK_CARET, "^", sp)
	case '~':
		t.push(TK_TILDE, "~", sp)
	case '!':
		if t.peekIs('=') {
			t.advance(); t.push(TK_NEQ, "!=", sp)
		} else {
			t.push(TK_NOT, "!", sp)
		}
	case '=':
		if t.peekIs('=') {
			t.advance(); t.push(TK_EQ, "==", sp)
		} else {
			t.push(TK_ASSIGN, "=", sp)
		}
	case '<':
		if t.peekIs('<') {
			t.advance(); t.push(TK_LSHIFT, "<<", sp)
		} else if t.peekIs('=') {
			t.advance(); t.push(TK_LTE, "<=", sp)
		} else {
			t.push(TK_LT, "<", sp)
		}
	case '>':
		if t.peekIs('>') {
			t.advance(); t.push(TK_RSHIFT, ">>", sp)
		} else if t.peekIs('=') {
			t.advance(); t.push(TK_GTE, ">=", sp)
		} else {
			t.push(TK_GT, ">", sp)
		}
	case '.':
		if t.peekIs('.') {
			t.advance(); t.push(TK_DOTDOT, "..", sp)
		} else {
			t.push(TK_DOT, ".", sp)
		}
	case '(':
		t.push(TK_LPAREN, "(", sp)
	case ')':
		t.push(TK_RPAREN, ")", sp)
	case '{':
		t.push(TK_LBRACE, "{", sp)
	case '}':
		t.push(TK_RBRACE, "}", sp)
	case '[':
		t.push(TK_LBRACKET, "[", sp)
	case ']':
		t.push(TK_RBRACKET, "]", sp)
	case ',':
		t.push(TK_COMMA, ",", sp)
	case ';':
		t.push(TK_SEMI, ";", sp)
	case ':':
		t.push(TK_COLON, ":", sp)
	default:
		errAt(sp, fmt.Sprintf("unexpected character %q", ch),
			"remove this character or check your syntax")
		t.ok = false
	}
}

func (t *Tokenizer) lexNumber() {
	sp := t.here(1)
	start := t.pos
	isFloat := false
	for !t.eof() && (isDigit(t.peek(0)) || t.peek(0) == '_') {
		t.advance()
	}
	if !t.eof() && t.peek(0) == '.' {
		// lookahead: don't consume ".." (range)
		if t.pos+1 < len(t.src) && t.src[t.pos+1] != '.' {
			isFloat = true
			t.advance()
			for !t.eof() && isDigit(t.peek(0)) {
				t.advance()
			}
		}
	}
	// optional exponent
	if !t.eof() && (t.peek(0) == 'e' || t.peek(0) == 'E') {
		isFloat = true
		t.advance()
		if !t.eof() && (t.peek(0) == '+' || t.peek(0) == '-') {
			t.advance()
		}
		for !t.eof() && isDigit(t.peek(0)) {
			t.advance()
		}
	}
	raw := string(t.src[start:t.pos])
	raw = strings.ReplaceAll(raw, "_", "") // allow 1_000_000
	sp.Len = t.pos - start
	if isFloat {
		t.push(TK_FLOAT, raw, sp)
	} else {
		t.push(TK_INT, raw, sp)
	}
}

func (t *Tokenizer) lexIdent() {
	sp := t.here(1)
	start := t.pos
	for !t.eof() && (isAlphaNum(t.peek(0)) || t.peek(0) == '_') {
		t.advance()
	}
	val := string(t.src[start:t.pos])
	sp.Len = len(val)
	if kw, ok := keywords[val]; ok {
		t.push(kw, val, sp)
	} else {
		t.push(TK_IDENT, val, sp)
	}
}

func (t *Tokenizer) lexString() {
	sp := t.here(1)
	t.advance() // consume "
	var sb strings.Builder
	col0 := t.col
	_ = col0
	for !t.eof() && t.peek(0) != '"' {
		ch := t.peek(0)
		if ch == '\n' {
			errAt(sp, "unterminated string literal", "add a closing \" before the end of the line")
			t.ok = false
			return
		}
		if ch == '\\' {
			t.advance()
			if t.eof() {
				break
			}
			esc := t.peek(0)
			t.advance()
			switch esc {
			case 'n':
				sb.WriteByte('\n')
			case 't':
				sb.WriteByte('\t')
			case 'r':
				sb.WriteByte('\r')
			case '\\':
				sb.WriteByte('\\')
			case '"':
				sb.WriteByte('"')
			case '0':
				sb.WriteByte(0)
			default:
				sb.WriteByte('\\')
				sb.WriteRune(esc)
			}
		} else {
			sb.WriteRune(ch)
			t.advance()
		}
	}
	if t.eof() {
		errAt(sp, "unterminated string literal", "add a closing \"")
		t.ok = false
		return
	}
	t.advance() // consume closing "
	sp.Len = t.pos - (sp.Col - 1) // rough
	t.push(TK_STRING, sb.String(), sp)
}

func (t *Tokenizer) lexCharLit() {
	sp := t.here(1)
	t.advance() // consume '
	if t.eof() {
		errAt(sp, "unterminated char literal", "add a closing '")
		t.ok = false
		return
	}
	var val rune
	if t.peek(0) == '\\' {
		t.advance()
		esc := t.peek(0)
		t.advance()
		switch esc {
		case 'n':
			val = '\n'
		case 't':
			val = '\t'
		case '\\':
			val = '\\'
		case '\'':
			val = '\''
		case '0':
			val = 0
		default:
			val = esc
		}
	} else {
		val = t.peek(0)
		t.advance()
	}
	if t.eof() || t.peek(0) != '\'' {
		errAt(sp, "char literal not closed", "add a closing '")
		t.ok = false
		return
	}
	t.advance()
	sp.Len = 3
	t.push(TK_INT, fmt.Sprintf("%d", val), sp) // chars are ints in C
}

func (t *Tokenizer) skipWhitespaceAndComments() {
	for !t.eof() {
		ch := t.peek(0)
		if ch == ' ' || ch == '\t' || ch == '\r' || ch == '\n' {
			t.advance()
			continue
		}
		// line comment: # or //
		if ch == '#' {
			for !t.eof() && t.peek(0) != '\n' {
				t.advance()
			}
			continue
		}
		if ch == '/' && t.pos+1 < len(t.src) && t.src[t.pos+1] == '/' {
			for !t.eof() && t.peek(0) != '\n' {
				t.advance()
			}
			continue
		}
		// block comment /* ... */
		if ch == '/' && t.pos+1 < len(t.src) && t.src[t.pos+1] == '*' {
			startSp := t.here(2)
			t.advance(); t.advance()
			for !t.eof() {
				if t.peek(0) == '*' && t.pos+1 < len(t.src) && t.src[t.pos+1] == '/' {
					t.advance(); t.advance()
					break
				}
				t.advance()
			}
			if t.eof() {
				errAt(startSp, "unterminated block comment", "add */ to close the comment")
				t.ok = false
				return
			}
			continue
		}
		break
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

func (t *Tokenizer) eof() bool { return t.pos >= len(t.src) }

func (t *Tokenizer) peek(offset int) rune {
	if t.pos+offset >= len(t.src) {
		return 0
	}
	return t.src[t.pos+offset]
}

func (t *Tokenizer) peekIs(ch rune) bool {
	return !t.eof() && t.peek(0) == ch
}

func (t *Tokenizer) advance() rune {
	if t.eof() {
		return 0
	}
	ch := t.src[t.pos]
	t.pos++
	if ch == '\n' {
		t.line++
		t.col = 1
	} else {
		t.col++
	}
	return ch
}

func (t *Tokenizer) here(length int) Span {
	return Span{File: t.file, Line: t.line, Col: t.col, Len: length}
}

func (t *Tokenizer) push(kind TK, value string, sp Span) {
	t.tokens = append(t.tokens, Token{Kind: kind, Value: value, Span: sp})
}

func isDigit(r rune) bool   { return r >= '0' && r <= '9' }
func isAlpha(r rune) bool   { return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') }
func isAlphaNum(r rune) bool { return isAlpha(r) || isDigit(r) }

// suppress "unused" warning
var _ = fmt.Sprintf
var _ = os.Stderr
