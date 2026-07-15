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
				Kind: dbprivileges.KindTable, Names: []string{"reward_campaigns"},
				Required: []dbprivileges.Privilege{
					dbprivileges.PrivilegeSelect, dbprivileges.PrivilegeInsert,
					dbprivileges.PrivilegeUpdate, dbprivileges.PrivilegeDelete,
				},
				Forbidden: []dbprivileges.Privilege{
					dbprivileges.PrivilegeTruncate,
					dbprivileges.PrivilegeReferences, dbprivileges.PrivilegeTrigger,
				},
				PublicForbidden: allTablePrivileges,
			},
			{
				Kind:      dbprivileges.KindTable,
				Names:     []string{"reward_redemptions", "reward_admin_actions"},
				Required:  []dbprivileges.Privilege{dbprivileges.PrivilegeSelect, dbprivileges.PrivilegeInsert},
				Forbidden: immutableAfterInsert, PublicForbidden: allTablePrivileges,
			},
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
				Kind: dbprivileges.KindTable, Names: []string{"reward_code_batches"},
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
					"guard_reward_campaign_rule_mutation()",
					"guard_reward_code_mutation()",
					"validate_reward_code_mode()",
					"guard_reward_code_batch_mutation()",
					"project_reward_code_counts()",
					"guard_single_use_reward_redemption()",
					"consume_single_use_reward_code()",
				},
				Forbidden:       []dbprivileges.Privilege{dbprivileges.PrivilegeExecute},
				PublicForbidden: []dbprivileges.Privilege{dbprivileges.PrivilegeExecute},
			},
		},
	}
}
