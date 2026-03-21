package main

import "strings"

// ─────────────────────────────────────────────────────────────────────────────
//  ZX Standard Library  v3.2
// ─────────────────────────────────────────────────────────────────────────────

type StdModule struct {
	Headers []string
	Fns     []StdFn
	Helpers []string
}

type StdFn struct {
	Name     string
	CFn      string
	Variadic bool
	Params   []Param
	Ret      *ZXType
	Macro    bool
}

var stdModules = map[string]*StdModule{

	// ── std::str ──────────────────────────────────────────────────────────────
	"std::str": {
		Headers: []string{"string.h", "stdio.h", "ctype.h", "stdlib.h"},
		Helpers: []string{
			`/* std::str helpers */`,
			`static char __zx_str_buf[4096];`,
			`static const char* __zx_str_repeat(const char* s, int n) {`,
			`    size_t l = strlen(s); int i;`,
			`    __zx_str_buf[0] = '\0';`,
			`    for (i = 0; i < n && strlen(__zx_str_buf) + l < sizeof(__zx_str_buf) - 1; i++)`,
			`        strcat(__zx_str_buf, s);`,
			`    return __zx_str_buf;`,
			`}`,
			`static const char* __zx_str_reverse(const char* s) {`,
			`    size_t l = strlen(s), i;`,
			`    for (i = 0; i < l; i++) __zx_str_buf[i] = s[l - 1 - i];`,
			`    __zx_str_buf[l] = '\0';`,
			`    return __zx_str_buf;`,
			`}`,
			`static const char* __zx_str_trim(const char* s) {`,
			`    while (*s == ' ' || *s == '\t' || *s == '\n' || *s == '\r') s++;`,
			`    size_t l = strlen(s);`,
			`    strncpy(__zx_str_buf, s, sizeof(__zx_str_buf) - 1);`,
			`    __zx_str_buf[l] = '\0';`,
			`    while (l > 0 && (__zx_str_buf[l-1]==' ' || __zx_str_buf[l-1]=='\t' ||`,
			`                      __zx_str_buf[l-1]=='\n' || __zx_str_buf[l-1]=='\r'))`,
			`        __zx_str_buf[--l] = '\0';`,
			`    return __zx_str_buf;`,
			`}`,
			`static const char* __zx_str_slice(const char* s, int from, int to) {`,
			`    size_t l = strlen(s);`,
			`    if (from < 0) { from = 0; }`,
			`    if (to < 0 || (size_t)to > l) { to = (int)l; }`,
			`    if (from >= to) { __zx_str_buf[0] = '\0'; return __zx_str_buf; }`,
			`    strncpy(__zx_str_buf, s + from, (size_t)(to - from));`,
			`    __zx_str_buf[to - from] = '\0';`,
			`    return __zx_str_buf;`,
			`}`,
			`static int __zx_str_starts(const char* s, const char* p) {`,
			`    return strncmp(s, p, strlen(p)) == 0;`,
			`}`,
			`static int __zx_str_ends(const char* s, const char* x) {`,
			`    size_t sl = strlen(s), xl = strlen(x);`,
			`    return sl >= xl && strcmp(s + sl - xl, x) == 0;`,
			`}`,
			`static int __zx_str_contains(const char* s, const char* needle) {`,
			`    return strstr(s, needle) != NULL;`,
			`}`,
			`static const char* __zx_str_replace_first(const char* s, const char* from, const char* to) {`,
			`    const char* p = strstr(s, from);`,
			`    if (!p) { strncpy(__zx_str_buf, s, sizeof(__zx_str_buf)-1); __zx_str_buf[sizeof(__zx_str_buf)-1]='\0'; return __zx_str_buf; }`,
			`    size_t pre = (size_t)(p - s);`,
			`    snprintf(__zx_str_buf, sizeof(__zx_str_buf), "%.*s%s%s", (int)pre, s, to, p + strlen(from));`,
			`    return __zx_str_buf;`,
			`}`,
			`static int __zx_str_index(const char* s, const char* needle) {`,
			`    const char* p = strstr(s, needle);`,
			`    return p ? (int)(p - s) : -1;`,
			`}`,
			`static const char* __zx_str_to_upper(const char* s) {`,
			`    size_t i;`,
			`    for (i = 0; s[i] && i < sizeof(__zx_str_buf) - 1; i++)`,
			`        __zx_str_buf[i] = (char)toupper((unsigned char)s[i]);`,
			`    __zx_str_buf[i] = '\0'; return __zx_str_buf;`,
			`}`,
			`static const char* __zx_str_to_lower(const char* s) {`,
			`    size_t i;`,
			`    for (i = 0; s[i] && i < sizeof(__zx_str_buf) - 1; i++)`,
			`        __zx_str_buf[i] = (char)tolower((unsigned char)s[i]);`,
			`    __zx_str_buf[i] = '\0'; return __zx_str_buf;`,
			`}`,
		},
		Fns: []StdFn{
			{Name: "str_len", CFn: "strlen", Params: []Param{{Name: "s", Type: TypStr}}, Ret: TypInt},
			{Name: "str_cmp", CFn: "strcmp", Params: []Param{{Name: "a", Type: TypStr}, {Name: "b", Type: TypStr}}, Ret: TypInt},
			{Name: "str_ncmp", CFn: "strncmp", Params: []Param{{Name: "a", Type: TypStr}, {Name: "b", Type: TypStr}, {Name: "n", Type: TypInt}}, Ret: TypInt},
			{Name: "str_cpy", CFn: "strcpy", Params: []Param{{Name: "dst", Type: TypStr}, {Name: "src", Type: TypStr}}, Ret: TypStr},
			{Name: "str_ncpy", CFn: "strncpy", Params: []Param{{Name: "dst", Type: TypStr}, {Name: "src", Type: TypStr}, {Name: "n", Type: TypInt}}, Ret: TypStr},
			{Name: "str_cat", CFn: "strcat", Params: []Param{{Name: "dst", Type: TypStr}, {Name: "src", Type: TypStr}}, Ret: TypStr},
			{Name: "str_dup", CFn: "strdup", Params: []Param{{Name: "s", Type: TypStr}}, Ret: TypStr},
			{Name: "str_find", CFn: "strstr", Params: []Param{{Name: "haystack", Type: TypStr}, {Name: "needle", Type: TypStr}}, Ret: TypStr},
			{Name: "str_chr", CFn: "strchr", Params: []Param{{Name: "s", Type: TypStr}, {Name: "c", Type: TypInt}}, Ret: TypStr},
			{Name: "str_rchr", CFn: "strrchr", Params: []Param{{Name: "s", Type: TypStr}, {Name: "c", Type: TypInt}}, Ret: TypStr},
			{Name: "str_index", CFn: "__zx_str_index", Params: []Param{{Name: "s", Type: TypStr}, {Name: "needle", Type: TypStr}}, Ret: TypInt},
			{Name: "str_contains", CFn: "__zx_str_contains", Params: []Param{{Name: "s", Type: TypStr}, {Name: "needle", Type: TypStr}}, Ret: TypBool},
			{Name: "str_starts", CFn: "__zx_str_starts", Params: []Param{{Name: "s", Type: TypStr}, {Name: "prefix", Type: TypStr}}, Ret: TypBool},
			{Name: "str_ends", CFn: "__zx_str_ends", Params: []Param{{Name: "s", Type: TypStr}, {Name: "suffix", Type: TypStr}}, Ret: TypBool},
			{Name: "str_trim", CFn: "__zx_str_trim", Params: []Param{{Name: "s", Type: TypStr}}, Ret: TypStr},
			{Name: "str_upper", CFn: "__zx_str_to_upper", Params: []Param{{Name: "s", Type: TypStr}}, Ret: TypStr},
			{Name: "str_lower", CFn: "__zx_str_to_lower", Params: []Param{{Name: "s", Type: TypStr}}, Ret: TypStr},
			{Name: "str_reverse", CFn: "__zx_str_reverse", Params: []Param{{Name: "s", Type: TypStr}}, Ret: TypStr},
			{Name: "str_repeat", CFn: "__zx_str_repeat", Params: []Param{{Name: "s", Type: TypStr}, {Name: "n", Type: TypInt}}, Ret: TypStr},
			{Name: "str_slice", CFn: "__zx_str_slice", Params: []Param{{Name: "s", Type: TypStr}, {Name: "from", Type: TypInt}, {Name: "to", Type: TypInt}}, Ret: TypStr},
			{Name: "str_replace", CFn: "__zx_str_replace_first", Params: []Param{{Name: "s", Type: TypStr}, {Name: "from", Type: TypStr}, {Name: "to", Type: TypStr}}, Ret: TypStr},
			{Name: "str_fmt", CFn: "sprintf", Variadic: true, Params: []Param{{Name: "buf", Type: TypStr}, {Name: "fmt", Type: TypStr}}, Ret: TypInt},
			{Name: "str_nfmt", CFn: "snprintf", Variadic: true, Params: []Param{{Name: "buf", Type: TypStr}, {Name: "n", Type: TypInt}, {Name: "fmt", Type: TypStr}}, Ret: TypInt},
			{Name: "str_to_int", CFn: "atoi", Params: []Param{{Name: "s", Type: TypStr}}, Ret: TypInt},
			{Name: "str_to_float", CFn: "atof", Params: []Param{{Name: "s", Type: TypStr}}, Ret: TypFloat},
			{Name: "is_alpha", CFn: "isalpha", Params: []Param{{Name: "c", Type: TypInt}}, Ret: TypBool},
			{Name: "is_digit", CFn: "isdigit", Params: []Param{{Name: "c", Type: TypInt}}, Ret: TypBool},
			{Name: "is_alnum", CFn: "isalnum", Params: []Param{{Name: "c", Type: TypInt}}, Ret: TypBool},
			{Name: "is_space", CFn: "isspace", Params: []Param{{Name: "c", Type: TypInt}}, Ret: TypBool},
			{Name: "is_upper", CFn: "isupper", Params: []Param{{Name: "c", Type: TypInt}}, Ret: TypBool},
			{Name: "is_lower", CFn: "islower", Params: []Param{{Name: "c", Type: TypInt}}, Ret: TypBool},
			{Name: "is_punct", CFn: "ispunct", Params: []Param{{Name: "c", Type: TypInt}}, Ret: TypBool},
			{Name: "is_print", CFn: "isprint", Params: []Param{{Name: "c", Type: TypInt}}, Ret: TypBool},
			{Name: "char_upper", CFn: "toupper", Params: []Param{{Name: "c", Type: TypInt}}, Ret: TypInt},
			{Name: "char_lower", CFn: "tolower", Params: []Param{{Name: "c", Type: TypInt}}, Ret: TypInt},
		},
	},

	// ── std::io ───────────────────────────────────────────────────────────────
	"std::io": {
		Headers: []string{"stdio.h"},
		Helpers: []string{
			`/* std::io helpers */`,
			`static char __zx_io_linebuf[4096];`,
			`static const char* __zx_io_readline(FILE* f) {`,
			`    if (!fgets(__zx_io_linebuf, sizeof(__zx_io_linebuf), f)) { return NULL; }`,
			`    size_t l = strlen(__zx_io_linebuf);`,
			`    if (l > 0 && __zx_io_linebuf[l-1] == '\n') { __zx_io_linebuf[l-1] = '\0'; }`,
			`    return __zx_io_linebuf;`,
			`}`,
			`static long __zx_io_file_size(FILE* f) {`,
			`    long cur = ftell(f); fseek(f, 0, SEEK_END);`,
			`    long sz = ftell(f); fseek(f, cur, SEEK_SET);`,
			`    return sz;`,
			`}`,
		},
		Fns: []StdFn{
			{Name: "open", CFn: "fopen", Params: []Param{{Name: "path", Type: TypStr}, {Name: "mode", Type: TypStr}}, Ret: PtrOf(TypVoid)},
			{Name: "close", CFn: "fclose", Params: []Param{{Name: "f", Type: PtrOf(TypVoid)}}, Ret: TypInt},
			{Name: "read", CFn: "fgets", Params: []Param{{Name: "buf", Type: TypStr}, {Name: "n", Type: TypInt}, {Name: "f", Type: PtrOf(TypVoid)}}, Ret: TypStr},
			{Name: "readline", CFn: "__zx_io_readline", Params: []Param{{Name: "f", Type: PtrOf(TypVoid)}}, Ret: TypStr},
			{Name: "write", CFn: "fputs", Params: []Param{{Name: "s", Type: TypStr}, {Name: "f", Type: PtrOf(TypVoid)}}, Ret: TypInt},
			{Name: "writef", CFn: "fprintf", Variadic: true, Params: []Param{{Name: "f", Type: PtrOf(TypVoid)}, {Name: "fmt", Type: TypStr}}, Ret: TypInt},
			{Name: "fread", CFn: "fread", Params: []Param{{Name: "ptr", Type: PtrOf(TypVoid)}, {Name: "size", Type: TypInt}, {Name: "count", Type: TypInt}, {Name: "f", Type: PtrOf(TypVoid)}}, Ret: TypInt},
			{Name: "fwrite", CFn: "fwrite", Params: []Param{{Name: "ptr", Type: PtrOf(TypVoid)}, {Name: "size", Type: TypInt}, {Name: "count", Type: TypInt}, {Name: "f", Type: PtrOf(TypVoid)}}, Ret: TypInt},
			{Name: "flush", CFn: "fflush", Params: []Param{{Name: "f", Type: PtrOf(TypVoid)}}, Ret: TypInt},
			{Name: "eof", CFn: "feof", Params: []Param{{Name: "f", Type: PtrOf(TypVoid)}}, Ret: TypBool},
			{Name: "error", CFn: "ferror", Params: []Param{{Name: "f", Type: PtrOf(TypVoid)}}, Ret: TypInt},
			{Name: "seek", CFn: "fseek", Params: []Param{{Name: "f", Type: PtrOf(TypVoid)}, {Name: "offset", Type: TypInt}, {Name: "whence", Type: TypInt}}, Ret: TypInt},
			{Name: "tell", CFn: "ftell", Params: []Param{{Name: "f", Type: PtrOf(TypVoid)}}, Ret: TypInt},
			{Name: "rewind", CFn: "rewind", Params: []Param{{Name: "f", Type: PtrOf(TypVoid)}}, Ret: TypVoid},
			{Name: "file_size", CFn: "__zx_io_file_size", Params: []Param{{Name: "f", Type: PtrOf(TypVoid)}}, Ret: TypInt},
			{Name: "printf", CFn: "printf", Variadic: true, Params: []Param{{Name: "fmt", Type: TypStr}}, Ret: TypInt},
			{Name: "scanf", CFn: "scanf", Variadic: true, Params: []Param{{Name: "fmt", Type: TypStr}}, Ret: TypInt},
			{Name: "sscanf", CFn: "sscanf", Variadic: true, Params: []Param{{Name: "s", Type: TypStr}, {Name: "fmt", Type: TypStr}}, Ret: TypInt},
			{Name: "getchar", CFn: "getchar", Params: []Param{}, Ret: TypInt},
			{Name: "putchar", CFn: "putchar", Params: []Param{{Name: "c", Type: TypInt}}, Ret: TypInt},
			{Name: "perror", CFn: "perror", Params: []Param{{Name: "msg", Type: TypStr}}, Ret: TypVoid},
		},
	},

	// ── std::math ─────────────────────────────────────────────────────────────
	"std::math": {
		Headers: []string{"math.h", "stdlib.h"},
		Helpers: []string{
			`/* std::math helpers */`,
			`#ifndef M_PI`,
			`#define M_PI 3.14159265358979323846`,
			`#endif`,
			`#ifndef M_E`,
			`#define M_E  2.71828182845904523536`,
			`#endif`,
			`static double __zx_math_clamp(double v, double lo, double hi) {`,
			`    if (v < lo) { return lo; } if (v > hi) { return hi; } return v;`,
			`}`,
			`static long long __zx_math_gcd(long long a, long long b) {`,
			`    if (a < 0) { a = -a; }`,
			`    if (b < 0) { b = -b; }`,
			`    while (b) { long long t = b; b = a % b; a = t; }`,
			`    return a;`,
			`}`,
			`static long long __zx_math_lcm(long long a, long long b) {`,
			`    long long g = __zx_math_gcd(a, b);`,
			`    return (g == 0) ? 0LL : (a / g) * b;`,
			`}`,
			`static int    __zx_math_is_nan(double x)  { return x != x; }`,
			`static int    __zx_math_is_inf(double x)  { return x == x && x - x != 0.0; }`,
			`static double __zx_math_lerp(double a, double b, double t) { return a + t * (b - a); }`,
			`static double __zx_math_deg2rad(double d) { return d * M_PI / 180.0; }`,
			`static double __zx_math_rad2deg(double r) { return r * 180.0 / M_PI; }`,
			`static long long __zx_math_sign(long long n)  { return (n > 0) ? 1LL : ((n < 0) ? -1LL : 0LL); }`,
			`static double    __zx_math_signf(double n)    { return (n > 0.0) ? 1.0 : ((n < 0.0) ? -1.0 : 0.0); }`,
			`static long long __zx_math_ipow(long long b, long long e) {`,
			`    long long r = 1; for (; e > 0; e--) { r *= b; } return r;`,
			`}`,
		},
		Fns: []StdFn{
			{Name: "sqrt", CFn: "sqrt", Params: []Param{{Name: "x", Type: TypFloat}}, Ret: TypFloat},
			{Name: "cbrt", CFn: "cbrt", Params: []Param{{Name: "x", Type: TypFloat}}, Ret: TypFloat},
			{Name: "pow", CFn: "pow", Params: []Param{{Name: "base", Type: TypFloat}, {Name: "exp", Type: TypFloat}}, Ret: TypFloat},
			{Name: "ipow", CFn: "__zx_math_ipow", Params: []Param{{Name: "base", Type: TypInt}, {Name: "exp", Type: TypInt}}, Ret: TypInt},
			{Name: "fabs", CFn: "fabs", Params: []Param{{Name: "x", Type: TypFloat}}, Ret: TypFloat},
			{Name: "floor", CFn: "floor", Params: []Param{{Name: "x", Type: TypFloat}}, Ret: TypFloat},
			{Name: "ceil", CFn: "ceil", Params: []Param{{Name: "x", Type: TypFloat}}, Ret: TypFloat},
			{Name: "round", CFn: "round", Params: []Param{{Name: "x", Type: TypFloat}}, Ret: TypFloat},
			{Name: "trunc", CFn: "trunc", Params: []Param{{Name: "x", Type: TypFloat}}, Ret: TypFloat},
			{Name: "fmod", CFn: "fmod", Params: []Param{{Name: "x", Type: TypFloat}, {Name: "y", Type: TypFloat}}, Ret: TypFloat},
			{Name: "fmax", CFn: "fmax", Params: []Param{{Name: "a", Type: TypFloat}, {Name: "b", Type: TypFloat}}, Ret: TypFloat},
			{Name: "fmin", CFn: "fmin", Params: []Param{{Name: "a", Type: TypFloat}, {Name: "b", Type: TypFloat}}, Ret: TypFloat},
			{Name: "sin", CFn: "sin", Params: []Param{{Name: "x", Type: TypFloat}}, Ret: TypFloat},
			{Name: "cos", CFn: "cos", Params: []Param{{Name: "x", Type: TypFloat}}, Ret: TypFloat},
			{Name: "tan", CFn: "tan", Params: []Param{{Name: "x", Type: TypFloat}}, Ret: TypFloat},
			{Name: "asin", CFn: "asin", Params: []Param{{Name: "x", Type: TypFloat}}, Ret: TypFloat},
			{Name: "acos", CFn: "acos", Params: []Param{{Name: "x", Type: TypFloat}}, Ret: TypFloat},
			{Name: "atan", CFn: "atan", Params: []Param{{Name: "x", Type: TypFloat}}, Ret: TypFloat},
			{Name: "atan2", CFn: "atan2", Params: []Param{{Name: "y", Type: TypFloat}, {Name: "x", Type: TypFloat}}, Ret: TypFloat},
			{Name: "sinh", CFn: "sinh", Params: []Param{{Name: "x", Type: TypFloat}}, Ret: TypFloat},
			{Name: "cosh", CFn: "cosh", Params: []Param{{Name: "x", Type: TypFloat}}, Ret: TypFloat},
			{Name: "tanh", CFn: "tanh", Params: []Param{{Name: "x", Type: TypFloat}}, Ret: TypFloat},
			{Name: "exp", CFn: "exp", Params: []Param{{Name: "x", Type: TypFloat}}, Ret: TypFloat},
			{Name: "exp2", CFn: "exp2", Params: []Param{{Name: "x", Type: TypFloat}}, Ret: TypFloat},
			{Name: "log", CFn: "log", Params: []Param{{Name: "x", Type: TypFloat}}, Ret: TypFloat},
			{Name: "log2", CFn: "log2", Params: []Param{{Name: "x", Type: TypFloat}}, Ret: TypFloat},
			{Name: "log10", CFn: "log10", Params: []Param{{Name: "x", Type: TypFloat}}, Ret: TypFloat},
			{Name: "hypot", CFn: "hypot", Params: []Param{{Name: "x", Type: TypFloat}, {Name: "y", Type: TypFloat}}, Ret: TypFloat},
			{Name: "clamp", CFn: "__zx_math_clamp", Params: []Param{{Name: "v", Type: TypFloat}, {Name: "lo", Type: TypFloat}, {Name: "hi", Type: TypFloat}}, Ret: TypFloat},
			{Name: "lerp", CFn: "__zx_math_lerp", Params: []Param{{Name: "a", Type: TypFloat}, {Name: "b", Type: TypFloat}, {Name: "t", Type: TypFloat}}, Ret: TypFloat},
			{Name: "gcd", CFn: "__zx_math_gcd", Params: []Param{{Name: "a", Type: TypInt}, {Name: "b", Type: TypInt}}, Ret: TypInt},
			{Name: "lcm", CFn: "__zx_math_lcm", Params: []Param{{Name: "a", Type: TypInt}, {Name: "b", Type: TypInt}}, Ret: TypInt},
			{Name: "is_nan", CFn: "__zx_math_is_nan", Params: []Param{{Name: "x", Type: TypFloat}}, Ret: TypBool},
			{Name: "is_inf", CFn: "__zx_math_is_inf", Params: []Param{{Name: "x", Type: TypFloat}}, Ret: TypBool},
			{Name: "deg2rad", CFn: "__zx_math_deg2rad", Params: []Param{{Name: "deg", Type: TypFloat}}, Ret: TypFloat},
			{Name: "rad2deg", CFn: "__zx_math_rad2deg", Params: []Param{{Name: "rad", Type: TypFloat}}, Ret: TypFloat},
			{Name: "sign", CFn: "__zx_math_sign", Params: []Param{{Name: "n", Type: TypInt}}, Ret: TypInt},
			{Name: "signf", CFn: "__zx_math_signf", Params: []Param{{Name: "n", Type: TypFloat}}, Ret: TypFloat},
			{Name: "rand", CFn: "rand", Params: []Param{}, Ret: TypInt},
			{Name: "srand", CFn: "srand", Params: []Param{{Name: "seed", Type: TypInt}}, Ret: TypVoid},
		},
	},

	// ── std::sys ──────────────────────────────────────────────────────────────
	"std::sys": {
		Headers: []string{"stdlib.h", "unistd.h", "stdio.h", "errno.h", "string.h"},
		Helpers: []string{
			`/* std::sys helpers */`,
			`static int __zx_system_ok(const char* cmd) { return system(cmd) == 0; }`,
			`static const char* __zx_strerror_wrap(int e) { return strerror(e); }`,
		},
		Fns: []StdFn{
			{Name: "run", CFn: "system", Params: []Param{{Name: "cmd", Type: TypStr}}, Ret: TypInt},
			{Name: "run_ok", CFn: "__zx_system_ok", Params: []Param{{Name: "cmd", Type: TypStr}}, Ret: TypBool},
			{Name: "getenv", CFn: "getenv", Params: []Param{{Name: "key", Type: TypStr}}, Ret: TypStr},
			{Name: "setenv", CFn: "setenv", Params: []Param{{Name: "key", Type: TypStr}, {Name: "val", Type: TypStr}, {Name: "overwrite", Type: TypInt}}, Ret: TypInt},
			{Name: "unsetenv", CFn: "unsetenv", Params: []Param{{Name: "key", Type: TypStr}}, Ret: TypInt},
			{Name: "sleep", CFn: "sleep", Params: []Param{{Name: "secs", Type: TypInt}}, Ret: TypInt},
			{Name: "usleep", CFn: "usleep", Params: []Param{{Name: "us", Type: TypInt}}, Ret: TypInt},
			{Name: "getpid", CFn: "getpid", Params: []Param{}, Ret: TypInt},
			{Name: "getppid", CFn: "getppid", Params: []Param{}, Ret: TypInt},
			{Name: "exit", CFn: "exit", Params: []Param{{Name: "code", Type: TypInt}}, Ret: TypVoid},
			{Name: "abort", CFn: "abort", Params: []Param{}, Ret: TypVoid},
			{Name: "strerror", CFn: "__zx_strerror_wrap", Params: []Param{{Name: "err", Type: TypInt}}, Ret: TypStr},
		},
	},

	// ── std::fs ───────────────────────────────────────────────────────────────
	"std::fs": {
		Headers: []string{"stdio.h", "stdlib.h", "string.h", "sys/stat.h"},
		Helpers: []string{
			`/* std::fs helpers */`,
			`static char* __zx_read_file(const char* path) {`,
			`    FILE* f = fopen(path, "rb");`,
			`    if (!f) { return NULL; }`,
			`    fseek(f, 0, SEEK_END); long sz = ftell(f); rewind(f);`,
			`    char* buf = (char*)malloc((size_t)sz + 1);`,
			`    if (!buf) { fclose(f); return NULL; }`,
			`    fread(buf, 1, (size_t)sz, f); buf[sz] = '\0'; fclose(f);`,
			`    return buf;`,
			`}`,
			`static int __zx_write_file(const char* path, const char* content) {`,
			`    FILE* f = fopen(path, "w");`,
			`    if (!f) { return -1; }`,
			`    fputs(content, f); fclose(f); return 0;`,
			`}`,
			`static int __zx_append_file(const char* path, const char* content) {`,
			`    FILE* f = fopen(path, "a");`,
			`    if (!f) { return -1; }`,
			`    fputs(content, f); fclose(f); return 0;`,
			`}`,
			`static int __zx_file_exists(const char* path) {`,
			`    FILE* f = fopen(path, "r");`,
			`    if (f) { fclose(f); return 1; } return 0;`,
			`}`,
			`static long long __zx_file_size(const char* path) {`,
			`    struct stat st;`,
			`    if (stat(path, &st) != 0) { return -1LL; }`,
			`    return (long long)st.st_size;`,
			`}`,
			`static int __zx_is_dir(const char* path) {`,
			`    struct stat st;`,
			`    if (stat(path, &st) != 0) { return 0; }`,
			`    return S_ISDIR(st.st_mode) ? 1 : 0;`,
			`}`,
			`static int __zx_is_file(const char* path) {`,
			`    struct stat st;`,
			`    if (stat(path, &st) != 0) { return 0; }`,
			`    return S_ISREG(st.st_mode) ? 1 : 0;`,
			`}`,
		},
		Fns: []StdFn{
			{Name: "read", CFn: "__zx_read_file", Params: []Param{{Name: "path", Type: TypStr}}, Ret: TypStr},
			{Name: "write", CFn: "__zx_write_file", Params: []Param{{Name: "path", Type: TypStr}, {Name: "content", Type: TypStr}}, Ret: TypInt},
			{Name: "append", CFn: "__zx_append_file", Params: []Param{{Name: "path", Type: TypStr}, {Name: "content", Type: TypStr}}, Ret: TypInt},
			{Name: "exists", CFn: "__zx_file_exists", Params: []Param{{Name: "path", Type: TypStr}}, Ret: TypBool},
			{Name: "size", CFn: "__zx_file_size", Params: []Param{{Name: "path", Type: TypStr}}, Ret: TypInt},
			{Name: "is_dir", CFn: "__zx_is_dir", Params: []Param{{Name: "path", Type: TypStr}}, Ret: TypBool},
			{Name: "is_file", CFn: "__zx_is_file", Params: []Param{{Name: "path", Type: TypStr}}, Ret: TypBool},
			{Name: "open", CFn: "fopen", Params: []Param{{Name: "path", Type: TypStr}, {Name: "mode", Type: TypStr}}, Ret: PtrOf(TypVoid)},
			{Name: "close", CFn: "fclose", Params: []Param{{Name: "f", Type: PtrOf(TypVoid)}}, Ret: TypInt},
			{Name: "remove", CFn: "remove", Params: []Param{{Name: "path", Type: TypStr}}, Ret: TypInt},
			{Name: "rename", CFn: "rename", Params: []Param{{Name: "old_path", Type: TypStr}, {Name: "new_path", Type: TypStr}}, Ret: TypInt},
			{Name: "mkdir", CFn: "mkdir", Params: []Param{{Name: "path", Type: TypStr}, {Name: "mode", Type: TypInt}}, Ret: TypInt},
		},
	},

	// ── std::cmd ──────────────────────────────────────────────────────────────
	"std::cmd": {
		Headers: []string{"stdio.h", "stdlib.h", "string.h", "sys/wait.h"},
		Helpers: []string{
			`/* std::cmd helpers */`,
			`static char __zx_cmd_out[65536];`,
			`static const char* __zx_capture(const char* cmd) {`,
			`    FILE* p = popen(cmd, "r");`,
			`    if (!p) { __zx_cmd_out[0] = '\0'; return __zx_cmd_out; }`,
			`    size_t total = 0; char tmp[1024];`,
			`    while (fgets(tmp, sizeof(tmp), p) && total + strlen(tmp) < sizeof(__zx_cmd_out) - 1) {`,
			`        memcpy(__zx_cmd_out + total, tmp, strlen(tmp)); total += strlen(tmp);`,
			`    }`,
			`    __zx_cmd_out[total] = '\0'; pclose(p); return __zx_cmd_out;`,
			`}`,
			`static int __zx_run_exit(const char* cmd) { int r = system(cmd); return WEXITSTATUS(r); }`,
			`static int __zx_cmd_ok(const char* cmd) { return system(cmd) == 0; }`,
		},
		Fns: []StdFn{
			{Name: "capture", CFn: "__zx_capture", Params: []Param{{Name: "cmd", Type: TypStr}}, Ret: TypStr},
			{Name: "run", CFn: "system", Params: []Param{{Name: "cmd", Type: TypStr}}, Ret: TypInt},
			{Name: "exitcode", CFn: "__zx_run_exit", Params: []Param{{Name: "cmd", Type: TypStr}}, Ret: TypInt},
			{Name: "ok", CFn: "__zx_cmd_ok", Params: []Param{{Name: "cmd", Type: TypStr}}, Ret: TypBool},
			{Name: "popen", CFn: "popen", Params: []Param{{Name: "cmd", Type: TypStr}, {Name: "mode", Type: TypStr}}, Ret: PtrOf(TypVoid)},
			{Name: "pclose", CFn: "pclose", Params: []Param{{Name: "p", Type: PtrOf(TypVoid)}}, Ret: TypInt},
		},
	},

	// ── std::mem ──────────────────────────────────────────────────────────────
	"std::mem": {
		Headers: []string{"stdlib.h", "string.h"},
		Helpers: []string{
			`/* std::mem helpers */`,
			`static void* __zx_mem_dup(const void* src, size_t n) {`,
			`    void* d = malloc(n); if (d) { memcpy(d, src, n); } return d;`,
			`}`,
		},
		Fns: []StdFn{
			{Name: "alloc", CFn: "malloc", Params: []Param{{Name: "size", Type: TypInt}}, Ret: PtrOf(TypVoid)},
			{Name: "zalloc", CFn: "calloc", Params: []Param{{Name: "count", Type: TypInt}, {Name: "size", Type: TypInt}}, Ret: PtrOf(TypVoid)},
			{Name: "realloc", CFn: "realloc", Params: []Param{{Name: "ptr", Type: PtrOf(TypVoid)}, {Name: "size", Type: TypInt}}, Ret: PtrOf(TypVoid)},
			{Name: "free", CFn: "free", Params: []Param{{Name: "ptr", Type: PtrOf(TypVoid)}}, Ret: TypVoid},
			{Name: "copy", CFn: "memcpy", Params: []Param{{Name: "dst", Type: PtrOf(TypVoid)}, {Name: "src", Type: PtrOf(TypVoid)}, {Name: "n", Type: TypInt}}, Ret: PtrOf(TypVoid)},
			{Name: "move", CFn: "memmove", Params: []Param{{Name: "dst", Type: PtrOf(TypVoid)}, {Name: "src", Type: PtrOf(TypVoid)}, {Name: "n", Type: TypInt}}, Ret: PtrOf(TypVoid)},
			{Name: "set", CFn: "memset", Params: []Param{{Name: "ptr", Type: PtrOf(TypVoid)}, {Name: "val", Type: TypInt}, {Name: "n", Type: TypInt}}, Ret: PtrOf(TypVoid)},
			{Name: "cmp", CFn: "memcmp", Params: []Param{{Name: "a", Type: PtrOf(TypVoid)}, {Name: "b", Type: PtrOf(TypVoid)}, {Name: "n", Type: TypInt}}, Ret: TypInt},
			{Name: "dup", CFn: "__zx_mem_dup", Params: []Param{{Name: "src", Type: PtrOf(TypVoid)}, {Name: "n", Type: TypInt}}, Ret: PtrOf(TypVoid)},
		},
	},

	// ── std::conv ─────────────────────────────────────────────────────────────
	"std::conv": {
		Headers: []string{"stdlib.h", "stdio.h", "string.h"},
		Helpers: []string{
			`/* std::conv helpers */`,
			`static char __zx_conv_buf[256];`,
			`static const char* __zx_int_to_str(long long n) { snprintf(__zx_conv_buf, sizeof(__zx_conv_buf), "%lld", n); return __zx_conv_buf; }`,
			`static const char* __zx_float_to_str(double f)  { snprintf(__zx_conv_buf, sizeof(__zx_conv_buf), "%g", f);   return __zx_conv_buf; }`,
			`static const char* __zx_bool_to_str(int b)       { return b ? "true" : "false"; }`,
			`static const char* __zx_int_to_hex(long long n)  { snprintf(__zx_conv_buf, sizeof(__zx_conv_buf), "%llx", (unsigned long long)n); return __zx_conv_buf; }`,
			`static const char* __zx_int_to_bin(long long n) {`,
			`    int bit = 63, i = 0;`,
			`    unsigned long long u = (unsigned long long)n;`,
			`    while (bit > 0 && !((u >> (unsigned)bit) & 1ULL)) { bit--; }`,
			`    for (; bit >= 0; bit--, i++) { __zx_conv_buf[i] = (char)('0' + (int)((u >> (unsigned)bit) & 1ULL)); }`,
			`    __zx_conv_buf[i] = '\0'; return __zx_conv_buf;`,
			`}`,
			`static long long __zx_hex_to_int(const char* s) { return (long long)strtoll(s, NULL, 16); }`,
			`static long long __zx_bin_to_int(const char* s) { return (long long)strtoll(s, NULL,  2); }`,
			`static long long __zx_oct_to_int(const char* s) { return (long long)strtoll(s, NULL,  8); }`,
		},
		Fns: []StdFn{
			{Name: "to_int", CFn: "atoi", Params: []Param{{Name: "s", Type: TypStr}}, Ret: TypInt},
			{Name: "to_float", CFn: "atof", Params: []Param{{Name: "s", Type: TypStr}}, Ret: TypFloat},
			{Name: "to_long", CFn: "atol", Params: []Param{{Name: "s", Type: TypStr}}, Ret: TypInt},
			{Name: "int_to_str", CFn: "__zx_int_to_str", Params: []Param{{Name: "n", Type: TypInt}}, Ret: TypStr},
			{Name: "float_to_str", CFn: "__zx_float_to_str", Params: []Param{{Name: "f", Type: TypFloat}}, Ret: TypStr},
			{Name: "bool_to_str", CFn: "__zx_bool_to_str", Params: []Param{{Name: "b", Type: TypBool}}, Ret: TypStr},
			{Name: "int_to_hex", CFn: "__zx_int_to_hex", Params: []Param{{Name: "n", Type: TypInt}}, Ret: TypStr},
			{Name: "int_to_bin", CFn: "__zx_int_to_bin", Params: []Param{{Name: "n", Type: TypInt}}, Ret: TypStr},
			{Name: "hex_to_int", CFn: "__zx_hex_to_int", Params: []Param{{Name: "s", Type: TypStr}}, Ret: TypInt},
			{Name: "bin_to_int", CFn: "__zx_bin_to_int", Params: []Param{{Name: "s", Type: TypStr}}, Ret: TypInt},
			{Name: "oct_to_int", CFn: "__zx_oct_to_int", Params: []Param{{Name: "s", Type: TypStr}}, Ret: TypInt},
		},
	},

	// ── std::time ─────────────────────────────────────────────────────────────
	"std::time": {
		Headers: []string{"time.h", "sys/time.h"},
		Helpers: []string{
			`/* std::time helpers */`,
			`static char __zx_time_buf[64];`,
			`static const char* __zx_time_format(long long t, const char* fmt) {`,
			`    time_t tt = (time_t)t;`,
			`    struct tm* tm_ptr = localtime(&tt);`,
			`    strftime(__zx_time_buf, sizeof(__zx_time_buf), fmt, tm_ptr);`,
			`    return __zx_time_buf;`,
			`}`,
			`static long long __zx_time_now_ms(void) {`,
			`    struct timeval tv; gettimeofday(&tv, NULL);`,
			`    return (long long)tv.tv_sec * 1000LL + (long long)tv.tv_usec / 1000LL;`,
			`}`,
			`static long long __zx_time_now_us(void) {`,
			`    struct timeval tv; gettimeofday(&tv, NULL);`,
			`    return (long long)tv.tv_sec * 1000000LL + (long long)tv.tv_usec;`,
			`}`,
		},
		Fns: []StdFn{
			{Name: "now", CFn: "time", Params: []Param{{Name: "_", Type: PtrOf(TypVoid)}}, Ret: TypInt},
			{Name: "now_ms", CFn: "__zx_time_now_ms", Params: []Param{}, Ret: TypInt},
			{Name: "now_us", CFn: "__zx_time_now_us", Params: []Param{}, Ret: TypInt},
			{Name: "clock", CFn: "clock", Params: []Param{}, Ret: TypInt},
			{Name: "diff", CFn: "difftime", Params: []Param{{Name: "t2", Type: TypInt}, {Name: "t1", Type: TypInt}}, Ret: TypFloat},
			{Name: "format", CFn: "__zx_time_format", Params: []Param{{Name: "t", Type: TypInt}, {Name: "fmt", Type: TypStr}}, Ret: TypStr},
		},
	},

	// ── std::os ───────────────────────────────────────────────────────────────
	"std::os": {
		Headers: []string{"stdlib.h", "stdio.h", "unistd.h"},
		Helpers: []string{
			`/* std::os helpers */`,
			`static char __zx_os_cwd_buf[4096];`,
			`static const char* __zx_os_cwd(void) {`,
			`    const char* r = getcwd(__zx_os_cwd_buf, sizeof(__zx_os_cwd_buf));`,
			`    return r ? r : "";`,
			`}`,
		},
		Fns: []StdFn{
			{Name: "getenv", CFn: "getenv", Params: []Param{{Name: "key", Type: TypStr}}, Ret: TypStr},
			{Name: "setenv", CFn: "setenv", Params: []Param{{Name: "key", Type: TypStr}, {Name: "val", Type: TypStr}, {Name: "overwrite", Type: TypInt}}, Ret: TypInt},
			{Name: "exit", CFn: "exit", Params: []Param{{Name: "code", Type: TypInt}}, Ret: TypVoid},
			{Name: "abort", CFn: "abort", Params: []Param{}, Ret: TypVoid},
			{Name: "getcwd", CFn: "__zx_os_cwd", Params: []Param{}, Ret: TypStr},
			{Name: "chdir", CFn: "chdir", Params: []Param{{Name: "path", Type: TypStr}}, Ret: TypInt},
			{Name: "getpid", CFn: "getpid", Params: []Param{}, Ret: TypInt},
		},
	},

	// ── std::fmt ──────────────────────────────────────────────────────────────
	"std::fmt": {
		Headers: []string{"stdio.h"},
		Fns: []StdFn{
			{Name: "print", CFn: "printf", Variadic: true, Params: []Param{{Name: "fmt", Type: TypStr}}, Ret: TypInt},
			{Name: "eprint", CFn: "fprintf", Variadic: true, Params: []Param{{Name: "f", Type: PtrOf(TypVoid)}, {Name: "fmt", Type: TypStr}}, Ret: TypInt},
			{Name: "sprintf", CFn: "sprintf", Variadic: true, Params: []Param{{Name: "buf", Type: TypStr}, {Name: "fmt", Type: TypStr}}, Ret: TypInt},
			{Name: "snprintf", CFn: "snprintf", Variadic: true, Params: []Param{{Name: "buf", Type: TypStr}, {Name: "n", Type: TypInt}, {Name: "fmt", Type: TypStr}}, Ret: TypInt},
		},
	},

	// ── std::net ──────────────────────────────────────────────────────────────
	"std::net": {
		Headers: []string{"stdio.h", "stdlib.h", "string.h", "sys/socket.h", "netinet/in.h", "arpa/inet.h", "unistd.h", "netdb.h"},
		Helpers: []string{
			`/* std::net helpers */`,
			`static int __zx_tcp_server(int port) {`,
			`    int fd = socket(AF_INET, SOCK_STREAM, 0); if (fd < 0) { return -1; }`,
			`    int opt = 1; setsockopt(fd, SOL_SOCKET, SO_REUSEADDR, &opt, sizeof(opt));`,
			`    struct sockaddr_in addr; memset(&addr, 0, sizeof(addr));`,
			`    addr.sin_family = AF_INET; addr.sin_addr.s_addr = INADDR_ANY;`,
			`    addr.sin_port = htons((uint16_t)port);`,
			`    if (bind(fd, (struct sockaddr*)&addr, sizeof(addr)) < 0) { close(fd); return -1; }`,
			`    listen(fd, 10); return fd;`,
			`}`,
			`static int __zx_tcp_connect(const char* host, int port) {`,
			`    struct hostent* he = gethostbyname(host); if (!he) { return -1; }`,
			`    int fd = socket(AF_INET, SOCK_STREAM, 0); if (fd < 0) { return -1; }`,
			`    struct sockaddr_in addr; memset(&addr, 0, sizeof(addr));`,
			`    addr.sin_family = AF_INET; addr.sin_port = htons((uint16_t)port);`,
			`    memcpy(&addr.sin_addr, he->h_addr_list[0], (size_t)he->h_length);`,
			`    if (connect(fd, (struct sockaddr*)&addr, sizeof(addr)) < 0) { close(fd); return -1; }`,
			`    return fd;`,
			`}`,
			`static int __zx_tcp_accept(int fd) { return accept(fd, NULL, NULL); }`,
			`static int __zx_tcp_send(int fd, const char* msg) { return (int)send(fd, msg, strlen(msg), 0); }`,
			`static char __zx_net_recv_buf[65536];`,
			`static const char* __zx_tcp_recv(int fd, int maxbytes) {`,
			`    if (maxbytes <= 0 || maxbytes > (int)(sizeof(__zx_net_recv_buf) - 1))`,
			`        maxbytes = (int)(sizeof(__zx_net_recv_buf) - 1);`,
			`    ssize_t n = recv(fd, __zx_net_recv_buf, (size_t)maxbytes, 0);`,
			`    if (n < 0) { __zx_net_recv_buf[0] = '\0'; return __zx_net_recv_buf; }`,
			`    __zx_net_recv_buf[n] = '\0'; return __zx_net_recv_buf;`,
			`}`,
		},
		Fns: []StdFn{
			{Name: "tcp_server", CFn: "__zx_tcp_server", Params: []Param{{Name: "port", Type: TypInt}}, Ret: TypInt},
			{Name: "tcp_connect", CFn: "__zx_tcp_connect", Params: []Param{{Name: "host", Type: TypStr}, {Name: "port", Type: TypInt}}, Ret: TypInt},
			{Name: "tcp_accept", CFn: "__zx_tcp_accept", Params: []Param{{Name: "fd", Type: TypInt}}, Ret: TypInt},
			{Name: "tcp_send", CFn: "__zx_tcp_send", Params: []Param{{Name: "fd", Type: TypInt}, {Name: "msg", Type: TypStr}}, Ret: TypInt},
			{Name: "tcp_recv", CFn: "__zx_tcp_recv", Params: []Param{{Name: "fd", Type: TypInt}, {Name: "maxbytes", Type: TypInt}}, Ret: TypStr},
			{Name: "close_fd", CFn: "close", Params: []Param{{Name: "fd", Type: TypInt}}, Ret: TypInt},
			{Name: "htons", CFn: "htons", Params: []Param{{Name: "port", Type: TypInt}}, Ret: TypInt},
			{Name: "htonl", CFn: "htonl", Params: []Param{{Name: "n", Type: TypInt}}, Ret: TypInt},
		},
	},

	// ── std::rand ─────────────────────────────────────────────────────────────
	"std::rand": {
		Headers: []string{"stdlib.h", "time.h"},
		Helpers: []string{
			`/* std::rand helpers */`,
			`static long long __zx_rand_range(long long lo, long long hi) {`,
			`    if (hi <= lo) { return lo; }`,
			`    return lo + (long long)((unsigned long long)rand() % (unsigned long long)(hi - lo));`,
			`}`,
			`static double __zx_rand_float(void) { return (double)rand() / ((double)RAND_MAX + 1.0); }`,
			`static void __zx_rand_seed_time(void) { srand((unsigned int)time(NULL)); }`,
		},
		Fns: []StdFn{
			{Name: "rand", CFn: "rand", Params: []Param{}, Ret: TypInt},
			{Name: "rand_range", CFn: "__zx_rand_range", Params: []Param{{Name: "lo", Type: TypInt}, {Name: "hi", Type: TypInt}}, Ret: TypInt},
			{Name: "rand_float", CFn: "__zx_rand_float", Params: []Param{}, Ret: TypFloat},
			{Name: "seed", CFn: "srand", Params: []Param{{Name: "s", Type: TypInt}}, Ret: TypVoid},
			{Name: "seed_time", CFn: "__zx_rand_seed_time", Params: []Param{}, Ret: TypVoid},
		},
	},

	// ── std::sort ─────────────────────────────────────────────────────────────
	"std::sort": {
		Headers: []string{"stdlib.h", "string.h"},
		Helpers: []string{
			`/* std::sort helpers */`,
			`static int __zx_cmp_int_asc(const void* a, const void* b)   { long long x=*(const long long*)a, y=*(const long long*)b; return (x>y)-(x<y); }`,
			`static int __zx_cmp_int_desc(const void* a, const void* b)  { long long x=*(const long long*)a, y=*(const long long*)b; return (x<y)-(x>y); }`,
			`static int __zx_cmp_float_asc(const void* a, const void* b) { double x=*(const double*)a, y=*(const double*)b; return (x>y)-(x<y); }`,
			`static void __zx_sort_ints(long long* arr, int n)       { qsort(arr, (size_t)n, sizeof(long long), __zx_cmp_int_asc); }`,
			`static void __zx_sort_ints_desc(long long* arr, int n)  { qsort(arr, (size_t)n, sizeof(long long), __zx_cmp_int_desc); }`,
			`static void __zx_sort_floats(double* arr, int n)        { qsort(arr, (size_t)n, sizeof(double),    __zx_cmp_float_asc); }`,
		},
		Fns: []StdFn{
			{Name: "sort_ints", CFn: "__zx_sort_ints", Params: []Param{{Name: "arr", Type: PtrOf(TypInt)}, {Name: "n", Type: TypInt}}, Ret: TypVoid},
			{Name: "sort_ints_desc", CFn: "__zx_sort_ints_desc", Params: []Param{{Name: "arr", Type: PtrOf(TypInt)}, {Name: "n", Type: TypInt}}, Ret: TypVoid},
			{Name: "sort_floats", CFn: "__zx_sort_floats", Params: []Param{{Name: "arr", Type: PtrOf(TypFloat)}, {Name: "n", Type: TypInt}}, Ret: TypVoid},
			{Name: "qsort", CFn: "qsort", Params: []Param{{Name: "arr", Type: PtrOf(TypVoid)}, {Name: "n", Type: TypInt}, {Name: "size", Type: TypInt}, {Name: "cmp", Type: PtrOf(TypVoid)}}, Ret: TypVoid},
			{Name: "bsearch", CFn: "bsearch", Params: []Param{{Name: "key", Type: PtrOf(TypVoid)}, {Name: "arr", Type: PtrOf(TypVoid)}, {Name: "n", Type: TypInt}, {Name: "size", Type: TypInt}, {Name: "cmp", Type: PtrOf(TypVoid)}}, Ret: PtrOf(TypVoid)},
		},
	},

	// ── std::bits ─────────────────────────────────────────────────────────────
	"std::bits": {
		Headers: []string{"stdint.h"},
		Helpers: []string{
			`/* std::bits helpers */`,
			`static int __zx_popcount(long long n) { int c=0; unsigned long long u=(unsigned long long)n; for(;u;u&=u-1){c++;} return c; }`,
			`static int __zx_clz(unsigned long long n) { int c=0; if(!n){return 64;} while(!(n&(1ULL<<63))){c++;n<<=1;} return c; }`,
			`static int __zx_ctz(unsigned long long n) { int c=0; if(!n){return 64;} while(!(n&1ULL)){c++;n>>=1;} return c; }`,
			`static long long __zx_rotl(long long n,int k) { unsigned long long u=(unsigned long long)n; return (long long)((u<<(unsigned)k)|(u>>(unsigned)(64-k))); }`,
			`static long long __zx_rotr(long long n,int k) { unsigned long long u=(unsigned long long)n; return (long long)((u>>(unsigned)k)|(u<<(unsigned)(64-k))); }`,
			`static int       __zx_bit_get(long long n,int p)    { return (int)((n>>p)&1LL); }`,
			`static long long __zx_bit_set(long long n,int p)    { return n|(1LL<<p); }`,
			`static long long __zx_bit_clear(long long n,int p)  { return n&~(1LL<<p); }`,
			`static long long __zx_bit_toggle(long long n,int p) { return n^(1LL<<p); }`,
		},
		Fns: []StdFn{
			{Name: "popcount", CFn: "__zx_popcount", Params: []Param{{Name: "n", Type: TypInt}}, Ret: TypInt},
			{Name: "clz", CFn: "__zx_clz", Params: []Param{{Name: "n", Type: TypInt}}, Ret: TypInt},
			{Name: "ctz", CFn: "__zx_ctz", Params: []Param{{Name: "n", Type: TypInt}}, Ret: TypInt},
			{Name: "rotl", CFn: "__zx_rotl", Params: []Param{{Name: "n", Type: TypInt}, {Name: "k", Type: TypInt}}, Ret: TypInt},
			{Name: "rotr", CFn: "__zx_rotr", Params: []Param{{Name: "n", Type: TypInt}, {Name: "k", Type: TypInt}}, Ret: TypInt},
			{Name: "bit_get", CFn: "__zx_bit_get", Params: []Param{{Name: "n", Type: TypInt}, {Name: "pos", Type: TypInt}}, Ret: TypInt},
			{Name: "bit_set", CFn: "__zx_bit_set", Params: []Param{{Name: "n", Type: TypInt}, {Name: "pos", Type: TypInt}}, Ret: TypInt},
			{Name: "bit_clear", CFn: "__zx_bit_clear", Params: []Param{{Name: "n", Type: TypInt}, {Name: "pos", Type: TypInt}}, Ret: TypInt},
			{Name: "bit_toggle", CFn: "__zx_bit_toggle", Params: []Param{{Name: "n", Type: TypInt}, {Name: "pos", Type: TypInt}}, Ret: TypInt},
		},
	},

	// ── std::hash ─────────────────────────────────────────────────────────────
	"std::hash": {
		Headers: []string{"stdint.h", "string.h"},
		Helpers: []string{
			`/* std::hash helpers */`,
			`static unsigned long long __zx_hash_fnv1a(const char* s) {`,
			`    unsigned long long h = 14695981039346656037ULL;`,
			`    for (; *s; s++) { h = (h ^ (unsigned char)*s) * 1099511628211ULL; } return h;`,
			`}`,
			`static unsigned long long __zx_hash_djb2(const char* s) {`,
			`    unsigned long long h = 5381; int c;`,
			`    while ((c = (unsigned char)*s++)) { h = ((h << 5) + h) + (unsigned long long)c; } return h;`,
			`}`,
			`static unsigned long long __zx_hash_bytes(const void* data, size_t len) {`,
			`    const unsigned char* p = (const unsigned char*)data;`,
			`    unsigned long long h = 14695981039346656037ULL; size_t i;`,
			`    for (i = 0; i < len; i++) { h = (h ^ p[i]) * 1099511628211ULL; } return h;`,
			`}`,
			`static unsigned long long __zx_hash_int(long long n) {`,
			`    unsigned long long h = (unsigned long long)n;`,
			`    h ^= h >> 33; h *= 0xff51afd7ed558ccdULL;`,
			`    h ^= h >> 33; h *= 0xc4ceb9fe1a85ec53ULL;`,
			`    h ^= h >> 33; return h;`,
			`}`,
		},
		Fns: []StdFn{
			{Name: "fnv1a", CFn: "__zx_hash_fnv1a", Params: []Param{{Name: "s", Type: TypStr}}, Ret: TypInt},
			{Name: "djb2", CFn: "__zx_hash_djb2", Params: []Param{{Name: "s", Type: TypStr}}, Ret: TypInt},
			{Name: "hash_bytes", CFn: "__zx_hash_bytes", Params: []Param{{Name: "data", Type: PtrOf(TypVoid)}, {Name: "len", Type: TypInt}}, Ret: TypInt},
			{Name: "hash_int", CFn: "__zx_hash_int", Params: []Param{{Name: "n", Type: TypInt}}, Ret: TypInt},
		},
	},

	// ── std::assert ───────────────────────────────────────────────────────────
	"std::assert": {
		Headers: []string{"stdio.h", "stdlib.h"},
		Helpers: []string{
			`/* std::assert helpers */`,
			`static void __zx_assert_fail(const char* expr, const char* file, int line, const char* msg) {`,
			`    fprintf(stderr, "%s:%d: assertion failed: %s", file, line, expr);`,
			`    if (msg && msg[0]) { fprintf(stderr, " -- %s", msg); }`,
			`    fprintf(stderr, "\n"); abort();`,
			`}`,
			`static void __zx_expect_eq_int(long long a, long long b, const char* msg) {`,
			`    if (a != b) { fprintf(stderr, "expect_eq failed: %lld != %lld", a, b);`,
			`        if (msg && msg[0]) { fprintf(stderr, " -- %s", msg); } fprintf(stderr, "\n"); abort(); }`,
			`}`,
			`static void __zx_expect_eq_str(const char* a, const char* b, const char* msg) {`,
			`    if (strcmp(a, b) != 0) { fprintf(stderr, "expect_eq failed: \"%s\" != \"%s\"", a, b);`,
			`        if (msg && msg[0]) { fprintf(stderr, " -- %s", msg); } fprintf(stderr, "\n"); abort(); }`,
			`}`,
			`static void __zx_expect_true(int cond, const char* msg) {`,
			`    if (!cond) { fprintf(stderr, "expect_true failed");`,
			`        if (msg && msg[0]) { fprintf(stderr, ": %s", msg); } fprintf(stderr, "\n"); abort(); }`,
			`}`,
		},
		Fns: []StdFn{
			{Name: "expect_eq_int", CFn: "__zx_expect_eq_int", Params: []Param{{Name: "a", Type: TypInt}, {Name: "b", Type: TypInt}, {Name: "msg", Type: TypStr}}, Ret: TypVoid},
			{Name: "expect_eq_str", CFn: "__zx_expect_eq_str", Params: []Param{{Name: "a", Type: TypStr}, {Name: "b", Type: TypStr}, {Name: "msg", Type: TypStr}}, Ret: TypVoid},
			{Name: "expect_true", CFn: "__zx_expect_true", Params: []Param{{Name: "cond", Type: TypBool}, {Name: "msg", Type: TypStr}}, Ret: TypVoid},
		},
	},

	// ── std::debug ────────────────────────────────────────────────────────────
	"std::debug": {
		Headers: []string{"stdio.h", "stdlib.h"},
		Helpers: []string{
			`/* std::debug helpers */`,
			`static void __zx_print_int(long long v, const char* name)   { fprintf(stderr, "[dbg] %s = %lld\n",  name, v); }`,
			`static void __zx_print_float(double v, const char* name)    { fprintf(stderr, "[dbg] %s = %g\n",    name, v); }`,
			`static void __zx_print_str(const char* v, const char* name) { fprintf(stderr, "[dbg] %s = \"%s\"\n",name, v); }`,
			`static void __zx_print_bool(int v, const char* name)        { fprintf(stderr, "[dbg] %s = %s\n",    name, v ? "true" : "false"); }`,
			`static void __zx_print_ptr(const void* v, const char* name) { fprintf(stderr, "[dbg] %s = %p\n",    name, v); }`,
		},
		Fns: []StdFn{
			{Name: "print_int", CFn: "__zx_print_int", Params: []Param{{Name: "v", Type: TypInt}, {Name: "name", Type: TypStr}}, Ret: TypVoid},
			{Name: "print_float", CFn: "__zx_print_float", Params: []Param{{Name: "v", Type: TypFloat}, {Name: "name", Type: TypStr}}, Ret: TypVoid},
			{Name: "print_str", CFn: "__zx_print_str", Params: []Param{{Name: "v", Type: TypStr}, {Name: "name", Type: TypStr}}, Ret: TypVoid},
			{Name: "print_bool", CFn: "__zx_print_bool", Params: []Param{{Name: "v", Type: TypBool}, {Name: "name", Type: TypStr}}, Ret: TypVoid},
			{Name: "print_ptr", CFn: "__zx_print_ptr", Params: []Param{{Name: "v", Type: PtrOf(TypVoid)}, {Name: "name", Type: TypStr}}, Ret: TypVoid},
		},
	},
}

