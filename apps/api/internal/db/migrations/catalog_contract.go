package migrations

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

type catalogProperty struct {
	kind     string
	object   string
	property string
}

type schemaContract struct {
	properties map[catalogProperty]string
}

// VerifySchemaSnapshotContract applies both schema release paths and compares
// their PostgreSQL catalog semantics. The caller owns two empty, isolated
// schemas and their connection lifecycle.
func VerifySchemaSnapshotContract(
	ctx context.Context,
	snapshotConn *sql.Conn,
	migrationConn *sql.Conn,
	snapshotSQL []byte,
) error {
	if snapshotConn == nil || migrationConn == nil {
		return fmt.Errorf("schema contract verification requires two database connections")
	}
	if len(snapshotSQL) == 0 {
		return fmt.Errorf("schema snapshot is empty")
	}
	if _, err := snapshotConn.ExecContext(ctx, string(snapshotSQL)); err != nil {
		return fmt.Errorf("apply schema snapshot: %w", err)
	}
	if err := upWithConn(ctx, migrationConn); err != nil {
		return fmt.Errorf("apply migration chain: %w", err)
	}

	snapshotContract, err := inspectSchemaContract(ctx, snapshotConn)
	if err != nil {
		return fmt.Errorf("inspect schema snapshot catalog: %w", err)
	}
	migrationContract, err := inspectSchemaContract(ctx, migrationConn)
	if err != nil {
		return fmt.Errorf("inspect migration catalog: %w", err)
	}
	return compareSchemaContracts(snapshotContract, migrationContract)
}

func inspectSchemaContract(ctx context.Context, conn *sql.Conn) (schemaContract, error) {
	if conn == nil {
		return schemaContract{}, fmt.Errorf("database connection is nil")
	}
	var schemaName string
	if err := conn.QueryRowContext(ctx, `SELECT current_schema()`).Scan(&schemaName); err != nil {
		return schemaContract{}, fmt.Errorf("resolve current schema: %w", err)
	}

	rows, err := conn.QueryContext(ctx, schemaCatalogQuery)
	if err != nil {
		return schemaContract{}, fmt.Errorf("query schema catalog: %w", err)
	}
	defer rows.Close()

	contract := schemaContract{properties: make(map[catalogProperty]string)}
	for rows.Next() {
		var property catalogProperty
		var value string
		if err := rows.Scan(&property.kind, &property.object, &property.property, &value); err != nil {
			return schemaContract{}, fmt.Errorf("scan schema catalog property: %w", err)
		}
		value = normalizeCatalogPropertyValue(property, value, schemaName)
		if previous, exists := contract.properties[property]; exists && previous != value {
			return schemaContract{}, fmt.Errorf(
				"catalog returned conflicting values for %s %s %s: %q and %q",
				property.kind,
				property.object,
				property.property,
				previous,
				value,
			)
		}
		contract.properties[property] = value
	}
	if err := rows.Err(); err != nil {
		return schemaContract{}, fmt.Errorf("read schema catalog: %w", err)
	}
	if err := rows.Close(); err != nil {
		return schemaContract{}, fmt.Errorf("close schema catalog rows: %w", err)
	}
	if err := inspectBuiltinSeed(ctx, conn, contract); err != nil {
		return schemaContract{}, err
	}
	return contract, nil
}

var builtinSeedQueries = []struct {
	object string
	table  string
	query  string
}{
	{
		object: "storage_profiles",
		table:  "storage_profiles",
		query: `SELECT COALESCE(jsonb_agg(to_jsonb(seed_row) ORDER BY seed_row.profile_id)::text, '[]')
FROM (
  SELECT profile_id, name, provider, enabled, is_current, builtin, status, config, created_by, updated_by, deleted_at
  FROM storage_profiles
  WHERE builtin
) AS seed_row`,
	},
	{
		object: "system_configs",
		table:  "system_configs",
		query: `SELECT COALESCE(jsonb_agg(to_jsonb(seed_row) ORDER BY seed_row.config_key)::text, '[]')
FROM (
  SELECT config_key, value
  FROM system_configs
  WHERE config_key IN ('api_outbound_allow_hosts')
) AS seed_row`,
	},
}

