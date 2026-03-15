package main

import "strings"

// ─────────────────────────────────────────────────────────────────────────────
//  ZX Standard Library  v0.5
// ─────────────────────────────────────────────────────────────────────────────

type StdModule struct {
	Headers []string
	Fns     []StdFn
	Helpers []string // raw C code to emit
}

type StdFn struct {
	Name     string
	CFn      string
	Variadic bool
	Params   []Param
	Ret      *ZXType
	Macro    bool // emit as macro, not fn call
}

var stdModules = map[string]*StdModule{

	// ── std::str ──────────────────────────────────────────────────────────────
	"std::str": {
		Headers: []string{"string.h", "stdio.h", "ctype.h", "stdlib.h"},
		Fns: []StdFn{
			{Name: "str_len", CFn: "strlen", Params: []Param{{Name: "s", Type: TypStr}}, Ret: TypInt},
			{Name: "str_cpy", CFn: "strcpy", Params: []Param{{Name: "d", Type: TypStr}, {Name: "s", Type: TypStr}}, Ret: TypStr},
			{Name: "str_cat", CFn: "strcat", Params: []Param{{Name: "d", Type: TypStr}, {Name: "s", Type: TypStr}}, Ret: TypStr},
			{Name: "str_ncpy", CFn: "strncpy", Params: []Param{{Name: "d", Type: TypStr}, {Name: "s", Type: TypStr}, {Name: "n", Type: TypInt}}, Ret: TypStr},
			{Name: "str_cmp", CFn: "strcmp", Params: []Param{{Name: "a", Type: TypStr}, {Name: "b", Type: TypStr}}, Ret: TypInt},
			{Name: "str_ncmp", CFn: "strncmp", Params: []Param{{Name: "a", Type: TypStr}, {Name: "b", Type: TypStr}, {Name: "n", Type: TypInt}}, Ret: TypInt},
			{Name: "str_find", CFn: "strstr", Params: []Param{{Name: "hay", Type: TypStr}, {Name: "needle", Type: TypStr}}, Ret: TypStr},
			{Name: "str_chr", CFn: "strchr", Params: []Param{{Name: "s", Type: TypStr}, {Name: "c", Type: TypInt}}, Ret: TypStr},
			{Name: "str_rchr", CFn: "strrchr", Params: []Param{{Name: "s", Type: TypStr}, {Name: "c", Type: TypInt}}, Ret: TypStr},
			{Name: "str_upper", CFn: "toupper", Params: []Param{{Name: "c", Type: TypInt}}, Ret: TypInt},
			{Name: "str_lower", CFn: "tolower", Params: []Param{{Name: "c", Type: TypInt}}, Ret: TypInt},
			{Name: "str_to_int", CFn: "atoi", Params: []Param{{Name: "s", Type: TypStr}}, Ret: TypInt},
			{Name: "str_to_float", CFn: "atof", Params: []Param{{Name: "s", Type: TypStr}}, Ret: TypFloat},
			{Name: "str_fmt", CFn: "sprintf", Variadic: true, Params: []Param{{Name: "buf", Type: TypStr}, {Name: "fmt", Type: TypStr}}, Ret: TypInt},
			{Name: "str_nfmt", CFn: "snprintf", Variadic: true, Params: []Param{{Name: "buf", Type: TypStr}, {Name: "n", Type: TypInt}, {Name: "fmt", Type: TypStr}}, Ret: TypInt},
			{Name: "str_dup", CFn: "strdup", Params: []Param{{Name: "s", Type: TypStr}}, Ret: TypStr},
			{Name: "is_alpha", CFn: "isalpha", Params: []Param{{Name: "c", Type: TypInt}}, Ret: TypInt},
			{Name: "is_digit", CFn: "isdigit", Params: []Param{{Name: "c", Type: TypInt}}, Ret: TypInt},
			{Name: "is_space", CFn: "isspace", Params: []Param{{Name: "c", Type: TypInt}}, Ret: TypInt},
			{Name: "is_upper", CFn: "isupper", Params: []Param{{Name: "c", Type: TypInt}}, Ret: TypInt},
			{Name: "is_lower", CFn: "islower", Params: []Param{{Name: "c", Type: TypInt}}, Ret: TypInt},
		},
	},

	// ── std::io ───────────────────────────────────────────────────────────────
	"std::io": {
		Headers: []string{"stdio.h"},
		Fns: []StdFn{
			{Name: "open", CFn: "fopen", Params: []Param{{Name: "path", Type: TypStr}, {Name: "mode", Type: TypStr}}, Ret: PtrOf(TypVoid)},
			{Name: "close", CFn: "fclose", Params: []Param{{Name: "f", Type: PtrOf(TypVoid)}}, Ret: TypInt},
			{Name: "read", CFn: "fgets", Params: []Param{{Name: "buf", Type: TypStr}, {Name: "n", Type: TypInt}, {Name: "f", Type: PtrOf(TypVoid)}}, Ret: TypStr},
			{Name: "write", CFn: "fputs", Params: []Param{{Name: "s", Type: TypStr}, {Name: "f", Type: PtrOf(TypVoid)}}, Ret: TypInt},
			{Name: "writef", CFn: "fprintf", Variadic: true, Params: []Param{{Name: "f", Type: PtrOf(TypVoid)}, {Name: "fmt", Type: TypStr}}, Ret: TypInt},
			{Name: "flush", CFn: "fflush", Params: []Param{{Name: "f", Type: PtrOf(TypVoid)}}, Ret: TypInt},
			{Name: "eof", CFn: "feof", Params: []Param{{Name: "f", Type: PtrOf(TypVoid)}}, Ret: TypInt},
			{Name: "seek", CFn: "fseek", Params: []Param{{Name: "f", Type: PtrOf(TypVoid)}, {Name: "off", Type: TypInt}, {Name: "whence", Type: TypInt}}, Ret: TypInt},
			{Name: "tell", CFn: "ftell", Params: []Param{{Name: "f", Type: PtrOf(TypVoid)}}, Ret: TypInt},
			{Name: "printf", CFn: "printf", Variadic: true, Params: []Param{{Name: "fmt", Type: TypStr}}, Ret: TypInt},
			{Name: "scanf", CFn: "scanf", Variadic: true, Params: []Param{{Name: "fmt", Type: TypStr}}, Ret: TypInt},
			{Name: "sscanf", CFn: "sscanf", Variadic: true, Params: []Param{{Name: "s", Type: TypStr}, {Name: "fmt", Type: TypStr}}, Ret: TypInt},
			{Name: "getchar", CFn: "getchar", Params: []Param{}, Ret: TypInt},
			{Name: "putchar", CFn: "putchar", Params: []Param{{Name: "c", Type: TypInt}}, Ret: TypInt},
			{Name: "gets", CFn: "fgets", Params: []Param{{Name: "buf", Type: TypStr}, {Name: "n", Type: TypInt}, {Name: "f", Type: PtrOf(TypVoid)}}, Ret: TypStr},
			{Name: "fread", CFn: "fread", Params: []Param{{Name: "ptr", Type: PtrOf(TypVoid)}, {Name: "sz", Type: TypInt}, {Name: "n", Type: TypInt}, {Name: "f", Type: PtrOf(TypVoid)}}, Ret: TypInt},
			{Name: "fwrite", CFn: "fwrite", Params: []Param{{Name: "ptr", Type: PtrOf(TypVoid)}, {Name: "sz", Type: TypInt}, {Name: "n", Type: TypInt}, {Name: "f", Type: PtrOf(TypVoid)}}, Ret: TypInt},
		},
	},

	// ── std::math ─────────────────────────────────────────────────────────────
	"std::math": {
		Headers: []string{"math.h"},
		Fns: []StdFn{
			{Name: "sqrt", CFn: "sqrt", Params: []Param{{Name: "x", Type: TypFloat}}, Ret: TypFloat},
			{Name: "cbrt", CFn: "cbrt", Params: []Param{{Name: "x", Type: TypFloat}}, Ret: TypFloat},
			{Name: "pow", CFn: "pow", Params: []Param{{Name: "b", Type: TypFloat}, {Name: "e", Type: TypFloat}}, Ret: TypFloat},
			{Name: "fabs", CFn: "fabs", Params: []Param{{Name: "x", Type: TypFloat}}, Ret: TypFloat},
			{Name: "floor", CFn: "floor", Params: []Param{{Name: "x", Type: TypFloat}}, Ret: TypFloat},
			{Name: "ceil", CFn: "ceil", Params: []Param{{Name: "x", Type: TypFloat}}, Ret: TypFloat},
			{Name: "round", CFn: "round", Params: []Param{{Name: "x", Type: TypFloat}}, Ret: TypFloat},
			{Name: "trunc", CFn: "trunc", Params: []Param{{Name: "x", Type: TypFloat}}, Ret: TypFloat},
			{Name: "sin", CFn: "sin", Params: []Param{{Name: "x", Type: TypFloat}}, Ret: TypFloat},
			{Name: "cos", CFn: "cos", Params: []Param{{Name: "x", Type: TypFloat}}, Ret: TypFloat},
			{Name: "tan", CFn: "tan", Params: []Param{{Name: "x", Type: TypFloat}}, Ret: TypFloat},
			{Name: "asin", CFn: "asin", Params: []Param{{Name: "x", Type: TypFloat}}, Ret: TypFloat},
			{Name: "acos", CFn: "acos", Params: []Param{{Name: "x", Type: TypFloat}}, Ret: TypFloat},
			{Name: "atan", CFn: "atan", Params: []Param{{Name: "x", Type: TypFloat}}, Ret: TypFloat},
			{Name: "atan2", CFn: "atan2", Params: []Param{{Name: "y", Type: TypFloat}, {Name: "x", Type: TypFloat}}, Ret: TypFloat},
			{Name: "log", CFn: "log", Params: []Param{{Name: "x", Type: TypFloat}}, Ret: TypFloat},
			{Name: "log2", CFn: "log2", Params: []Param{{Name: "x", Type: TypFloat}}, Ret: TypFloat},
			{Name: "log10", CFn: "log10", Params: []Param{{Name: "x", Type: TypFloat}}, Ret: TypFloat},
			{Name: "exp", CFn: "exp", Params: []Param{{Name: "x", Type: TypFloat}}, Ret: TypFloat},
			{Name: "fmod", CFn: "fmod", Params: []Param{{Name: "x", Type: TypFloat}, {Name: "y", Type: TypFloat}}, Ret: TypFloat},
			{Name: "fmax", CFn: "fmax", Params: []Param{{Name: "a", Type: TypFloat}, {Name: "b", Type: TypFloat}}, Ret: TypFloat},
			{Name: "fmin", CFn: "fmin", Params: []Param{{Name: "a", Type: TypFloat}, {Name: "b", Type: TypFloat}}, Ret: TypFloat},
		},
	},

	// ── std::sys ──────────────────────────────────────────────────────────────
	"std::sys": {
		Headers: []string{"stdlib.h", "unistd.h", "stdio.h"},
		Helpers: []string{
			`/* std::sys helpers */`,
			`static int __zx_system_ok(const char* cmd) { return system(cmd) == 0; }`,
		},
		Fns: []StdFn{
			{Name: "run", CFn: "system", Params: []Param{{Name: "cmd", Type: TypStr}}, Ret: TypInt},
			{Name: "run_ok", CFn: "__zx_system_ok", Params: []Param{{Name: "cmd", Type: TypStr}}, Ret: TypBool},
			{Name: "getenv", CFn: "getenv", Params: []Param{{Name: "k", Type: TypStr}}, Ret: TypStr},
			{Name: "setenv", CFn: "setenv", Params: []Param{{Name: "k", Type: TypStr}, {Name: "v", Type: TypStr}, {Name: "overwrite", Type: TypInt}}, Ret: TypInt},
			{Name: "sleep", CFn: "sleep", Params: []Param{{Name: "secs", Type: TypInt}}, Ret: TypInt},
			{Name: "usleep", CFn: "usleep", Params: []Param{{Name: "us", Type: TypInt}}, Ret: TypInt},
			{Name: "getpid", CFn: "getpid", Params: []Param{}, Ret: TypInt},
			{Name: "exit", CFn: "exit", Params: []Param{{Name: "code", Type: TypInt}}, Ret: TypVoid},
		},
	},

	// ── std::fs ───────────────────────────────────────────────────────────────
	"std::fs": {
		Headers: []string{"stdio.h", "stdlib.h", "string.h"},
		Helpers: []string{
			`/* std::fs helpers */`,
			`static char* __zx_read_file(const char* path) {`,
			`    FILE* f = fopen(path, "r");`,
			`    if (!f) return NULL;`,
			`    fseek(f, 0, SEEK_END);`,
			`    long sz = ftell(f);`,
			`    rewind(f);`,
			`    char* buf = (char*)malloc(sz + 1);`,
			`    if (!buf) { fclose(f); return NULL; }`,
			`    fread(buf, 1, sz, f);`,
			`    buf[sz] = '\0';`,
			`    fclose(f);`,
			`    return buf;`,
			`}`,
			`static int __zx_write_file(const char* path, const char* content) {`,
			`    FILE* f = fopen(path, "w");`,
			`    if (!f) return -1;`,
			`    fputs(content, f);`,
			`    fclose(f);`,
			`    return 0;`,
			`}`,
			`static int __zx_append_file(const char* path, const char* content) {`,
			`    FILE* f = fopen(path, "a");`,
			`    if (!f) return -1;`,
			`    fputs(content, f);`,
			`    fclose(f);`,
			`    return 0;`,
			`}`,
			`static int __zx_file_exists(const char* path) {`,
			`    FILE* f = fopen(path, "r");`,
			`    if (f) { fclose(f); return 1; }`,
			`    return 0;`,
			`}`,
		},
		Fns: []StdFn{
			{Name: "read", CFn: "__zx_read_file", Params: []Param{{Name: "path", Type: TypStr}}, Ret: TypStr},
			{Name: "write", CFn: "__zx_write_file", Params: []Param{{Name: "path", Type: TypStr}, {Name: "content", Type: TypStr}}, Ret: TypInt},
			{Name: "append", CFn: "__zx_append_file", Params: []Param{{Name: "path", Type: TypStr}, {Name: "content", Type: TypStr}}, Ret: TypInt},
			{Name: "exists", CFn: "__zx_file_exists", Params: []Param{{Name: "path", Type: TypStr}}, Ret: TypBool},
			{Name: "open", CFn: "fopen", Params: []Param{{Name: "path", Type: TypStr}, {Name: "mode", Type: TypStr}}, Ret: PtrOf(TypVoid)},
			{Name: "close", CFn: "fclose", Params: []Param{{Name: "f", Type: PtrOf(TypVoid)}}, Ret: TypInt},
			{Name: "remove", CFn: "remove", Params: []Param{{Name: "path", Type: TypStr}}, Ret: TypInt},
			{Name: "rename", CFn: "rename", Params: []Param{{Name: "old", Type: TypStr}, {Name: "new", Type: TypStr}}, Ret: TypInt},
		},
	},

	// ── std::cmd ──────────────────────────────────────────────────────────────
	"std::cmd": {
		Headers: []string{"stdio.h", "stdlib.h"},
		Helpers: []string{
			`/* std::cmd helpers */`,
			`static char* __zx_capture(const char* cmd) {`,
			`    FILE* p = popen(cmd, "r");`,
			`    if (!p) return "";`,
			`    static char buf[65536];`,
			`    size_t total = 0;`,
			`    char tmp[1024];`,
			`    while (fgets(tmp, sizeof(tmp), p) && total + strlen(tmp) < sizeof(buf)-1) {`,
			`        strcpy(buf + total, tmp); total += strlen(tmp);`,
			`    }`,
			`    buf[total] = '\0';`,
			`    pclose(p);`,
			`    return buf;`,
			`}`,
			`static int __zx_run_exit(const char* cmd) { int r = system(cmd); return WEXITSTATUS(r); }`,
		},
		Fns: []StdFn{
			{Name: "capture", CFn: "__zx_capture", Params: []Param{{Name: "cmd", Type: TypStr}}, Ret: TypStr},
			{Name: "run", CFn: "system", Params: []Param{{Name: "cmd", Type: TypStr}}, Ret: TypInt},
			{Name: "exitcode", CFn: "__zx_run_exit", Params: []Param{{Name: "cmd", Type: TypStr}}, Ret: TypInt},
			{Name: "popen", CFn: "popen", Params: []Param{{Name: "cmd", Type: TypStr}, {Name: "mode", Type: TypStr}}, Ret: PtrOf(TypVoid)},
			{Name: "pclose", CFn: "pclose", Params: []Param{{Name: "p", Type: PtrOf(TypVoid)}}, Ret: TypInt},
		},
	},

	// ── std::mem ──────────────────────────────────────────────────────────────
	"std::mem": {
		Headers: []string{"stdlib.h", "string.h"},
		Fns: []StdFn{
			{Name: "alloc", CFn: "malloc", Params: []Param{{Name: "size", Type: TypInt}}, Ret: PtrOf(TypVoid)},
			{Name: "zalloc", CFn: "calloc", Params: []Param{{Name: "n", Type: TypInt}, {Name: "sz", Type: TypInt}}, Ret: PtrOf(TypVoid)},
			{Name: "realloc", CFn: "realloc", Params: []Param{{Name: "p", Type: PtrOf(TypVoid)}, {Name: "sz", Type: TypInt}}, Ret: PtrOf(TypVoid)},
			{Name: "free", CFn: "free", Params: []Param{{Name: "p", Type: PtrOf(TypVoid)}}, Ret: TypVoid},
			{Name: "copy", CFn: "memcpy", Params: []Param{{Name: "d", Type: PtrOf(TypVoid)}, {Name: "s", Type: PtrOf(TypVoid)}, {Name: "n", Type: TypInt}}, Ret: PtrOf(TypVoid)},
			{Name: "set", CFn: "memset", Params: []Param{{Name: "p", Type: PtrOf(TypVoid)}, {Name: "c", Type: TypInt}, {Name: "n", Type: TypInt}}, Ret: PtrOf(TypVoid)},
			{Name: "cmp", CFn: "memcmp", Params: []Param{{Name: "a", Type: PtrOf(TypVoid)}, {Name: "b", Type: PtrOf(TypVoid)}, {Name: "n", Type: TypInt}}, Ret: TypInt},
			{Name: "move", CFn: "memmove", Params: []Param{{Name: "d", Type: PtrOf(TypVoid)}, {Name: "s", Type: PtrOf(TypVoid)}, {Name: "n", Type: TypInt}}, Ret: PtrOf(TypVoid)},
		},
	},

	// ── std::conv ─────────────────────────────────────────────────────────────
	"std::conv": {
		Headers: []string{"stdlib.h", "stdio.h"},
		Helpers: []string{
			`static char __zx_conv_buf[128];`,
			`static const char* __zx_int_to_str(long long n) { snprintf(__zx_conv_buf, sizeof(__zx_conv_buf), "%lld", n); return __zx_conv_buf; }`,
			`static const char* __zx_float_to_str(double f) { snprintf(__zx_conv_buf, sizeof(__zx_conv_buf), "%g", f); return __zx_conv_buf; }`,
		},
		Fns: []StdFn{
			{Name: "to_int", CFn: "atoi", Params: []Param{{Name: "s", Type: TypStr}}, Ret: TypInt},
			{Name: "to_float", CFn: "atof", Params: []Param{{Name: "s", Type: TypStr}}, Ret: TypFloat},
			{Name: "int_to_str", CFn: "__zx_int_to_str", Params: []Param{{Name: "n", Type: TypInt}}, Ret: TypStr},
			{Name: "float_to_str", CFn: "__zx_float_to_str", Params: []Param{{Name: "f", Type: TypFloat}}, Ret: TypStr},
		},
	},

	// ── std::time ─────────────────────────────────────────────────────────────
	"std::time": {
		Headers: []string{"time.h"},
		Fns: []StdFn{
			{Name: "now", CFn: "time", Params: []Param{{Name: "_", Type: PtrOf(TypVoid)}}, Ret: TypInt},
			{Name: "clock", CFn: "clock", Params: []Param{}, Ret: TypInt},
			{Name: "diff", CFn: "difftime", Params: []Param{{Name: "t2", Type: TypInt}, {Name: "t1", Type: TypInt}}, Ret: TypFloat},
		},
	},

	// ── std::os ───────────────────────────────────────────────────────────────
	"std::os": {
		Headers: []string{"stdlib.h", "stdio.h"},
		Fns: []StdFn{
			{Name: "getenv", CFn: "getenv", Params: []Param{{Name: "k", Type: TypStr}}, Ret: TypStr},
			{Name: "exit", CFn: "exit", Params: []Param{{Name: "code", Type: TypInt}}, Ret: TypVoid},
		},
	},

	// ── std::fmt ──────────────────────────────────────────────────────────────
	"std::fmt": {
		Headers: []string{"stdio.h"},
		Fns: []StdFn{
			{Name: "print", CFn: "printf", Variadic: true, Params: []Param{{Name: "fmt", Type: TypStr}}, Ret: TypInt},
			{Name: "eprint", CFn: "fprintf", Variadic: true, Params: []Param{{Name: "f", Type: PtrOf(TypVoid)}, {Name: "fmt", Type: TypStr}}, Ret: TypInt},
			{Name: "sprintf", CFn: "sprintf", Variadic: true, Params: []Param{{Name: "buf", Type: TypStr}, {Name: "fmt", Type: TypStr}}, Ret: TypInt},
		},
	},

	// ── std::net (basic socket helpers) ──────────────────────────────────────
	"std::net": {
		Headers: []string{"stdio.h", "stdlib.h", "string.h", "sys/socket.h", "netinet/in.h", "arpa/inet.h", "unistd.h"},
		Helpers: []string{
			`/* std::net helpers */`,
			`static int __zx_tcp_server(int port) {`,
			`    int fd = socket(AF_INET, SOCK_STREAM, 0);`,
			`    if (fd < 0) return -1;`,
			`    int opt = 1;`,
			`    setsockopt(fd, SOL_SOCKET, SO_REUSEADDR, &opt, sizeof(opt));`,
			`    struct sockaddr_in addr = {0};`,
			`    addr.sin_family = AF_INET;`,
			`    addr.sin_addr.s_addr = INADDR_ANY;`,
			`    addr.sin_port = htons((uint16_t)port);`,
			`    if (bind(fd, (struct sockaddr*)&addr, sizeof(addr)) < 0) { close(fd); return -1; }`,
			`    listen(fd, 10);`,
			`    return fd;`,
			`}`,
			`static int __zx_tcp_accept(int fd) { return accept(fd, NULL, NULL); }`,
			`static int __zx_tcp_send(int fd, const char* msg) { return (int)send(fd, msg, strlen(msg), 0); }`,
		},
		Fns: []StdFn{
			{Name: "tcp_server", CFn: "__zx_tcp_server", Params: []Param{{Name: "port", Type: TypInt}}, Ret: TypInt},
			{Name: "tcp_accept", CFn: "__zx_tcp_accept", Params: []Param{{Name: "fd", Type: TypInt}}, Ret: TypInt},
			{Name: "tcp_send", CFn: "__zx_tcp_send", Params: []Param{{Name: "fd", Type: TypInt}, {Name: "msg", Type: TypStr}}, Ret: TypInt},
			{Name: "close_fd", CFn: "close", Params: []Param{{Name: "fd", Type: TypInt}}, Ret: TypInt},
		},
	},
}

