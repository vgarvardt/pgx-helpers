package pgxhelpers

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v4"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type testEntity struct {
	ID        string    `db:"id"`
	CreatedAt time.Time `db:"created_at"`
	SomeData  string    `db:"some_data"`
}

type testMissingField struct {
	ID        string    `db:"id"`
	CreatedAt time.Time `db:"created_at"`
}

func TestScanStruct(t *testing.T) {
	connString := initDB(t)

	conn, err := pgx.Connect(context.Background(), connString)
	require.NoError(t, err)
	defer func() {
		err := conn.Close(context.Background())
		assert.NoError(t, err)
	}()

	e1, e2 := prepareData(t, conn)

	rows := selectRows(t, conn, e1.ID, e2.ID)
	result := new(testEntity)
	err = ScanStruct(rows, result)
	require.NoError(t, err)

	assert.Equal(t, e1.ID, result.ID)
	assert.Equal(t, e1.SomeData, result.SomeData)
	// compare unit timestamp to avoid milliseconds diff
	assert.Equal(t, e1.CreatedAt.Unix(), result.CreatedAt.Unix())

	// test some fail cases
	rowsEmpty := selectRows(t, conn, "foo", "bar")
	resultEmpty := new(testEntity)
	err = ScanStruct(rowsEmpty, resultEmpty)
	require.Error(t, err)
	require.Equal(t, err, pgx.ErrNoRows)
}

func TestScanStructs(t *testing.T) {
	connString := initDB(t)

	conn, err := pgx.Connect(context.Background(), connString)
	require.NoError(t, err)
	defer func() {
		err := conn.Close(context.Background())
		assert.NoError(t, err)
	}()

	e1, e2 := prepareData(t, conn)

	var result []*testEntity
	rows := selectRows(t, conn, e1.ID, e2.ID)
	err = ScanStructs(rows, func() interface{} {
		return new(testEntity)
	}, func(r interface{}) {
		result = append(result, r.(*testEntity))
	})
	require.NoError(t, err)
	require.Len(t, result, 2)
	rows.Close()

	assert.Equal(t, e1.ID, result[0].ID)
	assert.Equal(t, e1.SomeData, result[0].SomeData)
	// compare unit timestamp to avoid milliseconds diff
	assert.Equal(t, e1.CreatedAt.Unix(), result[0].CreatedAt.Unix())

	assert.Equal(t, e2.ID, result[1].ID)
	assert.Equal(t, e2.SomeData, result[1].SomeData)
	// compare unit timestamp to avoid milliseconds diff
	assert.Equal(t, e2.CreatedAt.Unix(), result[1].CreatedAt.Unix())

	// test some fail cases
	rowsFailStruct := selectRows(t, conn, e1.ID, e2.ID)
	err = ScanStructs(rowsFailStruct, func() interface{} {
		return testEntity{}
	}, func(r interface{}) {
	})
	require.Error(t, err)
	assert.Equal(t, "must return a pointer to a new struct, not a value, to ScanStructs destination", err.Error())
	rowsFailStruct.Close()

	rowsFailNil := selectRows(t, conn, e1.ID, e2.ID)
	err = ScanStructs(rowsFailNil, func() interface{} {
		var isNil *testEntity
		return isNil
	}, func(r interface{}) {
	})
	require.Error(t, err)
	assert.Equal(t, "nil pointer returned to ScanStructs destination", err.Error())
	rowsFailNil.Close()

	rowsFailMissing := selectRows(t, conn, e1.ID, e2.ID)
	err = ScanStructs(rowsFailMissing, func() interface{} {
		return new(testMissingField)
	}, func(r interface{}) {
	})
	require.Error(t, err)
	assert.Equal(t, `missing column "some_data" in dest *pgxhelpers.testMissingField`, err.Error())
	rowsFailMissing.Close()
}

func prepareData(t *testing.T, conn *pgx.Conn) (testEntity, testEntity) {
	t.Helper()

	e1 := testEntity{
		ID:        "scan-structs-1-" + time.Now().String(),
		CreatedAt: time.Now(),
		SomeData:  "foo bar baz",
	}
	e2 := testEntity{
		ID:        "scan-structs-2-" + time.Now().String(),
		CreatedAt: time.Now().Add(time.Hour),
		SomeData:  "foo bar baz qux",
	}

	_, err := conn.Exec(
		context.Background(),
		"INSERT INTO test (id, some_data, created_at) VALUES ($1, $2, $3), ($4, $5, $6)",
		e1.ID, e1.SomeData, e1.CreatedAt,
		e2.ID, e2.SomeData, e2.CreatedAt,
	)
	require.NoError(t, err)

	return e1, e2
}

func selectRows(t *testing.T, conn *pgx.Conn, id1, id2 string) pgx.Rows {
	t.Helper()

	rows, err := conn.Query(context.Background(), "SELECT * FROM test WHERE id IN ($1, $2) ORDER BY id ASC", id1, id2)
	require.NoError(t, err)

	return rows
}

func initDB(t *testing.T) string {
	t.Helper()

	// connString := "postgres://test:test@localhost:32771/test?sslmode=disable"
	connString := os.Getenv("TEST_POSTGRES")
	require.NotEmpty(t, connString)

	conn, err := pgx.Connect(context.Background(), connString)
	require.NoError(t, err)

	defer func() {
		err := conn.Close(context.Background())
		assert.NoError(t, err)
	}()

	_, err = conn.Exec(context.Background(), `
CREATE TABLE IF NOT EXISTS "test"
(
    id         text PRIMARY KEY,
    some_data  text        not null,
    created_at timestamptz not null
);
`)

	require.NoError(t, err)

	return connString
}
