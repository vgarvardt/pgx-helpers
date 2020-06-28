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

func TestScanStructs(t *testing.T) {
	connString := os.Getenv("TEST_POSTGRES")

	initDB(t, connString)

	conn, err := pgx.Connect(context.Background(), connString)
	require.NoError(t, err)
	defer func() {
		err := conn.Close(context.Background())
		assert.NoError(t, err)
	}()

	id1 := "scan-structs-1"
	data1 := "foo bar baz"
	time1 := time.Now()

	id2 := "scan-structs-2"
	data2 := "foo bar baz qux"
	time2 := time.Now().Add(time.Hour)

	_, err = conn.Exec(context.Background(), "INSERT INTO test (id, some_data, created_at) VALUES ($1, $2, $3), ($4, $5, $6)", id1, data1, time1, id2, data2, time2)
	require.NoError(t, err)

	var result []*testEntity
	rows, err := conn.Query(context.Background(), "SELECT * FROM test WHERE id IN ($1, $2) ORDER BY id ASC", id1, id2)
	require.NoError(t, err)
	defer rows.Close()

	err = ScanStructs(rows, func() interface{} {
		return new(testEntity)
	}, func(r interface{}) {
		result = append(result, r.(*testEntity))
	})
	require.NoError(t, err)
	require.Len(t, result, 2)

	assert.Equal(t, id1, result[0].ID)
	assert.Equal(t, data1, result[0].SomeData)
	// compare unit timestamp to avoid milliseconds diff
	assert.Equal(t, time1.Unix(), result[0].CreatedAt.Unix())

	assert.Equal(t, id2, result[1].ID)
	assert.Equal(t, data2, result[1].SomeData)
	// compare unit timestamp to avoid milliseconds diff
	assert.Equal(t, time2.Unix(), result[1].CreatedAt.Unix())

	// test some fail cases
	err = ScanStructs(nil, func() interface{} {
		return testEntity{}
	}, func(r interface{}) {
	})
	require.Error(t, err)
	assert.Equal(t, "must return a pointer to a new struct, not a value, to ScanStructs destination", err.Error())

	err = ScanStructs(nil, func() interface{} {
		var isNil *testEntity
		return isNil
	}, func(r interface{}) {
	})
	require.Error(t, err)
	assert.Equal(t, "nil pointer returned to ScanStructs destination", err.Error())

	rows, err = conn.Query(context.Background(), "SELECT * FROM test WHERE id IN ($1, $2) ORDER BY id ASC", id1, id2)
	require.NoError(t, err)
	defer rows.Close()

	err = ScanStructs(rows, func() interface{} {
		return new(testMissingField)
	}, func(r interface{}) {
	})
	require.Error(t, err)
	assert.Equal(t, "missing destination name some_data in *pgxhelpers.testMissingField", err.Error())
}

func initDB(t *testing.T, connString string) {
	t.Helper()

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
}
