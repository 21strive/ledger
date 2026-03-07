package repo

import (
	"database/sql"
	"time"
)

// toNullTime converts *time.Time to sql.NullTime for proper NULL handling in PostgreSQL.
// PostgreSQL driver converts nil *time.Time to zero timestamp (0001-01-01) instead of NULL.
// Use this helper to ensure nil pointers are properly stored as NULL in the database.
func toNullTime(t *time.Time) sql.NullTime {
	if t == nil {
		return sql.NullTime{Valid: false}
	}
	return sql.NullTime{Time: *t, Valid: true}
}

// toNullString converts string to sql.NullString for proper NULL handling in PostgreSQL.
// Empty strings are stored as NULL in the database.
func toNullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{Valid: false}
	}
	return sql.NullString{String: s, Valid: true}
}
