package database

import (
	"database/sql"
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

var ErrNotFound = sql.ErrNoRows

func IsNotFound(err error) bool {
	return errors.Is(err, ErrNotFound)
}

func IsDuplicateConstraintError(err error) bool {
	if err == nil {
		return false
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "23505"
	}
	return false
}

func IsMissingTableError(err error) bool {
	if err == nil {
		return false
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "42P01"
	}
	return false
}