func inspectBuiltinSeed(ctx context.Context, conn *sql.Conn, contract schemaContract) error {
	for _, seed := range builtinSeedQueries {
		property := catalogProperty{kind: "builtin_seed", object: seed.object, property: "rows"}
		var tableExists bool
		if err := conn.QueryRowContext(
			ctx,
			`SELECT pg_catalog.to_regclass(pg_catalog.format('%I.%I', current_schema(), $1::text)) IS NOT NULL`,
			seed.table,
		).Scan(&tableExists); err != nil {
			return fmt.Errorf("inspect builtin seed table %s: %w", seed.table, err)
		}
		if !tableExists {
			contract.properties[property] = "<table missing>"
			continue
		}
		var value string
		if err := conn.QueryRowContext(ctx, seed.query).Scan(&value); err != nil {
			return fmt.Errorf("inspect builtin seed %s: %w", seed.object, err)
		}
		contract.properties[property] = value
	}
	return nil
}

var (
	catalogArrayComparisonPattern       = regexp.MustCompile(`\b(ANY|ALL) \(\(ARRAY\[([^]]*)\]\)\)`)
	catalogParenthesizedTextLiteralCast = regexp.MustCompile(`\(('(?:''|[^'])*')\)::text`)
)

func normalizeCatalogPropertyValue(property catalogProperty, value, schemaName string) string {
	if schemaName == "" {
		return normalizeCatalogExpression(property, value)
	}
	value = strings.ReplaceAll(value, `"`+schemaName+`"`, `"<schema>"`)
	value = strings.ReplaceAll(value, schemaName, "<schema>")
	return normalizeCatalogExpression(property, value)
}

func normalizeCatalogExpression(property catalogProperty, value string) string {
	if property.kind != "constraint" && property.kind != "index" {
		return value
	}
	if property.property != "definition" && property.property != "predicate" {
		return value
	}

	// pg_dump and direct DDL can store the same varchar IN-list with the
	// varchar-to-text cast either on each element or on the array. Normalize
	// only that known PostgreSQL representation difference; operators, values,
	// grouping, and all other casts remain part of the compared contract.
	value = strings.ReplaceAll(value, "::character varying::text", "")
	value = strings.ReplaceAll(value, "::character varying", "")
	value = strings.ReplaceAll(value, "::text[]", "")
	value = catalogParenthesizedTextLiteralCast.ReplaceAllString(value, "$1")
	return catalogArrayComparisonPattern.ReplaceAllString(value, "$1 (ARRAY[$2])")
}

func compareSchemaContracts(snapshot, migration schemaContract) error {
	keys := make([]catalogProperty, 0, len(snapshot.properties)+len(migration.properties))
	seen := make(map[catalogProperty]struct{}, len(snapshot.properties)+len(migration.properties))
	for property := range snapshot.properties {
		seen[property] = struct{}{}
		keys = append(keys, property)
	}
	for property := range migration.properties {
		if _, exists := seen[property]; exists {
			continue
		}
		keys = append(keys, property)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].kind != keys[j].kind {
			return keys[i].kind < keys[j].kind
		}
		if keys[i].object != keys[j].object {
			return keys[i].object < keys[j].object
		}
		return keys[i].property < keys[j].property
	})

	diffs := make([]string, 0)
	for _, property := range keys {
		snapshotValue, snapshotExists := snapshot.properties[property]
		migrationValue, migrationExists := migration.properties[property]
		if snapshotExists && migrationExists && snapshotValue == migrationValue {
			continue
		}
		diffs = append(diffs, fmt.Sprintf(
			"%s %s %s: snapshot=%s, migration=%s",
			property.kind,
			property.object,
			property.property,
			formatCatalogValue(snapshotValue, snapshotExists),
			formatCatalogValue(migrationValue, migrationExists),
		))
	}
	if len(diffs) > 0 {
		return fmt.Errorf("schema snapshot and migration chain differ:\n%s", strings.Join(diffs, "\n"))
	}
	return nil
}

