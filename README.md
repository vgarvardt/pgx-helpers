# pgx-helpers

Various helpers for [`jackc/pgx`](https://github.com/jackc/pgx) PostgreSQL driver for Go.

## Versions

- v3 is compatible with pgx 3.x+
- v4 is compatible with pgx 4.x+

## Helpers

- [Scan row into struct](#scan-row-into-struct)
- [Scan rows into structs list](#scan-rows-into-structs-list)

### Scan row into struct

Unfortunately `conn.QueryRow()`/`pgx.Row` cannot be used with `ScanStruct()` in `pgx/v4` because of the interface changes.
See [pgx issue discussion](https://github.com/jackc/pgx/issues/627) for details.
Nevertheless, it is still possible to scan single row into struct - function works with `conn.QueryRow()`/`pgx.Rows`,
and it is up to a caller to ensure that only one row is selected, e.g. by adding `LIMIT 1` to a query or selecting by
primary or unique key.

```go
package main

import (
    "context"
    "log"
    "os"
    "time"

    "github.com/jackc/pgx/v4"
    pgxHelpers "github.com/vgarvardt/pgx-helpers/v4"
)

type MyEntity struct {
    ID        string    `db:"id"`
    CreatedAt time.Time `db:"created_at"`
    SomeData  string    `db:"some_data"`
}

func main() {
    conn, err := pgx.Connect(context.Background(), os.Getenv("PG_DSN"))
    if err != nil {
        log.Fatal(err)
    }

    rows, err := conn.Query(context.Background(), "SELECT * FROM my_entity WHERE id = $1", someID)
    if err != nil {
        log.Fatal(err)
    }
    defer rows.Close()

    result := new(MyEntity)
    err = pgxHelpers.ScanStruct(rows, result)
    if err != nil {
        log.Fatal(err)
    }
}
```

### Scan rows into structs list

```go
package main

import (
    "context"
    "log"
    "os"
    "time"

    "github.com/jackc/pgx/v4"
    pgxHelpers "github.com/vgarvardt/pgx-helpers/v4"
)

type MyEntity struct {
    ID        string    `db:"id"`
    CreatedAt time.Time `db:"created_at"`
    SomeData  string    `db:"some_data"`
}

func main() {
    conn, err := pgx.Connect(context.Background(), os.Getenv("PG_DSN"))
    if err != nil {
        log.Fatal(err)
    }

    rows, err := conn.Query(context.Background(), "SELECT * FROM my_entity WHERE created_at >= $1", time.Now().Add(-time.Hour))
    if err != nil {
        log.Fatal(err)
    }
    defer rows.Close()

    var results []*MyEntity
    err = pgxHelpers.ScanStructs(rows, func() interface{} {
        return new(MyEntity)
    }, func(r interface{}) {
        results = append(results, r.(*MyEntity))
    })
    if err != nil {
        log.Fatal(err)
    }
}
```
