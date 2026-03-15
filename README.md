# ZX Language  v0.2.0

```
 ▒███████▒ ▒██   ██▒
▒ ▒ ▒ ▄▀░ ▒▒ █ █ ▒░
░ ▒ ▄▀▒░  ░░  █   ░
  ▄▀▒   ░  ░ █ █  ░
▒███████▒ ▒██▒ ▒██▒  v0.2.0
```

**ZX** is a tiny, Perl-flavored language that compiles to C.
Simple syntax. Full C interop. Rust-style error messages.

---

## Build

```bash
cd zx
go build -o zxc .
```

Requires: **Go 1.21+** and **GCC** (for compiling the generated C).

---

## Usage

```
zxc <file.zx>              compile & run immediately
zxc build <file.zx>        compile to binary
zxc build <file.zx> -o x   compile to named binary
zxc emit  <file.zx>        print the generated C source
zxc check <file.zx>        type-check only (no output)
zxc version                print version
```

---

## Quick Examples

```zx
// hello.zx
println("Hello, World!");
```

```zx
// fib.zx
fn fib(n: int) -> int {
    if n <= 1 { return n; }
    return fib(n - 1) + fib(n - 2);
}
for i in 0..15 {
    println(fib(i));
}
```

---

## Language Reference

### Comments
```zx
// line comment
# also a line comment  (Perl-style)
/* block comment */
```

### Variables & Constants
```zx
let x: int = 42;          // mutable, explicit type
let y = 3.14;             // type inferred from value
const MAX: int = 255;     // immutable constant
```

### Types

| ZX type    | C type         | Notes                        |
|------------|----------------|------------------------------|
| `int`      | `long long`    | 64-bit signed integer        |
| `float`    | `double`       | 64-bit double precision      |
| `bool`     | `int`          | 0 = false, 1 = true          |
| `str`      | `const char*`  | string literal pointer       |
| `char`     | `char`         | single byte character        |
| `void`     | `void`         | no value (return types only) |
| `ptr<T>`   | `T*`           | raw pointer to T             |
| `[N]T`     | `T[N]`         | fixed-size array             |
| `MyStruct` | `MyStruct`     | user-defined struct          |

### Functions
```zx
fn add(a: int, b: int) -> int {
    return a + b;
}

fn greet(name: str) {      // void return — no arrow needed
    println("hi ", name);
}

fn variadic(fmt: str, ...) -> int {  // variadic (maps to C ...)
    return 0;
}
```

### Control Flow
```zx
// if / elif / else
if score >= 90 {
    println("A");
} elif score >= 80 {
    println("B");
} else {
    println("F");
}

// while
while i < 10 {
    i += 1;
}

// for range  (exclusive end, like Python range)
for i in 0..10 {
    println(i);       // 0, 1, ..., 9
}

// break / continue inside loops
while true {
    if done { break; }
    if skip { continue; }
}
```

### Structs
```zx
struct Point {
    x: float,
    y: float
}

let p: Point = new Point { x: 1.0, y: 2.0 };
println(p.x);
p.y = 5.0;

// pointer to struct — use . or ->
let ptr_p: ptr<Point> = &p;
println(ptr_p->x);          // sugar for (*ptr_p).x
```

### Importing C Headers
```zx
import "stdio.h"
import "math.h"
import "string.h"
```
Becomes `#include <stdio.h>` etc. in the generated C.

### Declaring C Functions (extern)
```zx
extern fn sqrt(x: float) -> float;
extern fn printf(fmt: str, ...) -> int;
extern fn strlen(s: str) -> int;
```

**Important:** ZX knows about all standard C library functions
(`printf`, `malloc`, `sqrt`, `strlen`, etc.). If you declare them
as `extern`, ZX's type checker uses the declaration for validation,
but does **not** re-emit it in the C output (avoiding conflicts with
the headers).

### Built-in print / println
```zx
print("x = ", x);       // no newline, space-separated
println("done!");        // appends newline
println(x, y, z);        // multiple args, space-separated
```
Types are auto-formatted: `int → %lld`, `float → %g`, `str → %s`, etc.

