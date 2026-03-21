# ZX

```
 ▒███████▒ ▒██   ██▒
▒ ▒ ▒ ▄▀░ ▒▒ █ █ ▒░
░ ▒ ▄▀▒░  ░░  █   ░
  ▄▀▒   ░  ░ █ █  ░
▒███████▒ ▒██▒ ▒██▒  v3.2.0
```

A fast, small language that compiles to C. Perl-flavored syntax, full C interop, Rust-style errors.

```sh
go build -o zxc .          # requires Go 1.21+ and GCC
zxc -c 'say "hello";'      # run a one-liner
```

---

## CLI

```
zxc <file.zx>                  compile and run
zxc build <file.zx>            compile to binary
zxc build <file.zx> -o name    compile to named binary
zxc build <file.zx> -O2        compile with optimizations
zxc emit  <file.zx>            print generated C
zxc check <file.zx>            type-check only
zxc test  <file.zx>            run @test functions
zxc repl                       interactive REPL
zxc -c "code"                  run a one-liner
zxc mods                       list stdlib modules
```

**Optimization flags:** `-O0` (default) `-O1` `-O2` `-O3` `-Os` `-Oz`

---

## Variables

```zx
let x = 42;               // inferred type
let y: float = 3.14;      // explicit type
const MAX = 255;          // immutable
our counter = 0;          // file-scope global
priv fn helper() { }      // priv = not importable from other files
```

**Keywords:** `let` `my` `const` `our` `priv`

---

## Types

| Type | C equivalent | Notes |
|------|-------------|-------|
| `int` | `long long` | 64-bit signed |
| `float` | `double` | 64-bit |
| `bool` | `int` | `true` / `false` |
| `str` | `const char*` | string pointer |
| `char` | `char` | single byte |
| `void` | `void` | return types only |
| `any` | `long long` | escape hatch |
| `ref T` | `T*` | pointer to T |
| `[N]T` | `T[N]` | fixed-size array |
| `MyStruct` | `MyStruct` | user struct |

**Casts:** `int(x)` `float(x)` `bool(x)` `char(x)`

---

## Functions

```zx
fn add(a: int, b: int) -> int {
    return a + b;
}

fn greet(name: str) {           // void — no arrow needed
    say "hello", name;
}

fn variadic(fmt: str, ...) -> int { }   // C variadic
```

**Method syntax:**
```zx
fn (self ref Point) scale(factor: float) -> Point {
    return new Point { x: self->x * factor, y: self->y * factor };
}
```

**Lambda:**
```zx
let double = |x: int| -> int { return x * 2; };
```

**Annotations:**
```zx
@inline  fn fast() { }
@cold    fn rare_path() { }
@hot     fn critical() { }
@deprecated fn old() { }
@test    fn test_add() { assert add(1, 2) == 3, "wrong"; }
```

---

## Control Flow

```zx
// if / elif / else
if score >= 90 { say "A"; }
elif score >= 80 { say "B"; }
else { say "F"; }

// unless (inverse if)
unless ready { return; }

// while / until
while i < 10 { i += 1; }
until done { step(); }

// for range  (exclusive end)
for i in 0..10 { say i; }
for i in 0..10 : 2 { say i; }    // step by 2

// match
match status {
    0 => { say "ok"; }
    1 | 2 => { say "warn"; }    // multi-pattern arm
    _ => { say "error"; }
}

// repeat N times
repeat 5 { say "hi"; }

// with — scoped alias
with expensive_call() as result {
    say result;
}

// try / catch / finally
try {
    risky();
} catch (err) {
    say "errno:", err;
} finally {
    cleanup();
}

// defer — runs at scope exit (LIFO)
defer free(ptr);

// break / continue
while true {
    if done { break; }
    if skip { continue; }
}
```

---

## Structs

```zx
struct Point {
    x: float,
    y: float
}

let p = new Point { x: 1.0, y: 2.0 };
let q: ref Point = &Point { x: 3.0, y: 4.0 };  // heap alloc

p.x = 5.0;
q->y = 6.0;

say sizeof(Point);
```

---

## Strings

```zx
let name = "world";

// f-string interpolation
let msg = f"hello {name}, you are {age} years old";

// multiline with ${ } interpolation
let body = @`
  name: ${name}
  lines: ${count}
