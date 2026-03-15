package main

// ─────────────────────────────────────────────────────────────────────────────
//  ZX Standard Library
//
//  use std::str    → string operations
//  use std::io     → file / socket I/O helpers
//  use std::math   → math functions
//  use std::sys    → system calls
//  use std::conv   → type conversions
//  use std::list   → array/list helpers
//  use std::mem    → memory (malloc/free)
//  use std::fmt    → formatting helpers
//  use std::time   → time functions
//  use std::os     → environment, args
// ─────────────────────────────────────────────────────────────────────────────

// StdModule describes a ZX stdlib module
type StdModule struct {
	Headers []string // C headers to include
	Fns     []StdFn  // exposed functions
}

// StdFn is a builtin function exposed by a module
type StdFn struct {
	Name     string
	CFn      string  // C function name (may differ)
	Variadic bool
	Params   []Param
	Ret      *ZXType
}

// Registry of all std modules
var stdModules = map[string]*StdModule{

	// ── std::str ──────────────────────────────────────────────────────────────
	"std::str": {
		Headers: []string{"string.h", "stdio.h", "ctype.h"},
		Fns: []StdFn{
			{Name: "str_len",     CFn: "strlen",  Params: []Param{{Name: "s", Type: TypStr}},  Ret: TypInt},
			{Name: "str_copy",    CFn: "strcpy",  Params: []Param{{Name: "dst", Type: TypStr}, {Name: "src", Type: TypStr}}, Ret: TypStr},
			{Name: "str_cat",     CFn: "strcat",  Params: []Param{{Name: "dst", Type: TypStr}, {Name: "src", Type: TypStr}}, Ret: TypStr},
			{Name: "str_cmp",     CFn: "strcmp",  Params: []Param{{Name: "a", Type: TypStr},   {Name: "b", Type: TypStr}},   Ret: TypInt},
			{Name: "str_find",    CFn: "strstr",  Params: []Param{{Name: "hay", Type: TypStr}, {Name: "needle", Type: TypStr}}, Ret: TypStr},
			{Name: "str_chr",     CFn: "strchr",  Params: []Param{{Name: "s", Type: TypStr},   {Name: "c", Type: TypInt}},   Ret: TypStr},
			{Name: "str_ncmp",    CFn: "strncmp", Params: []Param{{Name: "a", Type: TypStr},   {Name: "b", Type: TypStr}, {Name: "n", Type: TypInt}}, Ret: TypInt},
			{Name: "str_upper",   CFn: "toupper", Params: []Param{{Name: "c", Type: TypInt}},  Ret: TypInt},
			{Name: "str_lower",   CFn: "tolower", Params: []Param{{Name: "c", Type: TypInt}},  Ret: TypInt},
			{Name: "str_to_int",  CFn: "atoi",    Params: []Param{{Name: "s", Type: TypStr}},  Ret: TypInt},
			{Name: "str_to_float",CFn: "atof",    Params: []Param{{Name: "s", Type: TypStr}},  Ret: TypFloat},
			{Name: "sprintf",     CFn: "sprintf", Variadic: true,
				Params: []Param{{Name: "buf", Type: TypStr}, {Name: "fmt", Type: TypStr}}, Ret: TypInt},
			{Name: "snprintf",    CFn: "snprintf", Variadic: true,
				Params: []Param{{Name: "buf", Type: TypStr}, {Name: "n", Type: TypInt}, {Name: "fmt", Type: TypStr}}, Ret: TypInt},
		},
	},

	// ── std::io ───────────────────────────────────────────────────────────────
	"std::io": {
		Headers: []string{"stdio.h"},
		Fns: []StdFn{
			{Name: "open",   CFn: "fopen",  Params: []Param{{Name: "path", Type: TypStr}, {Name: "mode", Type: TypStr}}, Ret: PtrOf(TypVoid)},
			{Name: "close",  CFn: "fclose", Params: []Param{{Name: "f", Type: PtrOf(TypVoid)}}, Ret: TypInt},
			{Name: "read",   CFn: "fgets",  Params: []Param{{Name: "buf", Type: TypStr}, {Name: "n", Type: TypInt}, {Name: "f", Type: PtrOf(TypVoid)}}, Ret: TypStr},
			{Name: "write",  CFn: "fputs",  Params: []Param{{Name: "s", Type: TypStr}, {Name: "f", Type: PtrOf(TypVoid)}}, Ret: TypInt},
			{Name: "flush",  CFn: "fflush", Params: []Param{{Name: "f", Type: PtrOf(TypVoid)}}, Ret: TypInt},
			{Name: "eof",    CFn: "feof",   Params: []Param{{Name: "f", Type: PtrOf(TypVoid)}}, Ret: TypInt},
			{Name: "printf", CFn: "printf", Variadic: true, Params: []Param{{Name: "fmt", Type: TypStr}}, Ret: TypInt},
			{Name: "scanf",  CFn: "scanf",  Variadic: true, Params: []Param{{Name: "fmt", Type: TypStr}}, Ret: TypInt},
			{Name: "getchar",CFn: "getchar",Params: []Param{}, Ret: TypInt},
			{Name: "putchar",CFn: "putchar",Params: []Param{{Name: "c", Type: TypInt}}, Ret: TypInt},
		},
	},

	// ── std::math ─────────────────────────────────────────────────────────────
	"std::math": {
		Headers: []string{"math.h"},
		Fns: []StdFn{
			{Name: "sqrt",  CFn: "sqrt",  Params: []Param{{Name: "x", Type: TypFloat}}, Ret: TypFloat},
			{Name: "pow",   CFn: "pow",   Params: []Param{{Name: "b", Type: TypFloat}, {Name: "e", Type: TypFloat}}, Ret: TypFloat},
			{Name: "abs",   CFn: "fabs",  Params: []Param{{Name: "x", Type: TypFloat}}, Ret: TypFloat},
			{Name: "floor", CFn: "floor", Params: []Param{{Name: "x", Type: TypFloat}}, Ret: TypFloat},
			{Name: "ceil",  CFn: "ceil",  Params: []Param{{Name: "x", Type: TypFloat}}, Ret: TypFloat},
			{Name: "round", CFn: "round", Params: []Param{{Name: "x", Type: TypFloat}}, Ret: TypFloat},
			{Name: "sin",   CFn: "sin",   Params: []Param{{Name: "x", Type: TypFloat}}, Ret: TypFloat},
			{Name: "cos",   CFn: "cos",   Params: []Param{{Name: "x", Type: TypFloat}}, Ret: TypFloat},
			{Name: "tan",   CFn: "tan",   Params: []Param{{Name: "x", Type: TypFloat}}, Ret: TypFloat},
			{Name: "log",   CFn: "log",   Params: []Param{{Name: "x", Type: TypFloat}}, Ret: TypFloat},
			{Name: "log2",  CFn: "log2",  Params: []Param{{Name: "x", Type: TypFloat}}, Ret: TypFloat},
			{Name: "log10", CFn: "log10", Params: []Param{{Name: "x", Type: TypFloat}}, Ret: TypFloat},
			{Name: "exp",   CFn: "exp",   Params: []Param{{Name: "x", Type: TypFloat}}, Ret: TypFloat},
			{Name: "fmod",  CFn: "fmod",  Params: []Param{{Name: "x", Type: TypFloat}, {Name: "y", Type: TypFloat}}, Ret: TypFloat},
			{Name: "max",   CFn: "fmax",  Params: []Param{{Name: "a", Type: TypFloat}, {Name: "b", Type: TypFloat}}, Ret: TypFloat},
			{Name: "min",   CFn: "fmin",  Params: []Param{{Name: "a", Type: TypFloat}, {Name: "b", Type: TypFloat}}, Ret: TypFloat},
			{Name: "PI",    CFn: "M_PI",  Params: nil, Ret: TypFloat}, // constant
		},
	},

	// ── std::sys ──────────────────────────────────────────────────────────────
	"std::sys": {
		Headers: []string{"stdlib.h", "unistd.h"},
		Fns: []StdFn{
			{Name: "system",  CFn: "system",  Params: []Param{{Name: "cmd", Type: TypStr}}, Ret: TypInt},
			{Name: "getenv",  CFn: "getenv",  Params: []Param{{Name: "name", Type: TypStr}}, Ret: TypStr},
			{Name: "sleep",   CFn: "sleep",   Params: []Param{{Name: "secs", Type: TypInt}}, Ret: TypInt},
			{Name: "usleep",  CFn: "usleep",  Params: []Param{{Name: "us", Type: TypInt}}, Ret: TypInt},
			{Name: "getpid",  CFn: "getpid",  Params: []Param{}, Ret: TypInt},
			{Name: "exit",    CFn: "exit",    Params: []Param{{Name: "code", Type: TypInt}}, Ret: TypVoid},
		},
	},

	// ── std::conv ─────────────────────────────────────────────────────────────
	"std::conv": {
		Headers: []string{"stdlib.h", "stdio.h"},
		Fns: []StdFn{
			{Name: "to_int",   CFn: "atoi",    Params: []Param{{Name: "s", Type: TypStr}},   Ret: TypInt},
			{Name: "to_float", CFn: "atof",    Params: []Param{{Name: "s", Type: TypStr}},   Ret: TypFloat},
			{Name: "int_to_str", CFn: "__zx_int_to_str", Params: []Param{{Name: "n", Type: TypInt}}, Ret: TypStr},
		},
	},

	// ── std::mem ──────────────────────────────────────────────────────────────
	"std::mem": {
		Headers: []string{"stdlib.h", "string.h"},
		Fns: []StdFn{
			{Name: "alloc",  CFn: "malloc",  Params: []Param{{Name: "size", Type: TypInt}}, Ret: PtrOf(TypVoid)},
			{Name: "zalloc", CFn: "calloc",  Params: []Param{{Name: "n", Type: TypInt}, {Name: "size", Type: TypInt}}, Ret: PtrOf(TypVoid)},
			{Name: "realloc",CFn: "realloc", Params: []Param{{Name: "p", Type: PtrOf(TypVoid)}, {Name: "size", Type: TypInt}}, Ret: PtrOf(TypVoid)},
			{Name: "free",   CFn: "free",    Params: []Param{{Name: "p", Type: PtrOf(TypVoid)}}, Ret: TypVoid},
			{Name: "copy",   CFn: "memcpy",  Params: []Param{{Name: "dst", Type: PtrOf(TypVoid)}, {Name: "src", Type: PtrOf(TypVoid)}, {Name: "n", Type: TypInt}}, Ret: PtrOf(TypVoid)},
			{Name: "set",    CFn: "memset",  Params: []Param{{Name: "p", Type: PtrOf(TypVoid)}, {Name: "c", Type: TypInt}, {Name: "n", Type: TypInt}}, Ret: PtrOf(TypVoid)},
			{Name: "sizeof", CFn: "sizeof",  Params: []Param{{Name: "type", Type: TypAny}}, Ret: TypInt},
		},
	},

	// ── std::time ─────────────────────────────────────────────────────────────
	"std::time": {
		Headers: []string{"time.h"},
		Fns: []StdFn{
			{Name: "now",   CFn: "time",   Params: []Param{{Name: "_", Type: PtrOf(TypVoid)}}, Ret: TypInt},
			{Name: "clock", CFn: "clock",  Params: []Param{}, Ret: TypInt},
			{Name: "diff",  CFn: "difftime",Params: []Param{{Name: "t2", Type: TypInt}, {Name: "t1", Type: TypInt}}, Ret: TypFloat},
		},
	},

	// ── std::os ───────────────────────────────────────────────────────────────
	"std::os": {
		Headers: []string{"stdlib.h", "stdio.h"},
		Fns: []StdFn{
			{Name: "args",   CFn: "__zx_args",  Params: []Param{}, Ret: TypAny},
			{Name: "argc",   CFn: "__zx_argc",  Params: []Param{}, Ret: TypInt},
			{Name: "getenv", CFn: "getenv",     Params: []Param{{Name: "k", Type: TypStr}}, Ret: TypStr},
		},
	},

	// ── std::fmt ──────────────────────────────────────────────────────────────
	"std::fmt": {
		Headers: []string{"stdio.h"},
		Fns: []StdFn{
			{Name: "print",   CFn: "printf",  Variadic: true, Params: []Param{{Name: "fmt", Type: TypStr}}, Ret: TypInt},
			{Name: "eprint",  CFn: "fprintf", Variadic: true,
				Params: []Param{{Name: "f", Type: PtrOf(TypVoid)}, {Name: "fmt", Type: TypStr}}, Ret: TypInt},
			{Name: "format",  CFn: "sprintf", Variadic: true,
				Params: []Param{{Name: "buf", Type: TypStr}, {Name: "fmt", Type: TypStr}}, Ret: TypInt},
		},
	},
}

