package main

import (
	"fmt"
	"os"
	"strings"
)

type Severity int

const (
	SevError Severity = iota
	SevWarn
	SevNote
)

type Span struct {
	File string
	Line int
	Col  int
	Len  int
}

type Diagnostic struct {
	Sev     Severity
	Span    Span
	Message string
	Hint    string
	Notes   []string
}

var (
	allDiagnostics []Diagnostic
	hadError       bool
	diagCount      int
)

const maxDiags = 20

func resetDiags() {
	allDiagnostics = nil
	hadError = false
	diagCount = 0
}

func emitDiag(d Diagnostic) {
	allDiagnostics = append(allDiagnostics, d)
	if d.Sev == SevError {
		hadError = true
	}
	diagCount++
	if diagCount <= maxDiags {
		printDiag(d)
	} else if diagCount == maxDiags+1 {
		fmt.Fprintf(os.Stderr, "%s%stoo many errors — stopping here%s\n\n",
			colorBold, colorRed, colorReset)
	}
}

func errAt(span Span, msg string, hint string) {
	emitDiag(Diagnostic{Sev: SevError, Span: span, Message: msg, Hint: hint})
}
func warnAt(span Span, msg string, hint string) {
	emitDiag(Diagnostic{Sev: SevWarn, Span: span, Message: msg, Hint: hint})
}
func noteAt(span Span, msg string) {
	emitDiag(Diagnostic{Sev: SevNote, Span: span, Message: msg})
}

func printDiag(d Diagnostic) {
	w := os.Stderr
	var sevColor, sevLabel string
	switch d.Sev {
	case SevError:
		sevColor = colorRed
		sevLabel = "error"
	case SevWarn:
		sevColor = colorYellow
		sevLabel = "warning"
	case SevNote:
		sevColor = colorCyan
		sevLabel = "note"
	}
	fmt.Fprintf(w, "%s%s%s%s: %s%s%s\n",
		colorBold, sevColor, sevLabel, colorReset,
		colorBold, d.Message, colorReset)

	sp := d.Span
	if sp.File == "" {
		fmt.Fprintln(w)
		return
	}
	fmt.Fprintf(w, "  %s-->%s %s:%d:%d\n",
		colorBlue+colorBold, colorReset, sp.File, sp.Line, sp.Col)

	lines := getSourceLines(sp.File)
	if lines == nil || sp.Line < 1 || sp.Line > len(lines) {
		fmt.Fprintln(w)
		return
	}
	lineText := lines[sp.Line-1]

	if sp.Line > 1 {
		fmt.Fprintf(w, "  %s%4d%s %s│%s %s\n",
			colorDim, sp.Line-1, colorReset,
			colorBlue+colorBold, colorReset,
			colorDim+lines[sp.Line-2]+colorReset)
	}
	fmt.Fprintf(w, "  %s%4d%s %s│%s %s\n",
		colorBold, sp.Line, colorReset,
		colorBlue+colorBold, colorReset,
		lineText)

	underLen := sp.Len
	if underLen <= 0 {
		underLen = 1
	}
	under := buildUnderline(sp.Col-1, underLen, d.Sev)
	fmt.Fprintf(w, "       %s│%s %s\n", colorBlue+colorBold, colorReset, under)

	if d.Hint != "" {
		pad := strings.Repeat(" ", maxInt(sp.Col-1, 0))
		fmt.Fprintf(w, "       %s│%s %s%s%s %s%s\n",
			colorBlue+colorBold, colorReset,
			pad,
			colorGreen+colorBold, "hint:", colorReset,
			colorGreen+d.Hint+colorReset)
	}
	for _, n := range d.Notes {
		fmt.Fprintf(w, "       %s=%s %snote:%s %s\n",
			colorBlue+colorBold, colorReset,
			colorWhite+colorBold, colorReset, n)
	}
	fmt.Fprintln(w)
}

func buildUnderline(col0, length int, sev Severity) string {
	if col0 < 0 {
		col0 = 0
	}
	pad := strings.Repeat(" ", col0)
	var ch, color string
	switch sev {
	case SevError:
		ch = "^"; color = colorRed + colorBold
	case SevWarn:
		ch = "~"; color = colorYellow + colorBold
	default:
		ch = "-"; color = colorCyan + colorBold
	}
	return color + pad + strings.Repeat(ch, maxInt(length, 1)) + colorReset
}

var sourceCache = map[string][]string{}

func registerSource(file, src string) {
	sourceCache[file] = strings.Split(src, "\n")
}
func getSourceLines(file string) []string { return sourceCache[file] }
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
