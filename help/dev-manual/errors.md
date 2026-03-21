# ZXC Diagnostic Reference

All codes emitted by `zxc`. Errors abort compilation; warnings and style notes do not.

---

## Legend

| Prefix | Kind | Stops build? |
|--------|------|--------------|
| `E` | **Error** — type / semantic | ✅ yes |
| `W` | **Warning** — likely bug or bad style | ❌ no |
| `EI` | **Import error** | ✅ yes |
| `EM` | **Macro error** | ✅ yes |
| `EP` | **Privacy error** | ✅ yes |
| `EOB` | **Out-of-bounds error** | ✅ yes |
| `WE` | **Exhaustiveness warning** | ❌ no |
| `WP` | **Performance warning** | ❌ no |
| `WS` | **String-safety warning** | ❌ no |
| `WN` | **Negation warning** | ❌ no |

---

## Errors — `E`

| Code | Description |
|------|-------------|
| `E01` | Struct defined more than once |
| `E02` | Duplicate field in struct |
| `E03` | Function defined more than once |
| `E04` | Duplicate parameter name in function or method |
| `E07` | `break` used outside a loop |
| `E08` | `continue` used outside a loop |
| `E09` | Variable already declared in this scope |
| `E11` | Type mismatch — cannot assign RHS type to LHS type |
| `E12` | Variable cannot have type `void` |
| `E13` | `const` declared without an initializer |
| `E14` | `return` used outside a function |
| `E15` | Return with no value in a non-void function |
| `E16` | `void` function returns a value |
| `E17` | Return type mismatch — expected vs got |
| `E18` | Condition has type `void` (if / elif / ternary / unless) |
| `E20` | For-range start is not an integer |
| `E21` | For-range end is not an integer |
| `E22` | Assignment to a `const` variable |
| `E23` | Assignment to a function name |
| `E24` | Left side of assignment is a literal |
| `E25` | Type mismatch on assignment |
| `E26` | Compound operator (`+=` etc.) on a non-numeric type |
| `E27` | Undefined name |
| `E28` | Invalid cast between incompatible types |
| `E29` | Dereference (`^`) applied to a non-ref type |
| `E30` | Comparison of incompatible types with `==` / `!=` |
| `E31` | Comparison operator on non-numeric types |
| `E32` | `+` used to concatenate strings (not supported) |
| `E33` | Arithmetic operator on non-numeric operand |
| `E34` | Division by zero (integer literal) |
| `E35` | Modulo `%` on float operands |
| `E36` | Bitwise operator on non-integer operand |
| `E37` | Logical NOT `!` on a non-truthy type |
| `E38` | Unary minus on a non-numeric type |
| `E39` | Bitwise NOT `~` on a non-integer type |
| `E40` | Wrong argument type for `extern` function |
| `E41` | Wrong number of arguments |
| `E42` | Wrong number of arguments for user-defined or mod function |
| `E43` | Wrong argument type for function / mod function / std function |
| `E44` | Call to undefined function |
| `E45` | Calling a variable as if it were a function |
| `E46` | Indexing into a non-array, non-string type |
| `E47` | Array index is not an integer |
| `E48` | Field access on a non-struct type |
| `E49` | Struct type not defined (field access) |
| `E50` | Struct has no such field |
| `E51` | Undefined struct in struct literal |
| `E52` | Struct literal sets a non-existent field |
| `E53` | Field set more than once in struct literal |
| `E54` | Wrong type for struct field in literal |
| `E55` | Array element type mismatch |
| `E56` | Unknown type name |
| `E57` | Method defined on an undeclared struct |
| `E58` | Method defined more than once |
| `E59` | Struct has no such method |
| `E60` | `defer` used outside a function |
| `E71` | `assert` condition has type `void` |
| `E72` | `exit` code is not an integer |
| `E73` | `nil` assigned to a non-ref variable |
| `E74` | Duplicate `match` arm for the same value |
| `E75` | Assignment to an undeclared variable |
| `E76` | Left side of assignment is a function call result |
| `E77` | Shift by a negative amount (undefined behaviour) |
| `E78` | Method call on `nil` |
| `E80` | Mod block name used as a value |
| `E81` | Mod-private function called without its module prefix |
| `E82` | Call to an unknown module |
| `E83` | Module has no such function |
| `E94` | Parameter has type `void` |
| `EOB` | Compile-time array index out of bounds |

---

## Warnings — `W`

