# ZX Standard Library Reference

Import syntax: `use std::module_name`

```zx
use std::math
use std::str
use std::fs
```

---

## Built-ins (always available, no import needed)

| Function | Params | Returns | Description |
|---|---|---|---|
| `len(s)` | `s: str` | `int` | Length of a string |
| `abs(n)` | `n: any` | `any` | Absolute value |
| `min(a, b)` | `a, b: any` | `any` | Smaller of two values |
| `max(a, b)` | `a, b: any` | `any` | Larger of two values |
| `clamp(v, lo, hi)` | all `any` | `any` | Clamp v between lo and hi |
| `is_nil(p)` | `p: ref` | `bool` | True if pointer is NULL |
| `not_nil(p)` | `p: ref` | `bool` | True if pointer is not NULL |
| `is_zero(n)` | `n: any` | `bool` | True if value == 0 |
| `to_int(x)` | `x: any` | `int` | Cast to int |
| `to_float(x)` | `x: any` | `float` | Cast to float |
| `to_bool(x)` | `x: any` | `bool` | Cast to bool |
| `to_char(x)` | `x: any` | `char` | Cast to char |
| `alloc(n)` | `n: int` | `ref` | malloc(n) |
| `zalloc(n, sz)` | `n, sz: int` | `ref` | calloc(n, sz) |
| `free(p)` | `p: ref` | `void` | free(p) |
| `sizeof(T)` | type | `int` | Size of type in bytes |
| `system(cmd)` | `cmd: str` | `int` | Run shell command |
| `getenv(k)` | `k: str` | `str` | Read environment variable |
| `input()` | — | `str` | Read a line from stdin |
| `str_eq(a, b)` | `a, b: str` | `bool` | True if strings are equal |
| `str_ne(a, b)` | `a, b: str` | `bool` | True if strings differ |

---

## std::str

String manipulation and character classification.

| Function | Params | Returns | Description |
|---|---|---|---|
| `str_len(s)` | `s: str` | `int` | Length of string |
| `str_cmp(a, b)` | `a, b: str` | `int` | Compare strings (0 = equal) |
| `str_ncmp(a, b, n)` | `a, b: str, n: int` | `int` | Compare first n chars |
| `str_cpy(dst, src)` | `dst, src: str` | `str` | Copy src into dst |
| `str_ncpy(dst, src, n)` | `dst, src: str, n: int` | `str` | Copy at most n chars |
| `str_cat(dst, src)` | `dst, src: str` | `str` | Append src to dst |
| `str_dup(s)` | `s: str` | `str` | Heap-duplicate a string |
| `str_find(haystack, needle)` | `str, str` | `str` | Pointer to first match, or NULL |
| `str_chr(s, c)` | `s: str, c: int` | `str` | First occurrence of char c |
| `str_rchr(s, c)` | `s: str, c: int` | `str` | Last occurrence of char c |
| `str_index(s, needle)` | `s, needle: str` | `int` | Index of needle, -1 if not found |
| `str_contains(s, needle)` | `s, needle: str` | `bool` | True if needle is in s |
| `str_starts(s, prefix)` | `s, prefix: str` | `bool` | True if s starts with prefix |
| `str_ends(s, suffix)` | `s, suffix: str` | `bool` | True if s ends with suffix |
| `str_trim(s)` | `s: str` | `str` | Strip leading/trailing whitespace |
| `str_upper(s)` | `s: str` | `str` | Convert to uppercase |
| `str_lower(s)` | `s: str` | `str` | Convert to lowercase |
| `str_reverse(s)` | `s: str` | `str` | Reverse a string |
| `str_repeat(s, n)` | `s: str, n: int` | `str` | Repeat string n times |
| `str_slice(s, from, to)` | `s: str, from, to: int` | `str` | Substring from..to (exclusive) |
| `str_replace(s, from, to)` | `s, from, to: str` | `str` | Replace first occurrence |
| `str_fmt(buf, fmt, ...)` | variadic | `int` | sprintf into buf |
| `str_nfmt(buf, n, fmt, ...)` | variadic | `int` | snprintf into buf |
| `str_to_int(s)` | `s: str` | `int` | Parse integer from string |
| `str_to_float(s)` | `s: str` | `float` | Parse float from string |
| `is_alpha(c)` | `c: int` | `bool` | True if char is a letter |
| `is_digit(c)` | `c: int` | `bool` | True if char is 0–9 |
| `is_alnum(c)` | `c: int` | `bool` | True if letter or digit |
| `is_space(c)` | `c: int` | `bool` | True if whitespace |
| `is_upper(c)` | `c: int` | `bool` | True if uppercase |
| `is_lower(c)` | `c: int` | `bool` | True if lowercase |
| `is_punct(c)` | `c: int` | `bool` | True if punctuation |
| `is_print(c)` | `c: int` | `bool` | True if printable |
| `char_upper(c)` | `c: int` | `int` | Uppercase a single char |
| `char_lower(c)` | `c: int` | `int` | Lowercase a single char |

