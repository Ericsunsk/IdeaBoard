package dbutil

import (
	"context"
	"database/sql"
	"strings"
	"time"

	dbsqlc "api/internal/db/sqlc"
)

func WithTx(ctx context.Context, db *sql.DB, fn func(*dbsqlc.Queries) error) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	if err := fn(dbsqlc.New(db).WithTx(tx)); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	committed = true
	return nil
}

func NullTime(value *time.Time) sql.NullTime {
	if value == nil {
		return sql.NullTime{}
	}
	return sql.NullTime{Time: *value, Valid: true}
}

func TimePtr(value sql.NullTime) *time.Time {
	if !value.Valid {
		return nil
	}
	v := value.Time
	return &v
}

func NullString(value *string) sql.NullString {
	if value == nil {
		return sql.NullString{}
	}
	v := strings.TrimSpace(*value)
	return sql.NullString{String: v, Valid: v != ""}
}

func StringPtr(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	v := value.String
	return &v
}
