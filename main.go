package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const version = "0.5.0"

// ─────────────────────────────────────────────────────────────────────────────
//  Help text
// ─────────────────────────────────────────────────────────────────────────────

func printUsage() {
	fmt.Printf(`
%s▒███████▒ ▒██   ██▒%s
%s▒ ▒ ▒ ▄▀░ ▒▒ █ █ ▒░%s
%s░ ▒ ▄▀▒░  ░░  █   ░%s
%s  ▄▀▒   ░  ░ █ █  ░%s
%s▒███████▒ ▒██▒ ▒██▒%s  %sv%s%s

%sZX%s — a fast, safe, Perl-flavored language that compiles to C

%sUSAGE:%s
  zxc <file.zx>              compile and run
  zxc build <file.zx>        compile to binary (keeps the binary)
  zxc build <file.zx> -o x   compile to a named binary
  zxc emit  <file.zx>        print the generated C source code
  zxc check <file.zx>        type-check only, no output
  zxc -c "code"              run a one-liner snippet directly
  zxc mods                   list all standard library modules
  zxc version                print version

%sOPTIONS:%s
  -o <name>   output binary name (used with build)
  -v          verbose: print generated C before compiling
  -c "code"   execute a one-liner ZX snippet

%sONE-LINER EXAMPLES:%s
  zxc -c "say 'Hello, World!'"
  zxc -c "for i in 0..10 { say i }"
  zxc -c "say max(3, 9)"
  zxc -c "say cmd!('uname -a')"
  zxc -c "say readfile!('/etc/hostname')"
  zxc -c "let s = input('name: '); say f'hello, {s}!'"

%sSTD MODULES%s  (add at top of file with: use std::name)
  std::str     string ops  — str_len, str_cmp, str_cat, str_find, str_dup ...
  std::io      file I/O    — open, close, read, write, printf, scanf ...
  std::math    math        — sqrt, pow, sin, cos, log, floor, ceil, fmod ...
  std::sys     system      — run, run_ok, sleep, getenv, setenv, getpid ...
  std::fs      easy files  — read, write, append, exists, remove, rename ...
  std::cmd     shell       — capture, run, exitcode, popen, pclose ...
  std::mem     memory      — alloc, zalloc, realloc, free, copy, set ...
  std::conv    conversion  — to_int, to_float, int_to_str, float_to_str ...
  std::time    time        — now, clock, diff
  std::fmt     formatting  — print, eprint, sprintf
  std::net     networking  — tcp_server, tcp_accept, tcp_send, close_fd ...
  std::os      environment — getenv, exit

%sBUILTIN FUNCTIONS%s  (always available, no import needed)
  len(s)          string length (int)
  abs(x)          absolute value
  min(a, b)       minimum of two values
  max(a, b)       maximum of two values
  clamp(v,lo,hi)  clamp value to range
  to_int(x)       cast to int
  to_float(x)     cast to float
  to_bool(x)      cast to bool
  to_char(x)      cast to char
  is_nil(x)       check for null/nil  →  bool
  not_nil(x)      check not null      →  bool
  is_zero(x)      check for zero      →  bool
  alloc(n)        malloc n bytes      →  ref void
  free(p)         free allocated memory
  str_eq(a, b)    string equality     →  bool
  str_ne(a, b)    string inequality   →  bool
  system(cmd)     run shell command   →  int (exit code)
  getenv(k)       get env variable    →  str
  sizeof(T)       size of type        →  int

%sSPECIAL SYNTAX:%s
  f"hello {name}!"        template / interpolated strings
  cmd!("ls -la")          run shell command, capture output as str
  readfile!("path")       read entire file into a str
  writefile!("p", s)      write str to file
  input("prompt: ")       read a line from stdin
  value |> fn |> fn2      pipe operator: fn2(fn(value))
  cond ? then : else      ternary expression
  match x { 1 => {} }    pattern matching
  try { } catch (e) {}   errno-based error handling
  defer fn()              run at end of current scope
  assert cond, "msg"      runtime assertion (aborts on failure)
  @Struct{ field: val }   heap-allocate a struct (returns ref Struct)
  ^ expr                  dereference a ref  (friendly * replacement)
  @ expr                  take address        (friendly & replacement)
  ref T                   pointer type        (friendly *T replacement)
  ->                      field access through ref (preferred)
  .                       also works on refs (with a funny warning 😄)

%sPOINTER QUICK GUIDE:%s
  Old C style   →   ZX style
  *T            →   ref T
  &x            →   @x
  *p            →   ^p
  p->field      →   p->field  (or p.field with a warning)

`,
		colorCyan, colorReset,
		colorCyan, colorReset,
		colorCyan, colorReset,
		colorCyan, colorReset,
		colorCyan, colorReset,
		colorBold+colorYellow, version, colorReset,
		colorBold+colorCyan, colorReset,
		colorBold, colorReset,
		colorBold, colorReset,
		colorBold, colorReset,
		colorBold+colorGreen, colorReset,
		colorBold+colorGreen, colorReset,
		colorBold+colorYellow, colorReset,
		colorBold+colorYellow, colorReset,
	)
}

