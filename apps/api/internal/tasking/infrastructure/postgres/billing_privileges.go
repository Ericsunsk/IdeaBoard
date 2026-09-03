package postgres

import dbprivileges "ideaboard/api/internal/platform/database/privileges"

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
	readInsert := []dbprivileges.Privilege{
		dbprivileges.PrivilegeSelect,
		dbprivileges.PrivilegeInsert,
	}
	immutableAfterInsert := []dbprivileges.Privilege{
		dbprivileges.PrivilegeUpdate,
		dbprivileges.PrivilegeDelete,
		dbprivileges.PrivilegeTruncate,
		dbprivileges.PrivilegeReferences,
		dbprivileges.PrivilegeTrigger,
	}
	return dbprivileges.Policy{
		Name: "Task Billing",
		Objects: []dbprivileges.ObjectRule{
			{
				Kind:            dbprivileges.KindTable,
				Names:           []string{"task_submission_claims", "task_billing_snapshots"},
				Required:        readInsert,
				Forbidden:       immutableAfterInsert,
				PublicForbidden: allTablePrivileges,
			},
			{
				Kind:  dbprivileges.KindTable,
				Names: []string{"task_billing_settlements"},
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
				Kind: dbprivileges.KindFunction,
				Names: []string{
					"validate_task_billing_snapshot_insert()",
					"prevent_task_billing_snapshot_mutation()",
					"guard_task_billing_settlement_transition()",
					"guard_task_submission_claim()",
				},
				Forbidden:       []dbprivileges.Privilege{dbprivileges.PrivilegeExecute},
				PublicForbidden: []dbprivileges.Privilege{dbprivileges.PrivilegeExecute},
			},
		},
	}
}