`;

// common operations  (bang macros)
len!(s)                 // byte length
str_eq!(a, b)           // value equality  (not pointer)
str_contains!(s, "x")
str_starts!(s, "pre")
str_ends!(s, "suf")
str_to_int!(s)
str_to_float!(s)
int_to_str!(n)
float_to_str!(f)
str_upper!(s)
str_lower!(s)
str_trim!(s)
str_repeat!(s, 3)
```

---

## Pointers & Memory

```zx
let n = 42;
let p: ref int = &n;
*p = 100;

// manual memory
let buf = alloc!(1024);
free!(buf);
let z   = zalloc!(10, sizeof(int));   // calloc
memcpy!(dst, src, size);
memset!(ptr, 0, size);

// nil checks
is_nil!(ptr)
not_nil!(ptr)
```

---

## Output

```zx
print "x =", x;          // no newline
println "done!";          // with newline
say "hi";                 // alias for println
warn "debug info";        // prints to stderr
eprint "raw stderr";      // also stderr, no newline
```

---

## Input

```zx
let line = input();                  // read line from stdin
let name = input("enter name: ");    // with prompt
let raw  = read_line!();             // bang macro form
```

---

## File I/O

```zx
let src  = readfile!("data.txt");
writefile!("out.txt", content);
```

---

## Shell Commands

```zx
cmd!("ls -la");                    // run, discard output
let out = cmd!("git log --oneline");   // capture output
spawn git_pull();                      // fire and forget
```

---

## Macros

```zx
macro fn double |x: int| -> int {
    return x * 2;
}

// chain macros — pipeline style
score
    ifPositive: {
        say "positive";
    }
    ifNegative: {
        say "negative";
    }

// built-in chain macros
value
    ifTrue:     { ... }
    ifFalse:    { ... }
    ifNil:      { ... }
    ifNotNil:   { ... }
    ifZero:     { ... }
    ifNotZero:  { ... }
    ifEven:     { ... }
    ifOdd:      { ... }
    ifGt(10):   { ... }
    ifLt(0):    { ... }
    repeat:     { ... }      // run N times
    times:      { ... }      // alias
    whileTrue:  { ... }
    then:       { ... }      // always run
    map(fn):                 // transform value
    tap:        { ... }      // side-effect, keep value
    orDefault(x):            // replace if falsy
    clampTo(lo, hi):         // clamp in place
    printVal:                // print and continue

// pipe operator
value |> transform |> validate
```

---

## Bang Macros

Quick inline operations. All expand to C expressions — zero overhead.

**Debug / safety**
```zx
dbg!(x)              // print name=value to stderr, returns x
log!(x)              // print [log] value to stderr
time!(expr)          // print timing for expr to stderr
assert!(cond, msg)   // abort with message if false
ok!(cond)            // abort if false
panic!("reason")     // abort unconditionally
unreachable!()       // marks a code path as impossible
todo!("msg")         // marks unfinished code
try!(ptr)            // abort if ptr is nil, otherwise return it
```

**Math**
```zx
min!(a, b)
max!(a, b)
abs!(n)
clamp!(v, lo, hi)
between!(v, lo, hi)    // lo <= v <= hi
sign!(n)               // -1, 0, or 1
swap!(a, b)
```

**Bits**
```zx
bit_set!(val, n)       // test bit n
bit_on!(val, n)        // set bit n
bit_off!(val, n)       // clear bit n
```

**Strings**
```zx
len!(s)
str_eq!(a, b)
str_ne!(a, b)
str_contains!(s, sub)
str_starts!(s, pre)
str_ends!(s, suf)
str_to_int!(s)
str_to_float!(s)
int_to_str!(n)
float_to_str!(f)
str_upper!(s)
str_lower!(s)
str_trim!(s)
str_repeat!(s, n)
```

**Arrays**
```zx
count_of!(arr)         // element count (compile-time)
arr_fill!(arr, v, n)
arr_sum!(arr, n)
arr_min!(arr, n)
arr_max!(arr, n)
```

**Types / introspection**
```zx
type_of!(x)            // returns type name as str
size_of!(x)            // sizeof the value's type
cast!(type, value)     // raw C cast
sizeof(Type)           // sizeof a type (keyword form)
typeof(expr)           // type name as str (keyword form)
```

