# ZX Bang Macros (`!`)

Bang macros are built-in compiler-level macros called with a `!` suffix.
They expand inline at compile time — no function call overhead.

```perl
result = macro_name!(arg1, arg2);
```

---

## Debugging

### `dbg!(expr)`
Prints the expression name and its value to stderr, then returns the value.
Useful to inspect a variable without interrupting the flow.
```perl
let x = 42;
dbg!(x);                    // [dbg] x = 42
let y = dbg!(x * 2);        // [dbg] x * 2 = 84  →  y = 84
```

### `log!(expr)`
Like `dbg!` but prefixed with `[log]`. Use for non-debug runtime tracing.
```perl
log!(visit_count);          // [log] 5
```

### `assert!(cond)`  /  `assert!(cond, msg)`
Aborts with an error message if `cond` is falsy. Optional second arg is the message.
```perl
assert!(x > 0);
assert!(x > 0, "x must be positive");
```

### `ok!(expr)`
Asserts that `expr` is non-zero / non-null. Aborts if it is.
```perl
ok!(ptr);                   // aborts if ptr == NULL
ok!(file_opened);
```

### `try!(expr)`
Returns `expr` if non-zero/non-null, aborts if it is. Use for results that must succeed.
```perl
let conn = try!(open_socket());
```

---

## Errors & Control Flow

### `panic!(msg)`
Prints `msg` to stderr and aborts the program immediately.
```perl
panic!("something went very wrong");
```

### `unreachable!()`
Marks a code path that should never be reached. Aborts if it is.
```perl
match direction {
    0 => { say "north"; }
    1 => { say "south"; }
    _ => { unreachable!(); }
}
```

### `todo!(msg?)`
Marks unfinished code. Aborts with a TODO message when hit.
```perl
fn process(data: str) {
    todo!("implement this later");
}
```

### `exit_ok!()`
Exits the program with code `0` (success).
```perl
exit_ok!();
```

### `exit_err!(msg)`
Prints `msg` to stderr and exits with code `1` (failure).
```perl
exit_err!("fatal: config file not found");
```

---

## Math

### `max!(a, b)`
Returns the larger of two values.
```perl
let biggest = max!(score, high_score);
```

### `min!(a, b)`
Returns the smaller of two values.
```perl
let smallest = min!(a, b);
```

### `abs!(n)`
Returns the absolute value of `n`.
```perl
let dist = abs!(x - target);
```

### `clamp!(val, lo, hi)`
Clamps `val` to the range `[lo, hi]`.
```perl
let safe = clamp!(speed, 0, 120);
```

### `swap!(a, b)`
Swaps two variables in place. Both must be the same type.
```perl
swap!(x, y);
```

---

## Strings

### `len!(s)`
Returns the length of string `s` as an `int`.
```perl
let n = len!(name);
```

### `str_eq!(a, b)`
Returns `true` if strings `a` and `b` are equal.
```perl
if str_eq!(command, "quit") { exit_ok!(); }
```

### `str_ne!(a, b)`
Returns `true` if strings `a` and `b` differ.
```perl
if str_ne!(mode, "debug") { say "production mode"; }
```

### `str_contains!(haystack, needle)`
Returns `true` if `needle` is found anywhere in `haystack`.
```perl
if str_contains!(line, "ERROR") { Logger->error(line); }
```

### `str_starts!(s, prefix)`
Returns `true` if `s` starts with `prefix`.
```perl
if str_starts!(path, "/tmp") { say "temp file"; }
```

### `str_ends!(s, suffix)`
Returns `true` if `s` ends with `suffix`.
```perl
if str_ends!(filename, ".zx") { say "ZX source file"; }
```

### `str_to_int!(s)`
Parses string `s` as an integer. Returns `0` if invalid.
```perl
let n = str_to_int!(user_input);
```

### `str_to_float!(s)`
Parses string `s` as a float. Returns `0.0` if invalid.
```perl
let f = str_to_float!("3.14");
```

### `int_to_str!(n)`
Converts integer `n` to a decimal string.
```perl
let s = int_to_str!(count);
say f"Count: {s}";
```

### `float_to_str!(f)`
Converts float `f` to a string.
```perl
let s = float_to_str!(3.14159);
```

---

## I/O

### `print!(fmt, ...)`
Shorthand for `printf`. Identical behaviour.
```perl
print!("Hello, %s!\n", name);
```

### `eprint!(fmt, ...)`
Like `print!` but writes to stderr.
```perl
eprint!("warning: %s\n", msg);
```

### `read_line!()`
Reads one line from stdin. Returns a `str`.
```perl
let input = read_line!();
```

---

## Memory

