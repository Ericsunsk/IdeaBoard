package migrations

import (
	"context"
	"database/sql"
	_ "embed"
	"fmt"
	"sort"
	"strings"
)

//go:embed security_definer_routines.txt
var securityDefinerRoutinePolicy string

var securityDefinerRoutineSignatures = strings.Split(
	strings.TrimSpace(securityDefinerRoutinePolicy),
	"\n",
)

type securityDefinerRoutine struct {
	signature          string
	owner              string
	expectedOwner      string
	publicCanExecute   bool
	searchPath         string
	expectedSearchPath string
}

func verifySecurityDefinerRoutines(ctx context.Context, conn *sql.Conn) error {
	if conn == nil {
		return fmt.Errorf("migration connection is nil")
	}

	rows, err := conn.QueryContext(ctx, `
SELECT
  functions.proname || '(' || pg_catalog.oidvectortypes(functions.proargtypes) || ')' AS signature,
  pg_catalog.pg_get_userbyid(functions.proowner) AS owner,
  pg_catalog.pg_get_userbyid(schemas.nspowner) AS expected_owner,
  EXISTS (
    SELECT 1
    FROM pg_catalog.aclexplode(
      COALESCE(functions.proacl, pg_catalog.acldefault('f', functions.proowner))
    ) AS privileges
    WHERE privileges.grantee = 0
      AND privileges.privilege_type = 'EXECUTE'
  ) AS public_can_execute,
  COALESCE((
    SELECT option.option_value
    FROM pg_catalog.pg_options_to_table(functions.proconfig) AS option
    WHERE option.option_name = 'search_path'
  ), '') AS search_path,
  pg_catalog.format('%I, pg_catalog, pg_temp', schemas.nspname) AS expected_search_path
FROM pg_catalog.pg_proc AS functions
JOIN pg_catalog.pg_namespace AS schemas ON schemas.oid = functions.pronamespace
WHERE schemas.nspname = current_schema()
  AND functions.prosecdef
ORDER BY signature
`)
	if err != nil {
		return fmt.Errorf("inspect database routines: %w", err)
	}
	defer rows.Close()

	expected := make(map[string]struct{}, len(securityDefinerRoutineSignatures))
	for _, signature := range securityDefinerRoutineSignatures {
		expected[signature] = struct{}{}
	}
	var violations []string
	for rows.Next() {
		var routine securityDefinerRoutine
		if err := rows.Scan(
			&routine.signature,
			&routine.owner,
			&routine.expectedOwner,
			&routine.publicCanExecute,
			&routine.searchPath,
			&routine.expectedSearchPath,
		); err != nil {
			return fmt.Errorf("read database routine security facts: %w", err)
		}
		delete(expected, routine.signature)
		if routine.owner != routine.expectedOwner {
			violations = append(violations, fmt.Sprintf(
				"%s owner=%q, want schema owner %q",
				routine.signature,
				routine.owner,
				routine.expectedOwner,
			))
		}
		if routine.publicCanExecute {
			violations = append(violations, fmt.Sprintf(
				"%s grants EXECUTE to PUBLIC",
				routine.signature,
			))
		}
		if routine.searchPath != routine.expectedSearchPath {
			violations = append(violations, fmt.Sprintf(
				"%s search_path=%q, want %q",
				routine.signature,
				routine.searchPath,
				routine.expectedSearchPath,
			))
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("inspect database routines: %w", err)
	}
	for signature := range expected {
		violations = append(violations, fmt.Sprintf(
			"%s is missing or is not SECURITY DEFINER",
			signature,
		))
	}
	if len(violations) > 0 {
		sort.Strings(violations)
		return fmt.Errorf("unsafe routine(s): %s", strings.Join(violations, "; "))
	}
	return nil
}
