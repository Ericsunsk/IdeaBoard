package database

import (
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestIsConstraintViolation(t *testing.T) {
	t.Parallel()

	testCases := []struct {
		name           string
		err            error
		constraintName string
		want           bool
	}{
		{
			name:           "nil error",
			err:            nil,
			constraintName: "uq_table_col",
			want:           false,
		},
		{
			name:           "non-pg error",
			err:            errors.New("generic error"),
			constraintName: "uq_table_col",
			want:           false,
		},
		{
			name: "different pg code",
			err: &pgconn.PgError{
				Code:           "23503", // foreign_key_violation
				ConstraintName: "uq_table_col",
			},
			constraintName: "uq_table_col",
			want:           false,
		},
		{
			name: "exact match",
			err: &pgconn.PgError{
				Code:           "23505",
				ConstraintName: "uq_table_col",
			},
			constraintName: "uq_table_col",
			want:           true,
		},
		{
			name: "case-insensitive and trimmed match",
			err: &pgconn.PgError{
				Code:           "23505",
				ConstraintName: " UQ_TABLE_COL ",
			},
			constraintName: "uq_table_col",
			want:           true,
		},
		{
			name: "wrapped pg error",
			err: fmt.Errorf("query failed: %w", &pgconn.PgError{
				Code:           "23505",
				ConstraintName: "uq_table_col",
			}),
			constraintName: "uq_table_col",
			want:           true,
		},
		{
			name: "mismatched constraint name",
			err: &pgconn.PgError{
				Code:           "23505",
				ConstraintName: "uq_other_col",
			},
			constraintName: "uq_table_col",
			want:           false,
		},
	}

	for _, tc := range testCases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := IsConstraintViolation(tc.err, tc.constraintName)
			if got != tc.want {
				t.Errorf("IsConstraintViolation(%v, %q) = %v; want %v", tc.err, tc.constraintName, got, tc.want)
			}
		})
	}
}