// BuiltinFns are always available without any import
var builtinFns = map[string]*BuiltinDef{
	// Type checking
	"is_nil":   {Ret: TypBool,  Arity: 1, Emit: "((void*)(%s) == NULL)"},
	"is_int":   {Ret: TypBool,  Arity: -1, Emit: "1"}, // always true for typed
	"is_float": {Ret: TypBool,  Arity: -1, Emit: "1"},
	"is_str":   {Ret: TypBool,  Arity: -1, Emit: "1"},

	// Type conversions
	"to_int":   {Ret: TypInt,   Arity: 1, Emit: "((long long)(%s))"},
	"to_float": {Ret: TypFloat, Arity: 1, Emit: "((double)(%s))"},
	"to_bool":  {Ret: TypBool,  Arity: 1, Emit: "(!!((%s)))"},
	"to_char":  {Ret: TypChar,  Arity: 1, Emit: "((char)(%s))"},

	// Math shorthands (no import needed)
	"abs":   {Ret: TypFloat, Arity: 1, Emit: "fabs((double)(%s))"},
	"min":   {Ret: TypAny,  Arity: 2, Emit: "((%s) < (%s) ? (%s) : (%s))", DupArgs: true},
	"max":   {Ret: TypAny,  Arity: 2, Emit: "((%s) > (%s) ? (%s) : (%s))", DupArgs: true},

	// Memory
	"alloc":  {Ret: PtrOf(TypVoid), Arity: 1, Emit: "malloc((size_t)(%s))"},
	"free":   {Ret: TypVoid,        Arity: 1, Emit: "free(%s)"},

	// String
	"len":    {Ret: TypInt,  Arity: 1, Emit: "(long long)strlen((const char*)(%s))"},
	"str_eq": {Ret: TypBool, Arity: 2, Emit: "(strcmp(%s, %s) == 0)"},

	// System
	"system": {Ret: TypInt,  Arity: 1, Emit: "system(%s)"},
	"getenv": {Ret: TypStr,  Arity: 1, Emit: "getenv(%s)"},

	// Array
	"sizeof": {Ret: TypInt, Arity: -1, Emit: "(long long)sizeof(%s)"},
}

