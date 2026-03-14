package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const version = "0.1.0"

const usage = `
 ▒███████▒ ▒██   ██▒
▒ ▒ ▒ ▄▀░ ▒▒ █ █ ▒░
░ ▒ ▄▀▒░  ░░  █   ░
  ▄▀▒   ░  ░ █ █  ░
▒███████▒ ▒██▒ ▒██▒  v` + version + `

ZX — a fast, simple language that compiles to C

USAGE:
  zx <file.zx>              compile & run
  zx build <file.zx>        compile only  (outputs a.out or <file>)
  zx emit  <file.zx>        print generated C code
  zx check <file.zx>        type-check only, no output
  zx version                print version

OPTIONS:
  -o <output>   output binary name (with build)
  -v            verbose: show generated C before compiling

EXAMPLES:
  zx hello.zx
  zx build hello.zx -o hello
  zx emit  hello.zx
`

func main() {
	args := os.Args[1:]
	if len(args) == 0 {
		fmt.Print(usage)
		os.Exit(0)
	}

	switch args[0] {
	case "version", "--version", "-v":
		fmt.Printf("zx v%s\n", version)
		return
	case "help", "--help", "-h":
		fmt.Print(usage)
		return
	}

	// parse flags
	cmd := "run"
	outBin := ""
	verbose := false
	var sourceFile string

	i := 0
	if args[0] == "build" || args[0] == "emit" || args[0] == "check" {
		cmd = args[0]
		i = 1
	}

	for ; i < len(args); i++ {
		switch args[i] {
		case "-o":
			i++
			if i < len(args) {
				outBin = args[i]
			}
		case "-v", "--verbose":
			verbose = true
		default:
			if sourceFile == "" {
				sourceFile = args[i]
			}
		}
	}

	if sourceFile == "" {
		fmt.Fprintln(os.Stderr, "zx: no input file")
		os.Exit(1)
	}

	src, err := os.ReadFile(sourceFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "zx: cannot read %q: %v\n", sourceFile, err)
		os.Exit(1)
	}

	// ── pipeline ──────────────────────────────────────
	tokens := Tokenize(string(src), sourceFile)
	if tokens == nil {
		os.Exit(1)
	}

	program := Parse(tokens, string(src), sourceFile)
	if program == nil {
		os.Exit(1)
	}

	ok := TypeCheck(program, string(src), sourceFile)
	if !ok {
		os.Exit(1)
	}

	if cmd == "check" {
		fmt.Printf("%s✓ %s — OK%s\n", colorGreen, sourceFile, colorReset)
		return
	}

	cCode := Emit(program)

	if cmd == "emit" {
		fmt.Println(cCode)
		return
	}

	if verbose {
		printC(cCode)
	}

	// ── compile via gcc ───────────────────────────────
	tmpC, err := os.CreateTemp("", "zx_*.c")
	if err != nil {
		fatalf("cannot create temp file: %v", err)
	}
	defer os.Remove(tmpC.Name())

	if _, err := tmpC.WriteString(cCode); err != nil {
		fatalf("cannot write temp file: %v", err)
	}
	tmpC.Close()

	if outBin == "" {
		base := strings.TrimSuffix(filepath.Base(sourceFile), filepath.Ext(sourceFile))
		outBin = "./" + base
	}

	compileArgs := []string{tmpC.Name(), "-o", outBin, "-lm"}
	gccOut, err := exec.Command("gcc", compileArgs...).CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "%szx: gcc error:%s\n%s\n", colorRed, colorReset, string(gccOut))
		os.Exit(1)
	}

	if cmd == "build" {
		fmt.Printf("%s✓ built → %s%s\n", colorGreen, outBin, colorReset)
		return
	}

	// ── run ───────────────────────────────────────────
	runCmd := exec.Command(outBin)
	runCmd.Stdin = os.Stdin
	runCmd.Stdout = os.Stdout
	runCmd.Stderr = os.Stderr
	if err := runCmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		os.Exit(1)
	}
}

func printC(code string) {
	lines := strings.Split(code, "\n")
	fmt.Printf("%s%s── generated C ──%s\n", colorDim, colorCyan, colorReset)
	for i, l := range lines {
		fmt.Printf("%s%4d%s  %s\n", colorDim, i+1, colorReset, l)
	}
	fmt.Printf("%s%s─────────────────%s\n\n", colorDim, colorCyan, colorReset)
}

func fatalf(f string, a ...any) {
	fmt.Fprintf(os.Stderr, "zx: "+f+"\n", a...)
	os.Exit(1)
}