### `alloc!(n)`
Allocates `n` bytes on the heap. Returns a raw pointer (`void*`).
```perl
let buf = alloc!(1024);
```

### `zalloc!(n)`
Like `alloc!` but zero-initialises the memory.
```perl
let buf = zalloc!(sizeof(MyStruct));
```

### `free!(ptr)`
Frees a heap-allocated pointer.
```perl
free!(buf);
```

### `memcpy!(dst, src, n)`
Copies `n` bytes from `src` to `dst`.
```perl
memcpy!(dest_buf, src_buf, 256);
```

### `memset!(ptr, val, n)`
Fills `n` bytes at `ptr` with `val`.
```perl
memset!(buf, 0, 1024);
```

---

## Type Inspection

### `type_of!(expr)`
Returns the ZX type name of `expr` as a `str` at compile time.
```perl
say type_of!(42);           // "int"
say type_of!(3.14);         // "float"
say type_of!(true);         // "bool"
say type_of!("hello");      // "str"
```

### `size_of!(expr)`
Returns the size in bytes of `expr`'s type, as an `int`.
```perl
let sz = size_of!(my_struct);
printf("size = %lld bytes\n", sz);
```

### `count_of!(array)`
Returns the number of elements in a fixed-size array.
```perl
let nums: [5]int = [1,2,3,4,5];
let n = count_of!(nums);    // 5
```

---

## Environment

### `env!(name)`
Returns the value of environment variable `name` as a `str`. Returns `NULL` if not set.
```perl
let home = env!("HOME");
let path = env!("PATH");
```

---

## Performance Hints

### `likely!(cond)`
Hints to the compiler that `cond` is usually `true`. Can improve branch prediction.
```perl
if likely!(ptr != nil) { ... }
```

### `unlikely!(cond)`
Hints to the compiler that `cond` is usually `false`.
```perl
if unlikely!(err != 0) { panic!("unexpected error"); }
```

---

## Timing

### `time!(expr)`
Evaluates `expr`, prints its wall-clock time in milliseconds to stderr, and returns the result.
```perl
let result = time!(expensive_function());
// [time] expensive_function() = 42  (17ms)
```

---

## Quick Reference

| Macro | Args | Returns | Description |
|---|---|---|---|
| `dbg!` | 1 | value | Print name+value to stderr, return value |
| `log!` | 1 | value | Like dbg! but `[log]` prefix |
| `assert!` | 1–2 | void | Abort if condition is false |
| `ok!` | 1 | void | Abort if value is zero/null |
| `try!` | 1 | value | Return value or abort if null |
| `panic!` | 1 | void | Print message and abort |
| `unreachable!` | 0 | void | Mark unreachable code path |
| `todo!` | 0–1 | void | Mark unfinished code |
| `exit_ok!` | 0 | void | Exit with code 0 |
| `exit_err!` | 1 | void | Print message and exit 1 |
| `max!` | 2 | value | Larger of two values |
| `min!` | 2 | value | Smaller of two values |
| `abs!` | 1 | value | Absolute value |
| `clamp!` | 3 | value | Clamp to [lo, hi] |
| `swap!` | 2 | void | Swap two variables |
| `len!` | 1 | int | String length |
| `str_eq!` | 2 | bool | String equality |
| `str_ne!` | 2 | bool | String inequality |
| `str_contains!` | 2 | bool | Substring search |
| `str_starts!` | 2 | bool | Prefix check |
| `str_ends!` | 2 | bool | Suffix check |
| `str_to_int!` | 1 | int | Parse string to int |
| `str_to_float!` | 1 | float | Parse string to float |
| `int_to_str!` | 1 | str | Int to decimal string |
| `float_to_str!` | 1 | str | Float to string |
| `print!` | 1+ | void | printf shorthand |
| `eprint!` | 1+ | void | fprintf(stderr) shorthand |
| `read_line!` | 0 | str | Read line from stdin |
| `alloc!` | 1 | ptr | malloc(n) |
| `zalloc!` | 1 | ptr | calloc(n, 1) |
| `free!` | 1 | void | free(ptr) |
| `memcpy!` | 3 | ptr | Copy memory |
| `memset!` | 3 | ptr | Fill memory |
| `type_of!` | 1 | str | ZX type name |
| `size_of!` | 1 | int | Bytes of type |
| `count_of!` | 1 | int | Elements in fixed array |
| `env!` | 1 | str | Read env variable |
| `likely!` | 1 | bool | Branch prediction hint (true) |
| `unlikely!` | 1 | bool | Branch prediction hint (false) |
| `time!` | 1 | value | Time expression, print ms |
| `cast!` | 2 | value | Type cast |