// BuiltinDef describes how to emit a builtin
type BuiltinDef struct {
	Ret     *ZXType
	Arity   int    // -1 = any
	Emit    string // C format string; %s = arg
	DupArgs bool   // for min/max: need to emit args twice
}

// LookupBuiltin returns a BuiltinDef if name is a builtin
func LookupBuiltin(name string) *BuiltinDef {
	return builtinFns[name]
}

// LookupStdModule returns a module by name (e.g. "std::str")
func LookupStdModule(name string) *StdModule {
	return stdModules[name]
}

// StdHeaders returns all required C headers for imported std modules
func (prog *Program) StdHeaders() []string {
	seen := map[string]bool{}
	var headers []string
	for _, imp := range prog.Imports {
		if imp.IsStd {
			mod := LookupStdModule(imp.Module)
			if mod != nil {
				for _, h := range mod.Headers {
					if !seen[h] { seen[h] = true; headers = append(headers, h) }
				}
			}
		}
	}
	return headers
}

// AllStdFns returns all functions registered from imported std modules
func (prog *Program) AllStdFns() map[string]*StdFn {
	result := map[string]*StdFn{}
	for _, imp := range prog.Imports {
		if imp.IsStd {
			mod := LookupStdModule(imp.Module)
			if mod != nil {
				for i := range mod.Fns {
					fn := &mod.Fns[i]
					result[fn.Name] = fn
				}
			}
		}
	}
	return result
}

