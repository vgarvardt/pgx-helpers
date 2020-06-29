package pgxhelpers

import (
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/jackc/pgx/v4"
	"github.com/jmoiron/sqlx/reflectx"
)

var mapper = reflectx.NewMapperFunc("db", strings.ToLower)

// ScanStruct scans a pgx.Rows into destination struct passed by reference based on the "db" fields tags.
// This is workaround function for pgx.Rows with single row as pgx/v4 does not allow to get row metadata
// from pgx.Row - see https://github.com/jackc/pgx/issues/627 for details.
//
// If there are no rows pgx.ErrNoRows is returned.
// If there are more than one row in the result - they are ignored.
// Function call closes rows, so caller may skip it.
func ScanStruct(r pgx.Rows, dest interface{}) error {
	defer r.Close()

	v := reflect.ValueOf(dest)
	if v.Kind() != reflect.Ptr {
		return errors.New("dest must be a pointer to a struct, not a value")
	}
	if v.IsNil() {
		return errors.New("dest is nil pointer")
	}

	if !r.Next() {
		return pgx.ErrNoRows
	}

	columns, err := rowMetadata(r, v)
	if err != nil {
		return err
	}

	fields := mapper.TraversalsByName(v.Type(), columns)
	values := make([]interface{}, len(columns))

	err = fieldsByTraversal(v, fields, values)
	if err != nil {
		return err
	}

	return r.Scan(values...)
}

// ScanStructs scans a pgx.Rows into destination structs list passed by reference based on the "db" fields tags
func ScanStructs(r pgx.Rows, newDest func() interface{}, appendResult func(r interface{})) error {
	var (
		columns []string
		err     error
	)

	for r.Next() {
		dest := newDest()
		v := reflect.ValueOf(dest)
		if v.Kind() != reflect.Ptr {
			return errors.New("must return a pointer to a new struct, not a value, to ScanStructs destination")
		}
		if v.IsNil() {
			return errors.New("nil pointer returned to ScanStructs destination")
		}

		if len(columns) == 0 {
			columns, err = rowMetadata(r, v)
			if err != nil {
				return err
			}
		}

		fields := mapper.TraversalsByName(v.Type(), columns)
		values := make([]interface{}, len(columns))

		err := fieldsByTraversal(v, fields, values)
		if err != nil {
			return err
		}

		if err := r.Scan(values...); err != nil {
			return err
		}

		appendResult(dest)
	}

	return r.Err()
}

func rowMetadata(r pgx.Rows, v reflect.Value) (columns []string, err error) {
	fieldDescriptions := r.FieldDescriptions()
	columns = make([]string, len(fieldDescriptions))
	for i, fieldDescription := range fieldDescriptions {
		columns[i] = string(fieldDescription.Name)
	}

	fields := mapper.TraversalsByName(v.Type(), columns)

	// if we are not unsafe and are missing fields, return an error
	if f, err := missingFields(fields); err != nil {
		return columns, fmt.Errorf("missing column %q in dest %s", columns[f], v.Type())
	}

	return
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
