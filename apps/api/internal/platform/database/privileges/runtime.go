// Package privileges owns PostgreSQL privilege configuration and
// verification (policy engine) together with the API runtime policy
// (this file): one layer, one home. The former parent-level
// database.RuntimePrivilegePolicy was sunk here so policy ownership no
// longer splits across database/ and database/privileges.
package privileges

var apiOwnedTableNames = []string{
	"admin_brand_logo_overrides",
	"billing_accounts",
	"board_access_event_outbox",
	"board_documents",
	"board_document_media_refs",
	"board_memberships",
	"board_revisions",
	"board_y_updates",
	"boards",
	// control_idempotency_records shares the full-DML grant: Claim inserts
	// the in-progress fact, Complete/Abandon settle it, and PurgeExpired
	// reclaims expired completions (#891). Same privilege shape as the
	// api-owned tables, so it rides this rule instead of a duplicate.
	"control_idempotency_records",
	"event_deliveries",
	"event_delivery_attempts",
	"event_subscriptions",
	"feature_prompts",
	"library_assets",
	"library_asset_media_assets",
	"library_folders",
	"library_materializations",
	"media_assets",
	"organization_memberships",
	"organizations",
	"storage_profile_secret_versions",
	"storage_profiles",
	"system_configs",
	"task_billing_settlements",
	"task_billing_snapshots",
	"task_cancellations",
	"task_daily_stat_backfills",
	"task_daily_stats",
	"task_execution_attempts",
	"task_execution_metrics",
	"task_outputs",
	"task_stat_events",
	"tasks",
	"upload_sessions",
	"users",
	"users_used_uids",
	"workspace_events",
	"workspace_memberships",
	"workspaces",
}

var apiOwnedSequenceNames = []string{
	"billing_accounts_internal_id_seq",
	"board_access_event_outbox_internal_id_seq",
	"board_revisions_internal_id_seq",
	"board_y_updates_internal_id_seq",
	"boards_internal_id_seq",
	"control_audit_records_internal_id_seq",
	"event_deliveries_internal_id_seq",
	"event_delivery_attempts_internal_id_seq",
	"event_subscriptions_internal_id_seq",
	"library_assets_internal_id_seq",
	"library_folders_internal_id_seq",
	"library_materializations_internal_id_seq",
	"media_assets_internal_id_seq",
	"organizations_internal_id_seq",
	"storage_profile_secret_versions_id_seq",
	"system_configs_id_seq",
	"task_daily_stat_backfills_id_seq",
	"task_execution_attempts_id_seq",
	"task_daily_stats_id_seq",
	"task_outputs_internal_id_seq",
	"task_stat_events_id_seq",
	"tasks_internal_id_seq",
	"upload_sessions_internal_id_seq",
	"users_id_seq",
	"users_used_uids_id_seq",
	"workspace_events_internal_id_seq",
	"workspaces_internal_id_seq",
}

var protectedAuthTableNames = []string{"user", "session", "account", "verification"}

