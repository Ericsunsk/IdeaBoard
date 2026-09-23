package database

import dbprivileges "ideaboard/api/internal/platform/database/privileges"

var apiOwnedTableNames = []string{
	"api_account_secrets",
	"admin_brand_logo_overrides",
	"api_accounts",
	"board_members",
	"board_share_tokens",
	"boards",
	"feature_prompts",
	"library_assets",
	"library_asset_media_assets",
	"library_folders",
	"media_assets",
	"media_asset_sources",
	"media_derivatives",
	"media_refs",
	"provider_endpoints",
	"storage_profile_secret_versions",
	"storage_profiles",
	"system_configs",
	"task_cancellations",
	"task_daily_stat_backfills",
	"task_daily_stats",
	"task_execution_attempts",
	"task_execution_metrics",
	"task_outputs",
	"task_stat_events",
	"tasks",
	"users",
	"users_used_uids",
}

var apiOwnedSequenceNames = []string{
	"api_account_secrets_id_seq",
	"api_accounts_id_seq",
	"board_members_id_seq",
	"board_share_tokens_id_seq",
	"boards_id_seq",
	"library_assets_id_seq",
	"library_folders_id_seq",
	"media_assets_id_seq",
	"media_asset_sources_id_seq",
	"media_derivatives_id_seq",
	"media_refs_id_seq",
	"provider_endpoints_id_seq",
	"storage_profile_secret_versions_id_seq",
	"system_configs_id_seq",
	"task_daily_stat_backfills_id_seq",
	"task_execution_attempts_id_seq",
	"task_daily_stats_id_seq",
	"task_outputs_id_seq",
	"task_stat_events_id_seq",
	"tasks_id_seq",
	"users_id_seq",
	"users_used_uids_id_seq",
}

var protectedAuthTableNames = []string{"user", "session", "account", "verification"}

func RuntimePrivilegePolicy() dbprivileges.Policy {
	allTablePrivileges := []dbprivileges.Privilege{
		dbprivileges.PrivilegeSelect,
		dbprivileges.PrivilegeInsert,
		dbprivileges.PrivilegeUpdate,
		dbprivileges.PrivilegeDelete,
		dbprivileges.PrivilegeTruncate,
		dbprivileges.PrivilegeReferences,
		dbprivileges.PrivilegeTrigger,
	}
	allSequencePrivileges := []dbprivileges.Privilege{
		dbprivileges.PrivilegeUsage,
		dbprivileges.PrivilegeSelect,
		dbprivileges.PrivilegeUpdate,
	}
	return dbprivileges.Policy{
		Name:               "API runtime",
		RequireSchemaUsage: true,
		Objects: []dbprivileges.ObjectRule{
			{
				Kind:  dbprivileges.KindTable,
				Names: apiOwnedTableNames,
				Required: []dbprivileges.Privilege{
					dbprivileges.PrivilegeSelect,
					dbprivileges.PrivilegeInsert,
					dbprivileges.PrivilegeUpdate,
					dbprivileges.PrivilegeDelete,
				},
				Forbidden: []dbprivileges.Privilege{
					dbprivileges.PrivilegeTruncate,
					dbprivileges.PrivilegeReferences,
					dbprivileges.PrivilegeTrigger,
				},
				PublicForbidden: allTablePrivileges,
			},
			{
				Kind:            dbprivileges.KindSequence,
				Names:           apiOwnedSequenceNames,
				Required:        allSequencePrivileges,
				PublicForbidden: allSequencePrivileges,
			},
			{
				Kind:  dbprivileges.KindTable,
				Names: []string{"user_deletion_requests"},
				Required: []dbprivileges.Privilege{
					dbprivileges.PrivilegeSelect,
					dbprivileges.PrivilegeInsert,
					dbprivileges.PrivilegeUpdate,
				},
				Forbidden: []dbprivileges.Privilege{
					dbprivileges.PrivilegeDelete,
					dbprivileges.PrivilegeTruncate,
					dbprivileges.PrivilegeReferences,
					dbprivileges.PrivilegeTrigger,
				},
				PublicForbidden: allTablePrivileges,
			},
			{
				Kind:  dbprivileges.KindTable,
				Names: []string{"system_settings_admin_actions"},
				Required: []dbprivileges.Privilege{
					dbprivileges.PrivilegeSelect,
					dbprivileges.PrivilegeInsert,
				},
				Forbidden: []dbprivileges.Privilege{
					dbprivileges.PrivilegeUpdate,
					dbprivileges.PrivilegeDelete,
					dbprivileges.PrivilegeTruncate,
					dbprivileges.PrivilegeReferences,
					dbprivileges.PrivilegeTrigger,
				},
				PublicForbidden: allTablePrivileges,
			},
			{
				Kind:            dbprivileges.KindSequence,
				Names:           []string{"system_settings_admin_actions_id_seq"},
				Required:        []dbprivileges.Privilege{dbprivileges.PrivilegeUsage, dbprivileges.PrivilegeSelect},
				Forbidden:       []dbprivileges.Privilege{dbprivileges.PrivilegeUpdate},
				PublicForbidden: allSequencePrivileges,
			},
			{
				Kind:            dbprivileges.KindTable,
				Names:           protectedAuthTableNames,
				Optional:        true,
				Forbidden:       allTablePrivileges,
				PublicForbidden: allTablePrivileges,
			},
		},
	}
}
