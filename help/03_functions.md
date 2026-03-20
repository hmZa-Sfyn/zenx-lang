# Functions

---

## Basic declaration

```perl
fn greet(name: str) {
    say f"Hello, {name}!";
}

fn add(a: int, b: int) -> int {
    return a + b;
}

fn pi() -> float {
    return 3.14159;
}
```

`fn` and `sub` are identical — use whichever reads better.

```perl
sub cleanup(path: str) {
    say f"Cleaning up {path}";
}
```

---

## Return type

Specify the return type after `->`. If omitted the function returns nothing (`void`).

```perl
fn square(n: int) -> int {
    return n * n;
}

fn is_even(n: int) -> bool {
    return n % 2 == 0;
}

fn describe(x: int) -> str {
    return x > 0 ? "positive" : "non-positive";
}
```

---

## Multiple parameters

```perl
fn clamp(val: int, lo: int, hi: int) -> int {
    if val < lo { return lo; }
    if val > hi { return hi; }
    return val;
}
```

---

## Default parameters

```perl
fn greet(name: str, greeting: str = "Hello") {
    say f"{greeting}, {name}!";
}

greet("Alice");              // Hello, Alice!
greet("Bob", "Hiya");        // Hiya, Bob!
```

---

## Variadic functions

Use `...` to accept any number of trailing arguments (passed through to C as `va_list`):

```perl
extern fn printf(fmt: str, ...) -> int;

fn log(msg: str, ...) {
    printf("[LOG] ");
    printf(msg);
}
```

---

## Recursion

```perl
fn factorial(n: int) -> int {
    if n <= 1 { return 1; }
    return n * factorial(n - 1);
}

fn fib(n: int) -> int {
    if n <= 1 { return n; }
    return fib(n - 1) + fib(n - 2);
}
```

---

## Calling functions

```perl
let result = add(3, 4);
let f = factorial(10);
greet("World");
```

---

## Functions as first-class values

Functions can be stored in variables and passed around:

```perl
fn double(n: int) -> int { return n * 2; }

let f = double;
say f(5);          // 10
```

---

## Pipe operator `|>`

Chain function calls left to right. Each step receives the previous result
as its first argument:

```perl
let result = 3 |> double |> square;
// equivalent to: square(double(3))
```

With bang macros:

```perl
let result = 5 |> times2! |> squared!;
// = squared!(times2!(5)) = 100
```

---

## Inline functions (local fns)

Functions can be declared inside other functions:

```perl
fn process(data: str) -> int {
    fn helper(x: int) -> int {
        return x * 2;
    }
    return helper(len!(data));
}
```

---

## extern functions

Declare C functions from external libraries:

```perl
extern fn printf(fmt: str, ...) -> int;
extern fn strlen(s: str) -> int;
extern fn malloc(size: int) -> any;
extern fn free(ptr: any);
extern fn atoi(s: str) -> int;
extern fn sqrt(x: float) -> float;
```

Then call them like normal functions:

```perl
printf("Hello, %s!\n", "world");
let n = atoi("42");
let root = sqrt(2.0);
```

---

## Annotations

Add metadata to functions:

```perl
@test
fn test_add() {
    assert add(2, 3) == 5;
}

@test
@args={"n": 5}
@expect=25
fn test_square(n: int) -> int {
    return n * n;
}

@test
@ignore
fn test_wip() {
    // skipped
}

@deprecated
fn old_api() { }

@inline
fn hot_path(x: int) -> int { return x * 2; }
```

Run tests with: `./zxc test myfile.zx`
