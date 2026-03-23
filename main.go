package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const version = "3.5.0"

// ─────────────────────────────────────────────────────────────────────────────
//  Usage / help
// ─────────────────────────────────────────────────────────────────────────────

func printUsage() {
	fmt.Printf(`
%s▒███████▒ ▒██   ██▒
▒ ▒ ▒ ▄▀░ ▒▒ █ █ ▒░
░ ▒ ▄▀▒░  ░░  █   ░
  ▄▀▒   ░  ░ █ █  ░
▒███████▒ ▒██▒ ▒██▒%s  %sv%s%s

%sZX%s — a fast, safe, Perl-flavored language that compiles to C

%sUSAGE:%s
  zxc <file.zx>              compile and run
  zxc build <file.zx>        compile to binary
  zxc build <file.zx> -o x   compile to a named binary
  zxc build <file.zx> -O2    compile with optimizations
  zxc emit  <file.zx>        print the generated C source
  zxc check <file.zx>        type-check only, no output
  zxc test  <file.zx>        run all @test-annotated functions
  zxc repl                   start the interactive REPL
  zxc -c "code"              run a one-liner snippet
  zxc mods                   list all stdlib modules
  zxc version                print version

%sOPTIONS:%s
  -o <name>   output binary name (used with build)
  -O0         no optimization (default)
  -O1         basic optimizations
  -O2         full optimizations (recommended for release)
  -O3         aggressive optimizations (may increase binary size)
  -Os         optimize for binary size
  -Oz         optimize aggressively for binary size (clang only)
  -v          verbose: print generated C before compiling
  -c "code"   execute a one-liner ZX snippet
`,
		colorCyan, colorReset,
		colorBold+colorYellow, version, colorReset,
		colorBold+colorCyan, colorReset,
		colorBold, colorReset,
		colorBold, colorReset,
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
//  Main
// ─────────────────────────────────────────────────────────────────────────────

func main() {
	args := os.Args[1:]

	if len(args) == 0 {
		// no arguments → drop into the REPL
		RunREPL()
		return
	}

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
	case "repl", "--repl", "-i":
		RunREPL()
		return
	}

	// ── parse subcommand + flags ───────────────────────────────────────────────
	cmd := "run"
	outBin := ""
	verbose := false
	oneLiner := ""
	optLevel := "" // "" = no -O flag passed → gcc defaults to -O0
	var sourceFile string

	i := 0
	switch args[0] {
	case "build", "emit", "check", "test":
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
				fmt.Fprintln(os.Stderr, colorRed+"zxc: -o requires a filename"+colorReset)
				os.Exit(1)
			}
		case "-O", "-O0", "-O1", "-O2", "-O3", "-Os", "-Oz":
			optLevel = args[i]
			// -O alone means -O1 (same as gcc convention)
			if optLevel == "-O" {
				optLevel = "-O1"
			}
		case "-v", "--verbose":
			verbose = true
		case "-c", "--cmd", "-e", "--eval":
			i++
			if i < len(args) {
				oneLiner = strings.Join(args[i:], " ")
				i = len(args)
			} else {
				fmt.Fprintln(os.Stderr, colorRed+"zxc: -c requires a code snippet"+colorReset)
				os.Exit(1)
			}
		default:
			if sourceFile == "" {
				sourceFile = args[i]
			}
		}
	}

	// ── get source ────────────────────────────────────────────────────────────
	var src string

	if oneLiner != "" {
		src = wrapOneLiner(oneLiner)
		sourceFile = "<one-liner>"
	} else {
		if sourceFile == "" {
			fmt.Fprintln(os.Stderr, colorRed+"zxc: no input file specified"+colorReset)
			fmt.Fprintln(os.Stderr, "     run 'zxc --help' or try: zxc -c \"say 'hello'\"")
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

	// ── test mode ─────────────────────────────────────────────────────────────
	if cmd == "test" {
		RunTests(program, sourceFile, verbose)
		return
	}

	// ── check only ────────────────────────────────────────────────────────────
	if cmd == "check" {
		fmt.Printf("\n%s%s✓%s  %s — no errors found\n\n", colorBold, colorGreen, colorReset, sourceFile)
		tests := CollectTests(program)
		if len(tests) > 0 {
			fmt.Printf("  %s%d @test function(s) found — run 'zxc test %s' to execute them%s\n\n",
				colorCyan, len(tests), sourceFile, colorReset)
		}
		return
	}

	// ── emit C ────────────────────────────────────────────────────────────────
	cCode := Emit(program)

	if cmd == "emit" {
		fmt.Println(cCode)
		return
	}

	if verbose {
		printCSourceMain(cCode)
	}

	// ── compile ───────────────────────────────────────────────────────────────
	tmpC, err := os.CreateTemp("", "zx_*.c")
	if err != nil {
		fatalf("cannot create temp C file: %v", err)
	}
	defer os.Remove(tmpC.Name())

	if _, err := tmpC.WriteString(cCode); err != nil {
		fatalf("cannot write temp C file: %v", err)
	}
	tmpC.Close()

	if outBin == "" {
		if oneLiner != "" {
			outBin = filepath.Join(os.TempDir(), "zx_one")
		} else {
			base := strings.TrimSuffix(filepath.Base(sourceFile), filepath.Ext(sourceFile))
			outBin = "./" + base
		}
	}

	gccArgs := []string{
		"-x", "c", tmpC.Name(), "-o", outBin,
		"-lm", "-Wall",
		"-Wno-unused-variable",
		"-Wno-unused-but-set-variable",
		"-Wno-implicit-function-declaration",
		"-Wno-unused-function",
	}

	// ── optimization level ────────────────────────────────────────────────────
	// Default: -O0 (no optimizations, fast compile, best for debug)
	// The user chooses via -O0 / -O1 / -O2 / -O3 / -Os / -Oz
	if optLevel == "" {
		gccArgs = append(gccArgs, "-O0")
	} else {
		gccArgs = append(gccArgs, optLevel)
	}

	// For release-level optimization, add extra flags that consistently help:
	//   -fomit-frame-pointer  — frees a register in hot code
	//   -fstrict-aliasing     — enables alias-based optimizations (safe: our
	//                           generated C never violates strict aliasing)
	//   -funroll-loops        — unroll small counted loops (good for -O3)
	switch optLevel {
	case "-O2":
		gccArgs = append(gccArgs,
			"-fomit-frame-pointer",
			"-fstrict-aliasing",
		)
	case "-O3":
		gccArgs = append(gccArgs,
			"-fomit-frame-pointer",
			"-fstrict-aliasing",
			"-funroll-loops",
		)
	case "-Os", "-Oz":
		gccArgs = append(gccArgs,
			"-ffunction-sections",
			"-fdata-sections",
			"-Wl,--gc-sections",
		)
	}

	// In verbose mode, tell the user what optimization is active
	if verbose && optLevel != "" {
		fmt.Printf("%s  optimization:%s %s\n", colorDim, colorReset, optLevel)
	}

	gccOut, err := exec.Command("gcc", gccArgs...).CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "\n%s%s gcc compilation error%s — the generated C has a problem:\n\n",
			colorBold, colorRed, colorReset)
		for _, line := range strings.Split(strings.TrimSpace(string(gccOut)), "\n") {
			fmt.Fprintf(os.Stderr, "  %s%s%s\n", colorDim, line, colorReset)
		}
		fmt.Fprintf(os.Stderr, "\n%s run 'zxc emit %s' to inspect the generated C%s\n\n",
			colorYellow, sourceFile, colorReset)
		os.Exit(1)
	}

	if cmd == "build" {
		optTag := ""
		if optLevel != "" {
			optTag = fmt.Sprintf(" %s(%s)%s", colorDim, optLevel, colorReset)
		}
		fmt.Printf("\n%s%s✓%s  built → %s%s%s%s\n\n",
			colorBold, colorGreen, colorReset, colorCyan, outBin, colorReset, optTag)
		return
	}

	// ── run ───────────────────────────────────────────────────────────────────
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
		fmt.Fprintf(os.Stderr, "%szxc: program exited with error: %v%s\n", colorRed, err, colorReset)
		os.Exit(1)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
