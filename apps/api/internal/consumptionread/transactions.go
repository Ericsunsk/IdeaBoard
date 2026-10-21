// Package consumptionread owns PostgreSQL consumption projections: settled
// attribution for Usage/Billing Records and occupied Used Credits for caps.
// These retain distinct time and pending-consumption semantics. It reads facts only;
// Ledger, Tasking, and SearchCatalog retain their respective write ownership.
package consumptionread

import "fmt"

// Transactions returns an inline SQL relation containing Ledger transactions and
// their nullable consumption_display_name. workspaceParameter is the one-based
// bind ordinal for the authorized Workspace filter; the bound Workspace ID
// is zero for an account/member read. This filters by persisted consumption origin, never by
// live Workspace ownership. Unknown/mismatched origins are excluded from a
// Workspace read, while account/member reads retain the original Ledger facts.
// Callers still apply their account/actor authorization and keyset pagination.
//
// Keeping the complete filter inside this relation lets PostgreSQL push scope
// predicates before LIMIT and choose hashed EXISTS plans for many Captures.
// Unscoped aggregates skip all origin work and retain their covering-index path.
// The bind ordinal is formatted as an integer; no SQL identifiers or user text
// are interpolated. The relation owns all Task/Vision reference/join knowledge.
func Transactions(workspaceParameter int) string {
	if workspaceParameter <= 0 {
		panic("consumptionread: Workspace bind ordinal must be positive")
	}
	return fmt.Sprintf(`(
  SELECT ledger.*,
         CASE WHEN ledger.command_type = 'capture' AND ledger.business_reference_type = 'task' THEN (
           SELECT snapshots.display_name
           FROM task_billing_snapshots snapshots
           WHERE snapshots.task_id = ledger.business_reference_id
             AND snapshots.billing_account_id = ledger.billing_account_id
         ) END AS consumption_display_name
  FROM credit_transactions ledger
  WHERE ($%[1]d::bigint = 0 OR (ledger.command_type = 'capture' AND (
    (ledger.business_reference_type = 'task' AND EXISTS (
      SELECT 1 FROM task_billing_snapshots snapshots
      WHERE snapshots.task_id = ledger.business_reference_id
        AND snapshots.billing_account_id = ledger.billing_account_id
        AND snapshots.workspace_id = $%[1]d
    )) OR (ledger.business_reference_type = 'ai_search_operation' AND EXISTS (
      SELECT 1 FROM ai_search_operations operations
      WHERE operations.operation_id::text = ledger.business_reference_id
        AND operations.billing_account_id = ledger.billing_account_id
        AND operations.workspace_id = $%[1]d
    ))
  )))
)`, workspaceParameter)
}