// ── Always-available builtin functions ───────────────────────────────────────

type BuiltinDef struct {
	Ret     *ZXType
	Arity   int // -1 = variadic
	Emit    string
	DupArgs bool
	VarArgs bool
}

var builtinFns = map[string]*BuiltinDef{
	"is_nil":   {Ret: TypBool, Arity: 1, Emit: "((void*)(%s) == NULL)"},
	"not_nil":  {Ret: TypBool, Arity: 1, Emit: "((void*)(%s) != NULL)"},
	"is_zero":  {Ret: TypBool, Arity: 1, Emit: "((%s) == 0)"},
	"to_int":   {Ret: TypInt, Arity: 1, Emit: "((long long)(%s))"},
	"to_float": {Ret: TypFloat, Arity: 1, Emit: "((double)(%s))"},
	"to_bool":  {Ret: TypBool, Arity: 1, Emit: "(!!((%s)))"},
	"to_char":  {Ret: TypChar, Arity: 1, Emit: "((char)(%s))"},
	"to_str":   {Ret: TypStr, Arity: 1, Emit: "/* use std::conv int_to_str */"},
	"abs":      {Ret: TypAny, Arity: 1, Emit: "(((%s)<0)?-(%s):(%s))", DupArgs: true},
	"min":      {Ret: TypAny, Arity: 2, Emit: "((%s)<(%s)?(%s):(%s))", DupArgs: true},
	"max":      {Ret: TypAny, Arity: 2, Emit: "((%s)>(%s)?(%s):(%s))", DupArgs: true},
	"clamp":    {Ret: TypAny, Arity: 3, Emit: "((%s)<(%s)?(%s):((%s)>(%s)?(%s):(%s)))"},
	"len":      {Ret: TypInt, Arity: 1, Emit: "(long long)strlen((const char*)(%s))"},
	"str_eq":   {Ret: TypBool, Arity: 2, Emit: "(strcmp(%s,%s)==0)"},
	"str_ne":   {Ret: TypBool, Arity: 2, Emit: "(strcmp(%s,%s)!=0)"},
	"alloc":    {Ret: PtrOf(TypVoid), Arity: 1, Emit: "malloc((size_t)(%s))"},
	"zalloc":   {Ret: PtrOf(TypVoid), Arity: 2, Emit: "calloc((size_t)(%s),(size_t)(%s))"},
	"free":     {Ret: TypVoid, Arity: 1, Emit: "free(%s)"},
	"system":   {Ret: TypInt, Arity: 1, Emit: "system(%s)"},
	"getenv":   {Ret: TypStr, Arity: 1, Emit: "getenv(%s)"},
	"sizeof":   {Ret: TypInt, Arity: 1, Emit: "(long long)sizeof(%s)"},
	"input":    {Ret: TypStr, Arity: 0},
	"print":    {Ret: TypVoid, Arity: -1, VarArgs: true},
	"println":  {Ret: TypVoid, Arity: -1, VarArgs: true},
}

