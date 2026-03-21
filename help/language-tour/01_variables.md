# Variables

ZX has four variable declaration keywords, each with a different purpose.

---

## `my` — mutable local variable

The standard way to declare a variable inside a function or block.

```perl
my name: str  = "Alice";
my age:  int  = 30;
my score      = 99;       // type inferred from value
```

Type annotation is optional when the type can be inferred from the initializer.
`my` is an alias for `let`.

---

## `let` — mutable local variable

Identical to `my`. Both keywords exist so the language feels natural coming from
different backgrounds.

```perl
let x: int   = 10;
let y: float = 3.14;
let flag     = true;
```

---

## `const` — immutable constant

Declared at the top level or inside a block. Cannot be reassigned.
Convention: UPPER_CASE names.

```perl
const MAX_RETRIES: int = 5;
const PI: float        = 3.14159;
const APP_NAME: str    = "MyApp";
```

Attempting to assign to a const is a compile error:

```perl
MAX_RETRIES = 10;    // error: cannot assign to const
```

---

## `our` — global mutable variable

Declared at top level. Visible and writable from every function, method,
and mod block in the file. Emitted as a C file-scope variable.

```perl
our request_count: int = 0;
our current_user: str  = "";
our debug_mode: bool   = false;
```

Access and modify from anywhere:

```perl
fn handle_request() {
    request_count = request_count + 1;
    say f"Requests so far: {request_count}";
}

mod Stats {
    fn report() {
        say f"Total: {request_count}";
    }
}
```

---

## Types

| Keyword | C type | Description |
|---|---|---|
| `int` | `long long` | 64-bit signed integer |
| `float` | `double` | 64-bit floating point |
| `bool` | `int` | Boolean (0 or 1) |
| `str` | `const char*` | String (C string pointer) |
| `char` | `char` | Single byte character |
| `void` | `void` | No value |
| `any` | `long long` | Untyped (use sparingly) |
| `ref T` | `T*` | Pointer to T |
| `*T` | `T*` | Pointer to T (shorthand) |
| `[N]T` | `T[N]` | Fixed-size array of N elements |

```perl
my n:   int   = 42;
my f:   float = 1.5;
my b:   bool  = true;
my s:   str   = "hello";
my c:   char  = 'A';
my arr: [4]int = [1, 2, 3, 4];
my ptr: *int  = &n;
```

---

## Type inference

When a type annotation is omitted, ZX infers it from the initializer:

```perl
my x = 42;        // int
my y = 3.14;      // float
my z = "hello";   // str
my b = true;      // bool
```

---

## Multiple assignment

Variables can be reassigned freely (except `const` and `our`):

```perl
my score = 0;
score = 10;
score += 5;
score -= 2;
score *= 3;
score /= 2;
score %= 7;
```

---

## sizeof / typeof

```perl
say sizeof(int);       // 8
say sizeof(float);     // 8
say sizeof(Point);     // size of your struct

say typeof(42);        // "int"
say typeof(3.14);      // "float"
say typeof("hello");   // "str"
```
