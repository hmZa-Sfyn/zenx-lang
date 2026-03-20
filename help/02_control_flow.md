# Control Flow

---

## if / elif / else

```perl
if score >= 90 {
    say "A";
} elif score >= 80 {
    say "B";
} elif score >= 70 {
    say "C";
} else {
    say "F";
}
```

The condition does not need parentheses. Braces are always required.

---

## unless

`unless` is the inverse of `if` — the block runs when the condition is **false**.
Reads more naturally in some situations.

```perl
unless logged_in {
    say "Please log in first.";
}

// equivalent to:
if !logged_in {
    say "Please log in first.";
}
```

`unless` also supports `else`:

```perl
unless ready {
    say "Not ready.";
} else {
    say "Ready!";
}
```

---

## Ternary operator

Inline conditional expression:

```perl
my label: str = score >= 50 ? "pass" : "fail";
my abs_x: int = x >= 0 ? x : -x;
```

---

## match

Pattern matching against a value. Each arm uses `=>` followed by a block.
`_` is the wildcard (default) arm.

```perl
match status_code {
    200 => { say "OK"; }
    404 => { say "Not Found"; }
    500 => { say "Server Error"; }
    _   => { say "Unknown"; }
}
```

**Guards** — add an `if` condition to any arm:

```perl
match temp {
    _ if temp < 0   => { say "Below freezing"; }
    _ if temp < 20  => { say "Cold"; }
    _ if temp < 30  => { say "Warm"; }
    _               => { say "Hot"; }
}
```

Match on a string (uses the value as an integer internally — for string
matching use `str_eq!` or `if` chains):

```perl
match day_num {
    1 => { say "Monday"; }
    2 => { say "Tuesday"; }
    3 => { say "Wednesday"; }
    _ => { say "Other"; }
}
```

---

## while

Runs as long as the condition is true:

```perl
my i = 0;
while i < 10 {
    say i;
    i += 1;
}
```

---

## until

Runs until the condition becomes true (inverse of `while`):

```perl
my i = 0;
until i >= 10 {
    say i;
    i += 1;
}
```

---

## for (range loop)

`for var in start..end` iterates from `start` up to (but not including) `end`:

```perl
for i in 0..5 {
    say i;          // 0 1 2 3 4
}
```

**With step:**

```perl
for i in 0..20: 2 {
    say i;          // 0 2 4 6 8 10 12 14 16 18
}
```

**Reverse** — just write the loop backwards using a helper:

```perl
for i in 0..5 {
    say 4 - i;      // 4 3 2 1 0
}
```

---

## break / last

Exit a loop immediately. Both keywords are identical.

```perl
for i in 0..100 {
    if i == 5 { break; }
    say i;          // 0 1 2 3 4
}

my j = 0;
while true {
    if j >= 3 { last; }
    j += 1;
}
```

---

## continue / next

Skip to the next iteration. Both keywords are identical.

```perl
for i in 0..10 {
    if i % 2 == 0 { continue; }
    say i;          // 1 3 5 7 9
}

for i in 0..10 {
    if i % 3 == 0 { next; }
    say i;          // 1 2 4 5 7 8
}
```

---

## defer

Schedules a statement to run at the **end of the current block**, regardless of
how the block exits. Multiple defers run in reverse order (last-in, first-out).

```perl
fn open_file() {
    say "Opening...";
    defer say "File closed.";        // runs last
    defer say "Flushing buffer...";  // runs second-to-last
    say "Doing work.";
    say "More work.";
    // output:
    // Opening...
    // Doing work.
    // More work.
    // Flushing buffer...
    // File closed.
}
```

---

## try / catch / finally

```perl
try {
    // code that might fail
    let result = risky_operation();
} catch (err) {
    // err holds the errno value
    printf("Error: %d\n", err);
} finally {
    // always runs, even if no error
    say "Cleanup done.";
}
```

`catch` and `finally` are both optional:

```perl
try {
    do_something();
} finally {
    cleanup();
}
```

---

## assert

Checks a condition at runtime. Aborts with a message if it fails.

```perl
assert x > 0;
assert len!(name) > 0, "name cannot be empty";
assert factorial(5) == 120, "factorial is broken";
```

---

## die / throw / raise

All three keywords are identical — print a message to stderr and exit with
code 1. Use whichever feels most natural.

```perl
die "something went wrong";
throw "invalid state";
raise "unexpected value";
```

---

## exit

Exit the program with a specific code:

```perl
exit 0;        // success
exit 1;        // failure
exit(code);    // with parentheses also works
```

---

## spawn

Run a system command in the background (non-blocking):

```perl
spawn say "this runs in background";
```
