# pgx-helpers

Various helpers for [`jackc/pgx`](https://github.com/jackc/pgx) PostgreSQL driver for Go.

## Versions

- v3 is compatible with pgx 3.x+
- v4 is compatible with pgx 4.x+

## Scan row into struct

Unfortunately this helper can not work in `pgx v4` because of the interface changes. See [pgx issue discussion](https://github.com/jackc/pgx/issues/627) for details.

## Scan rows into structs list

```go
package main

import (
	"time"

	"github.com/jackc/pgx"
	pgxHelpers "github.com/vgarvardt/pgx-helpers"
)

type MyEntity struct {
    ID        string    `db:"id"`
    CreatedAt time.Time `db:"created_at"`
    SomeData  string    `db:"some_data"`
}

conn, _ := pgx.Connect(pgx.ConnConfig{...})

var results []*MyEntity
rows, _ := conn.Query("SELECT * FROM my_entity WHERE id = $1", someID)
defer rows.Close()
pgxHelpers.ScanStructs(rows, func() interface{} {
    return new(MyEntity)
}, func(r interface{}) {
    results = append(results, r.(*MyEntity))
})
```
