package main

import (
	"fmt"
	"strings"
)

// ─────────────────────────────────────────────────────────────────────────────
//  Token kinds
// ─────────────────────────────────────────────────────────────────────────────

type TK int

const (
	TK_INT TK = iota
	TK_FLOAT
	TK_STRING
	TK_BOOL
	TK_NIL
	TK_TEMPLATE_STR // f"..."
	TK_ANNOTATION   // @name or @name={"key":val}

	TK_IDENT

	// Keywords
	TK_LET
	TK_MY
	TK_CONST
	TK_OUR
	TK_FN
	TK_SUB
	TK_RETURN
	TK_IF
	TK_UNLESS
	TK_ELIF
	TK_ELSE
	TK_WHILE
	TK_UNTIL
	TK_FOR
	TK_FOREACH
	TK_IN
	TK_DO
	TK_IMPORT
	TK_USE
	TK_MOD
	TK_AS
	TK_EXTERN
	TK_STRUCT
	TK_TYPE
	TK_NEW
	TK_BREAK
	TK_NEXT
	TK_CONTINUE
	TK_LAST
	TK_PRINT
	TK_PRINTLN
	TK_SAY
	TK_WARN
	TK_EPRINT
	TK_EXIT
	TK_DIE
	TK_SIZEOF
	TK_LEN
	TK_PUSH
	TK_POP
	TK_MATCH
	TK_TRY
	TK_CATCH
	TK_FINALLY
	TK_THROW
	TK_RAISE
	TK_DEFER
	TK_ASSERT
	TK_SPAWN
	TK_CMD       // cmd!
	TK_READFILE  // readfile!
	TK_WRITEFILE // writefile!
	TK_INPUT     // input()
	TK_STDIN
	TK_STDOUT
	TK_STDERR

	// Types
	TK_TYPE_INT
	TK_TYPE_FLOAT
	TK_TYPE_BOOL
	TK_TYPE_STR
	TK_TYPE_VOID
	TK_TYPE_CHAR
	TK_TYPE_REF
	TK_TYPE_ANY

	// Operators
	TK_PLUS
	TK_MINUS
	TK_STAR
	TK_SLASH
	TK_PERCENT
	TK_AMP
	TK_PIPE
	TK_CARET
	TK_TILDE
	TK_LSHIFT
	TK_RSHIFT
	TK_EQ
	TK_NEQ
	TK_LT
	TK_GT
	TK_LTE
	TK_GTE
	TK_AND
	TK_OR
	TK_NOT
	TK_ASSIGN
	TK_PLUS_EQ
	TK_MINUS_EQ
	TK_STAR_EQ
	TK_SLASH_EQ
	TK_PERCENT_EQ
	TK_ARROW      // ->
	TK_FAT_ARROW  // =>
	TK_PIPE_ARROW // |>
	TK_QUESTION   // ?
	TK_AT         // @  addr-of
	TK_HAT        // ^  deref
	TK_DOT
	TK_DOTDOT
	TK_ELLIPSIS
	TK_LPAREN
	TK_RPAREN
	TK_LBRACE
	TK_RBRACE
	TK_LBRACKET
	TK_RBRACKET
	TK_COMMA
	TK_SEMI
	TK_COLON
	TK_DCOLON // ::

	TK_EOF
)

