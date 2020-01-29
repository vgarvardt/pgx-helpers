package pgxhelpers

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"unsafe"

	"github.com/jackc/pgconn"
	"github.com/jackc/pgproto3/v2"
	"github.com/jackc/pgx/v4"
	"github.com/jmoiron/sqlx/reflectx"
)

var mapper = reflectx.NewMapperFunc("db", strings.ToLower)
var rowsType reflect.Type

func init() {
	// XXX: hardcore unsafe part 1, just to make it work, not for production
	// see part 2 below to see why I need all this

	// all this unsafe black magic is required to get the instance of private *pgx.connRows for further usage
	// to do this - need to initialise fake connection with the minimal set of fields set just to make it return
	// instance of Rows

	// pgConn is required in the connection to check connection status - we'll just set it to closed
	pgConn := &pgconn.PgConn{}
	pointerValPgConn := reflect.ValueOf(pgConn)
	valPgConn := reflect.Indirect(pointerValPgConn)

	fieldStatus := valPgConn.FieldByName("status")
	ptrToStatus := unsafe.Pointer(fieldStatus.UnsafeAddr())
	realPtrToStatus := (*byte)(ptrToStatus)
	// connStatusClosed == 2
	*realPtrToStatus = byte(2)

	c := &pgx.Conn{}
	pointerValConn := reflect.ValueOf(c)
	valConn := reflect.Indirect(pointerValConn)

	// connection checks some config values, so it should be set
	fieldConfig := valConn.FieldByName("config")
	ptrToConfig := unsafe.Pointer(fieldConfig.UnsafeAddr())
	realPtrToConfig := (**pgx.ConnConfig)(ptrToConfig)
	*realPtrToConfig = &pgx.ConnConfig{}

	fieldPgConn := valConn.FieldByName("pgConn")
	ptrToPgConn := unsafe.Pointer(fieldPgConn.UnsafeAddr())
	realPtrToPgConn := (**pgconn.PgConn)(ptrToPgConn)
	*realPtrToPgConn = pgConn

	// we'll actually get an error as the connection is closed but we do not care - all we need is instance of rows
	r, _ := c.Query(context.Background(), "")

	rowsType = reflect.TypeOf(r).Elem()
}

// ScanStruct scans a pgx.Row into destination struct passed by reference based on the "db" fields tags
func ScanStruct(r pgx.Row, dest interface{}) error {
	v := reflect.ValueOf(dest)
	if v.Kind() != reflect.Ptr {
		return errors.New("must pass a pointer, not a value, to ScanStruct destination")
	}
	if v.IsNil() {
		return errors.New("nil pointer passed to ScanStruct destination")
	}

	// XXX: hardcore unsafe part 2, just to make it work, not for production
	// now, as we have rows instance - we can reproduce Row.Scan(...) behaviour
	// with the values prepared from struct fields
	rows := reflect.NewAt(rowsType, unsafe.Pointer(reflect.ValueOf(r).Pointer())).Interface().(pgx.Rows)

	if rows.Err() != nil {
		return rows.Err()
	}

	if !rows.Next() {
		if rows.Err() == nil {
			return pgx.ErrNoRows
		}
		return rows.Err()
	}

	fieldDescriptions := rows.FieldDescriptions()
	columns := make([]string, len(fieldDescriptions), len(fieldDescriptions))
	for i, fieldDescription := range fieldDescriptions {
		columns[i] = string(fieldDescription.Name)
	}

	fields := mapper.TraversalsByName(v.Type(), columns)

	// if we are not unsafe and are missing fields, return an error
	if f, err := missingFields(fields); err != nil {
		return fmt.Errorf("missing destination name %s in %T", columns[f], dest)
	}
	values := make([]interface{}, len(columns))

	err := fieldsByTraversal(v, fields, values)
	if err != nil {
		return err
	}

	// scan into the struct field pointers and append to our results
	rows.Scan(values...)
	rows.Close()
	return rows.Err()
}

// ScanStructs scans a pgx.Rows into destination structs list passed by reference based on the "db" fields tags
func ScanStructs(r pgx.Rows, newDest func() interface{}, appendResult func(r interface{})) error {
	dest := newDest()
	v := reflect.ValueOf(dest)
	if v.Kind() != reflect.Ptr {
		return errors.New("must return a pointer to a new struct, not a value, to ScanStructs destination")
	}
	if v.IsNil() {
		return errors.New("nil pointer returned to ScanStructs destination")
	}

	var (
		fieldDescriptions []pgproto3.FieldDescription
		columns           []string
	)

	for r.Next() {
		if len(fieldDescriptions) == 0 {
			fieldDescriptions = r.FieldDescriptions()
			columns = make([]string, len(fieldDescriptions), len(fieldDescriptions))
			for i, fieldDescription := range fieldDescriptions {
				columns[i] = string(fieldDescription.Name)
			}

			fields := mapper.TraversalsByName(v.Type(), columns)

			// if we are not unsafe and are missing fields, return an error
			if f, err := missingFields(fields); err != nil {
				return fmt.Errorf("missing destination name %s in %T", columns[f], dest)
			}
		}

		dest := newDest()
		v = reflect.ValueOf(dest)

		fields := mapper.TraversalsByName(v.Type(), columns)
		values := make([]interface{}, len(columns))

		err := fieldsByTraversal(v, fields, values)
		if err != nil {
			return err
		}

		if err := r.Scan(values...); err != nil {
			return nil
		}

		appendResult(dest)
	}

	return r.Err()
}

func missingFields(traversals [][]int) (field int, err error) {
	for i, t := range traversals {
		if len(t) == 0 {
			return i, errors.New("missing field")
		}
	}
	return 0, nil
}

func fieldsByTraversal(v reflect.Value, traversals [][]int, values []interface{}) error {
	v = reflect.Indirect(v)
	if v.Kind() != reflect.Struct {
		return errors.New("argument is not a struct")
	}

	for i, traversal := range traversals {
		if len(traversal) == 0 {
			values[i] = new(interface{})
			continue
		}

		f := reflectx.FieldByIndexes(v, traversal)
		if f.Kind() == reflect.Ptr {
			values[i] = f.Interface()
		} else {
			values[i] = f.Addr().Interface()
		}
	}

	return nil
}