---

## std::io

Low-level file I/O using C `FILE*` handles.

| Function | Params | Returns | Description |
|---|---|---|---|
| `open(path, mode)` | `path, mode: str` | `ref` | Open a file (`"r"`, `"w"`, `"a"`, ...) |
| `close(f)` | `f: ref` | `int` | Close file handle |
| `read(buf, n, f)` | `buf: str, n: int, f: ref` | `str` | Read up to n bytes |
| `readline(f)` | `f: ref` | `str` | Read one line, stripping newline |
| `write(s, f)` | `s: str, f: ref` | `int` | Write string to file |
| `writef(f, fmt, ...)` | variadic | `int` | fprintf to file |
| `fread(ptr, size, count, f)` | `ref, int, int, ref` | `int` | Raw binary read |
| `fwrite(ptr, size, count, f)` | `ref, int, int, ref` | `int` | Raw binary write |
| `flush(f)` | `f: ref` | `int` | Flush file buffer |
| `eof(f)` | `f: ref` | `bool` | True if at end of file |
| `error(f)` | `f: ref` | `int` | Non-zero if file error occurred |
| `seek(f, offset, whence)` | `f: ref, offset: int, whence: int` | `int` | Move file cursor |
| `tell(f)` | `f: ref` | `int` | Current cursor position |
| `rewind(f)` | `f: ref` | `void` | Reset cursor to start |
| `file_size(f)` | `f: ref` | `int` | Size of open file in bytes |
| `printf(fmt, ...)` | variadic | `int` | Print to stdout with format |
| `scanf(fmt, ...)` | variadic | `int` | Read from stdin with format |
| `sscanf(s, fmt, ...)` | variadic | `int` | Parse from string with format |
| `getchar()` | — | `int` | Read one char from stdin |
| `putchar(c)` | `c: int` | `int` | Write one char to stdout |
| `perror(msg)` | `msg: str` | `void` | Print errno message to stderr |

---

## std::math

Full math library. Constants `M_PI` and `M_E` are available after import.

