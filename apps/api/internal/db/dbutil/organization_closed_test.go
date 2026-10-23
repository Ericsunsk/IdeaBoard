package dbutil

import (
	"context"
	"errors"
	"testing"

	dbsqlc "ideaboard/api/internal/db/sqlc"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

type closureLookupDB struct {
	stubDBTX
	rows []pgx.Row
}

func (s *closureLookupDB) QueryRow(context.Context, string, ...any) pgx.Row {
	row := s.rows[0]
	s.rows = s.rows[1:]
	return row
}

type closureLookupRow func(...any) error

func (row closureLookupRow) Scan(dest ...any) error { return row(dest...) }

func workspaceClosureRow(organizationID pgtype.Int8) pgx.Row {
	return closureLookupRow(func(dest ...any) error {
		*dest[2].(*pgtype.Int8) = organizationID
		return nil
	})
}

func TestOrganizationClosedLookupPropagatesReadFailures(t *testing.T) {
	for _, failure := range []error{pgx.ErrNoRows, errors.New("database unavailable")} {
		for _, lookup := range []string{"workspace", "organization"} {
			t.Run(lookup+"/"+failure.Error(), func(t *testing.T) {
				rows := []pgx.Row{stubRow{err: failure}}
				if lookup == "organization" {
					rows = append([]pgx.Row{workspaceClosureRow(pgtype.Int8{Int64: 42, Valid: true})}, rows...)
				}
				q := dbsqlc.New(&closureLookupDB{rows: rows})
				if _, err := IsOrganizationClosedForWorkspace(t.Context(), q, "ws_lookup"); !errors.Is(err, failure) {
					t.Fatalf("lookup error = %v, want %v; an unknown closure state must not authorize access", err, failure)
				}
			})
		}
	}
}

func TestOrganizationClosedLookupKnownScopes(t *testing.T) {
	for _, tc := range []struct {
		name         string
		organization bool
		closed       bool
	}{
		{name: "personal"},
		{name: "open organization", organization: true},
		{name: "closed organization", organization: true, closed: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rows := []pgx.Row{workspaceClosureRow(pgtype.Int8{Int64: 42, Valid: tc.organization})}
			if tc.organization {
				rows = append(rows, closureLookupRow(func(dest ...any) error {
					*dest[5].(*pgtype.Timestamptz) = pgtype.Timestamptz{Valid: tc.closed}
					return nil
				}))
			}
			q := dbsqlc.New(&closureLookupDB{rows: rows})
			closed, err := IsOrganizationClosedForWorkspace(t.Context(), q, "ws_lookup")
			if err != nil || closed != tc.closed {
				t.Fatalf("closed = %v, error = %v; want %v, nil", closed, err, tc.closed)
			}
		})
	}
}
