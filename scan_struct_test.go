package pgxhelpers

import (
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v4"
	"github.com/ory/dockertest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var connString string

func TestMain(m *testing.M) {
	pool, resources := initDockerDeps()

	if err := initDB(); err != nil {
		purgeDockerDeps(pool, resources)
		log.Fatalf("Could not initialise DB: %s", err)
	}

	code := m.Run()

	// os.Exit does not execute deferred calls so need to call purge explicitly
	purgeDockerDeps(pool, resources)

	os.Exit(code)
}

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
	conn, err := pgx.Connect(context.Background(), connString)
	require.NoError(t, err)
	defer func() {
		err := conn.Close(context.Background())
		assert.NoError(t, err)
	}()

	id1 := "scan-struct-1"
	data1 := "foo bar"
	time1 := time.Now()

	_, err = conn.Exec(context.Background(), "INSERT INTO test (id, some_data, created_at) VALUES ($1, $2, $3)", id1, data1, time1)
	require.NoError(t, err)

	var result testEntity
	row := conn.QueryRow(context.Background(), "SELECT * FROM test WHERE id = $1", id1)
	err = ScanStruct(row, &result)
	require.NoError(t, err)

	assert.Equal(t, id1, result.ID)
	assert.Equal(t, data1, result.SomeData)
	// compare unit timestamp to avoid milliseconds diff
	assert.Equal(t, time1.Unix(), result.CreatedAt.Unix())

	// test some fail cases
	err = ScanStruct(nil, result)
	require.Error(t, err)
	assert.Equal(t, "must pass a pointer, not a value, to ScanStruct destination", err.Error())

	var isNil *testEntity
	err = ScanStruct(nil, isNil)
	require.Error(t, err)
	assert.Equal(t, "nil pointer passed to ScanStruct destination", err.Error())

	var resultMissingField testMissingField
	row = conn.QueryRow(context.Background(), "SELECT * FROM test WHERE id = $1", id1)
	err = ScanStruct(row, &resultMissingField)
	require.Error(t, err)
	assert.Equal(t, "missing destination name some_data in *pgxhelpers.testMissingField", err.Error())
}

func TestScanStructs(t *testing.T) {
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

func initDockerDeps() (*dockertest.Pool, []*dockertest.Resource) {
	// uses a sensible default on windows (tcp/http) and linux/osx (socket)
	pool, err := dockertest.NewPool("")
	if err != nil {
		log.Fatalf("Could not connect to docker: %s", err)
	}

	// pulls an image, creates a container based on it and runs it
	pgResource, err := pool.Run("postgres", "11.5-alpine", []string{
		"LC_ALL=C.UTF-8",
		"POSTGRES_DB=test",
		"POSTGRES_USER=test",
		"POSTGRES_PASSWORD=test",
	})
	if err != nil {
		log.Fatalf("Could not start postgres resource: %s", err)
	}

	dockerHost := "localhost"
	if endpoint, ok := os.LookupEnv("DOCKER_HOST"); ok {
		dockerHost = endpoint

		// check if host has port and strip it
		colon := strings.LastIndexByte(dockerHost, ':')
		if colon != -1 {
			dockerHost, _ = dockerHost[:colon], dockerHost[colon+1:]
		}
	}

	connString = fmt.Sprintf(
		"host=%s port=%s user=test password=test dbname=test sslmode=disable",
		dockerHost,
		pgResource.GetPort("5432/tcp"),
	)

	// exponential backoff-retry, because the application in the container might not be ready to accept connections yet
	if err := pool.Retry(func() error {
		conn, err := pgx.Connect(context.Background(), connString)
		if err != nil {
			return err
		}

		ctx, _ := context.WithTimeout(context.Background(), 5*time.Second)

		return conn.Ping(ctx)
	}); err != nil {
		log.Fatalf("Could not connect to postgres docker: %s [source: %s]", err, connString)
	}

	return pool, []*dockertest.Resource{pgResource}
}

func purgeDockerDeps(pool *dockertest.Pool, resources []*dockertest.Resource) {
	for _, r := range resources {
		if err := pool.Purge(r); err != nil {
			log.Fatalf("Could not purge docker resource: %s", err)
		}
	}
}

func initDB() error {
	conn, err := pgx.Connect(context.Background(), connString)
	if err != nil {
		return err
	}
	defer conn.Close(context.Background())

	_, err = conn.Exec(context.Background(), `
CREATE TABLE "test"
(
    id         text PRIMARY KEY,
    some_data  text        not null,
    created_at timestamptz not null
);
`)

	return err
}
