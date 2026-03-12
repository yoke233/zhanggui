package sqlite

import (
	"database/sql"
	"encoding/json"
)

// marshalJSON returns nil for nil/empty values, otherwise JSON bytes as string.
func marshalJSON(v any) (any, error) {
	if v == nil {
		return nil, nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	s := string(b)
	if s == "null" || s == "{}" || s == "[]" {
		return nil, nil
	}
	return s, nil
}

func unmarshalNullJSON(ns sql.NullString, dest any) {
	if ns.Valid {
		_ = json.Unmarshal([]byte(ns.String), dest)
	}
}