| Function | Params | Returns | Description |
|---|---|---|---|
| `sqrt(x)` | `x: float` | `float` | Square root |
| `cbrt(x)` | `x: float` | `float` | Cube root |
| `pow(base, exp)` | `base, exp: float` | `float` | Floating-point power |
| `ipow(base, exp)` | `base, exp: int` | `int` | Integer power |
| `fabs(x)` | `x: float` | `float` | Absolute value |
| `floor(x)` | `x: float` | `float` | Round down |
| `ceil(x)` | `x: float` | `float` | Round up |
| `round(x)` | `x: float` | `float` | Round to nearest |
| `trunc(x)` | `x: float` | `float` | Truncate toward zero |
| `fmod(x, y)` | `x, y: float` | `float` | Floating-point remainder |
| `fmax(a, b)` | `a, b: float` | `float` | Larger of two floats |
| `fmin(a, b)` | `a, b: float` | `float` | Smaller of two floats |
| `sin(x)` | `x: float` | `float` | Sine (radians) |
| `cos(x)` | `x: float` | `float` | Cosine (radians) |
| `tan(x)` | `x: float` | `float` | Tangent (radians) |
| `asin(x)` | `x: float` | `float` | Arc sine |
| `acos(x)` | `x: float` | `float` | Arc cosine |
| `atan(x)` | `x: float` | `float` | Arc tangent |
| `atan2(y, x)` | `y, x: float` | `float` | Arc tangent of y/x |
| `sinh(x)` | `x: float` | `float` | Hyperbolic sine |
| `cosh(x)` | `x: float` | `float` | Hyperbolic cosine |
| `tanh(x)` | `x: float` | `float` | Hyperbolic tangent |
| `exp(x)` | `x: float` | `float` | e^x |
| `exp2(x)` | `x: float` | `float` | 2^x |
| `log(x)` | `x: float` | `float` | Natural log |
| `log2(x)` | `x: float` | `float` | Log base 2 |
| `log10(x)` | `x: float` | `float` | Log base 10 |
| `hypot(x, y)` | `x, y: float` | `float` | sqrt(x²+y²) |
| `clamp(v, lo, hi)` | `v, lo, hi: float` | `float` | Clamp float to range |
| `lerp(a, b, t)` | `a, b, t: float` | `float` | Linear interpolation |
| `gcd(a, b)` | `a, b: int` | `int` | Greatest common divisor |
| `lcm(a, b)` | `a, b: int` | `int` | Least common multiple |
| `is_nan(x)` | `x: float` | `bool` | True if x is NaN |
| `is_inf(x)` | `x: float` | `bool` | True if x is infinity |
| `deg2rad(deg)` | `deg: float` | `float` | Degrees to radians |
| `rad2deg(rad)` | `rad: float` | `float` | Radians to degrees |
| `sign(n)` | `n: int` | `int` | -1, 0, or 1 |
| `signf(n)` | `n: float` | `float` | -1.0, 0.0, or 1.0 |
| `rand()` | — | `int` | Random integer |
| `srand(seed)` | `seed: int` | `void` | Seed the RNG |

---

## std::fs

High-level file system helpers.

| Function | Params | Returns | Description |
|---|---|---|---|
| `read(path)` | `path: str` | `str` | Read entire file into string (heap-alloc) |
| `write(path, content)` | `path, content: str` | `int` | Write string to file (truncate) |
| `append(path, content)` | `path, content: str` | `int` | Append string to file |
| `exists(path)` | `path: str` | `bool` | True if file exists |
| `size(path)` | `path: str` | `int` | File size in bytes, -1 on error |
| `is_dir(path)` | `path: str` | `bool` | True if path is a directory |
| `is_file(path)` | `path: str` | `bool` | True if path is a regular file |
| `open(path, mode)` | `path, mode: str` | `ref` | fopen wrapper |
| `close(f)` | `f: ref` | `int` | fclose wrapper |
| `remove(path)` | `path: str` | `int` | Delete a file |
| `rename(old_path, new_path)` | `old_path, new_path: str` | `int` | Rename/move a file |
| `mkdir(path, mode)` | `path: str, mode: int` | `int` | Create directory (mode e.g. `0755`) |

---

## std::sys

Process and environment control.

| Function | Params | Returns | Description |
|---|---|---|---|
| `run(cmd)` | `cmd: str` | `int` | Run shell command, return status |
| `run_ok(cmd)` | `cmd: str` | `bool` | True if command exits with 0 |
| `getenv(key)` | `key: str` | `str` | Read environment variable |
| `setenv(key, val, overwrite)` | `key, val: str, overwrite: int` | `int` | Set env variable |
| `unsetenv(key)` | `key: str` | `int` | Remove env variable |
| `sleep(secs)` | `secs: int` | `int` | Sleep for N seconds |
| `usleep(us)` | `us: int` | `int` | Sleep for N microseconds |
| `getpid()` | — | `int` | Current process ID |
| `getppid()` | — | `int` | Parent process ID |
| `exit(code)` | `code: int` | `void` | Exit with code |
| `abort()` | — | `void` | Abort the process |
| `strerror(err)` | `err: int` | `str` | Human-readable error for errno value |

---

## std::cmd

Run shell commands and capture their output.

| Function | Params | Returns | Description |
|---|---|---|---|
| `capture(cmd)` | `cmd: str` | `str` | Run command and return its stdout |
| `run(cmd)` | `cmd: str` | `int` | Run command, return raw status |
| `exitcode(cmd)` | `cmd: str` | `int` | Run command, return exit code |
| `ok(cmd)` | `cmd: str` | `bool` | True if command exits with 0 |
| `popen(cmd, mode)` | `cmd, mode: str` | `ref` | Open a pipe to a command |
| `pclose(p)` | `p: ref` | `int` | Close a pipe |