// ── Always-available builtin functions ───────────────────────────────────────

type BuiltinDef struct {
	Ret     *ZXType
	Arity   int    // -1 = any
	Emit    string // C format, %s = args
	DupArgs bool   // emit each arg twice (for min/max ternary)
	VarArgs bool   // accepts any number
}

var builtinFns = map[string]*BuiltinDef{
	// type checking
	"is_nil":  {Ret: TypBool, Arity: 1, Emit: "((void*)(%s) == NULL)"},
	"not_nil": {Ret: TypBool, Arity: 1, Emit: "((void*)(%s) != NULL)"},
	"is_zero": {Ret: TypBool, Arity: 1, Emit: "((%s) == 0)"},

	// type conversions
	"to_int":   {Ret: TypInt, Arity: 1, Emit: "((long long)(%s))"},
	"to_float": {Ret: TypFloat, Arity: 1, Emit: "((double)(%s))"},
	"to_bool":  {Ret: TypBool, Arity: 1, Emit: "(!!((%s)))"},
	"to_char":  {Ret: TypChar, Arity: 1, Emit: "((char)(%s))"},
	"to_str":   {Ret: TypStr, Arity: 1, Emit: "/* to_str: use int_to_str from std::conv */"},

	// math (always available without import)
	"abs":   {Ret: TypAny, Arity: 1, Emit: "(((%s) < 0) ? -(%s) : (%s))", DupArgs: true},
	"min":   {Ret: TypAny, Arity: 2, Emit: "((%s) < (%s) ? (%s) : (%s))", DupArgs: true},
	"max":   {Ret: TypAny, Arity: 2, Emit: "((%s) > (%s) ? (%s) : (%s))", DupArgs: true},
	"clamp": {Ret: TypAny, Arity: 3, Emit: "((%s) < (%s) ? (%s) : ((%s) > (%s) ? (%s) : (%s)))"},

	// string
	"len":    {Ret: TypInt, Arity: 1, Emit: "(long long)strlen((const char*)(%s))"},
	"str_eq": {Ret: TypBool, Arity: 2, Emit: "(strcmp(%s, %s) == 0)"},
	"str_ne": {Ret: TypBool, Arity: 2, Emit: "(strcmp(%s, %s) != 0)"},

	// memory
	"alloc":  {Ret: PtrOf(TypVoid), Arity: 1, Emit: "malloc((size_t)(%s))"},
	"zalloc": {Ret: PtrOf(TypVoid), Arity: 2, Emit: "calloc((size_t)(%s), (size_t)(%s))"},
	"free":   {Ret: TypVoid, Arity: 1, Emit: "free(%s)"},

	// system
	"system": {Ret: TypInt, Arity: 1, Emit: "system(%s)"},
	"getenv": {Ret: TypStr, Arity: 1, Emit: "getenv(%s)"},
	"sizeof": {Ret: TypInt, Arity: 1, Emit: "(long long)sizeof(%s)"},

	// I/O
	"input":   {Ret: TypStr, Arity: 0, Emit: "/* input: use fgets(buf, N, stdin) */"},
	"print":   {Ret: TypVoid, Arity: -1, VarArgs: true},
	"println": {Ret: TypVoid, Arity: -1, VarArgs: true},
}

