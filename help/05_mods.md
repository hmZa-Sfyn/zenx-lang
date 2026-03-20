# Mod Blocks

`mod` groups related functions under a namespace. Functions inside a mod can
**only** be called through their mod name — calling them by plain name is a
compile error.

---

## Declaring a mod

```perl
mod Math {
    fn square(n: int) -> int {
        return n * n;
    }

    fn cube(n: int) -> int {
        return n * n * n;
    }

    fn abs(n: int) -> int {
        if n < 0 { return -n; }
        return n;
    }
}
```

---

## Calling mod functions

Use `->` or `::` — both work:

```perl
say Math->square(5);      // 25
say Math->cube(3);        // 27
say Math::abs(-10);       // 10
```

Calling without the mod name is a **compile error**:

```perl
say square(5);            // error: "square" is a mod-private function
                          //   hint: call it as Math->square()
```

---

## Mods can contain structs

```perl
mod Geo {
    type Point struct {
        x: int;
        y: int;
    }

    fn make(x: int, y: int) -> Point {
        return Point{x: x, y: y};
    }

    fn distance(a: *Point, b: *Point) -> float {
        let dx = a->x - b->x;
        let dy = a->y - b->y;
        return sqrt(float(dx*dx + dy*dy));
    }
}

my p = Geo->make(3, 4);
```

---

## Mods can access `our` globals

```perl
our request_count: int = 0;

mod Counter {
    fn increment() {
        request_count += 1;
    }

    fn get() -> int {
        return request_count;
    }

    fn reset() {
        request_count = 0;
    }
}

Counter->increment();
Counter->increment();
say Counter->get();       // 2
Counter->reset();
say Counter->get();       // 0
```

---

## Nested mods

```perl
mod Net {
    mod Http {
        fn get(url: str) -> str {
            // ...
            return "";
        }

        fn post(url: str, body: str) -> str {
            // ...
            return "";
        }
    }

    mod Tcp {
        fn connect(host: str, port: int) -> int {
            return 0;
        }
    }
}

Net::Http->get("https://example.com");
Net::Tcp->connect("localhost", 8080);
```

---

## Mods with tests

Functions inside a mod can be annotated with `@test`:

```perl
mod Strings {
    fn reverse(s: str) -> str {
        // ... implementation ...
        return s;
    }

    fn capitalize(s: str) -> str {
        // ... implementation ...
        return s;
    }

    @test
    fn test_reverse() {
        // assert reverse("hello") == "olleh";
        assert 1 == 1;
    }

    @test
    fn test_capitalize() {
        assert 1 == 1;
    }
}
```

Run tests: `./zxc test myfile.zx`

---

## Full example — a Logger mod

```perl
our log_level: int = 1;   // 0=debug 1=info 2=warn 3=error

mod Logger {
    fn debug(msg: str) {
        if log_level <= 0 {
            printf("[DEBUG] %s\n", msg);
        }
    }

    fn info(msg: str) {
        if log_level <= 1 {
            printf("[INFO]  %s\n", msg);
        }
    }

    fn warn(msg: str) {
        if log_level <= 2 {
            printf("[WARN]  %s\n", msg);
        }
    }

    fn error(msg: str) {
        printf("[ERROR] %s\n", msg);
    }

    fn set_level(level: int) {
        log_level = level;
    }
}

Logger->info("Server starting...");
Logger->warn("Low memory");
Logger->error("Connection failed");
Logger->set_level(0);
Logger->debug("Verbose mode on");
```

---

## Full example — a Config mod

```perl
our _config_host: str  = "localhost";
our _config_port: int  = 8080;
our _config_debug: bool = false;

mod Config {
    fn host() -> str  { return _config_host; }
    fn port() -> int  { return _config_port; }
    fn debug() -> bool { return _config_debug; }

    fn set_host(h: str)  { _config_host = h; }
    fn set_port(p: int)  { _config_port = p; }
    fn enable_debug()    { _config_debug = true; }

    fn print_all() {
        printf("host  = %s\n", _config_host);
        printf("port  = %lld\n", _config_port);
        printf("debug = %d\n", _config_debug);
    }
}

Config->set_host("0.0.0.0");
Config->set_port(3000);
Config->enable_debug();
Config->print_all();
```

---

## Differences from classes

| Feature | OOP class | ZX mod |
|---|---|---|
| Encapsulation | via `private`/`public` | all functions are mod-only by default |
| Instantiation | `new MyClass()` | mods are not instantiated — they are singletons |
| State | instance fields | `our` globals (explicit) |
| Inheritance | `extends` | not supported — compose instead |
| Calling | `obj.method()` | `Mod->fn()` or `Mod::fn()` |
| Methods on data | class methods | use structs + `fn (recv *Type) method()` |

For **data + behaviour together**, use a struct with methods.
For **namespaced utility functions**, use a mod.