---

## std::mem

Raw memory allocation and manipulation.

| Function | Params | Returns | Description |
|---|---|---|---|
| `alloc(size)` | `size: int` | `ref` | malloc — uninitialized |
| `zalloc(count, size)` | `count, size: int` | `ref` | calloc — zero-initialized |
| `realloc(ptr, size)` | `ptr: ref, size: int` | `ref` | Resize an allocation |
| `free(ptr)` | `ptr: ref` | `void` | Free memory |
| `copy(dst, src, n)` | `dst, src: ref, n: int` | `ref` | memcpy |
| `move(dst, src, n)` | `dst, src: ref, n: int` | `ref` | memmove (safe overlap) |
| `set(ptr, val, n)` | `ptr: ref, val: int, n: int` | `ref` | memset |
| `cmp(a, b, n)` | `a, b: ref, n: int` | `int` | memcmp |
| `dup(src, n)` | `src: ref, n: int` | `ref` | Heap-copy n bytes from src |

---

## std::conv

Type conversion between numbers and strings.

| Function | Params | Returns | Description |
|---|---|---|---|
| `to_int(s)` | `s: str` | `int` | Parse decimal int from string |
| `to_float(s)` | `s: str` | `float` | Parse float from string |
| `to_long(s)` | `s: str` | `int` | Parse long from string |
| `int_to_str(n)` | `n: int` | `str` | Integer to decimal string |
| `float_to_str(f)` | `f: float` | `str` | Float to string |
| `bool_to_str(b)` | `b: bool` | `str` | `"true"` or `"false"` |
| `int_to_hex(n)` | `n: int` | `str` | Integer to hex string |
| `int_to_bin(n)` | `n: int` | `str` | Integer to binary string |
| `hex_to_int(s)` | `s: str` | `int` | Parse hex string to int |
| `bin_to_int(s)` | `s: str` | `int` | Parse binary string to int |
| `oct_to_int(s)` | `s: str` | `int` | Parse octal string to int |

---

## std::time

Time and date utilities.

| Function | Params | Returns | Description |
|---|---|---|---|
| `now(_)` | `_: ref` | `int` | Unix timestamp (seconds) — pass `nil` |
| `now_ms()` | — | `int` | Unix timestamp in milliseconds |
| `now_us()` | — | `int` | Unix timestamp in microseconds |
| `clock()` | — | `int` | CPU clock ticks since program start |
| `diff(t2, t1)` | `t2, t1: int` | `float` | Seconds between two timestamps |
| `format(t, fmt)` | `t: int, fmt: str` | `str` | Format timestamp with strftime pattern |

---

## std::os

OS-level helpers (thin wrappers, prefer `std::sys` for process control).

| Function | Params | Returns | Description |
|---|---|---|---|
| `getenv(key)` | `key: str` | `str` | Read env variable |
| `setenv(key, val, overwrite)` | `key, val: str, overwrite: int` | `int` | Set env variable |
| `exit(code)` | `code: int` | `void` | Exit program |
| `abort()` | — | `void` | Abort program |
| `getcwd()` | — | `str` | Current working directory |
| `chdir(path)` | `path: str` | `int` | Change directory |
| `getpid()` | — | `int` | Process ID |

---

## std::fmt

printf-family formatting functions.

| Function | Params | Returns | Description |
|---|---|---|---|
| `print(fmt, ...)` | `fmt: str` + variadic | `int` | printf to stdout |
| `eprint(f, fmt, ...)` | `f: ref, fmt: str` + variadic | `int` | fprintf to any file |
| `sprintf(buf, fmt, ...)` | `buf: str, fmt: str` + variadic | `int` | Format into buffer |
| `snprintf(buf, n, fmt, ...)` | `buf: str, n: int, fmt: str` + variadic | `int` | Safe format into buffer |

---

## std::net

Basic TCP socket helpers.

