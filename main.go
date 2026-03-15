package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const version = "0.4.0"

const banner = `
 ▒███████▒ ▒██   ██▒
▒ ▒ ▒ ▄▀░ ▒▒ █ █ ▒░
░ ▒ ▄▀▒░  ░░  █   ░
  ▄▀▒   ░  ░ █ █  ░
▒███████▒ ▒██▒ ▒██▒`

const usage = banner + `  v` + version + `

ZX — a fast, Perl-flavored language that compiles to C

USAGE:
  zxc <file.zx>              compile & run
  zxc build <file.zx>        compile to binary
  zxc build <file.zx> -o x   compile to named binary
  zxc emit  <file.zx>        print generated C source
  zxc check <file.zx>        type-check only
  zxc -c "code"              run a one-liner
  zxc version                print version

OPTIONS:
  -o <name>   output binary name (with build)
  -v          verbose: show generated C before compiling
  -c "code"   run one-liner ZX code directly

ONE-LINER EXAMPLES:
  zxc -c "say 'Hello, World!'"
  zxc -c "for i in 0..10 { say i }"
  zxc -c "say system('ls -la')"
  zxc -c "say max(3, 7)"

STD MODULES:
  use std::str   — string functions (str_len, str_cmp, str_cat, ...)
  use std::math  — math functions (sqrt, pow, sin, cos, ...)
  use std::io    — file I/O (open, close, read, write, ...)
  use std::sys   — system calls (system, getenv, sleep, ...)
  use std::conv  — conversions (to_int, to_float, int_to_str)
  use std::mem   — memory (alloc, free, copy, set)
  use std::time  — time (now, clock, diff)
  use std::fmt   — formatting (print, format)
  use std::os    — environment (getenv, argc, args)

BUILTIN FUNCTIONS (no import needed):
  len(s)         string length
  abs(x)         absolute value
  min(a, b)      minimum
  max(a, b)      maximum
  to_int(x)      cast to int
  to_float(x)    cast to float
  to_bool(x)     cast to bool
  to_char(x)     cast to char
  is_nil(x)      check for nil/NULL
  alloc(n)       allocate n bytes
  free(p)        free memory
  sizeof(T)      size of type
  system(cmd)    run shell command
  getenv(k)      get env variable
  str_eq(a, b)   compare strings

POINTER SYNTAX:
  @ expr         address-of  (friendly & replacement)
  ^ expr         dereference (friendly * replacement)
  ref T          pointer type (friendly *T replacement)
  p->field       field access via pointer (preferred)
  p.field        also works (but you'll get a funny warning 😄)

PIPE OPERATOR:
  value |> fn    passes value as first arg to fn
  value |> fn1 |> fn2   chains: fn2(fn1(value))
`

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		fmt.Print(usage)
		os.Exit(0)
	}
	switch args[0] {
	case "version", "--version":
		fmt.Printf("zxc v%s\n", version)
		return
	case "help", "--help", "-h":
		fmt.Print(usage)
		return
	}

	// ── parse flags ───────────────────────────────────────────────────────────
	cmd := "run"
	outBin := ""
	verbose := false
	oneLiner := ""
	var sourceFile string

	i := 0
	if len(args) > 0 && (args[0] == "build" || args[0] == "emit" || args[0] == "check") {
		cmd = args[0]; i = 1
	}
	for ; i < len(args); i++ {
		switch args[i] {
		case "-o":
			i++; if i < len(args) { outBin = args[i] }
		case "-v", "--verbose":
			verbose = true
		case "-c", "--cmd":
			// one-liner mode: rest is the code
			i++
			if i < len(args) {
				oneLiner = strings.Join(args[i:], " ")
				i = len(args) // consume all remaining
			}
		default:
			if sourceFile == "" { sourceFile = args[i] }
		}
	}

	var src string

	if oneLiner != "" {
		// wrap one-liner in a tiny scaffold
		src = wrapOneLiner(oneLiner)
		sourceFile = "<one-liner>"
	} else {
		if sourceFile == "" {
			fmt.Fprintln(os.Stderr, "zxc: no input file")
			fmt.Fprintln(os.Stderr, "     try:  zxc --help")
			os.Exit(1)
		}
		raw, err := os.ReadFile(sourceFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%szxc: cannot read %q: %v%s\n", colorRed, sourceFile, err, colorReset)
			os.Exit(1)
		}
		src = string(raw)
	}

	// ── pipeline ──────────────────────────────────────────────────────────────
	resetDiags()

	tokens := Tokenize(src, sourceFile)
	if tokens == nil || hadError { printSummary(); os.Exit(1) }

	program := Parse(tokens, src, sourceFile)
	if program == nil || hadError { printSummary(); os.Exit(1) }

	ok := TypeCheck(program, src, sourceFile)
	if !ok || hadError { printSummary(); os.Exit(1) }

	if cmd == "check" {
		fmt.Printf("%s%s✓%s %s — no errors\n", colorBold, colorGreen, colorReset, sourceFile)
		return
	}

	cCode := Emit(program)

	if cmd == "emit" { fmt.Println(cCode); return }

	if verbose { printCSource(cCode) }

	// ── compile ───────────────────────────────────────────────────────────────
	tmpC, err := os.CreateTemp("", "zx_*.c")
	if err != nil { fatalf("cannot create temp file: %v", err) }
	defer os.Remove(tmpC.Name())
	tmpC.WriteString(cCode); tmpC.Close()

	if outBin == "" {
		if oneLiner != "" {
			outBin = os.TempDir() + "/zx_oneliner"
		} else {
			base := strings.TrimSuffix(filepath.Base(sourceFile), filepath.Ext(sourceFile))
			outBin = "./" + base
		}
	}

	gccArgs := []string{"-x", "c", tmpC.Name(), "-o", outBin, "-lm",
		"-Wall", "-Wno-unused-variable", "-Wno-unused-but-set-variable", "-Wno-implicit-function-declaration"}
	gccOut, err := exec.Command("gcc", gccArgs...).CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "\n%s%s✖ gcc error%s — the generated C has a problem:\n\n",
			colorBold, colorRed, colorReset)
		fmt.Fprintf(os.Stderr, "%s%s%s\n", colorDim, string(gccOut), colorReset)
		fmt.Fprintf(os.Stderr, "\n%s💡 run 'zxc emit %s' to inspect the generated C%s\n",
			colorYellow, sourceFile, colorReset)
		os.Exit(1)
	}

	if cmd == "build" {
		fmt.Printf("%s%s✓%s built → %s%s%s\n", colorBold, colorGreen, colorReset, colorCyan, outBin, colorReset)
		return
	}

	// one-liner: clean up binary after run
	if oneLiner != "" { defer os.Remove(outBin) }

	// ── run ───────────────────────────────────────────────────────────────────
	runCmd := exec.Command(outBin)
	runCmd.Stdin = os.Stdin; runCmd.Stdout = os.Stdout; runCmd.Stderr = os.Stderr
	if err := runCmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok { os.Exit(exitErr.ExitCode()) }
		os.Exit(1)
	}
}

