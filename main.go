package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const version = "0.2.0"

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
  zxc build <file.zx>        compile only
  zxc emit  <file.zx>        print generated C
  zxc check <file.zx>        type-check only
  zxc version                print version

OPTIONS:
  -o <name>   output binary name (with build)
  -v          verbose: show generated C before compiling

EXAMPLES:
  zxc hello.zx
  zxc build demo.zx -o demo && ./demo
  zxc emit  fib.zx
  zxc check errors.zx
`

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		fmt.Print(usage)
		os.Exit(0)
	}

	switch args[0] {
	case "version", "--version":
		fmt.Printf("zx v%s\n", version)
		return
	case "help", "--help", "-h":
		fmt.Print(usage)
		return
	}

	// parse subcommand & flags
	cmd := "run"
	outBin := ""
	verbose := false
	var sourceFile string

	i := 0
	if args[0] == "build" || args[0] == "emit" || args[0] == "check" {
		cmd = args[0]; i = 1
	}
	for ; i < len(args); i++ {
		switch args[i] {
		case "-o":
			i++
			if i < len(args) { outBin = args[i] }
		case "-v", "--verbose":
			verbose = true
		default:
			if sourceFile == "" { sourceFile = args[i] }
		}
	}

	if sourceFile == "" {
		fmt.Fprintln(os.Stderr, "zxc: no input file specified")
		fmt.Fprintln(os.Stderr, "     run 'zxc --help' for usage")
		os.Exit(1)
	}

	src, err := os.ReadFile(sourceFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%szxc: cannot read %q: %v%s\n", colorRed, sourceFile, err, colorReset)
		os.Exit(1)
	}

	// ── pipeline ───────────────────────────────────────────────────────────
	resetDiags()

	tokens := Tokenize(string(src), sourceFile)
	if tokens == nil || hadError {
		printErrorSummary()
		os.Exit(1)
	}

	program := Parse(tokens, string(src), sourceFile)
	if program == nil || hadError {
		printErrorSummary()
		os.Exit(1)
	}

	ok := TypeCheck(program, string(src), sourceFile)
	if !ok || hadError {
		printErrorSummary()
		os.Exit(1)
	}

	if cmd == "check" {
		fmt.Printf("%s%s✓%s %s — no errors\n", colorBold, colorGreen, colorReset, sourceFile)
		return
	}

	cCode := Emit(program)

	if cmd == "emit" {
		fmt.Println(cCode)
		return
	}

	if verbose {
		printCSource(cCode)
	}

	// ── compile via gcc ────────────────────────────────────────────────────
	tmpC, err := os.CreateTemp("", "zx_*.c")
	if err != nil {
		fatalf("cannot create temp file: %v", err)
	}
	defer os.Remove(tmpC.Name())
	if _, err := tmpC.WriteString(cCode); err != nil {
		fatalf("cannot write temp C file: %v", err)
	}
	tmpC.Close()

	if outBin == "" {
		base := strings.TrimSuffix(filepath.Base(sourceFile), filepath.Ext(sourceFile))
		outBin = "./" + base
	}

	gccArgs := []string{"-x", "c", tmpC.Name(), "-o", outBin, "-lm", "-Wall", "-Wno-unused-variable"}
	gccOut, err := exec.Command("gcc", gccArgs...).CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "\n%s%s gcc error%s — the generated C has a problem:\n\n",
			colorBold, colorRed, colorReset)
		fmt.Fprintf(os.Stderr, "%s%s%s\n", colorDim, string(gccOut), colorReset)
		fmt.Fprintf(os.Stderr, "\n%srun 'zxc emit %s' to inspect the generated C code%s\n",
			colorYellow, sourceFile, colorReset)
		os.Exit(1)
	}

	if cmd == "build" {
		fmt.Printf("%s%s✓%s built → %s%s%s\n",
			colorBold, colorGreen, colorReset,
			colorCyan, outBin, colorReset)
		return
	}

	// ── run ────────────────────────────────────────────────────────────────
	runCmd := exec.Command(outBin)
	runCmd.Stdin  = os.Stdin
	runCmd.Stdout = os.Stdout
	runCmd.Stderr = os.Stderr
	if err := runCmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		os.Exit(1)
	}
}

func printErrorSummary() {
	errs := 0
	warns := 0
	for _, d := range allDiagnostics {
		if d.Sev == SevError { errs++ }
		if d.Sev == SevWarn  { warns++ }
	}
	parts := []string{}
	if errs > 0 {
		parts = append(parts, fmt.Sprintf("%s%s%d error(s)%s", colorBold, colorRed, errs, colorReset))
	}
	if warns > 0 {
		parts = append(parts, fmt.Sprintf("%s%s%d warning(s)%s", colorBold, colorYellow, warns, colorReset))
	}
	if len(parts) > 0 {
		fmt.Fprintf(os.Stderr, "%saborting due to %s%s\n",
			colorBold, strings.Join(parts, " and "), colorReset)
	}
}

func printCSource(code string) {
	lines := strings.Split(code, "\n")
	fmt.Printf("%s%s── generated C ──────────────────────────%s\n", colorBold, colorCyan, colorReset)
	for i, l := range lines {
		fmt.Printf("%s%4d%s  %s\n", colorDim, i+1, colorReset, l)
	}
	fmt.Printf("%s%s─────────────────────────────────────────%s\n\n", colorBold, colorCyan, colorReset)
}

func fatalf(f string, a ...any) {
	fmt.Fprintf(os.Stderr, "%szxc: "+f+"%s\n", append([]any{colorRed}, append(a, colorReset)...)...)
	os.Exit(1)
}
