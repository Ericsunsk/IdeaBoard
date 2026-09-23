package privileges

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
)

type resolvedRule struct {
	kind              ObjectKind
	name              string
	functionArguments string
	optional          bool
	required          []Privilege
	forbidden         []Privilege
	publicForbidden   []Privilege
	policyNames       []string
}

type discoveredObject struct {
	name              string
	functionArguments string
	selectorName      string
}

func resolvePolicies(
	ctx context.Context,
	db DBTX,
	schemaName string,
	policies []validatedPolicy,
) ([]resolvedRule, []string, error) {
	var resolved []resolvedRule
	var schemaPolicyNames []string
	for _, policy := range policies {
		if policy.requireSchemaUsage {
			schemaPolicyNames = append(schemaPolicyNames, policy.name)
		}
		for _, rule := range policy.objects {
			objects, err := discoverObjects(ctx, db, schemaName, rule)
			if err != nil {
				return nil, nil, fmt.Errorf("policy %q: resolve %s selector: %w", policy.name, rule.kind, err)
			}
			if len(objects) == 0 && (len(rule.required) > 0 || !rule.optional) {
				return nil, nil, fmt.Errorf("policy %q: %s selector %s matched no objects", policy.name, rule.kind, rule.selectorDescription())
			}
			for _, object := range objects {
				resolved = append(resolved, resolvedRule{
					kind: rule.kind, name: object.name, functionArguments: object.functionArguments,
					optional:        rule.optional,
					required:        slices.Clone(rule.required),
					forbidden:       slices.Clone(rule.forbidden),
					publicForbidden: slices.Clone(rule.publicForbidden),
					policyNames:     []string{policy.name},
				})
			}
		}
	}
	merged, err := mergeResolvedRules(resolved)
	if err != nil {
		return nil, nil, err
	}
	sort.Strings(schemaPolicyNames)
	return merged, schemaPolicyNames, nil
}

func discoverObjects(
	ctx context.Context,
	db DBTX,
	schemaName string,
	rule validatedRule,
) ([]discoveredObject, error) {
	switch rule.kind {
	case KindTable, KindSequence:
		objects, err := discoverRelations(ctx, db, schemaName, rule)
		if err != nil {
			return nil, err
		}
		if rule.namePrefix == "" {
			if missing := missingExplicitNames(rule.names, objects); len(missing) > 0 && (len(rule.required) > 0 || !rule.optional) {
				return nil, fmt.Errorf("selected objects do not exist: %s", strings.Join(missing, ", "))
			}
		}
		return objects, nil
	case KindFunction:
		objects, err := discoverFunctions(ctx, db, schemaName, rule)
		if err != nil {
			return nil, err
		}
		if rule.namePrefix == "" {
			if missing := missingExplicitFunctionNames(rule.names, objects); len(missing) > 0 && (len(rule.required) > 0 || !rule.optional) {
				return nil, fmt.Errorf("selected objects do not exist: %s", strings.Join(missing, ", "))
			}
		}
		return objects, nil
	default:
		return nil, fmt.Errorf("unsupported object kind %q", rule.kind)
	}
}