// ── Module registry helpers ───────────────────────────────────────────────────

func LookupBuiltin(name string) *BuiltinDef  { return builtinFns[name] }
func LookupStdModule(name string) *StdModule { return stdModules[name] }

func (prog *Program) StdHeaders() []string {
	seen := map[string]bool{"stdio.h": true, "stdlib.h": true, "string.h": true, "math.h": true}
	var headers []string
	for _, imp := range prog.Imports {
		if imp.IsStd {
			mod := LookupStdModule(imp.Module)
			if mod != nil {
				for _, h := range mod.Headers {
					if !seen[h] {
						seen[h] = true
						headers = append(headers, h)
					}
				}
			}
		}
	}
	return headers
}

func (prog *Program) StdHelpers() []string {
	var helpers []string
	for _, imp := range prog.Imports {
		if imp.IsStd {
			mod := LookupStdModule(imp.Module)
			if mod != nil {
				helpers = append(helpers, mod.Helpers...)
			}
		}
	}
	return helpers
}

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

var knownCFuncs = map[string]bool{
	"printf": true, "fprintf": true, "sprintf": true, "snprintf": true,
	"scanf": true, "fscanf": true, "sscanf": true,
	"fopen": true, "fclose": true, "fread": true, "fwrite": true,
	"fgets": true, "fputs": true, "feof": true, "fflush": true, "fseek": true, "ftell": true,
	"puts": true, "getchar": true, "putchar": true, "getc": true, "putc": true, "perror": true,
	"malloc": true, "calloc": true, "realloc": true, "free": true,
	"exit": true, "abort": true, "atoi": true, "atof": true, "atol": true,
	"rand": true, "srand": true, "abs": true, "labs": true,
	"strtol": true, "strtod": true, "qsort": true, "bsearch": true,
	"strlen": true, "strcpy": true, "strncpy": true, "strcat": true, "strncat": true,
	"strcmp": true, "strncmp": true, "strchr": true, "strrchr": true, "strstr": true, "strdup": true,
	"memcpy": true, "memmove": true, "memset": true, "memcmp": true,
	"sqrt": true, "pow": true, "fabs": true, "floor": true, "ceil": true,
	"sin": true, "cos": true, "tan": true, "asin": true, "acos": true, "atan": true,
	"atan2": true, "exp": true, "log": true, "log2": true, "log10": true,
	"fmod": true, "round": true, "trunc": true, "fmax": true, "fmin": true, "cbrt": true,
	"isalpha": true, "isdigit": true, "isspace": true, "isupper": true, "islower": true,
	"toupper": true, "tolower": true,
	"time": true, "clock": true, "difftime": true,
	"sleep": true, "usleep": true, "getpid": true,
	"system": true, "getenv": true, "setenv": true, "popen": true, "pclose": true,
	"remove": true, "rename": true, "rewind": true,
	"socket": true, "bind": true, "listen": true, "accept": true, "connect": true,
	"send": true, "recv": true, "close": true, "htons": true, "htonl": true,
	"setsockopt": true, "getsockopt": true,
}

// suppress unused import
var _ = strings.Join
