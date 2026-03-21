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

// CallFrame represents one entry in a diagnostic call-stack trace.
type CallFrame struct {
	Span  Span
	Label string // e.g. "in function 'foo'" or "called from here"
}

type Diagnostic struct {
	Sev       Severity
	Code      string
	Span      Span
	Message   string
	Hint      string
	Notes     []string
	Secondary []SecondarySpan
	// NEW: call-stack trace — shown as a chain of "→ in ..." lines
	Trace []CallFrame
	// NEW: suggested fix snippet (shown after hint in green)
	Fix string
	// NEW: link to docs
	DocURL string
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

const maxDiags = 80

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

// ── Convenience constructors ──────────────────────────────────────────────────

func errAt(span Span, msg, hint string) {
	emitDiag(Diagnostic{Sev: SevError, Span: span, Message: msg, Hint: hint})
}
func errCode(code string, span Span, msg, hint string) {
	emitDiag(Diagnostic{Sev: SevError, Code: code, Span: span, Message: msg, Hint: hint})
}
func errCodeSecondary(code string, span Span, msg, hint string, secondary []SecondarySpan) {
	emitDiag(Diagnostic{Sev: SevError, Code: code, Span: span, Message: msg, Hint: hint, Secondary: secondary})
}

func errCodeTrace(code string, span Span, msg, hint string, trace []CallFrame) {
	emitDiag(Diagnostic{Sev: SevError, Code: code, Span: span, Message: msg, Hint: hint, Trace: trace})
}

func errFull(code string, span Span, msg, hint, fix string, secondary []SecondarySpan, trace []CallFrame) {
	emitDiag(Diagnostic{
		Sev: SevError, Code: code, Span: span,
		Message: msg, Hint: hint, Fix: fix,
		Secondary: secondary, Trace: trace,
	})
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
	default:
		headerColor = colorBold
		label = "???"
	}

	// Header line
	codeStr := ""
	if d.Code != "" {
		codeStr = fmt.Sprintf("[%s%s%s%s]",
			colorDim, d.Code, colorReset, headerColor)
	}
	fmt.Fprintf(w, "\n%s%s%s%s: %s%s%s\n",
		headerColor, label, codeStr, colorReset,
		colorBold, d.Message, colorReset)

	sp := d.Span
	if sp.File == "" || sp.Line <= 0 {
		fmt.Fprintf(w, "\n")
		return
	}

	// Location arrow
	fmt.Fprintf(w, "  %s-->%s %s%s%s:%s%d%s:%s%d%s\n",
		colorBlue+colorBold, colorReset,
		colorCyan, sp.File, colorReset,
		colorYellow, sp.Line, colorReset,
		colorYellow, sp.Col, colorReset)

	// Source context
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

	fmt.Fprintf(w, "  %s\n", gutter(0, false))

	// line before
	if sp.Line > 1 {
		fmt.Fprintf(w, "  %s%s%s\n",
			gutter(sp.Line-1, false),
			colorDim, lines[sp.Line-2]+colorReset)
	}

	// highlighted line
	lineText := lines[sp.Line-1]
	fmt.Fprintf(w, "  %s%s\n", gutter(sp.Line, true), lineText)

	// underline
	underLen := maxInt(sp.Len, 1)
	col0 := maxInt(sp.Col-1, 0)
	visCol := visualWidth(lineText, col0)
	under := buildUnderline(visCol, underLen, d.Sev)
	fmt.Fprintf(w, "  %s%s\n", gutter(0, false), under)

	// line after
	if sp.Line < len(lines) {
		fmt.Fprintf(w, "  %s%s%s\n",
			gutter(sp.Line+1, false),
			colorDim, lines[sp.Line]+colorReset)
	}

	// Hint
	if d.Hint != "" {
		hColor := colorGreen + colorBold
		hLabel := "help"
		switch d.Sev {
		case SevWarn, SevFunny:
			hColor = colorYellow + colorBold
			hLabel = "suggestion"
		}
		fmt.Fprintf(w, "  %s%s%s%s: %s%s%s\n",
			gutter(0, false),
			hColor, hLabel, colorReset,
			colorGreen, d.Hint, colorReset)
	}

	// Fix
	if d.Fix != "" {
		fmt.Fprintf(w, "  %s%s= fix:%s %s%s%s\n",
			gutter(0, false),
			colorGreen+colorBold, colorReset,
			colorGreen, d.Fix, colorReset)
	}

	// Notes
	for _, n := range d.Notes {
		fmt.Fprintf(w, "  %s%s= note:%s %s\n",
			gutter(0, false),
			colorPurple+colorBold, colorReset, n)
	}

	// Doc URL
	if d.DocURL != "" {
		fmt.Fprintf(w, "  %s%s= docs:%s %s%s%s\n",
			gutter(0, false),
			colorCyan+colorBold, colorReset,
			colorCyan, d.DocURL, colorReset)
	}

	// Trace — fixed version (no garbage, colors preserved)
	if len(d.Trace) > 0 {
		fmt.Fprintf(w, "  %s%s= trace:%s\n",
			gutter(0, false),
			colorPurple+colorBold, colorReset)

		for i, frame := range d.Trace {
			indent := strings.Repeat("  ", i+1)

			// Safe location formatting — this prevents %!(EXTRA ...)
			loc := ""
			if frame.Span.Line > 0 {
				fileDisplay := frame.Span.File
				/*if fileDisplay != "" {
					fileDisplay = filepath.Base(fileDisplay)
				}*/
				loc = fmt.Sprintf(" (%s:%d", fileDisplay, frame.Span.Line)
				if frame.Span.Col > 0 {
					loc += fmt.Sprintf(":%d", frame.Span.Col)
				}
				loc += ")"
			}

			fmt.Fprintf(w, "  %s%s%s→%s %s%s%s%s\n",
				gutter(0, false),
				indent,
				colorPurple+colorBold, colorReset,
				colorBold, frame.Label, colorReset,
				colorDim+loc+colorReset,
			)

			// frame source line
			frameLines := getSourceLines(frame.Span.File)
			if frameLines != nil && frame.Span.Line >= 1 && frame.Span.Line <= len(frameLines) {
				fmt.Fprintf(w, "  %s%s  %s%s%s\n",
					gutter(0, false),
					indent,
					colorDim, frameLines[frame.Span.Line-1], colorReset)
			}
		}
	}

	// Secondary spans
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

			secCol0 := maxInt(sec.Span.Col-1, 0)
			secLen := maxInt(sec.Span.Len, 1)
			secVis := visualWidth(secLines[sec.Span.Line-1], secCol0)
			secUnder := fmt.Sprintf("%s%s%s%s  %s%s%s",
				colorBlue+colorBold,
				strings.Repeat(" ", secVis),
				strings.Repeat("-", secLen),
				colorReset,
				colorDim+colorBold, sec.Label, colorReset)
			fmt.Fprintf(w, "  %s%s\n", secGutter(0), secUnder)
		}
	}

	fmt.Fprintf(w, "\n")
}

