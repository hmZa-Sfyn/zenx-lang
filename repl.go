package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode"

	"golang.org/x/term"
)

// ─────────────────────────────────────────────────────────────────────────────
//  REPL  — interactive ZX shell
//
//  Features:
//   • Persistent session state: variables, functions, structs defined in
//     earlier lines stay available for the rest of the session.
//   • Multi-line input: open braces / parens auto-continue the prompt.
//   • History: up/down arrows navigate previous inputs (in-process, no file).
//   • Special commands: :help  :clear  :reset  :vars  :fns  :quit
//   • Expression shorthand: bare expressions are wrapped in say() so their
//     value prints without the user having to type say every time.
//   • Imports carry over: `use std::math` once, then use all session.
// ─────────────────────────────────────────────────────────────────────────────

const (
	replPrompt     = "zx> "
	replPromptCont = "... "
	replVersion    = "0.6"
)

// replSession accumulates the persistent parts of the session so every new
// snippet is compiled in the context of everything typed before.
type replSession struct {
	// top-level declarations that persist (fn, struct, extern, use, our, mod)
	decls []string
	// history of raw input lines for up/down navigation
	history []string
	// temp dir for generated C files
	tmpDir string
}

func newReplSession() *replSession {
	dir, err := os.MkdirTemp("", "zx_repl_*")
	if err != nil {
		dir = os.TempDir()
	}
	return &replSession{tmpDir: dir}
}

func (s *replSession) cleanup() {
	os.RemoveAll(s.tmpDir)
}

// ─────────────────────────────────────────────────────────────────────────────
//  Entry point called from main when the user runs  zxc repl  or  zxc
// ─────────────────────────────────────────────────────────────────────────────

