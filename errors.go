package main

import (
	"fmt"
	"os"
	"strings"
	"unicode/utf8"
)

type Severity int

const (
	SevError Severity = iota
	SevWarn
	SevNote
	SevFunny // style warnings
	SevHelp  // actionable help messages
)

type Span struct {
	File string
	Line int
	Col  int
	Len  int
}

type Diagnostic struct {
	Sev     Severity
	Code    string
	Span    Span
	Message string
	Hint    string
	Notes   []string
	// Secondary spans for multi-location errors (e.g. "previous declaration here")
	Secondary []SecondarySpan
}

type SecondarySpan struct {
	Span  Span
	Label string
}

var (
	allDiagnostics []Diagnostic
	hadError       bool
	diagCount      int
	warnCount      int
)

const maxDiags = 60

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
		fmt.Fprintf(os.Stderr,
			"\n%s%s error[E000]%s: too many diagnostics (%d) — stopping here\n"+
				"         fix the errors above and re-compile\n\n",
			colorBold, colorRed, colorReset, diagCount)
	}
}

func errAt(span Span, msg, hint string) {
	emitDiag(Diagnostic{Sev: SevError, Span: span, Message: msg, Hint: hint})
}
func errCode(code string, span Span, msg, hint string) {
	emitDiag(Diagnostic{Sev: SevError, Code: code, Span: span, Message: msg, Hint: hint})
}
func errCodeSecondary(code string, span Span, msg, hint string, secondary []SecondarySpan) {
	emitDiag(Diagnostic{Sev: SevError, Code: code, Span: span, Message: msg, Hint: hint, Secondary: secondary})
}
func warnAt(span Span, msg, hint string) {
	emitDiag(Diagnostic{Sev: SevWarn, Span: span, Message: msg, Hint: hint})
}
func warnCode(code string, span Span, msg, hint string) {
	emitDiag(Diagnostic{Sev: SevWarn, Code: code, Span: span, Message: msg, Hint: hint})
}
func funnyWarn(span Span, msg, hint string) {
	emitDiag(Diagnostic{Sev: SevFunny, Span: span, Message: msg, Hint: hint})
}
func noteAt(span Span, msg string) {
	emitDiag(Diagnostic{Sev: SevNote, Span: span, Message: msg})
}
func helpAt(span Span, msg, hint string) {
	emitDiag(Diagnostic{Sev: SevHelp, Span: span, Message: msg, Hint: hint})
}

// ── Rendering ─────────────────────────────────────────────────────────────────

func printDiag(d Diagnostic) {
	w := os.Stderr

	var headerColor, label string
	switch d.Sev {
	case SevError:
		headerColor = colorRed + colorBold
		label = "error"
	case SevWarn:
		headerColor = colorYellow + colorBold
		label = "warning"
	case SevNote:
		headerColor = colorCyan + colorBold
		label = "note"
	case SevHelp:
		headerColor = colorGreen + colorBold
		label = "help"
	case SevFunny:
		headerColor = colorOrange + colorBold
		label = "style"
	}

	// ── Header line ─────────────────────────────────────────────────────────
	// error[E27]: undefined variable or function "foo"
	codeStr := ""
	if d.Code != "" {
		codeStr = fmt.Sprintf("[%s%s%s%s]",
			colorDim, d.Code, colorReset, headerColor)
	}
	fmt.Fprintf(w, "\n%s%s%s%s: %s%s%s\n",
		headerColor, label, codeStr, colorReset,
		colorBold, d.Message, colorReset)

	sp := d.Span
	if sp.File == "" {
		fmt.Fprintf(w, "\n")
		return
	}

	// ── Location arrow ───────────────────────────────────────────────────────
	// --> src/main.zx:12:5
	fmt.Fprintf(w, "  %s-->%s %s%s%s:%s%d%s:%s%d%s\n",
		colorBlue+colorBold, colorReset,
		colorCyan, sp.File, colorReset,
		colorYellow, sp.Line, colorReset,
		colorYellow, sp.Col, colorReset)

	// ── Source context ───────────────────────────────────────────────────────
	lines := getSourceLines(sp.File)
	if lines == nil || sp.Line < 1 || sp.Line > len(lines) {
		fmt.Fprintf(w, "\n")
		return
	}

	lineNumWidth := digits(sp.Line + 1)
	gutter := func(n int, active bool) string {
		if n == 0 {
			return fmt.Sprintf("%s%s |%s ", colorBlue+colorBold, padLeft("", lineNumWidth), colorReset)
		}
		marker := "|"
		if active {
			marker = colorBlue + colorBold + "|" + colorReset
		}
		return fmt.Sprintf("%s%s%s %s ",
			colorBlue+colorBold, padLeft(fmt.Sprintf("%d", n), lineNumWidth), colorBlue,
			marker)
	}

	// blank gutter before context
	fmt.Fprintf(w, "  %s\n", gutter(0, false))

	// optional: line before
	if sp.Line > 1 {
		fmt.Fprintf(w, "  %s%s%s\n",
			gutter(sp.Line-1, false),
			colorDim, lines[sp.Line-2]+colorReset)
	}

	// the highlighted line
	lineText := lines[sp.Line-1]
	fmt.Fprintf(w, "  %s%s\n", gutter(sp.Line, true), lineText)

	// underline
	underLen := sp.Len
	if underLen <= 0 {
		underLen = 1
	}
	col0 := sp.Col - 1
	if col0 < 0 {
		col0 = 0
	}
	// account for tab width in visual alignment
	visCol := visualWidth(lineText, col0)
	under := buildUnderline(visCol, underLen, d.Sev)
	fmt.Fprintf(w, "  %s%s\n", gutter(0, false), under)

	// ── Hint / suggestion ────────────────────────────────────────────────────
	if d.Hint != "" {
		hColor := colorGreen + colorBold
		hLabel := "help"
		switch d.Sev {
		case SevWarn, SevFunny:
			hColor = colorYellow + colorBold
			hLabel = "suggestion"
		case SevError:
			hColor = colorGreen + colorBold
			hLabel = "help"
		}
		fmt.Fprintf(w, "  %s%s%s%s: %s%s%s\n",
			gutter(0, false),
			hColor, hLabel, colorReset,
			colorGreen, d.Hint, colorReset)
	}

	// ── Notes ────────────────────────────────────────────────────────────────
	for _, n := range d.Notes {
		fmt.Fprintf(w, "  %s%s= note:%s %s\n",
			gutter(0, false),
			colorPurple+colorBold, colorReset, n)
	}

	// ── Secondary spans (e.g. "previous declaration here") ──────────────────
	for _, sec := range d.Secondary {
		if sec.Span.File == "" {
			continue
		}
		fmt.Fprintf(w, "\n  %s...%s %s%s%s:%s%d%s:%s%d%s\n",
			colorBlue+colorBold, colorReset,
			colorCyan, sec.Span.File, colorReset,
			colorYellow, sec.Span.Line, colorReset,
			colorYellow, sec.Span.Col, colorReset)

		secLines := getSourceLines(sec.Span.File)
		if secLines != nil && sec.Span.Line >= 1 && sec.Span.Line <= len(secLines) {
			secGutter := func(n int) string {
				if n == 0 {
					return fmt.Sprintf("%s%s |%s ", colorBlue+colorBold, padLeft("", lineNumWidth), colorReset)
				}
				return fmt.Sprintf("%s%s%s | ",
					colorBlue+colorBold, padLeft(fmt.Sprintf("%d", n), lineNumWidth), colorReset)
			}
			fmt.Fprintf(w, "  %s%s%s\n",
				secGutter(sec.Span.Line),
				colorDim, secLines[sec.Span.Line-1]+colorReset)
			secCol0 := sec.Span.Col - 1
			if secCol0 < 0 {
				secCol0 = 0
			}
			secLen := sec.Span.Len
			if secLen <= 0 {
				secLen = 1
			}
			secVis := visualWidth(secLines[sec.Span.Line-1], secCol0)
			secUnder := fmt.Sprintf("%s%s%s%s  %s%s%s",
				colorBlue+colorBold,
				strings.Repeat(" ", maxInt(secVis, 0)),
				strings.Repeat("-", maxInt(secLen, 1)),
				colorReset,
				colorDim+colorBold, sec.Label, colorReset)
			fmt.Fprintf(w, "  %s%s\n", secGutter(0), secUnder)
		}
	}

	// ── Closing line ─────────────────────────────────────────────────────────
	fmt.Fprintf(w, "\n")
}