func RuntimePrivilegePolicy() Policy {
	allTablePrivileges := []Privilege{
		PrivilegeSelect,
		PrivilegeInsert,
		PrivilegeUpdate,
		PrivilegeDelete,
		PrivilegeTruncate,
		PrivilegeReferences,
		PrivilegeTrigger,
	}
	allSequencePrivileges := []Privilege{
		PrivilegeUsage,
		PrivilegeSelect,
		PrivilegeUpdate,
	}
	return Policy{
		Name:               "API runtime",
		RequireSchemaUsage: true,
		Objects: []ObjectRule{
			{
				Kind:  KindTable,
				Names: []string{"media_asset_task_origins"},
				Required: []Privilege{
					PrivilegeSelect,
					PrivilegeInsert,
				},
				Forbidden: []Privilege{
					PrivilegeUpdate,
					PrivilegeDelete,
					PrivilegeTruncate,
					PrivilegeReferences,
					PrivilegeTrigger,
				},
				PublicForbidden: allTablePrivileges,
			},
			{
				Kind:  KindTable,
				Names: []string{"identity_admin_reconciliation_runs"},
				Required: []Privilege{
					PrivilegeSelect,
					PrivilegeInsert,
					PrivilegeUpdate,
				},
				Forbidden: []Privilege{
					PrivilegeDelete,
					PrivilegeTruncate,
					PrivilegeReferences,
					PrivilegeTrigger,
				},
				PublicForbidden: allTablePrivileges,
			},
			{
				Kind:  KindTable,
				Names: []string{"identity_admin_reconciliation_findings"},
				Required: []Privilege{
					PrivilegeSelect,
					PrivilegeInsert,
					PrivilegeUpdate,
				},
				Forbidden: []Privilege{
					PrivilegeDelete,
					PrivilegeTruncate,
					PrivilegeReferences,
					PrivilegeTrigger,
				},
				PublicForbidden: allTablePrivileges,
			},
			{
				Kind:  KindTable,
				Names: []string{"identity_admin_reconciliation_run_findings"},
				Required: []Privilege{
					PrivilegeSelect,
					PrivilegeInsert,
				},
				Forbidden: []Privilege{
					PrivilegeUpdate,
					PrivilegeDelete,
					PrivilegeTruncate,
					PrivilegeReferences,
					PrivilegeTrigger,
				},
				PublicForbidden: allTablePrivileges,
			},
			{
				Kind:  KindTable,
				Names: apiOwnedTableNames,
				Required: []Privilege{
					PrivilegeSelect,
					PrivilegeInsert,
					PrivilegeUpdate,
					PrivilegeDelete,
				},
				Forbidden: []Privilege{
					PrivilegeTruncate,
					PrivilegeReferences,
					PrivilegeTrigger,
				},
				PublicForbidden: allTablePrivileges,
			},
			{
				Kind:            KindSequence,
				Names:           apiOwnedSequenceNames,
				Required:        allSequencePrivileges,
				PublicForbidden: allSequencePrivileges,
			},
			{
				Kind:  KindTable,
				Names: []string{"user_deletion_requests"},
				Required: []Privilege{
					PrivilegeSelect,
					PrivilegeInsert,
					PrivilegeUpdate,
				},
				Forbidden: []Privilege{
					PrivilegeDelete,
					PrivilegeTruncate,
					PrivilegeReferences,
					PrivilegeTrigger,
				},
				PublicForbidden: allTablePrivileges,
			},
			{
				Kind:  KindTable,
				Names: []string{"system_settings_admin_actions"},
				Required: []Privilege{
					PrivilegeSelect,
					PrivilegeInsert,
				},
				Forbidden: []Privilege{
					PrivilegeUpdate,
					PrivilegeDelete,
					PrivilegeTruncate,
					PrivilegeReferences,
					PrivilegeTrigger,
				},
				PublicForbidden: allTablePrivileges,
			},
			{
				// Control audit is an append-only sink: the runtime writes
				// audit rows and reads them back via INSERT ... RETURNING
				// (which requires SELECT), but must never mutate history.
				// Its identity sequence stays in apiOwnedSequenceNames.
				Kind:  KindTable,
				Names: []string{"control_audit_records"},
				Required: []Privilege{
					PrivilegeSelect,
					PrivilegeInsert,
				},
				Forbidden: []Privilege{
					PrivilegeUpdate,
					PrivilegeDelete,
					PrivilegeTruncate,
					PrivilegeReferences,
					PrivilegeTrigger,
				},
				PublicForbidden: allTablePrivileges,
			},
			{
				Kind:            KindSequence,
				Names:           []string{"system_settings_admin_actions_id_seq"},
				Required:        []Privilege{PrivilegeUsage, PrivilegeSelect},
				Forbidden:       []Privilege{PrivilegeUpdate},
				PublicForbidden: allSequencePrivileges,
			},
			{
				Kind:     KindSequence,
				Names:    []string{"identity_admin_reconciliation_runs_id_seq", "identity_admin_reconciliation_findings_id_seq"},
				Required: []Privilege{PrivilegeUsage, PrivilegeSelect},
				Forbidden: []Privilege{
					PrivilegeUpdate,
				},
				PublicForbidden: allSequencePrivileges,
			},
			{
				Kind: KindFunction,
				Names: []string{
					"start_identity_admin_reconciliation(text, bigint, timestamptz, boolean)",
					"fail_identity_admin_reconciliation(bigint, text)",
				},
				Required:        []Privilege{PrivilegeExecute},
				PublicForbidden: []Privilege{PrivilegeExecute},
			},
			{
				Kind:            KindTable,
				Names:           protectedAuthTableNames,
				Optional:        true,
				Forbidden:       allTablePrivileges,
				PublicForbidden: allTablePrivileges,
			},
		},
	}
}