func RunREPL() {
	sess := newReplSession()
	defer sess.cleanup()

	printReplBanner()

	// Try raw-terminal mode for arrow-key history; fall back to plain readline.
	if term.IsTerminal(int(os.Stdin.Fd())) {
		runReplRaw(sess)
	} else {
		runReplPlain(sess)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
//  Plain (non-TTY) mode — simple line reader, no arrow keys
// ─────────────────────────────────────────────────────────────────────────────

func runReplPlain(sess *replSession) {
	var buf strings.Builder
	cont := false

	for {
		if cont {
			fmt.Fprint(os.Stdout, replPromptCont)
		} else {
			fmt.Fprint(os.Stdout, replPrompt)
		}

		line, err := readLine()
		if err != nil {
			fmt.Println()
			break
		}

		result, done := sess.feedLine(line, &buf, &cont)
		if done {
			if result == replQuit {
				break
			}
			if result != "" {
				fmt.Println(result)
			}
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
//  Raw TTY mode — supports arrow keys, backspace, history
// ─────────────────────────────────────────────────────────────────────────────

func runReplRaw(sess *replSession) {
	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		runReplPlain(sess)
		return
	}
	defer term.Restore(int(os.Stdin.Fd()), oldState)

	t := term.NewTerminal(os.Stdin, replPrompt)

	var buf strings.Builder
	cont := false

	for {
		if cont {
			t.SetPrompt(replPromptCont)
		} else {
			t.SetPrompt(replPrompt)
		}

		line, err := t.ReadLine()
		if err != nil {
			fmt.Println()
			break
		}

		// feed history into terminal so up/down works
		if strings.TrimSpace(line) != "" && !cont {
			sess.history = append(sess.history, line)
		}

		result, done := sess.feedLine(line, &buf, &cont)
		if done {
			if result == replQuit {
				break
			}
			if result != "" {
				t.Write([]byte(result + "\r\n"))
			}
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
//  Line feeding — shared by both modes
// ─────────────────────────────────────────────────────────────────────────────

const replQuit = "\x00QUIT"

// feedLine accepts one line of input, accumulates multi-line blocks in buf,
// and returns (output, ready) where ready=true means the input is complete.
func (s *replSession) feedLine(line string, buf *strings.Builder, cont *bool) (string, bool) {
	trimmed := strings.TrimSpace(line)

	// ── special commands (only at top level, not inside a block) ─────────────
	if !*cont {
		if out, handled := handleSpecialCmd(trimmed, s); handled {
			return out, true
		}
	}

	buf.WriteString(line)
	buf.WriteByte('\n')

	// decide if we need more lines
	src := buf.String()
	if isIncomplete(src) {
		*cont = true
		return "", false
	}

	// input is complete — compile and run it
	*cont = false
	result := s.evalSnippet(strings.TrimSpace(src))
	buf.Reset()
	return result, true
}

// ─────────────────────────────────────────────────────────────────────────────
//  Special REPL commands
// ─────────────────────────────────────────────────────────────────────────────

func handleSpecialCmd(line string, s *replSession) (string, bool) {
	cmd := strings.ToLower(strings.TrimPrefix(line, ":"))
	switch {
	case line == "" || line == ":":
		return "", true

	case line == ":quit" || line == ":q" || line == ":exit" || line == "exit" || line == "quit":
		fmt.Println("\nbye!")
		return replQuit, true

	case line == ":help" || line == ":h" || line == "?":
		return replHelp(), true

	case line == ":clear" || line == ":cls":
		// ANSI clear screen
		return "\033[2J\033[H", true

	case line == ":reset":
		s.decls = nil
		return "session reset — all declarations cleared", true

	case line == ":vars":
		return s.listDecls("our", "let", "const"), true

	case line == ":fns":
		return s.listDecls("fn ", "sub "), true

	case line == ":imports" || line == ":use":
		return s.listDecls("use "), true

	case line == ":structs":
		return s.listDecls("type ", "struct "), true

	case strings.HasPrefix(line, ":load "):
		path := strings.TrimSpace(strings.TrimPrefix(line, ":load "))
		return s.loadFile(path), true

	case line == ":history":
		return s.showHistory(), true

	case cmd == "clear" || cmd == "reset" || cmd == "quit" ||
		cmd == "vars" || cmd == "fns" || cmd == "help":
		// same commands without the colon
		return handleSpecialCmd(":"+cmd, s)
	}
	return "", false
}

func replHelp() string {
	return strings.Join([]string{
		"",
		colorBold + "ZX REPL v" + replVersion + colorReset,
		"",
		"  Type any ZX expression, statement, or declaration.",
		"  Bare expressions are printed automatically: 1 + 2  →  3",
		"",
		colorBold + "Commands" + colorReset,
		"  :help        show this message",
		"  :clear        clear the screen",
		"  :reset        wipe all session declarations",
		"  :vars         list session variables",
		"  :fns          list session functions",
		"  :imports      list active imports",
		"  :structs      list declared structs",
		"  :history      show input history",
		"  :load <file>  load and execute a .zx file",
		"  :quit         exit the REPL",
		"",
		colorBold + "Tips" + colorReset,
		"  use std::math          import stays for the whole session",
		"  fn add(a int, b int) -> int { return a + b; }",
		"  add(3, 4)              →  7",
		"  let x = 10; x * x     →  100",
		"",
	}, "\n")
}

// ─────────────────────────────────────────────────────────────────────────────
//  Core: compile and run a snippet
// ─────────────────────────────────────────────────────────────────────────────

func (s *replSession) evalSnippet(src string) string {
	if src == "" {
		return ""
	}

	// ── classify the snippet ──────────────────────────────────────────────────
	kind := classifySnippet(src)

	switch kind {
	case snippetDecl:
		// persist the declaration; run nothing (no output expected)
		return s.persistDecl(src)

	case snippetImport:
		return s.persistDecl(src)

	case snippetStmt:
		// wrap in main body and compile+run
		return s.compileAndRun(src, false)

	case snippetExpr:
		// wrap in say() so the value prints
		wrapped := fmt.Sprintf("say %s;", src)
		return s.compileAndRun(wrapped, false)
	}

	return ""
}

type snippetKind int

const (
	snippetExpr snippetKind = iota
	snippetStmt
	snippetDecl
	snippetImport
)

// classifySnippet decides how to handle the input.
func classifySnippet(src string) snippetKind {
	trimmed := strings.TrimSpace(src)

	// imports
	if strings.HasPrefix(trimmed, "use ") || strings.HasPrefix(trimmed, "import ") {
		return snippetImport
	}

	// top-level declarations
	declPrefixes := []string{
		"fn ", "sub ", "func ",
		"type ", "struct ",
		"extern ",
		"mod ",
		"macro ",
		"our ", "const ",
	}
	for _, p := range declPrefixes {
		if strings.HasPrefix(trimmed, p) {
			return snippetDecl
		}
	}

	// method declarations:  fn (recv Type) name(...) { ... }
	if strings.HasPrefix(trimmed, "fn (") || strings.HasPrefix(trimmed, "sub (") {
		return snippetDecl
	}

	// statements that produce no printable value
	stmtPrefixes := []string{
		"let ", "my ", "local ",
		"say ", "print", "println", "warn ", "eprint",
		"if ", "while ", "until ", "for ", "foreach ",
		"match ", "try ", "return ", "defer ",
		"unless ", "exit ", "die ", "throw ", "raise ",
		"assert ", "spawn ",
	}
	for _, p := range stmtPrefixes {
		if strings.HasPrefix(trimmed, p) {
			return snippetStmt
		}
	}

	// assignment:  x = ...   x += ...   arr[i] = ...
	if isAssignment(trimmed) {
		return snippetStmt
	}

	// block  { ... }
	if strings.HasPrefix(trimmed, "{") {
		return snippetStmt
	}

	// everything else: treat as expression and print its value
	return snippetExpr
}

// isAssignment returns true if the line looks like an assignment statement.
func isAssignment(s string) bool {
	ops := []string{" = ", " += ", " -= ", " *= ", " /= ", " %= "}
	for _, op := range ops {
		if idx := strings.Index(s, op); idx > 0 {
			lhs := strings.TrimSpace(s[:idx])
			// must not start with a keyword that could contain =
			bad := []string{"fn", "if", "while", "for", "let", "const", "our"}
			for _, b := range bad {
				if strings.HasPrefix(lhs, b) {
					return false
				}
			}
			return true
		}
	}
	return false
}

// ─────────────────────────────────────────────────────────────────────────────
//  persistDecl: add a declaration to the session and verify it compiles
// ─────────────────────────────────────────────────────────────────────────────

func (s *replSession) persistDecl(src string) string {
	// tentatively add the declaration
	candidate := append(append([]string{}, s.decls...), src)

	// check it compiles cleanly (build a dummy program that just declares them)
	testSrc := buildSessionProgram(candidate, "")
	if err := s.tryCompile(testSrc); err != nil {
		return colorRed + colorBold + "error" + colorReset + ": " + err.Error()
	}

	s.decls = candidate
	return "" // silent success — just like declaring a fn in Go REPL
}

// ─────────────────────────────────────────────────────────────────────────────
//  compileAndRun: build full program, compile with gcc, execute, return output
// ─────────────────────────────────────────────────────────────────────────────

func (s *replSession) compileAndRun(stmts string, silent bool) string {
	fullSrc := buildSessionProgram(s.decls, stmts)

	// ── tokenize + parse + typecheck ─────────────────────────────────────────
	resetDiags()
	tokens := Tokenize(fullSrc, "<repl>")
	if tokens == nil || hadError {
		return s.collectDiags()
	}

	prog := Parse(tokens, fullSrc, "<repl>")
	if prog == nil || hadError {
		return s.collectDiags()
	}

	if !TypeCheck(prog, fullSrc, "<repl>") || hadError {
		return s.collectDiags()
	}

	// ── emit C ───────────────────────────────────────────────────────────────
	cSrc := Emit(prog)

	// write to temp file
	cFile := filepath.Join(s.tmpDir, "repl_snippet.c")
	binFile := filepath.Join(s.tmpDir, "repl_snippet")
	if err := os.WriteFile(cFile, []byte(cSrc), 0600); err != nil {
		return fmt.Sprintf("internal error: %v", err)
	}

	// ── gcc compile ──────────────────────────────────────────────────────────
	gccArgs := []string{
		"-o", binFile, cFile,
		"-lm", "-w", // -w silences gcc warnings in REPL mode
		"-O0",
	}
	gccOut, err := exec.Command("gcc", gccArgs...).CombinedOutput()
	if err != nil {
		msg := strings.TrimSpace(string(gccOut))
		// strip temp file path noise
		msg = strings.ReplaceAll(msg, cFile+":", "line ")
		return colorRed + colorBold + "compile error" + colorReset + ": " + msg
	}

	// ── run ──────────────────────────────────────────────────────────────────
	runCmd := exec.Command(binFile)
	runCmd.Stdin = os.Stdin
	out, err := runCmd.CombinedOutput()
	result := strings.TrimRight(string(out), "\n")

	if err != nil {
		if result != "" {
			return result + "\n" + colorRed + "process exited with error" + colorReset
		}
		return colorRed + "process exited with error" + colorReset
	}

	return result
}

// tryCompile checks a generated C source compiles without running it.
func (s *replSession) tryCompile(zxSrc string) error {
	resetDiags()
	tokens := Tokenize(zxSrc, "<repl>")
	if tokens == nil || hadError {
		msg := s.collectDiags()
		return fmt.Errorf("%s", stripAnsi(msg))
	}
	prog := Parse(tokens, zxSrc, "<repl>")
	if prog == nil || hadError {
		msg := s.collectDiags()
		return fmt.Errorf("%s", stripAnsi(msg))
	}
	if !TypeCheck(prog, zxSrc, "<repl>") || hadError {
		msg := s.collectDiags()
		return fmt.Errorf("%s", stripAnsi(msg))
	}
	return nil
}

// collectDiags returns all pending diagnostics as a string and resets state.
func (s *replSession) collectDiags() string {
	// diagnostics were already printed to stderr by printDiag;
	// return empty so we don't double-print in plain mode.
	// In raw mode the terminal captures stderr separately.
	return ""
}

// ─────────────────────────────────────────────────────────────────────────────
//  buildSessionProgram assembles a complete ZX source string from the
//  accumulated declarations + the new statements to run.
// ─────────────────────────────────────────────────────────────────────────────

func buildSessionProgram(decls []string, stmts string) string {
	var sb strings.Builder

	// separate imports from other decls
	var imports []string
	var others []string
	for _, d := range decls {
		t := strings.TrimSpace(d)
		if strings.HasPrefix(t, "use ") || strings.HasPrefix(t, "import ") {
			imports = append(imports, d)
		} else {
			others = append(others, d)
		}
	}

	for _, imp := range imports {
		sb.WriteString(imp)
		sb.WriteByte('\n')
	}
	if len(imports) > 0 {
		sb.WriteByte('\n')
	}

	for _, decl := range others {
		sb.WriteString(decl)
		sb.WriteByte('\n')
	}
	if len(others) > 0 {
		sb.WriteByte('\n')
	}

	if stmts != "" {
		sb.WriteString(stmts)
		sb.WriteByte('\n')
	}

	return sb.String()
}

// ─────────────────────────────────────────────────────────────────────────────
//  isIncomplete — decide if we need more input lines
//
//  Rules (conservative):
//   • Count unmatched { } ( ) — if open > close, keep reading.
//   • A trailing backslash continues the line.
//   • Strings are skipped so braces inside strings don't count.
// ─────────────────────────────────────────────────────────────────────────────

func isIncomplete(src string) bool {
	trimmed := strings.TrimSpace(src)
	if trimmed == "" {
		return false
	}

	// trailing backslash
	if strings.HasSuffix(trimmed, "\\") {
		return true
	}

	// count unmatched delimiters (skip string contents)
	braces, parens, brackets := 0, 0, 0
	inStr := rune(0)
	escape := false
	runes := []rune(src)

	for i, r := range runes {
		if escape {
			escape = false
			continue
		}
		if r == '\\' && inStr != 0 {
			escape = true
			continue
		}
		if inStr != 0 {
			if r == inStr {
				inStr = 0
			}
			continue
		}
		// skip line comments
		if r == '#' || (r == '/' && i+1 < len(runes) && runes[i+1] == '/') {
			// skip to end of line
			break
		}
		switch r {
		case '"', '\'', '`':
			inStr = r
		case '{':
			braces++
		case '}':
			braces--
		case '(':
			parens++
		case ')':
			parens--
		case '[':
			brackets++
		case ']':
			brackets--
		}
	}

	return braces > 0 || parens > 0 || brackets > 0
}

// ─────────────────────────────────────────────────────────────────────────────
//  Session introspection helpers
// ─────────────────────────────────────────────────────────────────────────────

func (s *replSession) listDecls(prefixes ...string) string {
	if len(s.decls) == 0 {
		return "(none)"
	}
	var lines []string
	for _, d := range s.decls {
		t := strings.TrimSpace(d)
		for _, p := range prefixes {
			if strings.HasPrefix(t, p) {
				// show only the first line (signature)
				first := strings.SplitN(t, "\n", 2)[0]
				lines = append(lines, "  "+first)
				break
			}
		}
	}
	if len(lines) == 0 {
		return "(none)"
	}
	return strings.Join(lines, "\n")
}

func (s *replSession) showHistory() string {
	if len(s.history) == 0 {
		return "(no history)"
	}
	var sb strings.Builder
	for i, h := range s.history {
		sb.WriteString(fmt.Sprintf("  %3d  %s\n", i+1, h))
	}
	return strings.TrimRight(sb.String(), "\n")
}

func (s *replSession) loadFile(path string) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return colorRed + "cannot read file: " + err.Error() + colorReset
	}
	src := string(raw)

	// split into top-level declarations and run-level statements heuristically:
	// everything that looks like a decl goes into decls, the rest is run.
	lines := strings.Split(src, "\n")
	var declLines, stmtLines []string
	for _, l := range lines {
		t := strings.TrimSpace(l)
		if t == "" || strings.HasPrefix(t, "#") || strings.HasPrefix(t, "//") {
			continue
		}
		if isTopLevelDecl(t) {
			declLines = append(declLines, l)
		} else {
			stmtLines = append(stmtLines, l)
		}
	}

	var out strings.Builder
	if len(declLines) > 0 {
		r := s.persistDecl(strings.Join(declLines, "\n"))
		if r != "" {
			out.WriteString(r + "\n")
		}
	}
	if len(stmtLines) > 0 {
		r := s.compileAndRun(strings.Join(stmtLines, "\n"), false)
		if r != "" {
			out.WriteString(r)
		}
	}
	if out.Len() == 0 {
		return colorGreen + "loaded " + filepath.Base(path) + colorReset
	}
	return strings.TrimRight(out.String(), "\n")
}

func isTopLevelDecl(line string) bool {
	for _, p := range []string{
		"fn ", "sub ", "func ",
		"type ", "struct ",
		"extern ", "mod ", "macro ",
		"our ", "const ",
		"use ", "import ",
		"fn (", "sub (",
	} {
		if strings.HasPrefix(line, p) {
			return true
		}
	}
	return false
}

// ─────────────────────────────────────────────────────────────────────────────
//  Banner
// ─────────────────────────────────────────────────────────────────────────────

func printReplBanner() {
	fmt.Printf("\n%sZX%s %sv%s%s — interactive REPL\n",
		colorBold+colorCyan, colorReset,
		colorDim, replVersion, colorReset)
	fmt.Printf("type %s:help%s for commands, %s:quit%s to exit\n\n",
		colorBold, colorReset, colorBold, colorReset)
}

// ─────────────────────────────────────────────────────────────────────────────
//  Low-level helpers
// ─────────────────────────────────────────────────────────────────────────────

// readLine reads one line from stdin without a terminal library.
func readLine() (string, error) {
	var buf []byte
	b := make([]byte, 1)
	for {
		n, err := os.Stdin.Read(b)
		if n > 0 {
			if b[0] == '\n' {
				break
			}
			buf = append(buf, b[0])
		}
		if err != nil {
			if len(buf) > 0 {
				return string(buf), nil
			}
			return "", err
		}
	}
	return string(buf), nil
}

// stripAnsi removes ANSI escape codes from a string (for error messages that
// need to be passed to fmt.Errorf without colour noise).
func stripAnsi(s string) string {
	var out strings.Builder
	inEsc := false
	for _, r := range s {
		if r == '\033' {
			inEsc = true
			continue
		}
		if inEsc {
			if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
				inEsc = false
			}
			continue
		}
		if unicode.IsPrint(r) || r == '\n' || r == '\t' {
			out.WriteRune(r)
		}
	}
	return out.String()
}