// buildUnderline creates the "^^^" or "~~~" indicator under a span.
func buildUnderline(col0, length int, sev Severity) string {
	if col0 < 0 {
		col0 = 0
	}
	var ch, color string
	switch sev {
	case SevError:
		ch, color = "^", colorRed+colorBold
	case SevWarn, SevFunny:
		ch, color = "~", colorYellow+colorBold
	case SevHelp:
		ch, color = "-", colorGreen+colorBold
	default:
		ch, color = "-", colorCyan+colorBold
	}
	pad := strings.Repeat(" ", col0)
	carets := strings.Repeat(ch, maxInt(length, 1))
	return color + pad + carets + colorReset
}

// ── Source registry ───────────────────────────────────────────────────────────

var sourceCache = map[string][]string{}

func registerSource(file, src string) {
	sourceCache[file] = strings.Split(src, "\n")
}
func getSourceLines(file string) []string { return sourceCache[file] }

// ── Formatting helpers ────────────────────────────────────────────────────────

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func digits(n int) int {
	if n <= 0 {
		return 1
	}
	d := 0
	for n > 0 {
		d++
		n /= 10
	}
	return d
}

func padLeft(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return strings.Repeat(" ", width-len(s)) + s
}

// visualWidth returns the visual column offset for position pos in s,
// treating tabs as 4-wide stops.
func visualWidth(s string, pos int) int {
	vis := 0
	i := 0
	for _, r := range s {
		if i >= pos {
			break
		}
		if r == '\t' {
			vis = (vis/4 + 1) * 4
		} else {
			vis += utf8.RuneLen(r)
		}
		i++
	}
	return vis
}

// ── Summary printer ───────────────────────────────────────────────────────────

// PrintDiagSummary prints a compact summary at the end of compilation.
func PrintDiagSummary() {
	w := os.Stderr
	errs := 0
	warns := 0
	for _, d := range allDiagnostics {
		switch d.Sev {
		case SevError:
			errs++
		case SevWarn, SevFunny:
			warns++
		}
	}
	if errs == 0 && warns == 0 {
		return
	}
	parts := []string{}
	if errs > 0 {
		noun := "error"
		if errs != 1 {
			noun = "errors"
		}
		parts = append(parts, fmt.Sprintf("%s%s%d %s%s", colorRed+colorBold, "", errs, noun, colorReset))
	}
	if warns > 0 {
		noun := "warning"
		if warns != 1 {
			noun = "warnings"
		}
		parts = append(parts, fmt.Sprintf("%s%d %s%s", colorYellow+colorBold, warns, noun, colorReset))
	}
	fmt.Fprintf(w, "%saborting due to%s %s\n\n",
		colorBold, colorReset,
		strings.Join(parts, " and "))
}