**Misc**
```zx
env!("HOME")           // getenv
print!(fmt, ...)       // raw printf
eprint!(fmt, ...)      // fprintf to stderr
read_line!()           // read stdin line
exit_ok!()             // exit(0)
exit_err!("msg")       // print msg, exit(1)
alloc!(n)              // malloc(n)
zalloc!(n)             // calloc(n, 1)
free!(ptr)
memcpy!(dst, src, n)
memset!(ptr, v, n)
is_nil!(ptr)
not_nil!(ptr)
likely!(cond)          // branch hint
unlikely!(cond)        // branch hint
once!(expr)            // evaluate once, cache forever
apply!(macro, val)     // apply a named macro as an expression
```

---

## Modules

```zx
// declare a module
mod Math {
    fn square(x: int) -> int { return x * x; }
    priv fn internal() { }     // not importable
}

// call module functions
Math->square(5);
Math::square(5);         // both work

// module init — runs once before main
mod Config {
    fn __init__() {
        // setup code
    }
}
```

---

## Imports

```zx
use "stdio.h"             // C header: becomes #include <stdio.h>
use std::math             // stdlib module
use std::str
use std::io
use std::fs
use std::sys
use std::cmd
use std::mem
use std::conv
use std::time
use std::net

import _/utils            // ./utils.zx
import __/shared/types    // ../shared/types.zx
import ___/common         // ../../common.zx
import _/logger (Logger)  // import only the Logger mod block
import std/net/socket     // $ZENX_STD_PATH/net/socket.zx
import usr/mylib/util     // $ZENX_USR_PATH/mylib/util.zx
```

---

## Extern (C interop)

```zx
extern fn malloc(size: int) -> ref void;
extern fn printf(fmt: str, ...) -> int;
extern fn sqrt(x: float) -> float;
```

ZX knows all standard C library functions already — declare `extern` only for third-party or non-standard functions.

---

## Visibility (`priv`)

By default everything is **public** and importable. Use `priv` to make a declaration file-private:

```zx
priv fn helper() { }          // not accessible from other files
priv struct Internal { ... }  // same
priv macro cleanup |x| { }   // same
priv mod Details { ... }      // entire mod is private

mod Api {
    fn public_fn() { }
    priv fn private_fn() { }  // private within the mod
}
```

Accessing a `priv` name from another file is a compile-time error (`EP1`–`EP6`).

---

## Diagnostics

Errors show file, line, column, source underline, and a fix suggestion:

```
error[E11]: type mismatch — cannot assign str to int
  --> src/main.zx:4:16
   3 |  // declare score as int
   4 |  let score: int = "ninety";
                         ^^^^^^^^^
   4 |  help  change the type to str, or cast: int("ninety")
   4 |  fix   let score: str = "ninety";
```

All diagnostic codes are listed in **[errors.md](errors.md)**.

---

## Numeric Literals

```zx
1_000_000      // underscores ignored
0xFF           // hex
0b1010         // binary
0o755          // octal
1.5e10         // scientific notation
'A'            // char literal → int (65)
```

---

## Operators

```
Arithmetic    +  -  *  /  %
Comparison    ==  !=  <  >  <=  >=
Logical       &&  ||  !
Bitwise       &  |  ^  ~  <<  >>
Assignment    =  +=  -=  *=  /=  %=
Range         ..   (exclusive end)
Pipe          |>   (left to right chaining)
Address       &expr   *expr   ^expr
Ternary       cond ? then : else
```

---

## Quick Reference

```zx
// hello world
say "hello, world";

// fibonacci
fn fib(n: int) -> int {
    if n <= 1 { return n; }
    return fib(n - 1) + fib(n - 2);
}
for i in 0..10 { say fib(i); }

// struct + method
struct Vec2 { x: float, y: float }
fn (v Vec2) len() -> float { return sqrt(v.x*v.x + v.y*v.y); }

// file-private helper + public api
priv fn validate(x: int) -> bool { return x > 0; }
fn process(x: int) -> int {
    if !validate(x) { panic!("bad input"); }
    return x * 2;
}

// macro pipeline
let result = getUserScore();
result
    ifGt(100): { say "bonus!"; }
    clampTo(0, 100):
    printVal:
```