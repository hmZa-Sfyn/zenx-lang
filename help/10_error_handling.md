# Error Handling

---

## try / catch / finally

Maps to errno-based C error handling. `errno` is reset before the `try` block;
if it is non-zero after, the `catch` block runs.

```perl
try {
    let f = fopen("data.txt", "r");
    // ... do work ...
} catch (err) {
    printf("Error code: %d\n", err);
} finally {
    say "Cleanup always runs.";
}
```

Both `catch` and `finally` are optional:

```perl
try {
    risky();
} finally {
    cleanup();
}

try {
    risky();
} catch (err) {
    printf("Failed: %d\n", err);
}
```

---

## assert

Halts the program with a message if the condition is false.
Use for invariants and preconditions.

```perl
assert x > 0;
assert x > 0, "x must be positive";
assert len!(name) > 0, "name cannot be empty";
assert ptr != nil, "pointer must not be null";
```

---

## die / throw / raise

All three are identical — print a message to stderr and exit with code 1.
Choose the one that reads most naturally:

```perl
die "something went catastrophically wrong";
throw "invalid state — this should never happen";
raise "file not found: config.ini";
```

---

## panic! (bang macro)

Like `die` but also calls `abort()`, which generates a core dump on most systems:

```perl
panic!("out of memory");
panic!(f"unexpected value: {val}");
```

---

## exit_err! (bang macro)

Print to stderr and exit with code 1:

```perl
exit_err!("configuration file missing");
exit_err!(f"cannot connect to {host}:{port}");
```

---

## ok! and try! (bang macros)

```perl
// ok! — assert non-zero/non-null, abort otherwise
let fd = open("file.txt", 0);
ok!(fd);                       // abort if fd == 0

// try! — return the value if non-zero, abort otherwise
let conn = try!(tcp_connect("localhost", 8080));
```

---

## unreachable! and todo!

```perl
match direction {
    0 => { move_north(); }
    1 => { move_south(); }
    _ => { unreachable!(); }   // program logic guarantees this never runs
}

fn process_payment(method: str) {
    if str_eq!(method, "card") {
        charge_card();
    } elif str_eq!(method, "cash") {
        todo!("cash payments not implemented yet");
    }
}
```

---

## Result-style pattern (manual)

ZX doesn't have a built-in `Result` type, but you can model it with a struct:

```perl
type Result struct {
    ok:    bool;
    value: int;
    error: str;
}

fn divide(a: int, b: int) -> Result {
    if b == 0 {
        return Result{ok: false, value: 0, error: "division by zero"};
    }
    return Result{ok: true, value: a / b, error: ""};
}

let r = divide(10, 2);
if r.ok {
    printf("Result: %lld\n", r.value);
} else {
    printf("Error: %s\n", r.error);
}

let r2 = divide(10, 0);
if !r2.ok {
    printf("Failed: %s\n", r2.error);
}
```

---

## defer for cleanup

`defer` ensures cleanup code always runs even if the block exits early:

```perl
fn process_file(path: str) {
    let f = fopen(path, "r");
    if f == nil {
        die f"cannot open {path}";
    }
    defer fclose(f);        // runs when function exits, no matter what

    // ... read and process ...
    // no need to remember to call fclose
}
```

Multiple defers run in reverse order (last defer runs first):

```perl
fn example() {
    defer say "3. last";    // runs last
    defer say "2. middle";
    defer say "1. first";   // runs first
    say "0. body";
    // output:
    // 0. body
    // 1. first
    // 2. middle
    // 3. last
}
```

---

## Error checking patterns

**Check before use:**

```perl
fn read_config(path: str) -> str {
    if !file_exists(path) {
        die f"config file not found: {path}";
    }
    return readfile!(path);
}
```

**Return early on error:**

```perl
fn connect(host: str, port: int) -> int {
    if str_eq!(host, "") {
        say "Error: empty host";
        return -1;
    }
    if port <= 0 || port > 65535 {
        say "Error: invalid port";
        return -1;
    }
    // ... connect ...
    return 0;
}
```

**Chain with ifNil:**

```perl
let data = read_data()

data
    ifNil: {
        die "failed to read data";
    }
    ifNotNil: {
        process(data);
    }
```
