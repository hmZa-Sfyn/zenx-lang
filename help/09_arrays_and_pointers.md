# Arrays and Pointers

---

## Fixed-size arrays

Declare with `[N]type`:

```perl
my nums: [5]int    = [1, 2, 3, 4, 5];
my names: [3]str   = ["Alice", "Bob", "Charlie"];
my flags: [4]bool  = [true, false, true, false];
```

### Indexing

Zero-based, like C:

```perl
say nums[0];       // 1
say nums[4];       // 5
nums[2] = 99;
```

### Iterating

```perl
for i in 0..5 {
    printf("nums[%lld] = %lld\n", i, nums[i]);
}
```

### Size of an array

```perl
let n = count_of!(nums);       // 5
let sz = sizeof(nums);         // 40 (5 * 8 bytes)
```

---

## Array literals

```perl
my primes: [5]int = [2, 3, 5, 7, 11];
my empty:  [0]int = [];
```

---

## Pointers and references

ZX uses `*T` (or `ref T`) to declare pointer types:

```perl
my n: int = 42;
my p: *int = &n;        // take address of n

say *p;                 // dereference: 42  (use ^ or * prefix)
say ^p;                 // same: 42
*p = 100;               // modify through pointer
say n;                  // 100
```

The `^` prefix operator dereferences a pointer:

```perl
my val = ^p;            // dereference
```

The `&` prefix operator takes the address:

```perl
my ptr = &someVar;
```

---

## Heap allocation

```perl
my p: *int = alloc!(sizeof(int));    // allocate one int
^p = 42;
printf("heap value: %lld\n", ^p);
free!(p);
```

Allocate a struct on the heap using `&StructName{...}`:

```perl
my pt = &Point{x: 10, y: 20};    // heap-allocated *Point
say pt->x;                        // 10
```

Or manually:

```perl
my pt: *Point = alloc!(sizeof(Point));
pt->x = 10;
pt->y = 20;
free!(pt);
```

---

## Pointer arithmetic

Use standard C arithmetic through casts:

```perl
my buf: *char = alloc!(256);
my offset: *char = buf + 10;     // offset by 10 bytes
```

---

## Passing arrays to functions

Arrays decay to pointers when passed:

```perl
fn sum_array(arr: *int, n: int) -> int {
    my total = 0;
    for i in 0..n {
        total += arr[i];
    }
    return total;
}

my nums: [5]int = [1, 2, 3, 4, 5];
let total = sum_array(nums, 5);
say total;    // 15
```

---

## Null pointers

Use `nil` for null:

```perl
my p: *int = nil;

if p == nil {
    say "pointer is null";
}

// or with macros:
if is_nil!(p) {
    say "null!";
}
if not_nil!(p) {
    say p->value;
}
```

---

## Memory operations

```perl
let buf = alloc!(1024);              // malloc
let buf = zalloc!(1024);             // calloc (zeroed)
memset!(buf, 0, 1024);              // fill with zeros
memcpy!(dest, src, 256);            // copy 256 bytes
free!(buf);                          // free
```

---

## ref keyword (alias for pointer type)

`ref T` and `*T` are identical:

```perl
fn modify(p: ref int) {
    ^p = 99;
}

my x = 42;
modify(&x);
say x;         // 99
```

---

## Pointer to struct — common pattern

```perl
type Node struct {
    value: int;
    next:  *Node;
}

fn make_node(val: int) -> *Node {
    my n = &Node{value: val, next: nil};
    return n;
}

my head = make_node(1);
head->next = make_node(2);
head->next->next = make_node(3);

my cur = head;
while cur != nil {
    printf("%lld ", cur->value);
    cur = cur->next;
}
// 1 2 3
```
