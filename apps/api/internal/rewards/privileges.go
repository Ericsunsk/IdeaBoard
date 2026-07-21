package rewards

import dbprivileges "ideaboard/api/internal/platform/database/privileges"

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
					dbprivileges.PrivilegeSelect, dbprivileges.PrivilegeInsert, dbprivileges.PrivilegeUpdate,
				},
				Forbidden: []dbprivileges.Privilege{
					dbprivileges.PrivilegeDelete, dbprivileges.PrivilegeTruncate,
					dbprivileges.PrivilegeReferences, dbprivileges.PrivilegeTrigger,
				},
				PublicForbidden: allTablePrivileges,
			},
			{
				Kind: dbprivileges.KindTable, Names: []string{"reward_redemptions", "reward_admin_actions", "reward_code_digest_aliases"},
				Required:  []dbprivileges.Privilege{dbprivileges.PrivilegeSelect, dbprivileges.PrivilegeInsert},
				Forbidden: immutableAfterInsert, PublicForbidden: allTablePrivileges,
			},
			{
				Kind: dbprivileges.KindSequence, NamePrefix: "reward_", Optional: true,
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
		},
	}
}