// ── Registry helpers ──────────────────────────────────────────────────────────

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
	"fgets": true, "fputs": true, "feof": true, "ferror": true,
	"fflush": true, "fseek": true, "ftell": true, "rewind": true,
	"puts": true, "getchar": true, "putchar": true, "getc": true, "putc": true, "perror": true,
	"malloc": true, "calloc": true, "realloc": true, "free": true,
	"exit": true, "abort": true, "atoi": true, "atof": true, "atol": true,
	"rand": true, "srand": true, "abs": true, "labs": true,
	"strtol": true, "strtod": true, "strtoll": true, "qsort": true, "bsearch": true,
	"strlen": true, "strcpy": true, "strncpy": true, "strcat": true, "strncat": true,
	"strcmp": true, "strncmp": true, "strchr": true, "strrchr": true, "strstr": true, "strdup": true,
	"memcpy": true, "memmove": true, "memset": true, "memcmp": true,
	"sqrt": true, "pow": true, "fabs": true, "floor": true, "ceil": true,
	"sin": true, "cos": true, "tan": true, "asin": true, "acos": true, "atan": true,
	"atan2": true, "exp": true, "exp2": true, "log": true, "log2": true, "log10": true,
	"fmod": true, "round": true, "trunc": true, "fmax": true, "fmin": true, "cbrt": true,
	"sinh": true, "cosh": true, "tanh": true, "hypot": true,
	"isalpha": true, "isdigit": true, "isspace": true, "isupper": true, "islower": true,
	"isalnum": true, "ispunct": true, "isprint": true,
	"toupper": true, "tolower": true,
	"time": true, "clock": true, "difftime": true, "localtime": true, "strftime": true,
	"gettimeofday": true,
	"sleep":        true, "usleep": true, "getpid": true, "getppid": true,
	"system": true, "getenv": true, "setenv": true, "unsetenv": true,
	"popen": true, "pclose": true,
	"remove": true, "rename": true, "mkdir": true, "stat": true,
	"getcwd": true, "chdir": true,
	"socket": true, "bind": true, "listen": true, "accept": true, "connect": true,
	"send": true, "recv": true, "close": true, "htons": true, "htonl": true,
	"setsockopt": true, "getsockopt": true, "gethostbyname": true,
	"strerror": true,
}

var _ = strings.Join
