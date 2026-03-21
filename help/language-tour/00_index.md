# ZX Language Documentation

ZX is a fast, Perl-flavoured language that compiles to C.
Write expressive high-level code; get native performance.

```
./zxc file.zx           compile and run
./zxc build file.zx     compile to binary
./zxc test  file.zx     run @test functions
./zxc emit  file.zx     print generated C
./zxc check file.zx     type-check only
./zxc -c "say 'hello'"  one-liner
```

---

## Contents

| File | Topic |
|---|---|
| [01 — Variables](01_variables.md) | `my`, `let`, `const`, `our`, types, inference |
| [02 — Control Flow](02_control_flow.md) | `if/elif/else`, `unless`, `match`, `while`, `until`, `for`, `defer`, `try/catch` |
| [03 — Functions](03_functions.md) | Declaration, return types, defaults, recursion, pipes, externs, annotations |
| [04 — Structs & Methods](04_structs_and_methods.md) | `type`, fields, stack vs heap, receiver methods |
| [05 — Mods](05_mods.md) | Namespaced function groups, nested mods, `our` globals, tests |
| [06 — Imports](06_imports.md) | C headers, stdlib modules, file imports, `_/path`, `__/path` |
| [07 — Macros](07_macros.md) | Bang macros `name!()`, chain macros, user-defined macros |
| [08 — Strings](08_strings.md) | Literals, f-strings, multiline `@\`...\``, operations |
| [09 — Arrays & Pointers](09_arrays_and_pointers.md) | Fixed arrays, `*T`, `ref T`, heap allocation, pointer patterns |
| [10 — Error Handling](10_error_handling.md) | `try/catch`, `assert`, `die`, `defer`, result patterns |

---

## Quick syntax cheat-sheet

```perl
// Variables
my x: int    = 42;
let y        = 3.14;
const MAX    = 100;
our counter  = 0;

// Control flow
if x > 0 { say "pos"; } elif x < 0 { say "neg"; } else { say "zero"; }
unless done { keep_going(); }
match x { 1 => { say "one"; }  _ => { say "other"; } }

// Loops
for i in 0..10 { say i; }
while running { tick(); }
until done { work(); }

// Functions
fn add(a: int, b: int) -> int { return a + b; }
fn (p *Point) move(dx: int, dy: int) { p->x += dx; p->y += dy; }

// Structs
type Point struct { x: int; y: int; }
my p = &Point{x: 3, y: 4};
say p->x;

// Mods
mod Math { fn square(n: int) -> int { return n * n; } }
say Math->square(5);      // 25
square(5);                // ERROR — must use mod name

// Imports
use "stdio.h"
use std::str
import _/utils
import _/utils (Helper)
import std/net/socket (TcpServer)

// Strings
say f"Hello, {name}!";
my html = @`<h1>${title}</h1>`;

// Bang macros
dbg!(x);
let s = int_to_str!(42);
assert!(x > 0, "must be positive");

// Chain macros
score
    ifTrue:  { say "has score"; }
    doubled: { printf("doubled: %lld\n", score); }
    then:    { say "done"; }
```