var tkNames = map[TK]string{
	TK_INT: "int-literal", TK_FLOAT: "float-literal",
	TK_STRING: "string-literal", TK_TEMPLATE_STR: "f-string",
	TK_ANNOTATION: "annotation", TK_BOOL: "bool-literal", TK_NIL: "nil",
	TK_IDENT: "identifier",
	TK_LET:   "let", TK_MY: "my", TK_CONST: "const", TK_OUR: "our",
	TK_FN: "fn", TK_SUB: "sub", TK_RETURN: "return",
	TK_IF: "if", TK_UNLESS: "unless", TK_ELIF: "elif", TK_ELSE: "else",
	TK_WHILE: "while", TK_UNTIL: "until",
	TK_FOR: "for", TK_FOREACH: "foreach", TK_IN: "in", TK_DO: "do",
	TK_IMPORT: "import", TK_USE: "use", TK_MOD: "mod", TK_AS: "as", TK_EXTERN: "extern",
	TK_STRUCT: "struct", TK_TYPE: "type", TK_NEW: "new",
	TK_BREAK: "break", TK_NEXT: "next", TK_CONTINUE: "continue", TK_LAST: "last",
	TK_PRINT: "print", TK_PRINTLN: "println", TK_SAY: "say", TK_WARN: "warn", TK_EPRINT: "eprint",
	TK_EXIT: "exit", TK_DIE: "die", TK_SIZEOF: "sizeof",
	TK_LEN: "len", TK_PUSH: "push", TK_POP: "pop", TK_MATCH: "match",
	TK_TRY: "try", TK_CATCH: "catch", TK_FINALLY: "finally", TK_THROW: "throw", TK_RAISE: "raise",
	TK_DEFER: "defer", TK_ASSERT: "assert", TK_SPAWN: "spawn",
	TK_CMD: "cmd!", TK_READFILE: "readfile!", TK_WRITEFILE: "writefile!",
	TK_INPUT: "input", TK_STDIN: "stdin", TK_STDOUT: "stdout", TK_STDERR: "stderr",
	TK_TYPE_INT: "int", TK_TYPE_FLOAT: "float", TK_TYPE_BOOL: "bool",
	TK_TYPE_STR: "str", TK_TYPE_VOID: "void", TK_TYPE_CHAR: "char",
	TK_TYPE_REF: "ref", TK_TYPE_ANY: "any",
	TK_PLUS: "+", TK_MINUS: "-", TK_STAR: "*", TK_SLASH: "/", TK_PERCENT: "%",
	TK_AMP: "&", TK_PIPE: "|", TK_CARET: "^", TK_TILDE: "~",
	TK_LSHIFT: "<<", TK_RSHIFT: ">>",
	TK_EQ: "==", TK_NEQ: "!=", TK_LT: "<", TK_GT: ">", TK_LTE: "<=", TK_GTE: ">=",
	TK_AND: "&&", TK_OR: "||", TK_NOT: "!",
	TK_ASSIGN: "=", TK_PLUS_EQ: "+=", TK_MINUS_EQ: "-=",
	TK_STAR_EQ: "*=", TK_SLASH_EQ: "/=", TK_PERCENT_EQ: "%=",
	TK_ARROW: "->", TK_FAT_ARROW: "=>", TK_PIPE_ARROW: "|>",
	TK_QUESTION: "?", TK_AT: "@", TK_HAT: "^",
	TK_DOT: ".", TK_DOTDOT: "..", TK_ELLIPSIS: "...",
	TK_LPAREN: "(", TK_RPAREN: ")", TK_LBRACE: "{", TK_RBRACE: "}",
	TK_LBRACKET: "[", TK_RBRACKET: "]",
	TK_COMMA: ",", TK_SEMI: ";", TK_COLON: ":", TK_DCOLON: "::",
	TK_EOF: "<EOF>",
}

func (t TK) String() string {
	if s, ok := tkNames[t]; ok {
		return s
	}
	return fmt.Sprintf("tk(%d)", int(t))
}