// known C stdlib function names — not re-declared even if in extern
var knownCFuncs = map[string]bool{
	"printf":true,"fprintf":true,"sprintf":true,"snprintf":true,
	"scanf":true,"fscanf":true,"sscanf":true,
	"fopen":true,"fclose":true,"fread":true,"fwrite":true,
	"fgets":true,"fputs":true,"feof":true,"fflush":true,
	"puts":true,"getchar":true,"putchar":true,"getc":true,"putc":true,"perror":true,
	"malloc":true,"calloc":true,"realloc":true,"free":true,
	"exit":true,"abort":true,"atoi":true,"atof":true,"atol":true,
	"rand":true,"srand":true,"abs":true,"labs":true,
	"strtol":true,"strtod":true,"qsort":true,"bsearch":true,
	"strlen":true,"strcpy":true,"strncpy":true,"strcat":true,"strncat":true,
	"strcmp":true,"strncmp":true,"strchr":true,"strrchr":true,"strstr":true,
	"memcpy":true,"memmove":true,"memset":true,"memcmp":true,
	"sqrt":true,"pow":true,"fabs":true,"floor":true,"ceil":true,
	"sin":true,"cos":true,"tan":true,"asin":true,"acos":true,"atan":true,
	"atan2":true,"exp":true,"log":true,"log2":true,"log10":true,
	"fmod":true,"round":true,"trunc":true,"fmax":true,"fmin":true,
	"isalpha":true,"isdigit":true,"isspace":true,"isupper":true,"islower":true,
	"toupper":true,"tolower":true,
	"time":true,"clock":true,"difftime":true,
	"sleep":true,"usleep":true,"getpid":true,
	"system":true,"getenv":true,
}
