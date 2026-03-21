# Macros

ZX has two kinds of macros: **bang macros** (`name!(args)`) and **chain macros**
(`value name: { body }`). Both are inlined at compile time — no function call overhead.

---

## Bang macros — `name!(args)`

Called with a `!` suffix. They expand to inline C expressions.

### Built-in bang macros

```perl
// Debugging
dbg!(x)                   // print "x = <value>" to stderr, return value
log!(x)                   // same but with [log] prefix
assert!(cond)             // abort if false
assert!(cond, "message")
ok!(expr)                 // abort if zero/null
try!(expr)                // return expr or abort if null

// Errors
panic!("message")         // print and abort
unreachable!()            // mark unreachable — abort if hit
todo!("not done yet")     // abort with TODO message
exit_ok!()                // exit(0)
exit_err!("message")      // print and exit(1)

// Math
max!(a, b)                // larger of two values
min!(a, b)                // smaller
abs!(n)                   // absolute value
clamp!(val, lo, hi)       // clamp to [lo, hi]
swap!(a, b)               // swap two variables in place

// Strings
len!(s)                   // string length
str_eq!(a, b)             // string equality (1/0)
str_ne!(a, b)             // string inequality
str_contains!(s, sub)     // 1 if sub found in s
str_starts!(s, prefix)    // 1 if s starts with prefix
str_ends!(s, suffix)      // 1 if s ends with suffix
str_to_int!(s)            // parse string to int
str_to_float!(s)          // parse string to float
int_to_str!(n)            // int to decimal string
float_to_str!(f)          // float to string

// I/O
print!(fmt, ...)          // printf shorthand
eprint!(fmt, ...)         // fprintf(stderr, ...) shorthand
read_line!()              // read line from stdin

// Memory
alloc!(n)                 // malloc(n)
zalloc!(n)                // calloc(n, 1) — zero initialised
free!(ptr)                // free(ptr)
memcpy!(dst, src, n)      // copy n bytes
memset!(ptr, val, n)      // fill n bytes with val

// Type inspection
type_of!(expr)            // ZX type name as str: "int", "float", etc.
size_of!(expr)            // sizeof the expression's type in bytes
count_of!(array)          // number of elements in fixed array

// Environment
env!("VAR")               // getenv("VAR") — returns str

// Performance hints
likely!(cond)             // branch prediction: usually true
unlikely!(cond)           // branch prediction: usually false

// Timing
time!(expr)               // evaluate expr, print time in ms to stderr, return result
```

### User-defined bang macros

Write `macro fn name(params) -> ret { body }` — called as `name!(args)`:

```perl
macro fn double(n: int) -> int {
    return n * 2;
}

macro fn clamp_zero(n: int) -> int {
    if n < 0 { return 0; }
    return n;
}

macro fn greet(name: str) -> str {
    return f"Hello, {name}!";
}

let d = double!(7);            // 14
let c = clamp_zero!(-5);       // 0
let g = greet!("World");       // "Hello, World!"
```

**Pipe with bang macros:**

```perl
let result = 3 |> double! |> double!;   // 12
```

---

## Chain macros — `value name: { body }`

Chain macros pipe a value through a series of steps. Each step can run the
`{ body }` block conditionally or in a loop.

### Syntax

```perl
value
    macroName: { body }
    macroName: { body }
    macroName: { body }
```

`do` keyword is optional:

```perl
value
    ifTrue: do { body }    // same as:
    ifTrue: { body }       // both work
```

### Built-in chain macros (no declaration needed)

These work without any `macro fn` declaration:

```perl
// Conditional
value ifTrue:     { }      // run if value != 0
value ifFalse:    { }      // run if value == 0
value unless:     { }      // alias for ifFalse
value ifNil:      { }      // run if value == NULL
value ifNotNil:   { }      // run if value != NULL
value ifZero:     { }      // run if value == 0
value ifNotZero:  { }      // run if value != 0
value ifPositive: { }      // run if value > 0
value ifNegative: { }      // run if value < 0

// Unconditional
value then:    { }          // always run
value always:  { }          // alias for then

// Loops
value whileTrue: { }        // while value { body }
value repeat:    { }        // run body value times
value times:     { }        // alias for repeat
```

### User-defined chain macros

Declare with `|input, doStmt| -> |output|`:

- `input` — the piped value
- `doStmt` — write `doStmt;` inside the body to inline the user's `{ }` block
- `output` — the result passed to the next chain step

```perl
// Run block only if truthy
macro fn ifTrue |input, doStmt| -> |output| {
    output = input;
    if input {
        doStmt;
    }
}

// Double the value, then run block
macro fn doubled |input, doStmt| -> |output| {
    output = input * 2;
    doStmt;
}

// Run block N times
macro fn repeat |input, doStmt| -> |output| {
    output = input;
    for i in 0..input {
        doStmt;
    }
}

// Run block while countdown > 0
macro fn countdown |input, doStmt| -> |output| {
    output = input;
    while output > 0 {
        doStmt;
        output -= 1;
    }
}
```

Usage:

```perl
my score = 95;

score
    ifTrue: { say "score is truthy"; }
    doubled: { printf("doubled: %lld\n", score); }
    then: { say "done"; }

my n = 3;
n
    repeat: { say "hello!"; }      // prints 3 times

n
    countdown: { printf("tick %lld\n", n); }
```

### Chain macros with explicit arguments

Declare extra params between `input` and `doStmt`:

```perl
macro fn inRange |input, lo: int, hi: int, doStmt| -> |output| {
    output = input;
    if input >= lo && input <= hi {
        doStmt;
    }
}

macro fn clampedTo |input, lo: int, hi: int, doStmt| -> |output| {
    if input < lo { output = lo; }
    elif input > hi { output = hi; }
    else { output = input; }
    doStmt;
}
```

Call with explicit args before the `:`:

```perl
my temp = 25;

temp
    inRange(0, 100): { printf("temp %lld is in range\n", temp); }
    clampedTo(0, 50): { printf("clamped: %lld\n", temp); }
    then: { say "done"; }
```

---

## Combining both styles

```perl
my val = 10;

// Bang macro to compute, then chain to decide what to do
let doubled = times2!(val);

doubled
    ifPositive: { say "doubled is positive"; }
    ifTrue: {
        let s = int_to_str!(doubled);
        say f"value = {s}";
    }
    then: { say "all done"; }
```