var keywords = map[string]TK{
	"let": TK_LET, "my": TK_MY, "local": TK_MY,
	"const": TK_CONST, "our": TK_OUR,
	"fn": TK_FN, "func": TK_FN, "sub": TK_SUB,
	"return": TK_RETURN,
	"if":     TK_IF, "unless": TK_UNLESS,
	"elif": TK_ELIF, "elsif": TK_ELIF, "elseif": TK_ELIF, "else": TK_ELSE,
	"while": TK_WHILE, "until": TK_UNTIL,
	"for": TK_FOR, "foreach": TK_FOREACH, "in": TK_IN, "do": TK_DO,
	"import": TK_IMPORT, "use": TK_USE, "mod": TK_MOD, "as": TK_AS, "extern": TK_EXTERN,
	"struct": TK_STRUCT, "type": TK_TYPE, "new": TK_NEW,
	"break": TK_BREAK, "last": TK_LAST, "continue": TK_CONTINUE, "next": TK_NEXT,
	"print": TK_PRINT, "println": TK_PRINTLN, "say": TK_SAY, "warn": TK_WARN, "eprint": TK_EPRINT,
	"exit": TK_EXIT, "die": TK_DIE, "sizeof": TK_SIZEOF,
	"len": TK_LEN, "push": TK_PUSH, "pop": TK_POP, "match": TK_MATCH,
	"try": TK_TRY, "catch": TK_CATCH, "finally": TK_FINALLY, "throw": TK_THROW, "raise": TK_RAISE,
	"defer": TK_DEFER, "assert": TK_ASSERT, "spawn": TK_SPAWN,
	"input": TK_INPUT, "stdin": TK_STDIN, "stdout": TK_STDOUT, "stderr": TK_STDERR,
	"true": TK_BOOL, "false": TK_BOOL,
	"nil": TK_NIL, "null": TK_NIL, "NULL": TK_NIL, "undef": TK_NIL,
	// types
	"int": TK_TYPE_INT, "float": TK_TYPE_FLOAT, "bool": TK_TYPE_BOOL,
	"str": TK_TYPE_STR, "string": TK_TYPE_STR,
	"void": TK_TYPE_VOID, "char": TK_TYPE_CHAR,
	"ref": TK_TYPE_REF, "any": TK_TYPE_ANY,
	// Go/Rust type aliases
	"int8": TK_TYPE_INT, "int16": TK_TYPE_INT, "int32": TK_TYPE_INT,
	"int64": TK_TYPE_INT, "uint": TK_TYPE_INT, "uint64": TK_TYPE_INT,
	"float32": TK_TYPE_FLOAT, "float64": TK_TYPE_FLOAT,
	"byte": TK_TYPE_CHAR, "rune": TK_TYPE_INT, "boolean": TK_TYPE_BOOL,
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
	return fmt.Sprintf("Token(%s %q %d:%d)", t.Kind, t.Value, t.Span.Line, t.Span.Col)
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
	t := &Tokenizer{src: []rune(src), file: file, pos: 0, line: 1, col: 1, ok: true}
	t.run()
	if !t.ok {
		return nil
	}
	t.tokens = mergeAdjacentStrings(t.tokens)
	return t.tokens
}

// mergeAdjacentStrings joins consecutive TK_STRING tokens into one.
// "foo" "bar" → "foobar"  (multi-line string concat)
func mergeAdjacentStrings(in []Token) []Token {
	out := make([]Token, 0, len(in))
	for i := 0; i < len(in); i++ {
		if in[i].Kind == TK_STRING {
			m := in[i]
			for i+1 < len(in) && in[i+1].Kind == TK_STRING {
				i++
				m.Value += in[i].Value
				m.Span.Len += in[i].Span.Len
			}
			out = append(out, m)
		} else {
			out = append(out, in[i])
		}
	}
	return out
}

func (t *Tokenizer) run() {
	for !t.eof() {
		t.skipWS()
		if t.eof() {
			break
		}
		t.nextToken()
		if !t.ok {
			return
		}
	}
	t.push(TK_EOF, "", t.here(1))
}