| Code | Description |
|------|-------------|
| `W01` | Variable or parameter shadows an outer variable |
| `W02` | `const` name is not UPPER_CASE |
| `W03` | Division by `0.0` — produces `Inf` or `NaN` |
| `W10` | Function name is a C keyword — will be prefixed `__zx_` |
| `W20` | Struct has no fields |
| `W21` | Struct has more than 32 fields |
| `W22` | `extern` re-declared — previous declaration shadowed |
| `W23` | Function has more than 8 parameters |
| `W30` | Variable declared but never used |
| `W31` | Variable name is excessively long (> 50 chars) |
| `W32` | String literal is very long (> 4096 bytes) |
| `W40` | Non-void function may not return a value on all paths |
| `W50` | `if` / `unless` condition is always true or always false |
| `W51` | `while(true)` loop has no `break` |
| `W52` | `assert` condition is always true |
| `W53` | For-range will never execute (start ≥ end) |
| `W54` | Duplicate wildcard arm in `match` |
| `W60` | Return value of a call is silently discarded |
| `W61` | Ternary branches have different types |
| `W62` | Negative array index literal |
| `W63` | Redundant cast — value is already of that type |
| `W64` | Comparing or assigning a variable to itself |
| `W65` | Non-truthy operand to `&&` / `\|\|` |
| `W66` | Shift amount ≥ 64 — undefined behaviour |
| `W70` | Struct field not set in literal — zero-initialized |
| `W80` | Duplicate import |
| `W90` | Field name is the same as its containing struct |
| `W91` | Private function is defined but never called (dead code) |
| `W92` | Recursive function has no obvious base case |
| `W93` | Function body is empty but return type is non-void |
| `W95` | `while(false)` loop will never execute |
| `W96` | `repeat` count is zero or negative |
| `W97` | Single-character variable name (except `i j k n x y z`) |
| `W98` | `bool` variable initialized with an integer literal |
| `W99` | Both branches of `if`/`else` are structurally identical |
| `WE1` | `match` has no wildcard arm — unmatched values fall through |
| `WN1` | Double negation `!!` — use the expression directly |
| `WP1` | For-range iterates more than 10,000,000 times |
| `WS1` | Strings compared with `==` — compares pointers, not contents |

---

## Import Errors — `EI`

| Code | Description |
|------|-------------|
| `EI01` | C header import path is empty |
| `EI02` | Unknown stdlib module name |
| `EI03` | Stdlib file import missing env prefix (compiler bug) |
| `EI04` | Import requires at least one path segment |
| `EI05` | Invalid path segment in import (not a valid identifier) |
| `EI07` | Local import requires at least one path segment |
| `EI08` | Invalid path segment in local import |
| `EI10` | Local import path could not be resolved |
| `EI20` | Import has no resolved file path (compiler bug) |
| `EI21` | Cannot read imported file |
| `EI22` | Tokenization failed in imported file |
| `EI23` | Parse errors in imported file |
| `EI24` | Imported file has no mod with the requested name |

---

## Macro Errors — `EM`

| Code | Description |
|------|-------------|
| `EM01` | Macro name is a C keyword |
| `EM03` | Macro defined more than once |
| `EM04` | Macro name collides with a function name |
| `EM05` | Duplicate parameter name in macro |
| `EM07` | Call to undefined macro |
| `EM08` | Wrong number of arguments to macro |
| `EM09` | Wrong argument type for macro parameter |

---

## Privacy Errors — `EP`

> Triggered when a declaration marked `priv` is accessed from another file.

| Code | Description |
|------|-------------|
| `EP1` | Calling a `priv` function from another file |
| `EP2` | Calling a `priv` mod function from another file |
| `EP3` | Calling a `priv` macro from another file |
| `EP4` | Accessing fields of a `priv` struct from another file |
| `EP5` | Constructing a `priv` struct from another file |
| `EP6` | Importing a `priv` mod block |

---

## Quick-fix cheatsheet

```
E27  undefined name        → declare: let x = ...   or   fn x() { }
E32  string concat         → use f"{a}{b}"  or  str_cat(a, b)
E34  division by zero      → guard: if d != 0 { x / d }
E73  nil to non-ref        → declare as ref<T>
E94  void param            → use any, int, str, or a struct type
W01  shadowing             → rename the inner variable
W40  missing return        → add return <value>  or  change to -> void
WS1  string == compare     → use str_eq!(a, b)
EP1  priv fn access        → remove priv in the declaring file
```