func printMods() {
	fmt.Printf("\n%sZX Standard Library Modules%s\n\n", colorBold+colorCyan, colorReset)
	for name, mod := range stdModules {
		fmt.Printf("  %s%s%s\n", colorBold+colorGreen, name, colorReset)
		fmt.Printf("    Headers: %s\n", strings.Join(mod.Headers, ", "))
		fnNames := make([]string, 0, len(mod.Fns))
		for _, fn := range mod.Fns {
			fnNames = append(fnNames, fn.Name)
		}
		fmt.Printf("    Functions: %s\n\n", strings.Join(fnNames, ", "))
	}
}

// ─────────────────────────────────────────────────────────────────────────────
//  Main entry point
// ─────────────────────────────────────────────────────────────────────────────

func main() {
	args := os.Args[1:]

	if len(args) == 0 {
		printUsage()
		os.Exit(0)
	}

	// top-level subcommands that don't need a file
	switch args[0] {
	case "version", "--version", "-V":
		fmt.Printf("zxc v%s\n", version)
		return
	case "help", "--help", "-h":
		printUsage()
		return
	case "mods", "modules", "stdlib":
		printMods()
		return
	}

	// ── parse flags ───────────────────────────────────────────────────────────
	cmd := "run"
	outBin := ""
	verbose := false
	oneLiner := ""
	var sourceFile string

	i := 0

	// optional subcommand as first arg
	switch args[0] {
	case "build", "emit", "check":
		cmd = args[0]
		i = 1
	}

	for ; i < len(args); i++ {
		switch args[i] {
		case "-o":
			i++
			if i < len(args) {
				outBin = args[i]
			} else {
				fmt.Fprintln(os.Stderr, colorRed+"zxc: -o requires a filename argument"+colorReset)
				os.Exit(1)
			}

		case "-v", "--verbose":
			verbose = true

		case "-c", "--cmd", "-e", "--eval":
			// everything after -c is the one-liner code
			i++
			if i < len(args) {
				oneLiner = strings.Join(args[i:], " ")
				i = len(args) // consume all remaining args
			} else {
				fmt.Fprintln(os.Stderr, colorRed+"zxc: -c requires a code snippet argument"+colorReset)
				fmt.Fprintln(os.Stderr, `  example: zxc -c "say 'hello'"`)
				os.Exit(1)
			}

		default:
			if sourceFile == "" {
				sourceFile = args[i]
			} else {
				fmt.Fprintf(os.Stderr, colorYellow+"zxc: warning: extra argument %q ignored\n"+colorReset, args[i])
			}
		}
	}

	// ── get source code ───────────────────────────────────────────────────────
	var src string

	if oneLiner != "" {
		src = wrapOneLiner(oneLiner)
		sourceFile = "<one-liner>"
	} else {
		if sourceFile == "" {
			fmt.Fprintln(os.Stderr, colorRed+"zxc: no input file specified"+colorReset)
			fmt.Fprintln(os.Stderr, "     run 'zxc --help' to see usage, or use -c for a one-liner")
			os.Exit(1)
		}

		raw, err := os.ReadFile(sourceFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%szxc: cannot read %q: %v%s\n",
				colorRed, sourceFile, err, colorReset)
			os.Exit(1)
		}
		src = string(raw)
	}

	// ── pipeline: tokenize → parse → typecheck ────────────────────────────────
	resetDiags()

	tokens := Tokenize(src, sourceFile)
	if tokens == nil || hadError {
		printSummary(sourceFile)
		os.Exit(1)
	}

	program := Parse(tokens, src, sourceFile)
	if program == nil || hadError {
		printSummary(sourceFile)
		os.Exit(1)
	}

	ok := TypeCheck(program, src, sourceFile)
	if !ok || hadError {
		printSummary(sourceFile)
		os.Exit(1)
	}

	// ── check only ────────────────────────────────────────────────────────────
	if cmd == "check" {
		fmt.Printf("\n%s%s✓%s  %s — no errors found\n\n",
			colorBold, colorGreen, colorReset, sourceFile)
		return
	}

	// ── emit C ────────────────────────────────────────────────────────────────
	cCode := Emit(program)

	if cmd == "emit" {
		fmt.Println(cCode)
		return
	}

	if verbose {
		printCSource(cCode)
	}

	// ── compile with gcc ──────────────────────────────────────────────────────
	tmpC, err := os.CreateTemp("", "zx_*.c")
	if err != nil {
		fatalf("cannot create temp C file: %v", err)
	}
	defer os.Remove(tmpC.Name())

	if _, err := tmpC.WriteString(cCode); err != nil {
		fatalf("cannot write temp C file: %v", err)
	}
	tmpC.Close()

	// determine output binary path
	if outBin == "" {
		if oneLiner != "" {
			outBin = filepath.Join(os.TempDir(), "zx_oneliner")
		} else {
			base := strings.TrimSuffix(filepath.Base(sourceFile), filepath.Ext(sourceFile))
			outBin = "./" + base
		}
	}

	gccArgs := []string{
		"-x", "c",
		tmpC.Name(),
		"-o", outBin,
		"-lm",
		"-Wall",
		"-Wno-unused-variable",
		"-Wno-unused-but-set-variable",
		"-Wno-implicit-function-declaration",
		"-Wno-unused-function",
	}

	gccOut, err := exec.Command("gcc", gccArgs...).CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "\n%s%s✖ gcc compilation error%s — the generated C has a problem:\n\n",
			colorBold, colorRed, colorReset)
		// print gcc output with dimming
		for _, line := range strings.Split(strings.TrimSpace(string(gccOut)), "\n") {
			fmt.Fprintf(os.Stderr, "  %s%s%s\n", colorDim, line, colorReset)
		}
		fmt.Fprintf(os.Stderr, "\n%s💡 Tip: run 'zxc emit %s' to inspect the generated C code%s\n\n",
			colorYellow, sourceFile, colorReset)
		os.Exit(1)
	}

	// ── build only — don't run ────────────────────────────────────────────────
	if cmd == "build" {
		fmt.Printf("\n%s%s✓%s  built → %s%s%s\n\n",
			colorBold, colorGreen, colorReset,
			colorCyan, outBin, colorReset)
		return
	}

	// ── run ───────────────────────────────────────────────────────────────────
	// clean up the one-liner binary after running
	if oneLiner != "" {
		defer os.Remove(outBin)
	}

	runCmd := exec.Command(outBin)
	runCmd.Stdin = os.Stdin
	runCmd.Stdout = os.Stdout
	runCmd.Stderr = os.Stderr

	if err := runCmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		fmt.Fprintf(os.Stderr, "%szxc: program exited with error: %v%s\n",
			colorRed, err, colorReset)
		os.Exit(1)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
