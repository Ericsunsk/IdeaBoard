package billing

import dbprivileges "ideaboard/api/internal/platform/database/privileges"

var billingTableNames = []string{
	"billing_skus",
	"billing_price_versions",
	"billing_price_version_states",
}

var billingNoDirectAccessTableNames = []string{"billing_admin_actions"}

var billingCommandFunctionSignatures = []string{
	"create_billing_sku(TEXT, TEXT, TEXT, BOOLEAN, BIGINT, TEXT)",
	"update_billing_sku(TEXT, TEXT, TEXT, BOOLEAN, BIGINT, TEXT)",
	"create_billing_price_version(TEXT, BIGINT, BOOLEAN, BIGINT, TEXT)",
	"set_billing_price_version_enabled(TEXT, BIGINT, BOOLEAN, BIGINT, TEXT)",
}

var billingInternalFunctionSignatures = []string{
	"prevent_billing_sku_identity_mutation()",
	"prevent_billing_price_version_mutation()",
	"prevent_billing_price_state_delete()",
	"prevent_billing_admin_action_mutation()",
}

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
	return dbprivileges.Policy{
		Name: "Billing",
		Objects: []dbprivileges.ObjectRule{
			{
				Kind:     dbprivileges.KindTable,
				Names:    billingTableNames,
				Required: []dbprivileges.Privilege{dbprivileges.PrivilegeSelect},
				Forbidden: []dbprivileges.Privilege{
					dbprivileges.PrivilegeInsert,
					dbprivileges.PrivilegeUpdate,
					dbprivileges.PrivilegeDelete,
					dbprivileges.PrivilegeTruncate,
					dbprivileges.PrivilegeReferences,
					dbprivileges.PrivilegeTrigger,
				},
				PublicForbidden: allTablePrivileges,
			},
			{
				Kind:            dbprivileges.KindTable,
				Names:           billingNoDirectAccessTableNames,
				Forbidden:       allTablePrivileges,
				PublicForbidden: allTablePrivileges,
			},
			{
				Kind:            dbprivileges.KindSequence,
				NamePrefix:      "billing_",
				Optional:        true,
				Forbidden:       []dbprivileges.Privilege{dbprivileges.PrivilegeUsage, dbprivileges.PrivilegeSelect, dbprivileges.PrivilegeUpdate},
				PublicForbidden: []dbprivileges.Privilege{dbprivileges.PrivilegeUsage, dbprivileges.PrivilegeSelect, dbprivileges.PrivilegeUpdate},
			},
			{
				Kind:            dbprivileges.KindFunction,
				Names:           billingCommandFunctionSignatures,
				Required:        []dbprivileges.Privilege{dbprivileges.PrivilegeExecute},
				PublicForbidden: []dbprivileges.Privilege{dbprivileges.PrivilegeExecute},
			},
			{
				Kind:            dbprivileges.KindFunction,
				Names:           billingInternalFunctionSignatures,
				Forbidden:       []dbprivileges.Privilege{dbprivileges.PrivilegeExecute},
				PublicForbidden: []dbprivileges.Privilege{dbprivileges.PrivilegeExecute},
			},
		},
	}
}
