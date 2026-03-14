# ZX Language

**ZX** is a tiny, fast, Perl-flavored language that compiles to C.  
Write simple scripts. Get blazing-fast native binaries.

```
 ▒███████▒ ▒██   ██▒
▒ ▒ ▒ ▄▀░ ▒▒ █ █ ▒░
░ ▒ ▄▀▒░  ░░  █   ░
  ▄▀▒   ░  ░ █ █  ░
▒███████▒ ▒██▒ ▒██▒  v0.1.0
```

---

## Quick Start

```bash
# compile the compiler
cd zx
go build -o zxc .

# run a ZX program
./zxc examples/hello.zx

# compile only
./zxc build examples/demo.zx -o demo

# emit the generated C
./zxc emit examples/fib.zx

# type-check only
./zxc check examples/demo.zx
```

---

## Language Reference

### Comments
```zx
// line comment
# also a line comment (Perl style)
/* block comment */
```

### Variables
```zx
let x: int = 42;          // mutable variable, explicit type
let y = 3.14;              // type inferred from initializer
const MAX: int = 255;      // immutable constant
```

### Types
| ZX type  | C type        | Notes                  |
|----------|---------------|------------------------|
| `int`    | `long long`   | 64-bit integer         |
| `float`  | `double`      | 64-bit float           |
| `bool`   | `int`         | 0 / 1                  |
| `str`    | `const char*` | string literal         |
| `char`   | `char`        | single character       |
| `void`   | `void`        | no value               |
| `ptr<T>` | `T*`          | raw pointer to T       |
| `MyStruct` | `MyStruct`  | user-defined struct    |

### Functions
```zx
fn add(a: int, b: int) -> int {
    return a + b;
}

fn greet(name: str) -> void {
    println("Hello, ", name);
}
```

### If / Elif / Else
```zx
if score >= 90 {
    println("A");
} elif score >= 80 {
    println("B");
} else {
    println("F");
}
```

### While
```zx
let i: int = 0;
while i < 10 {
    println(i);
    i += 1;
}
```

### For Range
```zx
for i in 0..10 {
    println(i);       // prints 0 to 9
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
```

### Imports (C headers)
```zx
import "stdio.h"
import "math.h"
```
This becomes `#include <stdio.h>` in the emitted C.

### Extern (declare C functions)
```zx
extern fn sqrt(x: float) -> float;
extern fn printf(fmt: str, ..) -> int;
```
After declaring, call them directly:
```zx
let r: float = sqrt(2.0);
printf("root 2 = %f\n", r);
```

### Print / Println
```zx
print("x = ", x);        // no newline, space-separated
println("done!");         // with newline
```
Types are auto-formatted (int → `%lld`, float → `%g`, str → `%s`, etc.)

### Cast
```zx
let f: float = 3.99;
let i: int = int(f);      // explicit cast
let c: char = char(65);   // 'A'
```

### Pointers (advanced)
```zx
let n: int = 42;
let p: ptr<int> = &n;
*p = 100;                 // n is now 100
```

### Operators
```
+  -  *  /  %          arithmetic
==  !=  <  >  <=  >=   comparison
&&  ||  !               logical
&  |  ^  ~  <<  >>     bitwise
+=  -=  *=  /=         compound assignment
..                      range (for loops)
->                      return type arrow
```

---

## CLI Commands

```
zxc <file.zx>             compile & run immediately
zxc build <file.zx>       compile to binary
zxc build <file.zx> -o x  compile to named binary
zxc emit <file.zx>        print generated C source
zxc check <file.zx>       type-check only
zxc version               print version
```

---

## Error Messages

ZX gives you Rust-style errors with file, line, column, underline, and hints:

```
error: type mismatch: cannot assign str to int
  --> examples/errors.zx:3:15
   3 │ let x: int = "hello";
                    ^^^^^^^
     │              hint: change the type annotation to str, or cast the value
```

---

## Full Example

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

```bash
./zxc examples/fib.zx
```

---

## Requirements

- Go 1.21+ (to build the compiler)
- GCC (to compile the generated C)