### Cast
```zx
let f: float = 9.99;
let i: int   = int(f);       // truncates to 9
let c: char  = char(65);     // 'A'
let b: bool  = bool(0);      // false
```

### Pointers
```zx
let n: int       = 42;
let p: ptr<int>  = &n;     // take address
*p = 100;                  // dereference & write
println(n);                // 100
```

### Arrays
```zx
let arr: [5]int = [10, 20, 30, 40, 50];
println(arr[0]);           // 10
arr[2] = 999;
```

### sizeof
```zx
println(sizeof(int));      // 8  (long long on 64-bit)
println(sizeof(float));    // 8  (double)
println(sizeof(char));     // 1
```

### Operators
```
Arithmetic:   +  -  *  /  %
Comparison:   ==  !=  <  >  <=  >=
Logical:      &&  ||  !
Bitwise:      &  |  ^  ~  <<  >>
Assignment:   =  +=  -=  *=  /=  %=
Range:        ..    (for i in 0..10)
Address:      &expr   *expr
Arrow:        ->  (return type + pointer field access)
```

### Hex & Numeric Literals
```zx
let big:  int = 1_000_000;    // underscores ignored
let hex:  int = 0xFF;         // hex
let ch:   int = 'A';          // char literal → integer
```

---

## Error Messages

ZX gives Rust-style errors with file path, line, column,
source underline, and a green hint:

```
error: E11: type mismatch — cannot initialize int variable with str value
  --> examples/errors.zx:3:18
     2 │ // E11: type mismatch on init
     3 │ let score: int = "oops";
               │                  ^^^^^^
               │                  hint: cast with int(...) or change the type to str
```

### Complete Error Code List

| Code | Description |
|------|-------------|
| E01  | Struct defined more than once |
| E02  | Duplicate field in struct definition |
| E03  | Function defined more than once |
| E04  | Duplicate parameter name in function |
| E06  | exit() argument must be int |
| E07  | break outside of loop |
| E08  | continue outside of loop |
| E09  | Variable redeclared in same scope |
| E10  | Cannot infer type — no annotation and no initializer |
| E11  | Type mismatch in variable initialization |
| E12  | Variable declared with type void |
| E13  | const without initializer |
| E14  | return outside of function |
| E15  | Missing return value in non-void function |
| E16  | Returning value from void function |
| E17  | Wrong return type |
| E18  | if/elif condition is not a truthy type |
| E19  | while condition is not a truthy type |
| E20  | for-range start is not int |
| E21  | for-range end is not int |
| E22  | Assignment to const variable |
| E23  | Assignment to a function name |
| E24  | Left-hand side of assignment is a literal |
| E25  | Type mismatch in assignment |
| E26  | Compound operator (+=, etc.) on non-numeric type |
| E27  | Undefined variable or function (with typo suggestion) |
| E28  | Invalid cast between incompatible types |
| E29  | Dereference of non-pointer type |
| E30  | Comparing incompatible types with == or != |
| E31  | Relational operator on non-numeric types |
| E32  | String concatenation with + (not supported) |
| E33  | Arithmetic operator on non-numeric type |
| E34  | Division by zero literal |
| E35  | Modulo on float operands |
| E36  | Bitwise operator on non-integer type |
| E37  | Logical NOT (!) on non-boolean type |
| E38  | Unary minus on non-numeric type |
| E39  | Bitwise NOT (~) on non-integer type |
| E40  | Extern function argument type mismatch |
| E41  | Wrong argument count for extern function |
| E42  | Wrong argument count for user function |
| E43  | User function argument type mismatch |
| E44  | Call to undefined function (with typo suggestion) |
| E45  | Calling a non-function variable |
| E46  | Indexing into a non-array/pointer type |
| E47  | Non-integer array index |
| E48  | Field access on non-struct type |
| E49  | Field access on unknown struct type |
| E50  | Struct field does not exist |
| E51  | Undefined struct in initializer |
| E52  | Unknown field name in struct initializer |
| E53  | Duplicate field in struct initializer |
| E54  | Field value type mismatch in struct initializer |
| E55  | Inconsistent types in array literal |
| E56  | Unknown type used in annotation or parameter |