//  One-liner scaffold
// ─────────────────────────────────────────────────────────────────────────────

// wrapOneLiner wraps a one-liner code snippet in the minimal ZX scaffold
// and auto-imports commonly needed std modules based on what it uses.
func wrapOneLiner(code string) string {
	var imports []string

	// auto-detect needed std modules
	if containsAny(code, "sqrt", "pow", "sin", "cos", "log", "floor", "ceil", "fmod", "fabs") {
		imports = append(imports, "use std::math")
	}
	if containsAny(code, "run(", "run_ok(", "sleep(", "setenv(", "getpid(") {
		imports = append(imports, "use std::sys")
	}
	if containsAny(code, "str_len", "str_cmp", "str_cat", "str_find", "str_dup", "is_alpha", "is_digit") {
		imports = append(imports, "use std::str")
	}
	if containsAny(code, "fs::read", "fs::write", "fs::exists") {
		imports = append(imports, "use std::fs")
	}
	if containsAny(code, "int_to_str", "float_to_str", "str_to_int") {
		imports = append(imports, "use std::conv")
	}
	if containsAny(code, "tcp_server", "tcp_accept", "tcp_send") {
		imports = append(imports, "use std::net")
	}

	prefix := strings.Join(imports, "\n")
	if prefix != "" {
		prefix += "\n\n"
	}

	return prefix + code + "\n"
}

func containsAny(s string, substrs ...string) bool {
	for _, sub := range substrs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

// ─────────────────────────────────────────────────────────────────────────────
//  Output helpers
// ─────────────────────────────────────────────────────────────────────────────

func printSummary(sourceFile string) {
	errs := 0
	warns := 0
	funnies := 0

	for _, d := range allDiagnostics {
		switch d.Sev {
		case SevError:
			errs++
		case SevWarn:
			warns++
		case SevFunny:
			funnies++
		}
	}

	var parts []string
	if errs > 0 {
		parts = append(parts, fmt.Sprintf("%s%s%d error(s)%s", colorBold, colorRed, errs, colorReset))
	}
	if warns > 0 {
		parts = append(parts, fmt.Sprintf("%s%s%d warning(s)%s", colorBold, colorYellow, warns, colorReset))
	}
	if funnies > 0 {
		parts = append(parts, fmt.Sprintf("%s%s%d style issue(s)%s", colorBold, colorOrange, funnies, colorReset))
	}

	if len(parts) > 0 {
		fmt.Fprintf(os.Stderr, "\n%s%s aborting%s: %s\n\n",
			colorBold, colorRed, colorReset,
			strings.Join(parts, " and "))
	}

	if errs > 0 && sourceFile != "" && sourceFile != "<one-liner>" {
		fmt.Fprintf(os.Stderr, "  %s💡 run 'zxc check %s' to see only errors%s\n\n",
			colorDim, sourceFile, colorReset)
	}
}

func printCSource(code string) {
	lines := strings.Split(code, "\n")
	fmt.Printf("\n%s%s── generated C (%d lines) ──────────────────────%s\n",
		colorBold, colorCyan, len(lines), colorReset)
	for i, l := range lines {
		fmt.Printf("%s%4d%s  %s\n", colorDim, i+1, colorReset, l)
	}
	fmt.Printf("%s%s────────────────────────────────────────────%s\n\n",
		colorBold, colorCyan, colorReset)
}

func fatalf(format string, args ...any) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(os.Stderr, "%szxc: %s%s\n", colorRed, msg, colorReset)
	os.Exit(1)
}
