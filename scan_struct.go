package pgxhelpers

import (
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/jackc/pgproto3/v2"
	"github.com/jackc/pgx/v4"
	"github.com/jmoiron/sqlx/reflectx"
)

var mapper = reflectx.NewMapperFunc("db", strings.ToLower)

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
