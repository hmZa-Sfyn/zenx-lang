# Strings

ZX strings are `const char*` under the hood (null-terminated C strings).

---

## String literals

```perl
my s: str = "Hello, world!";
my empty: str = "";
my path: str = "/usr/local/bin";
```

### Escape sequences

| Escape | Meaning |
|---|---|
| `\n` | newline |
| `\t` | tab |
| `\r` | carriage return |
| `\\` | backslash |
| `\"` | double quote |
| `\0` | null byte |
| `\a` | alert/bell |

---

## f-strings (interpolated strings)

Prefix with `f`. Embed any expression inside `{ }`:

```perl
my name = "Alice";
my age  = 30;

say f"Hello, {name}!";
say f"Age: {age}";
say f"2 + 2 = {2 + 2}";
say f"Name length: {len!(name)}";
say f"Upper score: {score * 2}";
```

Nested expressions work too:

```perl
say f"Is even: {age % 2 == 0 ? 'yes' : 'no'}";
```

---

## Multiline strings `@\`...\``

Start with `@\`` and end with `\``. Spans multiple lines freely.
Backticks inside are escaped as `` \` ``.

```perl
my html: str = @`
<html>
  <head><title>My Page</title></head>
  <body>
    <h1>Hello!</h1>
  </body>
</html>`;
```

### Interpolation in multiline strings

Use `${expr}` for expressions:

```perl
my site = "ZX Lang";
my version = "1.0";

my page: str = @`
<html>
  <head><title>${site}</title></head>
  <body>
    <p>Version: ${version}</p>
    <p>2 + 2 = ${2 + 2}</p>
  </body>
</html>`;
```

Use `${ statements }` for statement blocks (stdout is captured):

```perl
my items: [3]str = ["apple", "banana", "cherry"];

my list: str = @`
<ul>
  ${
    for i in 0..3 {
        printf("<li>%s</li>\n", items[i]);
    }
  }
</ul>`;
```

Escape a literal backtick with `` \` ``:

```perl
my code: str = @`use backticks like \` this \` in shell`;
```

---

## Printing strings

```perl
say "Hello!";                    // println to stdout
print "no newline";              // print without newline
println "with newline";          // explicit println
warn "warning message";          // println to stderr
eprint "stderr, no newline";
```

Using printf format:

```perl
printf("%s\n", name);
printf("Name: %s, Age: %lld\n", name, age);
```

---

## String operations (bang macros)

```perl
let n = len!(s);                  // length
let eq = str_eq!(a, b);           // 1 if equal
let ne = str_ne!(a, b);           // 1 if not equal
let has = str_contains!(s, sub);  // 1 if contains
let sw  = str_starts!(s, "http"); // 1 if starts with
let ew  = str_ends!(s, ".zx");    // 1 if ends with
let i   = str_to_int!("42");      // parse to int
let f   = str_to_float!("3.14");  // parse to float
let s   = int_to_str!(42);        // int to string
let s   = float_to_str!(3.14);    // float to string
```

---

## Extern C string functions

After `use "string.h"`:

```perl
extern fn strlen(s: str) -> int;
extern fn strcmp(a: str, b: str) -> int;
extern fn strncmp(a: str, b: str, n: int) -> int;
extern fn strstr(haystack: str, needle: str) -> str;
extern fn strcpy(dst: str, src: str) -> str;
extern fn strcat(dst: str, src: str) -> str;
extern fn strchr(s: str, c: int) -> str;

let len = strlen("hello");        // 5
let cmp = strcmp("a", "b");       // negative
let sub = strstr("hello", "ll");  // pointer to "llo"
```

---

## Reading a string from stdin

```perl
let line = read_line!();           // reads one line, strips newline
printf("You typed: %s\n", line);

// with prompt:
let name = input("What is your name? ");
say f"Hello, {name}!";
```

---

## typeof on strings

```perl
say typeof("hello");      // "str"
```

---

## Char literals

Single characters use single quotes and are stored as `int`:

```perl
my c: char = 'A';             // 65
my nl: char = '\n';           // 10
my tab: char = '\t';          // 9

printf("%c\n", c);            // A
printf("%d\n", int(c));       // 65
```

---

## String indexing

Strings can be indexed to get individual bytes as `char`:

```perl
my s: str = "Hello";
my first = s[0];              // 'H' (char, value 72)
my last  = s[4];              // 'o' (char, value 111)
```

---

## Format specifiers for printf

| Type | Specifier |
|---|---|
| `str` | `%s` |
| `int` | `%lld` |
| `float` | `%g` or `%f` |
| `char` | `%c` |
| `bool` (0/1) | `%d` |
| pointer | `%p` |