//  Helpers
// ─────────────────────────────────────────────────────────────────────────────

func wrapOneLiner(code string) string {
	var imports []string
	if containsAny(code, "sqrt", "pow", "sin", "cos", "log", "floor", "ceil", "fmod") {
		imports = append(imports, "use std::math")
	}
	if containsAny(code, "run(", "run_ok(", "sleep(", "setenv(") {
		imports = append(imports, "use std::sys")
	}
	if containsAny(code, "str_len", "str_cmp", "str_cat", "str_find", "is_alpha", "is_digit") {
		imports = append(imports, "use std::str")
	}
	if containsAny(code, "int_to_str", "float_to_str", "str_to_int") {
		imports = append(imports, "use std::conv")
	}
	if containsAny(code, "tcp_server", "tcp_accept") {
		imports = append(imports, "use std::net")
	}
	if containsAny(code, "fs::read", "fs::write", "read(", "write(", "exists(") {
		imports = append(imports, "use std::fs")
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

func printSummary(sourceFile string) {
	errs, warns, funnies := 0, 0, 0
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
		fmt.Fprintf(os.Stderr, "\n%s%s aborting:%s %s\n\n",
			colorBold, colorRed, colorReset, strings.Join(parts, " and "))
	}
	if errs > 0 && sourceFile != "" && sourceFile != "<one-liner>" {
		fmt.Fprintf(os.Stderr, "  %s fix errors and re-run: zxc %s%s\n\n",
			colorDim, sourceFile, colorReset)
	}
}

func printCSourceMain(code string) {
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