func formatCatalogValue(value string, exists bool) string {
	if !exists {
		return "<missing>"
	}
	return fmt.Sprintf("%q", value)
}

const schemaCatalogQuery = `
WITH target_schema AS (
  SELECT current_schema() AS name
),
tables AS (
  SELECT class.oid, class.relname, class.relkind, class.relpersistence, class.relispartition
  FROM pg_catalog.pg_class AS class
  JOIN pg_catalog.pg_namespace AS namespace ON namespace.oid = class.relnamespace
  JOIN target_schema ON target_schema.name = namespace.nspname
  WHERE class.relkind IN ('r', 'p')
    AND class.relname <> 'schema_migrations'
),
functions AS (
  SELECT function.*,
         language.lanname,
         function.proname || '(' || pg_catalog.pg_get_function_identity_arguments(function.oid) || ')' AS signature
  FROM pg_catalog.pg_proc AS function
  JOIN pg_catalog.pg_namespace AS namespace ON namespace.oid = function.pronamespace
  JOIN pg_catalog.pg_language AS language ON language.oid = function.prolang
  JOIN target_schema ON target_schema.name = namespace.nspname
  WHERE function.proname <> 'prevent_schema_migrations_mutation'
),
constraints AS (
  SELECT constraint_info.*,
         table_info.relname AS table_name,
         row_number() OVER (
           PARTITION BY table_info.oid, constraint_info.conname
           ORDER BY pg_catalog.pg_get_constraintdef(constraint_info.oid, true)
         ) AS name_ordinal,
         count(*) OVER (PARTITION BY table_info.oid, constraint_info.conname) AS name_count
  FROM tables AS table_info
  JOIN pg_catalog.pg_constraint AS constraint_info ON constraint_info.conrelid = table_info.oid
),
properties AS (
  SELECT 'table'::text AS kind, table_info.relname::text AS object, 'exists'::text AS property, 'true'::text AS value
  FROM tables AS table_info
  UNION ALL
  SELECT 'table', table_info.relname::text, 'kind', table_info.relkind::text
  FROM tables AS table_info
  UNION ALL
  SELECT 'table', table_info.relname::text, 'persistence', table_info.relpersistence::text
  FROM tables AS table_info
  UNION ALL
  SELECT 'table', table_info.relname::text, 'partitioned', table_info.relispartition::text
  FROM tables AS table_info

  UNION ALL
  SELECT 'column', table_info.relname || '.' || attribute.attname, 'exists', 'true'
  FROM tables AS table_info
  JOIN pg_catalog.pg_attribute AS attribute ON attribute.attrelid = table_info.oid
  WHERE attribute.attnum > 0 AND NOT attribute.attisdropped
  UNION ALL
  SELECT 'column', table_info.relname || '.' || attribute.attname, 'type', pg_catalog.format_type(attribute.atttypid, attribute.atttypmod)
  FROM tables AS table_info
  JOIN pg_catalog.pg_attribute AS attribute ON attribute.attrelid = table_info.oid
  WHERE attribute.attnum > 0 AND NOT attribute.attisdropped
  UNION ALL
  SELECT 'column', table_info.relname || '.' || attribute.attname, 'nullable', (NOT attribute.attnotnull)::text
  FROM tables AS table_info
  JOIN pg_catalog.pg_attribute AS attribute ON attribute.attrelid = table_info.oid
  WHERE attribute.attnum > 0 AND NOT attribute.attisdropped
  UNION ALL
  SELECT 'column', table_info.relname || '.' || attribute.attname, 'default', COALESCE(pg_catalog.pg_get_expr(default_value.adbin, default_value.adrelid), '')
  FROM tables AS table_info
  JOIN pg_catalog.pg_attribute AS attribute ON attribute.attrelid = table_info.oid
  LEFT JOIN pg_catalog.pg_attrdef AS default_value ON default_value.adrelid = table_info.oid AND default_value.adnum = attribute.attnum
  WHERE attribute.attnum > 0 AND NOT attribute.attisdropped
  UNION ALL
  SELECT 'column', table_info.relname || '.' || attribute.attname, 'identity', attribute.attidentity::text
  FROM tables AS table_info
  JOIN pg_catalog.pg_attribute AS attribute ON attribute.attrelid = table_info.oid
  WHERE attribute.attnum > 0 AND NOT attribute.attisdropped
  UNION ALL
  SELECT 'column', table_info.relname || '.' || attribute.attname, 'generated', attribute.attgenerated::text
  FROM tables AS table_info
  JOIN pg_catalog.pg_attribute AS attribute ON attribute.attrelid = table_info.oid
  WHERE attribute.attnum > 0 AND NOT attribute.attisdropped

  UNION ALL
  SELECT 'constraint', constraint_info.table_name || '.' || constraint_info.conname ||
    CASE WHEN constraint_info.name_count > 1 THEN '#' || constraint_info.name_ordinal::text ELSE '' END,
    'exists', 'true'
  FROM constraints AS constraint_info
  UNION ALL
  SELECT 'constraint', constraint_info.table_name || '.' || constraint_info.conname ||
    CASE WHEN constraint_info.name_count > 1 THEN '#' || constraint_info.name_ordinal::text ELSE '' END,
    'type', constraint_info.contype::text
  FROM constraints AS constraint_info
  UNION ALL
  SELECT 'constraint', constraint_info.table_name || '.' || constraint_info.conname ||
    CASE WHEN constraint_info.name_count > 1 THEN '#' || constraint_info.name_ordinal::text ELSE '' END,
    'definition', pg_catalog.pg_get_constraintdef(constraint_info.oid, true)
  FROM constraints AS constraint_info
  UNION ALL
  SELECT 'constraint', constraint_info.table_name || '.' || constraint_info.conname ||
    CASE WHEN constraint_info.name_count > 1 THEN '#' || constraint_info.name_ordinal::text ELSE '' END,
    'deferrable', constraint_info.condeferrable::text
  FROM constraints AS constraint_info
  UNION ALL
  SELECT 'constraint', constraint_info.table_name || '.' || constraint_info.conname ||
    CASE WHEN constraint_info.name_count > 1 THEN '#' || constraint_info.name_ordinal::text ELSE '' END,
    'initially_deferred', constraint_info.condeferred::text
  FROM constraints AS constraint_info
  UNION ALL
  SELECT 'constraint', constraint_info.table_name || '.' || constraint_info.conname ||
    CASE WHEN constraint_info.name_count > 1 THEN '#' || constraint_info.name_ordinal::text ELSE '' END,
    'validated', constraint_info.convalidated::text
  FROM constraints AS constraint_info

  UNION ALL
  SELECT 'index', table_info.relname || '.' || index_class.relname, 'exists', 'true'
  FROM tables AS table_info
  JOIN pg_catalog.pg_index AS index_info ON index_info.indrelid = table_info.oid
  JOIN pg_catalog.pg_class AS index_class ON index_class.oid = index_info.indexrelid
  UNION ALL
  SELECT 'index', table_info.relname || '.' || index_class.relname, 'definition', pg_catalog.pg_get_indexdef(index_info.indexrelid)
  FROM tables AS table_info
  JOIN pg_catalog.pg_index AS index_info ON index_info.indrelid = table_info.oid
  JOIN pg_catalog.pg_class AS index_class ON index_class.oid = index_info.indexrelid
  UNION ALL
  SELECT 'index', table_info.relname || '.' || index_class.relname, 'unique', index_info.indisunique::text
  FROM tables AS table_info
  JOIN pg_catalog.pg_index AS index_info ON index_info.indrelid = table_info.oid
  JOIN pg_catalog.pg_class AS index_class ON index_class.oid = index_info.indexrelid
  UNION ALL
  SELECT 'index', table_info.relname || '.' || index_class.relname, 'primary', index_info.indisprimary::text
  FROM tables AS table_info
  JOIN pg_catalog.pg_index AS index_info ON index_info.indrelid = table_info.oid
  JOIN pg_catalog.pg_class AS index_class ON index_class.oid = index_info.indexrelid
  UNION ALL
  SELECT 'index', table_info.relname || '.' || index_class.relname, 'valid', index_info.indisvalid::text
  FROM tables AS table_info
  JOIN pg_catalog.pg_index AS index_info ON index_info.indrelid = table_info.oid
  JOIN pg_catalog.pg_class AS index_class ON index_class.oid = index_info.indexrelid
  UNION ALL
  SELECT 'index', table_info.relname || '.' || index_class.relname, 'predicate', COALESCE(pg_catalog.pg_get_expr(index_info.indpred, index_info.indrelid), '')
  FROM tables AS table_info
  JOIN pg_catalog.pg_index AS index_info ON index_info.indrelid = table_info.oid
  JOIN pg_catalog.pg_class AS index_class ON index_class.oid = index_info.indexrelid

  UNION ALL
  SELECT 'function', function_info.signature, 'exists', 'true'
  FROM functions AS function_info
  UNION ALL
  SELECT 'function', function_info.signature, 'result', pg_catalog.pg_get_function_result(function_info.oid)
  FROM functions AS function_info
  UNION ALL
  SELECT 'function', function_info.signature, 'language', function_info.lanname
  FROM functions AS function_info
  UNION ALL
  SELECT 'function', function_info.signature, 'kind', function_info.prokind::text
  FROM functions AS function_info
  UNION ALL
  SELECT 'function', function_info.signature, 'volatility', function_info.provolatile::text
  FROM functions AS function_info
  UNION ALL
  SELECT 'function', function_info.signature, 'parallel', function_info.proparallel::text
  FROM functions AS function_info
  UNION ALL
  SELECT 'function', function_info.signature, 'strict', function_info.proisstrict::text
  FROM functions AS function_info
  UNION ALL
  SELECT 'function', function_info.signature, 'security_definer', function_info.prosecdef::text
  FROM functions AS function_info
  UNION ALL
  SELECT 'function', function_info.signature, 'leakproof', function_info.proleakproof::text
  FROM functions AS function_info
  UNION ALL
  SELECT 'function', function_info.signature, 'configuration', COALESCE(array_to_string(function_info.proconfig, ','), '')
  FROM functions AS function_info
  UNION ALL
  SELECT 'function', function_info.signature, 'public_execute', EXISTS (
    SELECT 1
    FROM pg_catalog.aclexplode(COALESCE(function_info.proacl, pg_catalog.acldefault('f', function_info.proowner))) AS privilege
    WHERE privilege.grantee = 0 AND privilege.privilege_type = 'EXECUTE'
  )::text
  FROM functions AS function_info

  UNION ALL
  SELECT 'trigger', table_info.relname || '.' || trigger_info.tgname, 'exists', 'true'
  FROM tables AS table_info
  JOIN pg_catalog.pg_trigger AS trigger_info ON trigger_info.tgrelid = table_info.oid
  WHERE NOT trigger_info.tgisinternal
  UNION ALL
  SELECT 'trigger', table_info.relname || '.' || trigger_info.tgname, 'definition', pg_catalog.pg_get_triggerdef(trigger_info.oid, true)
  FROM tables AS table_info
  JOIN pg_catalog.pg_trigger AS trigger_info ON trigger_info.tgrelid = table_info.oid
  WHERE NOT trigger_info.tgisinternal
  UNION ALL
  SELECT 'trigger', table_info.relname || '.' || trigger_info.tgname, 'enabled', trigger_info.tgenabled::text
  FROM tables AS table_info
  JOIN pg_catalog.pg_trigger AS trigger_info ON trigger_info.tgrelid = table_info.oid
  WHERE NOT trigger_info.tgisinternal
)
SELECT kind, object, property, value
FROM properties
ORDER BY kind, object, property
`