func discoverRelations(
	ctx context.Context,
	db DBTX,
	schemaName string,
	rule validatedRule,
) ([]discoveredObject, error) {
	relationKinds := []string{"r", "p", "v", "m", "f"}
	if rule.kind == KindSequence {
		relationKinds = []string{"S"}
	}
	var (
		rowsQuery string
		args      []any
	)
	if rule.namePrefix != "" {
		rowsQuery = `
SELECT c.relname
FROM pg_catalog.pg_class AS c
JOIN pg_catalog.pg_namespace AS n ON n.oid = c.relnamespace
WHERE n.nspname = $1
  AND c.relkind::text = ANY($2)
  AND left(c.relname, length($3)) = $3
ORDER BY c.relname`
		args = []any{schemaName, relationKinds, rule.namePrefix}
	} else {
		rowsQuery = `
SELECT c.relname
FROM pg_catalog.pg_class AS c
JOIN pg_catalog.pg_namespace AS n ON n.oid = c.relnamespace
WHERE n.nspname = $1
  AND c.relkind::text = ANY($2)
  AND c.relname = ANY($3)
ORDER BY c.relname`
		args = []any{schemaName, relationKinds, rule.names}
	}
	rows, err := db.Query(ctx, rowsQuery, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var objects []discoveredObject
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		objects = append(objects, discoveredObject{name: name})
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()
	return objects, nil
}

func discoverFunctions(
	ctx context.Context,
	db DBTX,
	schemaName string,
	rule validatedRule,
) ([]discoveredObject, error) {
	if rule.namePrefix != "" {
		rows, err := db.Query(ctx, `
SELECT p.proname, pg_catalog.oidvectortypes(p.proargtypes)
FROM pg_catalog.pg_proc AS p
JOIN pg_catalog.pg_namespace AS n ON n.oid = p.pronamespace
WHERE n.nspname = $1
  AND p.prokind IN ('f', 'w')
  AND left(p.proname, length($2)) = $2
ORDER BY p.proname, pg_catalog.oidvectortypes(p.proargtypes)`, schemaName, rule.namePrefix)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var objects []discoveredObject
		for rows.Next() {
			var object discoveredObject
			if err := rows.Scan(&object.name, &object.functionArguments); err != nil {
				return nil, err
			}
			objects = append(objects, object)
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		rows.Close()
		return objects, nil
	}

	selectors := make([]string, len(rule.names))
	functionNames := make([]string, len(rule.names))
	functionArguments := make([]string, len(rule.names))
	for index, signature := range rule.names {
		functionName, arguments, _ := parseFunctionSignature(signature)
		selectors[index] = signature
		functionNames[index] = functionName
		functionArguments[index] = arguments
	}
	rows, err := db.Query(ctx, `/* ideaboard:resolve-explicit-functions */
WITH requested AS (
  SELECT selector_name, function_name, function_arguments, ordinal
  FROM unnest($2::TEXT[], $3::TEXT[], $4::TEXT[]) WITH ORDINALITY
    AS input(selector_name, function_name, function_arguments, ordinal)
)
SELECT requested.selector_name,
       resolved.proname,
       resolved.function_arguments
FROM requested
LEFT JOIN LATERAL (
  SELECT p.proname,
         pg_catalog.oidvectortypes(p.proargtypes) AS function_arguments
  FROM pg_catalog.pg_proc AS p
  JOIN pg_catalog.pg_namespace AS n ON n.oid = p.pronamespace
  WHERE n.nspname = $1
    AND p.prokind IN ('f', 'w')
    AND p.oid = to_regprocedure(
      format('%I.%I(%s)', $1::TEXT, requested.function_name, requested.function_arguments)
    )
) AS resolved ON TRUE
ORDER BY requested.ordinal`,
		schemaName, selectors, functionNames, functionArguments,
	)
	if err != nil {
		return nil, fmt.Errorf("resolve explicit functions: %w", err)
	}
	defer rows.Close()
	objects := make([]discoveredObject, 0, len(rule.names))
	for rows.Next() {
		var (
			selectorName string
			name         pgtype.Text
			arguments    pgtype.Text
		)
		if err := rows.Scan(&selectorName, &name, &arguments); err != nil {
			return nil, fmt.Errorf("scan explicit function: %w", err)
		}
		if !name.Valid && !arguments.Valid {
			continue
		}
		if !name.Valid || !arguments.Valid {
			return nil, fmt.Errorf("resolve function %q returned incomplete identity", selectorName)
		}
		objects = append(objects, discoveredObject{
			name: name.String, functionArguments: arguments.String, selectorName: selectorName,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read explicit functions: %w", err)
	}
	rows.Close()
	sort.Slice(objects, func(i, j int) bool {
		return objects[i].name+"("+objects[i].functionArguments+")" <
			objects[j].name+"("+objects[j].functionArguments+")"
	})
	return objects, nil
}

func missingExplicitNames(names []string, objects []discoveredObject) []string {
	found := make(map[string]struct{}, len(objects))
	for _, object := range objects {
		found[object.name] = struct{}{}
	}
	var missing []string
	for _, name := range names {
		if _, ok := found[name]; !ok {
			missing = append(missing, name)
		}
	}
	return missing
}

func missingExplicitFunctionNames(names []string, objects []discoveredObject) []string {
	found := make(map[string]struct{}, len(objects))
	for _, object := range objects {
		found[object.selectorName] = struct{}{}
	}
	var missing []string
	for _, name := range names {
		if _, ok := found[name]; !ok {
			missing = append(missing, name)
		}
	}
	return missing
}

func mergeResolvedRules(input []resolvedRule) ([]resolvedRule, error) {
	sort.Slice(input, func(i, j int) bool { return input[i].sortKey() < input[j].sortKey() })
	merged := make([]resolvedRule, 0, len(input))
	for _, rule := range input {
		if len(merged) == 0 || merged[len(merged)-1].sortKey() != rule.sortKey() {
			merged = append(merged, rule)
			continue
		}
		previous := &merged[len(merged)-1]
		if !resolvedContractsEqual(*previous, rule) {
			return nil, fmt.Errorf(
				"policies %s define conflicting rules for %s %q",
				formatConflictingPolicies(previous.policyNames, rule.policyNames), rule.kind, rule.identityName(),
			)
		}
		previous.policyNames = append(previous.policyNames, rule.policyNames...)
		previous.policyNames = sortedUnique(previous.policyNames)
	}
	return merged, nil
}

func resolvedContractsEqual(left, right resolvedRule) bool {
	return left.optional == right.optional &&
		slices.Equal(left.required, right.required) &&
		slices.Equal(left.forbidden, right.forbidden) &&
		slices.Equal(left.publicForbidden, right.publicForbidden)
}

func (r validatedRule) selectorDescription() string {
	if r.namePrefix != "" {
		return fmt.Sprintf("prefix %q", r.namePrefix)
	}
	return fmt.Sprintf("names %q", r.names)
}

func (r resolvedRule) identityName() string {
	if r.kind == KindFunction {
		return r.name + "(" + r.functionArguments + ")"
	}
	return r.name
}

func (r resolvedRule) sortKey() string {
	order := map[ObjectKind]string{KindTable: "1", KindSequence: "2", KindFunction: "3"}
	return order[r.kind] + "\x00" + r.identityName()
}

func sortedUnique(values []string) []string {
	sort.Strings(values)
	return slices.Compact(values)
}

func formatConflictingPolicies(left, right []string) string {
	combined := sortedUnique(append(slices.Clone(left), right...))
	quoted := make([]string, 0, len(combined))
	for _, name := range combined {
		quoted = append(quoted, fmt.Sprintf("%q", name))
	}
	return strings.Join(quoted, " and ")
}
