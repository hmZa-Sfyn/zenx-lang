# Structs and Methods

ZX uses Go-style structs and receiver-based methods. There are no classes —
just data (structs) and behaviour (methods attached to structs).

---

## Defining a struct

Use `type Name struct { }` or `struct Name { }`:

```perl
type Point struct {
    x: int;
    y: int;
}

type Person struct {
    name: str;
    age:  int;
    email: str;
}

type Rect struct {
    x:      int;
    y:      int;
    width:  int;
    height: int;
}
```

Fields are separated by `;` or `,`.

---

## Creating instances

**Stack-allocated** (value type):

```perl
my origin = Point{x: 0, y: 0};
my p      = Point{x: 3, y: 4};
```

**Heap-allocated** (pointer) — prefix with `&` or use `@`:

```perl
my p = &Point{x: 10, y: 20};    // returns *Point
my r = &Rect{x: 0, y: 0, width: 100, height: 50};
```

---

## Accessing fields

Use `.` for value types and `->` for pointer types:

```perl
// value
my p = Point{x: 5, y: 10};
say p.x;         // 5
say p.y;         // 10

// pointer
my p = &Point{x: 5, y: 10};
say p->x;        // 5
say p->y;        // 10
```

ZX accepts `.` on pointers too (with a style warning), but `->` is idiomatic.

---

## Modifying fields

```perl
my p = &Point{x: 0, y: 0};
p->x = 10;
p->y = 20;
p->x += 5;
```

---

## Methods

Methods are declared at the top level with a **receiver** before the function name.
The receiver type can be a value or a pointer:

```perl
// pointer receiver — can modify the struct
fn (p *Point) move(dx: int, dy: int) {
    p->x += dx;
    p->y += dy;
}

// value receiver — read-only copy
fn (p Point) length_squared() -> int {
    return p.x * p.x + p.y * p.y;
}

fn (p *Point) to_str() -> str {
    return f"({p->x}, {p->y})";
}
```

**Calling methods:**

```perl
my pt = &Point{x: 3, y: 4};
say pt->to_str();              // (3, 4)
say pt->length_squared();      // 25
pt->move(1, -1);
say pt->to_str();              // (4, 3)
```

---

## Full example — a linked list node

```perl
type Node struct {
    value: int;
    next:  *Node;
}

fn (n *Node) append(val: int) {
    my cur = n;
    while cur->next != nil {
        cur = cur->next;
    }
    cur->next = &Node{value: val, next: nil};
}

fn (n *Node) print_all() {
    my cur = n;
    while cur != nil {
        printf("%lld ", cur->value);
        cur = cur->next;
    }
    say "";
}

my list = &Node{value: 1, next: nil};
list->append(2);
list->append(3);
list->append(4);
list->print_all();    // 1 2 3 4
```

---

## Full example — a 2D vector

```perl
type Vec2 struct {
    x: float;
    y: float;
}

fn (v *Vec2) add(other: *Vec2) -> Vec2 {
    return Vec2{x: v->x + other->x, y: v->y + other->y};
}

fn (v *Vec2) scale(factor: float) {
    v->x = v->x * factor;
    v->y = v->y * factor;
}

fn (v *Vec2) magnitude() -> float {
    return sqrt(v->x * v->x + v->y * v->y);
}

fn (v *Vec2) to_str() -> str {
    return f"Vec2({v->x}, {v->y})";
}

my a = &Vec2{x: 3.0, y: 4.0};
say a->magnitude();            // 5
say a->to_str();               // Vec2(3, 4)

a->scale(2.0);
say a->to_str();               // Vec2(6, 8)
```

---

## Nested structs

```perl
type Address struct {
    street: str;
    city:   str;
    zip:    str;
}

type User struct {
    name:    str;
    age:     int;
    address: Address;
}

my u = User{
    name: "Alice",
    age: 30,
    address: Address{
        street: "123 Main St",
        city: "Springfield",
        zip: "12345",
    },
};

say u.name;
say u.address.city;
```

---

## Passing structs to functions

By value (copy):

```perl
fn print_point(p: Point) {
    printf("(%lld, %lld)\n", p.x, p.y);
}
```

By pointer (reference — can modify):

```perl
fn reset(p: *Point) {
    p->x = 0;
    p->y = 0;
}
```

---

## sizeof on structs

```perl
say sizeof(Point);     // size in bytes
say sizeof(Person);
```

---

## Naming conventions

| Thing | Convention | Example |
|---|---|---|
| Struct type | PascalCase | `type HttpServer struct` |
| Field name | snake_case | `request_count: int` |
| Method name | snake_case | `fn (s *Server) handle_request()` |
| Receiver name | short abbreviation | `fn (s *Server)`, `fn (p *Point)` |
