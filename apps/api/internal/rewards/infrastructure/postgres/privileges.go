package postgres

import (
	dbprivileges "ideaboard/api/internal/platform/database/privileges"
)

// RuntimePrivilegePolicy returns the runtime privilege policy for Rewards.
func RuntimePrivilegePolicy() dbprivileges.Policy {
	allTablePrivileges := []dbprivileges.Privilege{
		dbprivileges.PrivilegeSelect, dbprivileges.PrivilegeInsert,
		dbprivileges.PrivilegeUpdate, dbprivileges.PrivilegeDelete,
		dbprivileges.PrivilegeTruncate, dbprivileges.PrivilegeReferences,
		dbprivileges.PrivilegeTrigger,
	}
	immutableAfterInsert := []dbprivileges.Privilege{
		dbprivileges.PrivilegeUpdate, dbprivileges.PrivilegeDelete,
		dbprivileges.PrivilegeTruncate, dbprivileges.PrivilegeReferences,
		dbprivileges.PrivilegeTrigger,
	}
	return dbprivileges.Policy{
		Name: "Rewards",
		Objects: []dbprivileges.ObjectRule{
			{
				Kind: dbprivileges.KindTable, Names: []string{"reward_codes"},
				Required: []dbprivileges.Privilege{
					dbprivileges.PrivilegeSelect, dbprivileges.PrivilegeInsert,
				},
				Forbidden: []dbprivileges.Privilege{
					dbprivileges.PrivilegeUpdate, dbprivileges.PrivilegeDelete, dbprivileges.PrivilegeTruncate,
					dbprivileges.PrivilegeReferences, dbprivileges.PrivilegeTrigger,
				},
				PublicForbidden: allTablePrivileges,
			},
			{
				Kind: dbprivileges.KindTable, Names: []string{"reward_redemptions", "reward_admin_actions"},
				Required:  []dbprivileges.Privilege{dbprivileges.PrivilegeSelect, dbprivileges.PrivilegeInsert},
				Forbidden: immutableAfterInsert, PublicForbidden: allTablePrivileges,
			},
			{
				Kind: dbprivileges.KindTable, Names: []string{"reward_code_digest_aliases"},
				Required: []dbprivileges.Privilege{dbprivileges.PrivilegeSelect, dbprivileges.PrivilegeInsert},
				Forbidden: []dbprivileges.Privilege{
					dbprivileges.PrivilegeUpdate, dbprivileges.PrivilegeDelete, dbprivileges.PrivilegeTruncate,
					dbprivileges.PrivilegeReferences, dbprivileges.PrivilegeTrigger,
				},
				PublicForbidden: allTablePrivileges,
			},
			{
				Kind: dbprivileges.KindSequence, Names: []string{
					"reward_codes_id_seq",
					"reward_redemptions_id_seq",
					"reward_admin_actions_id_seq",
				},
				Required: []dbprivileges.Privilege{
					dbprivileges.PrivilegeUsage, dbprivileges.PrivilegeSelect, dbprivileges.PrivilegeUpdate,
				},
				PublicForbidden: []dbprivileges.Privilege{
					dbprivileges.PrivilegeUsage, dbprivileges.PrivilegeSelect, dbprivileges.PrivilegeUpdate,
				},
			},
			{
				Kind: dbprivileges.KindFunction, Names: []string{
					"prevent_reward_admin_action_mutation()",
					"guard_reward_code_identity()",
				},
				Forbidden:       []dbprivileges.Privilege{dbprivileges.PrivilegeExecute},
				PublicForbidden: []dbprivileges.Privilege{dbprivileges.PrivilegeExecute},
			},
			{
				Kind: dbprivileges.KindFunction, Names: []string{
					"update_reward_code_definition_v1(bigint,text,bigint,bigint,timestamp with time zone,bigint)",
					"set_reward_code_enabled_v1(bigint,boolean,bigint)",
					"rotate_reward_code_v1(bigint,text,bytea,text,text,bytea,bigint)",
					"delete_reward_code_v1(bigint,bigint)",
					"reserve_reward_code_redemption_v1(bigint)",
				},
				Required:        []dbprivileges.Privilege{dbprivileges.PrivilegeExecute},
				PublicForbidden: []dbprivileges.Privilege{dbprivileges.PrivilegeExecute},
			},
		},
	}
}