func (t *Tokenizer) nextToken() {
	ch := t.peek(0)

	// f"..." interpolated string
	if ch == 'f' && t.pos+1 < len(t.src) && t.src[t.pos+1] == '"' {
		t.advance() // consume 'f'
		t.lexTemplateStr()
		return
	}
	if isDigit(ch) {
		t.lexNumber()
		return
	}
	if isAlpha(ch) || ch == '_' {
		t.lexIdent()
		return
	}
	if ch == '"' {
		t.lexString('"')
		return
	}
	if ch == '\'' {
		t.lexCharLit()
		return
	}
	if ch == '`' {
		t.lexString('`')
		return
	}

	sp := t.here(1)
	t.advance()

	switch ch {
	case '@':
		// @name or @name={...}  → annotation token
		// bare @ (no ident after) → address-of operator
		if !t.eof() && (isAlpha(t.peek(0)) || t.peek(0) == '_') {
			t.lexAnnotation(sp)
		} else {
			t.push(TK_AT, "@", sp)
		}
	case '+':
		if t.tryEat('=') {
			t.push(TK_PLUS_EQ, "+=", sp)
		} else {
			t.push(TK_PLUS, "+", sp)
		}
	case '-':
		if t.tryEat('>') {
			t.push(TK_ARROW, "->", sp)
		} else if t.tryEat('=') {
			t.push(TK_MINUS_EQ, "-=", sp)
		} else {
			t.push(TK_MINUS, "-", sp)
		}
	case '*':
		if t.tryEat('=') {
			t.push(TK_STAR_EQ, "*=", sp)
		} else {
			t.push(TK_STAR, "*", sp)
		}
	case '/':
		if t.tryEat('=') {
			t.push(TK_SLASH_EQ, "/=", sp)
		} else {
			t.push(TK_SLASH, "/", sp)
		}
	case '%':
		if t.tryEat('=') {
			t.push(TK_PERCENT_EQ, "%=", sp)
		} else {
			t.push(TK_PERCENT, "%", sp)
		}
	case '&':
		if t.tryEat('&') {
			t.push(TK_AND, "&&", sp)
		} else {
			t.push(TK_AMP, "&", sp)
		}
	case '|':
		if t.tryEat('|') {
			t.push(TK_OR, "||", sp)
		} else if t.tryEat('>') {
			t.push(TK_PIPE_ARROW, "|>", sp)
		} else {
			t.push(TK_PIPE, "|", sp)
		}
	case '^':
		t.push(TK_HAT, "^", sp) // deref operator
	case '~':
		t.push(TK_TILDE, "~", sp)
	case '?':
		t.push(TK_QUESTION, "?", sp)
	case '!':
		if t.tryEat('=') {
			t.push(TK_NEQ, "!=", sp)
		} else {
			t.push(TK_NOT, "!", sp)
		}
	case '=':
		if t.tryEat('=') {
			t.push(TK_EQ, "==", sp)
		} else if t.tryEat('>') {
			t.push(TK_FAT_ARROW, "=>", sp)
		} else {
			t.push(TK_ASSIGN, "=", sp)
		}
	case '<':
		if t.tryEat('<') {
			t.push(TK_LSHIFT, "<<", sp)
		} else if t.tryEat('=') {
			t.push(TK_LTE, "<=", sp)
		} else {
			t.push(TK_LT, "<", sp)
		}
	case '>':
		if t.tryEat('>') {
			t.push(TK_RSHIFT, ">>", sp)
		} else if t.tryEat('=') {
			t.push(TK_GTE, ">=", sp)
		} else {
			t.push(TK_GT, ">", sp)
		}
	case '.':
		if t.tryEat('.') {
			if t.tryEat('.') {
				t.push(TK_ELLIPSIS, "...", sp)
			} else {
				t.push(TK_DOTDOT, "..", sp)
			}
		} else {
			t.push(TK_DOT, ".", sp)
		}
	case ':':
		if t.tryEat(':') {
			t.push(TK_DCOLON, "::", sp)
		} else {
			t.push(TK_COLON, ":", sp)
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
	default:
		errAt(sp, fmt.Sprintf("unexpected character %q", ch),
			"remove this character or check your syntax")
		t.ok = false
	}
}

// lexAnnotation: reads @name or @name={...}
// The entire annotation including optional ={...} is stored in Value.
func (t *Tokenizer) lexAnnotation(sp Span) {
	start := t.pos
	for !t.eof() && (isAlphaNum(t.peek(0)) || t.peek(0) == '_') {
		t.advance()
	}
	name := string(t.src[start:t.pos])
	raw := name

	// optional =value or ={"key":val}
	if !t.eof() && t.peek(0) == '=' {
		t.advance()
		raw += "="
		if !t.eof() && t.peek(0) == '{' {
			// collect balanced {...}
			depth := 0
			for !t.eof() {
				c := t.peek(0)
				raw += string(c)
				t.advance()
				if c == '{' {
					depth++
				} else if c == '}' {
					depth--
					if depth == 0 {
						break
					}
				}
			}
		} else {
			// scalar value: @expect=42  @timeout=500
			for !t.eof() && t.peek(0) != '\n' && t.peek(0) != ' ' && t.peek(0) != '\t' {
				raw += string(t.peek(0))
				t.advance()
			}
		}
	}
	t.push(TK_ANNOTATION, raw, sp)
}

func (t *Tokenizer) lexTemplateStr() {
	sp := t.here(1)
	t.advance() // consume opening "
	var sb strings.Builder
	for !t.eof() && t.peek(0) != '"' {
		ch := t.peek(0)
		if ch == '\n' {
			errAt(sp, "unterminated f-string — newline before closing quote",
				`close the f-string with " on the same line`)
			t.ok = false
			return
		}
		sb.WriteRune(ch)
		t.advance()
	}
	if t.eof() {
		errAt(sp, "unterminated f-string — reached end of file", `add a closing "`)
		t.ok = false
		return
	}
	t.advance() // closing "
	t.push(TK_TEMPLATE_STR, sb.String(), sp)
}

func (t *Tokenizer) lexNumber() {
	sp := t.here(1)
	start := t.pos
	isFloat := false
	for !t.eof() && (isDigit(t.peek(0)) || t.peek(0) == '_') {
		t.advance()
	}
	justOne := t.pos-start == 1 && t.src[start] == '0'
	if justOne && !t.eof() && (t.peek(0) == 'x' || t.peek(0) == 'X') {
		t.advance()
		for !t.eof() && (isHex(t.peek(0)) || t.peek(0) == '_') {
			t.advance()
		}
		t.push(TK_INT, strings.ReplaceAll(string(t.src[start:t.pos]), "_", ""), sp)
		return
	}
	if justOne && !t.eof() && (t.peek(0) == 'b' || t.peek(0) == 'B') {
		t.advance()
		bs := t.pos
		for !t.eof() && (t.peek(0) == '0' || t.peek(0) == '1' || t.peek(0) == '_') {
			t.advance()
		}
		binStr := strings.ReplaceAll(string(t.src[bs:t.pos]), "_", "")
		val := int64(0)
		for _, c := range binStr {
			val = val*2 + int64(c-'0')
		}
		t.push(TK_INT, fmt.Sprintf("%d", val), sp)
		return
	}
	if justOne && !t.eof() && (t.peek(0) == 'o' || t.peek(0) == 'O') {
		t.advance()
		for !t.eof() && (t.peek(0) >= '0' && t.peek(0) <= '7' || t.peek(0) == '_') {
			t.advance()
		}
		t.push(TK_INT, strings.ReplaceAll(string(t.src[start:t.pos]), "_", ""), sp)
		return
	}
	if !t.eof() && t.peek(0) == '.' && t.pos+1 < len(t.src) && t.src[t.pos+1] != '.' {
		isFloat = true
		t.advance()
		for !t.eof() && isDigit(t.peek(0)) {
			t.advance()
		}
	}
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
	raw := strings.ReplaceAll(string(t.src[start:t.pos]), "_", "")
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
	// check for bang-suffixed builtins: cmd! readfile! writefile!
	if !t.eof() && t.peek(0) == '!' {
		switch val {
		case "cmd", "sh", "shell", "run":
			t.advance()
			t.push(TK_CMD, val+"!", sp)
			return
		case "readfile", "slurp", "read_file":
			t.advance()
			t.push(TK_READFILE, val+"!", sp)
			return
		case "writefile", "write_file":
			t.advance()
			t.push(TK_WRITEFILE, val+"!", sp)
			return
		}
	}
	if kw, ok := keywords[val]; ok {
		t.push(kw, val, sp)
	} else {
		t.push(TK_IDENT, val, sp)
	}
}

func (t *Tokenizer) lexString(quote rune) {
	sp := t.here(1)
	t.advance() // consume opening quote
	var sb strings.Builder
	for !t.eof() && t.peek(0) != quote {
		ch := t.peek(0)
		if ch == '\n' && quote != '`' {
			errAt(sp, "unterminated string — newline inside string literal",
				fmt.Sprintf("add a closing %c before the end of the line, or use backtick strings for multi-line", quote))
			t.ok = false
			return
		}
		if ch == '\\' && quote != '`' {
			t.advance()
			if t.eof() {
				break
			}
			switch t.peek(0) {
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
			case '\'':
				sb.WriteByte('\'')
			case '0':
				sb.WriteByte(0)
			case 'a':
				sb.WriteByte('\a')
			default:
				warnAt(sp, fmt.Sprintf("unknown escape \\%c in string", t.peek(0)),
					"valid escapes: \\n \\t \\r \\\\ \\\" \\0 \\a")
				sb.WriteByte('\\')
				sb.WriteRune(t.peek(0))
			}
			t.advance()
		} else {
			sb.WriteRune(ch)
			t.advance()
		}
	}
	if t.eof() {
		errAt(sp, "unterminated string literal — reached end of file",
			fmt.Sprintf("add a closing %c", quote))
		t.ok = false
		return
	}
	t.advance() // closing quote
	t.push(TK_STRING, sb.String(), sp)
}

func (t *Tokenizer) lexCharLit() {
	sp := t.here(1)
	t.advance()
	if t.eof() || t.peek(0) == '\n' {
		errAt(sp, "unterminated char literal", "add a closing '")
		t.ok = false
		return
	}
	var val rune
	if t.peek(0) == '\\' {
		t.advance()
		if t.eof() {
			errAt(sp, "unterminated escape in char literal", "")
			t.ok = false
			return
		}
		switch t.peek(0) {
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
			val = t.peek(0)
		}
		t.advance()
	} else {
		val = t.peek(0)
		t.advance()
	}
	if t.eof() || t.peek(0) != '\'' {
		errAt(sp, "char literal not closed",
			"single quotes are for chars: 'a' — for strings use double quotes: \"abc\"")
		t.ok = false
		return
	}
	t.advance()
	t.push(TK_INT, fmt.Sprintf("%d", val), sp)
}

func (t *Tokenizer) skipWS() {
	for !t.eof() {
		ch := t.peek(0)
		if ch == ' ' || ch == '\t' || ch == '\r' || ch == '\n' {
			t.advance()
			continue
		}
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
		if ch == '/' && t.pos+1 < len(t.src) && t.src[t.pos+1] == '*' {
			sp2 := t.here(2)
			t.advance()
			t.advance()
			for !t.eof() {
				if t.peek(0) == '*' && t.pos+1 < len(t.src) && t.src[t.pos+1] == '/' {
					t.advance()
					t.advance()
					break
				}
				t.advance()
			}
			if t.eof() {
				errAt(sp2, "unterminated block comment /* ... */",
					"add */ to close the comment")
				t.ok = false
				return
			}
			continue
		}
		break
	}
}

func (t *Tokenizer) eof() bool { return t.pos >= len(t.src) }
func (t *Tokenizer) peek(n int) rune {
	if t.pos+n >= len(t.src) {
		return 0
	}
	return t.src[t.pos+n]
}
func (t *Tokenizer) tryEat(ch rune) bool {
	if !t.eof() && t.peek(0) == ch {
		t.advance()
		return true
	}
	return false
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
func (t *Tokenizer) here(l int) Span {
	return Span{File: t.file, Line: t.line, Col: t.col, Len: l}
}
func (t *Tokenizer) push(k TK, v string, sp Span) {
	t.tokens = append(t.tokens, Token{Kind: k, Value: v, Span: sp})
}

func isDigit(r rune) bool    { return r >= '0' && r <= '9' }
func isAlpha(r rune) bool    { return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') }
func isAlphaNum(r rune) bool { return isAlpha(r) || isDigit(r) }
func isHex(r rune) bool {
	return isDigit(r) || (r >= 'a' && r <= 'f') || (r >= 'A' && r <= 'F')
}
