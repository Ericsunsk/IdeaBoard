package postgres

import dbprivileges "ideaboard/api/internal/platform/database/privileges"

var billingTableNames = []string{
	"billing_skus",
	"billing_price_versions",
	"billing_current_prices",
}

var billingNoDirectAccessTableNames = []string{"billing_admin_actions"}

// billingNoDirectAccessSequenceNames lists the Billing-domain identity
// sequences the runtime must never touch directly. This stays an explicit
// list (not a billing_ prefix) because the tenancy billing_accounts
// identity sequence shares that prefix but is owned by the API runtime
// policy; a prefix selector would merge-conflict with it at Configure and
// Verify time.
var billingNoDirectAccessSequenceNames = []string{
	"billing_admin_actions_id_seq",
	"billing_price_versions_price_version_id_seq",
}

var billingCommandFunctionSignatures = []string{
	"publish_billing_prices(TEXT, TEXT, TEXT, JSONB, BIGINT)",
	"set_billing_item_availability(TEXT, BOOLEAN, BIGINT)",
	"delete_archived_billing_item(TEXT, BIGINT)",
}

var billingInternalFunctionSignatures = []string{
	"prevent_billing_sku_identity_mutation()",
	"prevent_billing_price_version_mutation()",
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
				Names:           billingNoDirectAccessSequenceNames,
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