| Function | Params | Returns | Description |
|---|---|---|---|
| `tcp_server(port)` | `port: int` | `int` | Create + bind + listen, returns fd |
| `tcp_connect(host, port)` | `host: str, port: int` | `int` | Connect to host:port, returns fd |
| `tcp_accept(fd)` | `fd: int` | `int` | Accept incoming connection |
| `tcp_send(fd, msg)` | `fd: int, msg: str` | `int` | Send string over socket |
| `tcp_recv(fd, maxbytes)` | `fd: int, maxbytes: int` | `str` | Receive up to maxbytes |
| `close_fd(fd)` | `fd: int` | `int` | Close socket |
| `htons(port)` | `port: int` | `int` | Host to network byte order (16-bit) |
| `htonl(n)` | `n: int` | `int` | Host to network byte order (32-bit) |

---

## std::rand

Random number generation.

| Function | Params | Returns | Description |
|---|---|---|---|
| `rand()` | — | `int` | Random int (0 to RAND_MAX) |
| `rand_range(lo, hi)` | `lo, hi: int` | `int` | Random int in [lo, hi) |
| `rand_float()` | — | `float` | Random float in [0.0, 1.0) |
| `seed(s)` | `s: int` | `void` | Seed the RNG with a fixed value |
| `seed_time()` | — | `void` | Seed the RNG from current time |

---

## std::sort

Sorting and searching.

| Function | Params | Returns | Description |
|---|---|---|---|
| `sort_ints(arr, n)` | `arr: ref int, n: int` | `void` | Sort int array ascending |
| `sort_ints_desc(arr, n)` | `arr: ref int, n: int` | `void` | Sort int array descending |
| `sort_floats(arr, n)` | `arr: ref float, n: int` | `void` | Sort float array ascending |
| `qsort(arr, n, size, cmp)` | `arr: ref, n: int, size: int, cmp: ref` | `void` | Generic qsort |
| `bsearch(key, arr, n, size, cmp)` | all `ref` / `int` | `ref` | Binary search, returns pointer or NULL |

---

## std::bits

Bitwise operations on 64-bit integers.

| Function | Params | Returns | Description |
|---|---|---|---|
| `popcount(n)` | `n: int` | `int` | Count set bits |
| `clz(n)` | `n: int` | `int` | Count leading zeros |
| `ctz(n)` | `n: int` | `int` | Count trailing zeros |
| `rotl(n, k)` | `n: int, k: int` | `int` | Rotate left by k bits |
| `rotr(n, k)` | `n: int, k: int` | `int` | Rotate right by k bits |
| `bit_get(n, pos)` | `n: int, pos: int` | `int` | Get bit at position |
| `bit_set(n, pos)` | `n: int, pos: int` | `int` | Set bit at position |
| `bit_clear(n, pos)` | `n: int, pos: int` | `int` | Clear bit at position |
| `bit_toggle(n, pos)` | `n: int, pos: int` | `int` | Toggle bit at position |

---

## std::hash

Hash functions for strings and raw data.

| Function | Params | Returns | Description |
|---|---|---|---|
| `fnv1a(s)` | `s: str` | `int` | FNV-1a hash of a string |
| `djb2(s)` | `s: str` | `int` | DJB2 hash of a string |
| `hash_bytes(data, len)` | `data: ref, len: int` | `int` | FNV-1a hash of raw bytes |
| `hash_int(n)` | `n: int` | `int` | MurmurHash3 finalizer for integers |

---

## std::assert

Test assertions — abort on failure.

| Function | Params | Returns | Description |
|---|---|---|---|
| `expect_eq_int(a, b, msg)` | `a, b: int, msg: str` | `void` | Abort if a != b |
| `expect_eq_str(a, b, msg)` | `a, b: str, msg: str` | `void` | Abort if strings differ |
| `expect_true(cond, msg)` | `cond: bool, msg: str` | `void` | Abort if cond is false |

---

## std::debug

Labeled debug printing to stderr.

| Function | Params | Returns | Description |
|---|---|---|---|
| `print_int(v, name)` | `v: int, name: str` | `void` | Print `[dbg] name = value` |
| `print_float(v, name)` | `v: float, name: str` | `void` | Print `[dbg] name = value` |
| `print_str(v, name)` | `v: str, name: str` | `void` | Print `[dbg] name = "value"` |
| `print_bool(v, name)` | `v: bool, name: str` | `void` | Print `[dbg] name = true/false` |
| `print_ptr(v, name)` | `v: ref, name: str` | `void` | Print `[dbg] name = 0x...` |