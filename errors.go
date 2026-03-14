package main

import (
	"fmt"
	"os"
	"strings"
)

// Severity levels
type Severity int

const (
	SevError Severity = iota
	SevWarn
	SevHint
)

// Span holds source location
type Span struct {
	File  string
	Line  int // 1-based
	Col   int // 1-based
	Len   int // width of the underline (chars), 0 = single caret
}

// Diagnostic is one error/warning
type Diagnostic struct {
	Sev     Severity
	Span    Span
	Message string
	Hint    string // optional suggestion
	Notes   []string
}

var diagnostics []Diagnostic
var hadError bool

func emitDiag(d Diagnostic) {
	diagnostics = append(diagnostics, d)
	if d.Sev == SevError {
		hadError = true
	}
	printDiag(d)
}

func errAt(span Span, msg string, hint string) {
	emitDiag(Diagnostic{Sev: SevError, Span: span, Message: msg, Hint: hint})
}

func warnAt(span Span, msg string, hint string) {
	emitDiag(Diagnostic{Sev: SevWarn, Span: span, Message: msg, Hint: hint})
}

func printDiag(d Diagnostic) {
	w := os.Stderr

	// ── header line ──────────────────────────────────────────────
	var sevColor, sevLabel string
	switch d.Sev {
	case SevError:
		sevColor = colorRed
		sevLabel = "error"
	case SevWarn:
		sevColor = colorYellow
		sevLabel = "warning"
	case SevHint:
		sevColor = colorCyan
		sevLabel = "hint"
	}

	fmt.Fprintf(w, "%s%s%s%s: %s%s%s\n",
		colorBold, sevColor, sevLabel, colorReset,
		colorBold, d.Message, colorReset,
	)

	// ── location ──────────────────────────────────────────────────
	sp := d.Span
	fmt.Fprintf(w, "  %s-->%s %s:%d:%d\n",
		colorBlue+colorBold, colorReset,
		sp.File, sp.Line, sp.Col,
	)

	// ── source snippet ────────────────────────────────────────────
	lines := getSourceLines(sp.File)
	if lines != nil && sp.Line >= 1 && sp.Line <= len(lines) {
		lineText := lines[sp.Line-1]

		// print the line
		gutter := fmt.Sprintf("%4d", sp.Line)
		fmt.Fprintf(w, "%s%s%s │%s %s\n",
			colorBlue+colorBold, gutter, colorReset,
			colorReset, lineText,
		)

		// underline
		underLen := d.Span.Len
		if underLen <= 0 {
			underLen = 1
		}
		under := buildUnderline(sp.Col-1, underLen, d.Sev)
		fmt.Fprintf(w, "     %s│%s %s\n", colorBlue+colorBold, colorReset, under)

		// hint on same underline row
		if d.Hint != "" {
			hintPad := strings.Repeat(" ", maxInt(sp.Col-1, 0))
			fmt.Fprintf(w, "     %s│%s %s%s%s %s%s\n",
				colorBlue+colorBold, colorReset,
				hintPad,
				colorGreen+colorBold, "hint:", colorReset,
				colorGreen+d.Hint+colorReset,
			)
		}
	}

	// notes
	for _, n := range d.Notes {
		fmt.Fprintf(w, "     %s=%s %snote:%s %s\n",
			colorBlue+colorBold, colorReset,
			colorWhite+colorBold, colorReset, n,
		)
	}

	fmt.Fprintln(w)
}

func buildUnderline(col0 int, length int, sev Severity) string {
	pad := strings.Repeat(" ", maxInt(col0, 0))
	var ch string
	var color string
	switch sev {
	case SevError:
		ch = "^"
		color = colorRed + colorBold
	case SevWarn:
		ch = "~"
		color = colorYellow + colorBold
	default:
		ch = "-"
		color = colorCyan + colorBold
	}
	return color + pad + strings.Repeat(ch, maxInt(length, 1)) + colorReset
}

// ── source line cache ─────────────────────────────────────────────────────────

var sourceCache = map[string][]string{}

func registerSource(file, src string) {
	sourceCache[file] = strings.Split(src, "\n")
}

func getSourceLines(file string) []string {
	return sourceCache[file]
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
