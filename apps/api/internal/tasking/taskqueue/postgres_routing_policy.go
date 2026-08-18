package taskqueue

import (
	"context"
	"database/sql"
	"time"

	"ideaboard/api/internal/platform/accountlimits"
	"ideaboard/api/internal/tasking/tasks"
	"ideaboard/api/internal/workflow"
)

const taskAssignmentAgeBoostAfter = 15 * time.Minute

// chooseTaskRoutingTx keeps dispatch eligibility, capacity, load selection, and
// fairness on one PostgreSQL snapshot. The selected candidate and every
// candidate that may defer it therefore share the same locked account pool.
func chooseTaskRoutingTx(
	ctx context.Context,
	tx *sql.Tx,
	taskID string,
	current taskAssignmentTaskState,
	now time.Time,
	routing workflow.RoutingRequirement,
) (taskAssignmentSelectedAccount, bool, error) {
	currentPriority := taskAssignmentEffectivePriority(current.Priority, current.CreatedAt, now)
	ageBoostCutoff := now.Add(-taskAssignmentAgeBoostAfter)

	capability := routing.Capability()
	modelRequired := routing.ModelRequirement() == workflow.RoutingModelRequired
	outputResolution := routing.OutputResolution()
	var selected taskAssignmentSelectedAccount
	var shouldDefer bool
	err := tx.QueryRowContext(ctx, `
WITH locked_accounts AS (
  SELECT
    a.id AS account_id,
    a.provider_endpoint_id,
    pe.api_type,
    pe.protocol AS provider_endpoint_protocol,
    pe.name AS provider_endpoint_name,
    pe.base_url AS provider_endpoint_base_url,
    credential.id AS credential_id,
    credential.fingerprint AS credential_fingerprint,
    credential.key_version AS credential_key_version,
    GREATEST(LEAST(a.max_concurrent, $8), 0) AS account_max_concurrent
  FROM api_accounts AS a
  JOIN provider_endpoints AS pe ON pe.id = a.provider_endpoint_id
    AND pe.status = 1
  JOIN LATERAL (
    SELECT id, fingerprint, key_version
    FROM api_account_secrets
    WHERE account_id = a.id
      AND status = 'active'
      AND secret_material_kind = 'encrypted'
      AND btrim(secret_payload) <> ''
    ORDER BY rotated_at DESC, id DESC
    LIMIT 1
  ) AS credential ON TRUE
  WHERE a.status = 1
    AND a.deleted_at IS NULL
    AND EXISTS (
      SELECT 1
      FROM provider_endpoint_capabilities capability
      WHERE capability.provider_endpoint_id = a.provider_endpoint_id
        AND capability.capability = $9
    )
  ORDER BY a.id ASC
  FOR UPDATE OF a SKIP LOCKED
),
modeless_routing_candidates AS (
  SELECT
    a.account_id,
    a.provider_endpoint_id,
    a.api_type,
    a.provider_endpoint_protocol,
    a.provider_endpoint_name,
    a.provider_endpoint_base_url,
    a.credential_id,
    a.credential_fingerprint,
    a.credential_key_version,
    a.account_max_concurrent,
    ''::text AS model_name,
    a.account_max_concurrent AS model_max_concurrent,
    0::int AS model_sort_order,
    0::bigint AS model_id
  FROM locked_accounts a
  WHERE $10 = FALSE
),
model_routing_candidates AS (
  SELECT
    a.account_id,
    a.provider_endpoint_id,
    a.api_type,
    a.provider_endpoint_protocol,
    a.provider_endpoint_name,
    a.provider_endpoint_base_url,
    a.credential_id,
    a.credential_fingerprint,
    a.credential_key_version,
    a.account_max_concurrent,
    m.model_name,
    GREATEST(LEAST(COALESCE(m.max_concurrent, a.account_max_concurrent), $8), 0) AS model_max_concurrent,
    m.sort_order AS model_sort_order,
    m.id AS model_id
  FROM locked_accounts a
  JOIN api_account_models AS m ON m.account_id = a.account_id
    AND m.enabled = TRUE
  LEFT JOIN provider_endpoint_models AS pem
    ON pem.provider_endpoint_id = a.provider_endpoint_id
   AND lower(pem.model_name) = lower(m.model_name)
  WHERE $10 = TRUE
    AND (
      ($11 IN ('default', '1k') AND pem.supports_2k IS NOT TRUE AND pem.supports_4k IS NOT TRUE)
      OR ($11 = '2k' AND pem.supports_2k = TRUE)
      OR ($11 = '4k' AND pem.supports_4k = TRUE)
    )
),
routing_candidates AS (
  SELECT * FROM modeless_routing_candidates
  UNION ALL
  SELECT * FROM model_routing_candidates
),
active_leases AS (
  SELECT
    tl.id,
    tl.account_id,
    tl.provider_endpoint_id,
    tl.resolved_model,
    t.user_id
  FROM task_leases tl
  JOIN tasks t ON t.task_id = tl.task_id
  WHERE tl.released_at IS NULL
),
candidate_load AS (
  SELECT
    candidate.account_id,
    candidate.provider_endpoint_id,
    candidate.api_type,
    candidate.provider_endpoint_protocol,
    candidate.provider_endpoint_name,
    candidate.provider_endpoint_base_url,
    candidate.credential_id,
    candidate.credential_fingerprint,
    candidate.credential_key_version,
    candidate.account_max_concurrent,
    candidate.model_name,
    candidate.model_max_concurrent,
    candidate.model_sort_order,
    candidate.model_id,
    COUNT(lease.id) AS running_count,
    COUNT(lease.id) FILTER (
      WHERE lower(btrim(lease.resolved_model)) = lower(candidate.model_name)
    ) AS model_running_count
  FROM routing_candidates candidate
  LEFT JOIN active_leases lease ON lease.account_id = candidate.account_id
  GROUP BY
    candidate.account_id,
    candidate.provider_endpoint_id,
    candidate.api_type,
    candidate.provider_endpoint_protocol,
    candidate.provider_endpoint_name,
    candidate.provider_endpoint_base_url,
    candidate.credential_id,
    candidate.credential_fingerprint,
    candidate.credential_key_version,
    candidate.account_max_concurrent,
    candidate.model_name,
    candidate.model_max_concurrent,
    candidate.model_sort_order,
    candidate.model_id
),
dispatchable_candidates AS (
  SELECT *
  FROM candidate_load
  WHERE running_count < account_max_concurrent
    AND model_running_count < model_max_concurrent
),
selected_candidate AS (
  SELECT *
  FROM dispatchable_candidates
  ORDER BY
    GREATEST(
      running_count::numeric / account_max_concurrent::numeric,
      model_running_count::numeric / model_max_concurrent::numeric
    ) ASC,
    model_sort_order ASC,
    model_id ASC,
    account_id ASC
  LIMIT 1
),
active_share AS (
  SELECT
    COALESCE(bool_or(lease.user_id = $2), FALSE) AS active_user,
    COALESCE(bool_or(lease.provider_endpoint_id = selected.provider_endpoint_id), FALSE) AS active_tenant
  FROM selected_candidate selected
  LEFT JOIN active_leases lease ON (
    ($2 > 0 AND lease.user_id = $2)
    OR lease.provider_endpoint_id = selected.provider_endpoint_id
  )
  GROUP BY selected.provider_endpoint_id
),
pending_candidates AS (
  SELECT
    pending.user_id,
    pending.created_at,
    CASE
      WHEN pending.created_at <= $3 THEN $4
      ELSE pending.priority
    END AS effective_priority
  FROM tasks pending
  WHERE pending.status = $7
    AND pending.task_id <> $1
)
SELECT
  selected.account_id,
  selected.provider_endpoint_id,
  selected.credential_id,
  selected.model_name,
  selected.api_type,
  selected.provider_endpoint_protocol,
  selected.provider_endpoint_name,
  selected.provider_endpoint_base_url,
  selected.credential_fingerprint,
  selected.credential_key_version,
  EXISTS (
    SELECT 1
    FROM pending_candidates pending
    CROSS JOIN dispatchable_candidates candidate
    WHERE
      pending.effective_priority < $5
      OR (pending.effective_priority = $5 AND pending.created_at < $6)
      OR (
        (share.active_user OR share.active_tenant)
        AND pending.effective_priority <= $5
        AND (
          (share.active_user AND pending.user_id <> $2)
          OR (
            share.active_tenant
            AND candidate.provider_endpoint_id <> selected.provider_endpoint_id
          )
        )
      )
  ) AS should_defer
FROM selected_candidate selected
CROSS JOIN active_share share`,
		taskID,
		current.UserID,
		ageBoostCutoff,
		tasks.TaskPriorityInteractive,
		currentPriority,
		current.CreatedAt,
		tasks.StatusPending,
		accountlimits.MaxMaxConcurrent,
		string(capability),
		modelRequired,
		outputResolution,
	).Scan(
		&selected.AccountID,
		&selected.ProviderEndpointID,
		&selected.CredentialID,
		&selected.ResolvedModel,
		&selected.APIType,
		&selected.ProviderEndpointProtocol,
		&selected.ProviderEndpointName,
		&selected.ProviderEndpointBaseURL,
		&selected.CredentialFingerprint,
		&selected.CredentialKeyVersion,
		&shouldDefer,
	)
	if err != nil {
		return taskAssignmentSelectedAccount{}, false, err
	}
	return selected, shouldDefer, nil
}

func taskAssignmentEffectivePriority(priority int, createdAt time.Time, now time.Time) int {
	priority = tasks.EffectiveTaskPriority(priority)
	if !createdAt.IsZero() && !now.IsZero() && !createdAt.After(now.Add(-taskAssignmentAgeBoostAfter)) {
		return tasks.TaskPriorityInteractive
	}
	return priority
}