// wrapOneLiner wraps a one-liner snippet in minimal ZX scaffolding
func wrapOneLiner(code string) string {
	// auto-add common std imports if code uses them
	var imports []string
	if strings.Contains(code, "sqrt") || strings.Contains(code, "pow") || strings.Contains(code, "sin") {
		imports = append(imports, `use std::math`)
	}
	if strings.Contains(code, "system") || strings.Contains(code, "getenv") {
		imports = append(imports, `use std::sys`)
	}
	if strings.Contains(code, "str_") {
		imports = append(imports, `use std::str`)
	}
	return strings.Join(imports, "\n") + "\n" + code + "\n"
}

func printSummary() {
	errs, warns := 0, 0
	for _, d := range allDiagnostics {
		if d.Sev == SevError { errs++ }
		if d.Sev == SevWarn || d.Sev == SevFunny { warns++ }
	}
	var parts []string
	if errs > 0 { parts = append(parts, fmt.Sprintf("%s%s%d error(s)%s", colorBold, colorRed, errs, colorReset)) }
	if warns > 0 { parts = append(parts, fmt.Sprintf("%s%s%d warning(s)%s", colorBold, colorYellow, warns, colorReset)) }
	if len(parts) > 0 { fmt.Fprintf(os.Stderr, "%saborting: %s%s\n", colorBold, strings.Join(parts, " and "), colorReset) }
}

func printCSource(code string) {
	lines := strings.Split(code, "\n")
	fmt.Printf("%s%s── generated C ──────────────────────────%s\n", colorBold, colorCyan, colorReset)
	for i, l := range lines { fmt.Printf("%s%4d%s  %s\n", colorDim, i+1, colorReset, l) }
	fmt.Printf("%s%s─────────────────────────────────────────%s\n\n", colorBold, colorCyan, colorReset)
}

func fatalf(f string, a ...any) {
	fmt.Fprintf(os.Stderr, "%szxc: "+f+"%s\n", append([]any{colorRed}, append(a, colorReset)...)...)
	os.Exit(1)
}
