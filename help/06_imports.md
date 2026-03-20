# Imports

---

## Raw C headers

Include a C system header directly:

```perl
use "stdio.h"
use "stdlib.h"
use "string.h"
use "math.h"
use "pthread.h"
```

This emits `#include <header.h>` in the generated C.

---

## Built-in ZX stdlib modules

Inline modules — no file is loaded, symbols become available immediately:

```perl
use std::str      // str_len, str_cat, str_find, str_eq, is_alpha ...
use std::math     // sqrt, pow, sin, cos, floor, ceil, fmod ...
use std::io       // read_line, write_line ...
use std::conv     // int_to_str, float_to_str, str_to_int ...
use std::sys      // run, sleep, getenv, setenv, env_var ...
use std::fs       // read_file, write_file, exists, delete ...
use std::cmd      // exec, capture, shell ...
use std::mem      // alloc, realloc, free, memcpy ...
use std::time     // now, sleep_ms, timestamp ...
use std::net      // tcp_connect, tcp_listen, tcp_accept ...
```

---

## File imports from stdlib — `std/...`

Load a `.zx` file from `$ZENX_STD_PATH/`:

```perl
// Import everything from $ZENX_STD_PATH/net/socket.zx
import std/net/socket

// Import everything from $ZENX_STD_PATH/crypto/sha256.zx
import std/crypto/sha256

// Import only the mod `HttpServer` from $ZENX_STD_PATH/net/http.zx
// Compile error if HttpServer doesn't exist in that file!
import std/net/http (HttpServer)

// Deep path — $ZENX_STD_PATH/data/formats/json.zx
import std/data/formats/json (JsonParser)
```

---

## File imports from user libraries — `usr/...`

Load a `.zx` file from `$ZENX_USR_PATH/`:

```perl
// Import everything from $ZENX_USR_PATH/mylib/utils.zx
import usr/mylib/utils

// Import only mod Postgres from $ZENX_USR_PATH/db/postgres.zx
import usr/db/postgres (Postgres)
```

---

## Local file imports — `_/...`

Import relative to the current file's directory:

```perl
// _ = current directory
import _/utils             // ./utils.zx — import everything
import _/utils (Helper)    // ./utils.zx — import only mod Helper
import _/net/client        // ./net/client.zx — import everything
import _/net/client (Tcp)  // ./net/client.zx — import only mod Tcp
```

---

## Parent directory imports — `__/...`

```perl
// __ = one directory up  (../)
import __/shared           // ../shared.zx — import everything
import __/shared (Config)  // ../shared.zx — import only mod Config
import __/lib/crypto       // ../lib/crypto.zx — import everything
```

---

## Grandparent and beyond

Each additional underscore goes one level higher:

```perl
// ___ = two levels up  (../../)
import ___/common/types          // ../../common/types.zx

// ____ = three levels up  (../../../)
import ____/platform/os (PosixOs)
```

---

## How `(ModName)` works

When you write `import _/logger (Logger)`, ZX:

1. Reads `./logger.zx` and parses it
2. Looks for a `mod Logger { }` block inside it
3. If found — merges only that mod into the current file
4. If NOT found — **compile error** listing the mods that do exist

```perl
// logger.zx contains:
//   mod Logger { fn info() { } fn warn() { } }
//   mod InternalHelper { fn format() { } }

import _/logger (Logger)

Logger->info("hello");          // works
InternalHelper->format();       // error: InternalHelper was not imported
```

Without `(ModName)` — everything is merged:

```perl
import _/logger

Logger->info("hello");          // works
InternalHelper->format();       // also works
```

---

## Errors caught at compile time

```perl
import std/net/http (Missing)    // error: mod "Missing" not found in file
                                 //        available: HttpServer, HttpClient

import _/nonexistent             // error: cannot read ./nonexistent.zx: no such file

import __                        // error: missing path segment after __
import _/                        // error: expected a name after /
import std/                      // error: expected a path segment after std/
use std::fake                    // error: unknown stdlib module "std::fake"
import _/a ()                    // error: expected a mod name inside ()
```

---

## Quick reference

| Syntax | Resolves to | Imports |
|---|---|---|
| `use "stdio.h"` | C `#include` | C header |
| `use std::str` | built-in ZX module | all symbols |
| `import std/a/b` | `$ZENX_STD_PATH/a/b.zx` | everything |
| `import std/a/b (M)` | `$ZENX_STD_PATH/a/b.zx` | only mod `M` |
| `import usr/a/b` | `$ZENX_USR_PATH/a/b.zx` | everything |
| `import usr/a/b (M)` | `$ZENX_USR_PATH/a/b.zx` | only mod `M` |
| `import _/a` | `./a.zx` | everything |
| `import _/a (M)` | `./a.zx` | only mod `M` |
| `import __/a` | `../a.zx` | everything |
| `import __/a (M)` | `../a.zx` | only mod `M` |
| `import ___/a` | `../../a.zx` | everything |
