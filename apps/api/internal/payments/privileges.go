package payments

import dbprivileges "ideaboard/api/internal/platform/database/privileges"

func RuntimePrivilegePolicy() dbprivileges.Policy {
	allTables := []dbprivileges.Privilege{
		dbprivileges.PrivilegeSelect, dbprivileges.PrivilegeInsert, dbprivileges.PrivilegeUpdate,
		dbprivileges.PrivilegeDelete, dbprivileges.PrivilegeTruncate, dbprivileges.PrivilegeReferences,
		dbprivileges.PrivilegeTrigger,
	}
	allSequences := []dbprivileges.Privilege{
		dbprivileges.PrivilegeUsage, dbprivileges.PrivilegeSelect, dbprivileges.PrivilegeUpdate,
	}
	return dbprivileges.Policy{
		Name: "Payments",
		Objects: []dbprivileges.ObjectRule{
			{
				Kind: dbprivileges.KindTable, Names: []string{
					"payment_stripe_configurations", "payment_webhook_events", "payment_customers", "payment_orders", "payment_order_reconciliations",
				},
				Required:        []dbprivileges.Privilege{dbprivileges.PrivilegeSelect, dbprivileges.PrivilegeInsert, dbprivileges.PrivilegeUpdate},
				Forbidden:       []dbprivileges.Privilege{dbprivileges.PrivilegeDelete, dbprivileges.PrivilegeTruncate, dbprivileges.PrivilegeReferences, dbprivileges.PrivilegeTrigger},
				PublicForbidden: allTables,
			},
			{
				Kind: dbprivileges.KindTable, Names: []string{"payment_admin_actions"},
				Required:        []dbprivileges.Privilege{dbprivileges.PrivilegeSelect, dbprivileges.PrivilegeInsert},
				Forbidden:       []dbprivileges.Privilege{dbprivileges.PrivilegeUpdate, dbprivileges.PrivilegeDelete, dbprivileges.PrivilegeTruncate, dbprivileges.PrivilegeReferences, dbprivileges.PrivilegeTrigger},
				PublicForbidden: allTables,
			},
			{
				Kind: dbprivileges.KindSequence, Names: []string{"payment_admin_actions_id_seq"},
				Required: allSequences, PublicForbidden: allSequences,
			},
			{
				Kind: dbprivileges.KindFunction, Names: []string{
					"prevent_payment_admin_action_mutation()", "guard_payment_order_lifecycle()", "initialize_payment_order_reconciliation()",
				},
				Forbidden:       []dbprivileges.Privilege{dbprivileges.PrivilegeExecute},
				PublicForbidden: []dbprivileges.Privilege{dbprivileges.PrivilegeExecute},
			},
		},
	}
}
