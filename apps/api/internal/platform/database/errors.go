package database

import (
	"database/sql"
	"errors"
	"strings"

	mysqlDriver "github.com/go-sql-driver/mysql"
)

var ErrNotFound = sql.ErrNoRows

func IsNotFound(err error) bool {
	return errors.Is(err, ErrNotFound)
}

func IsDuplicateConstraintError(err error) bool {
	if err == nil {
		return false
	}
	var mysqlErr *mysqlDriver.MySQLError
	if errors.As(err, &mysqlErr) {
		return mysqlErr.Number == 1062
	}
	msg := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(msg, "duplicate") ||
		strings.Contains(msg, "unique constraint") ||
		strings.Contains(msg, "unique failed")
}

func IsMissingTableError(err error) bool {
	if err == nil {
		return false
	}
	var mysqlErr *mysqlDriver.MySQLError
	if errors.As(err, &mysqlErr) {
		return mysqlErr.Number == 1146
	}
	return strings.Contains(strings.ToLower(err.Error()), "doesn't exist")
}
