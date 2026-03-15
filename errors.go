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
	SevFunny // funny/style warnings (dot instead of arrow, etc.)
)

type Span struct {
	File string
	Line int
	Col  int
	Len  int
}

type Diagnostic struct {
	Sev     Severity
	Code    string // E01, W01, etc.
	Span    Span
	Message string
	Hint    string
	Notes   []string
}

var (
	allDiagnostics []Diagnostic
	hadError       bool
	diagCount      int
	warnCount      int
)

const maxDiags = 30

func resetDiags() {
	allDiagnostics = nil
	hadError = false
	diagCount = 0
	warnCount = 0
}

func emitDiag(d Diagnostic) {
	allDiagnostics = append(allDiagnostics, d)
	if d.Sev == SevError {
		hadError = true
	}
	if d.Sev == SevWarn || d.Sev == SevFunny {
		warnCount++
	}
	diagCount++
	if diagCount <= maxDiags {
		printDiag(d)
	} else if diagCount == maxDiags+1 {
		fmt.Fprintf(os.Stderr, "%s%s⚡ too many diagnostics — suppressing the rest%s\n\n",
			colorBold, colorRed, colorReset)
	}
}

func errAt(span Span, msg string, hint string) {
	emitDiag(Diagnostic{Sev: SevError, Span: span, Message: msg, Hint: hint})
}
func errCode(code string, span Span, msg string, hint string) {
	emitDiag(Diagnostic{Sev: SevError, Code: code, Span: span, Message: msg, Hint: hint})
}
func warnAt(span Span, msg string, hint string) {
	emitDiag(Diagnostic{Sev: SevWarn, Span: span, Message: msg, Hint: hint})
}
func funnyWarn(span Span, msg string, hint string) {
	emitDiag(Diagnostic{Sev: SevFunny, Span: span, Message: msg, Hint: hint})
}
func noteAt(span Span, msg string) {
	emitDiag(Diagnostic{Sev: SevNote, Span: span, Message: msg})
}

func printDiag(d Diagnostic) {
	w := os.Stderr
	var icon, sevColor, sevLabel string
	switch d.Sev {
	case SevError:
		icon = "✖ "; sevColor = colorRed;    sevLabel = "error"
	case SevWarn:
		icon = "⚠ "; sevColor = colorYellow; sevLabel = "warning"
	case SevNote:
		icon = "ℹ "; sevColor = colorCyan;   sevLabel = "note"
	case SevFunny:
		icon = "😅 "; sevColor = colorOrange; sevLabel = "style"
	}

	code := ""
	if d.Code != "" {
		code = fmt.Sprintf("[%s] ", d.Code)
	}
	fmt.Fprintf(w, "%s%s%s%s%s: %s%s%s\n",
		colorBold, sevColor, icon, code, sevLabel+colorReset,
		colorBold, d.Message, colorReset)

	sp := d.Span
	if sp.File == "" {
		fmt.Fprintln(w)
		return
	}
	fmt.Fprintf(w, "   %s╭─%s %s:%d:%d\n",
		colorBlue+colorBold, colorReset, sp.File, sp.Line, sp.Col)

	lines := getSourceLines(sp.File)
	if lines == nil || sp.Line < 1 || sp.Line > len(lines) {
		fmt.Fprintln(w)
		return
	}
	lineText := lines[sp.Line-1]

	// context line above
	if sp.Line > 1 {
		fmt.Fprintf(w, "   %s│%s %s%4d%s  %s\n",
			colorBlue+colorBold, colorReset,
			colorDim, sp.Line-1, colorReset,
			colorDim+lines[sp.Line-2]+colorReset)
	}
	// offending line
	fmt.Fprintf(w, "   %s│%s %s%4d%s  %s\n",
		colorBlue+colorBold, colorReset,
		colorBold, sp.Line, colorReset,
		lineText)

	// underline
	underLen := sp.Len
	if underLen <= 0 {
		underLen = 1
	}
	under := buildUnderline(sp.Col-1, underLen, d.Sev)
	fmt.Fprintf(w, "   %s│%s       %s\n", colorBlue+colorBold, colorReset, under)

	// hint
	if d.Hint != "" {
		pad := strings.Repeat(" ", maxInt(sp.Col-1, 0))
		hColor := colorGreen
		hLabel := "hint"
		if d.Sev == SevFunny {
			hColor = colorOrange
			hLabel = "suggestion"
		}
		fmt.Fprintf(w, "   %s│%s       %s%s%s%s %s%s\n",
			colorBlue+colorBold, colorReset,
			pad, hColor+colorBold, hLabel+":"+colorReset,
			hColor, d.Hint, colorReset)
	}
	for _, n := range d.Notes {
		fmt.Fprintf(w, "   %s│%s       %s= note:%s %s\n",
			colorBlue+colorBold, colorReset,
			colorWhite+colorBold, colorReset, n)
	}
	fmt.Fprintf(w, "   %s╰─%s\n\n", colorBlue+colorBold, colorReset)
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
	case SevFunny:
		ch = "~"; color = colorOrange + colorBold
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