func buildUnderline(col0, length int, sev Severity) string {
	col0 = maxInt(col0, 0)
	var ch, col string
	switch sev {
	case SevError:
		ch, col = "^", colorRed+colorBold
	case SevWarn, SevFunny:
		ch, col = "~", colorYellow+colorBold
	default:
		ch, col = "-", colorCyan+colorBold
	}
	pad := strings.Repeat(" ", col0)
	carets := strings.Repeat(ch, maxInt(length, 1))
	return col + pad + carets + colorReset
}

// ── Source registry ───────────────────────────────────────────────────────────

var sourceCache = map[string][]string{}

func registerSource(file, src string) {
	sourceCache[file] = strings.Split(src, "\n")
}

func getSourceLines(file string) []string {
	return sourceCache[file]
}

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
		parts = append(parts, fmt.Sprintf("%s%d %s%s", colorRed+colorBold, errs, noun, colorReset))
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

// ── TraceBuilder ──────────────────────────────────────────────────────────────

type TraceBuilder struct {
	frames []CallFrame
}

func (tb *TraceBuilder) Push(sp Span, label string) {
	tb.frames = append(tb.frames, CallFrame{Span: sp, Label: label})
}

func (tb *TraceBuilder) Pop() {
	if len(tb.frames) > 0 {
		tb.frames = tb.frames[:len(tb.frames)-1]
	}
}

func (tb *TraceBuilder) Snapshot() []CallFrame {
	if len(tb.frames) == 0 {
		return nil
	}
	out := make([]CallFrame, len(tb.frames))
	copy(out, tb.frames)
	return out
}

func (tb *TraceBuilder) Len() int { return len(tb.frames) }
