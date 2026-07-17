package credits

import dbprivileges "ideaboard/api/internal/platform/database/privileges"

var creditTableNames = []string{
	"credit_wallets",
	"credit_accounts",
	"credit_transactions",
	"credit_postings",
	"credit_holds",
	"credit_charges",
	"credit_welcome_claims",
	"credit_welcome_identity_key_guard",
	"credit_wallet_restrictions",
	"credit_reconciliation_runs",
	"credit_reconciliation_findings",
	"credit_reconciliation_run_findings",
	"credit_reconciliation_alerts",
}

var creditInternalFunctionSignatures = []string{
	"prevent_credit_ledger_mutation()",
	"guard_credit_account_projection()",
	"apply_credit_posting_balances()",
	"validate_credit_transaction_balance()",
	"guard_credit_posting_transaction_scope()",
	"execute_credit_command_v2_before_wallet_restrictions(TEXT, BIGINT, BIGINT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT)",
}

const creditCommandFunctionSignature = "execute_credit_command_v2(TEXT, BIGINT, BIGINT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT)"

var creditLifecycleFunctionSignatures = []string{
	"bind_credit_welcome_identity_key(TEXT)",
	"provision_credit_wallet(BIGINT)",
	"set_credit_wallet_status(BIGINT, TEXT)",
	"claim_credit_welcome(TEXT, BIGINT)",
}

var creditRestrictionFunctionSignatures = []string{
	"create_credit_wallet_restriction(BIGINT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT, TEXT)",
	"release_credit_wallet_restriction(TEXT, TEXT, TEXT, TEXT)",
}

var creditReconciliationFunctionSignatures = []string{
	"start_credit_reconciliation(BIGINT, BIGINT, TEXT)",
	"execute_credit_reconciliation(BIGINT)",
	"fail_credit_reconciliation(BIGINT, TEXT)",
}

func RuntimePrivilegePolicy() dbprivileges.Policy {
	requiredFunctions := []string{creditCommandFunctionSignature}
	requiredFunctions = append(requiredFunctions, creditLifecycleFunctionSignatures...)
	requiredFunctions = append(requiredFunctions, creditRestrictionFunctionSignatures...)
	requiredFunctions = append(requiredFunctions, creditReconciliationFunctionSignatures...)
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
		Name: "Credits",
		Objects: []dbprivileges.ObjectRule{
			{
				Kind:     dbprivileges.KindTable,
				Names:    creditTableNames,
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
				Kind:            dbprivileges.KindSequence,
				NamePrefix:      "credit_",
				Optional:        true,
				Forbidden:       []dbprivileges.Privilege{dbprivileges.PrivilegeUsage, dbprivileges.PrivilegeSelect, dbprivileges.PrivilegeUpdate},
				PublicForbidden: []dbprivileges.Privilege{dbprivileges.PrivilegeUsage, dbprivileges.PrivilegeSelect, dbprivileges.PrivilegeUpdate},
			},
			{
				Kind:            dbprivileges.KindFunction,
				Names:           requiredFunctions,
				Required:        []dbprivileges.Privilege{dbprivileges.PrivilegeExecute},
				PublicForbidden: []dbprivileges.Privilege{dbprivileges.PrivilegeExecute},
			},
			{
				Kind:            dbprivileges.KindFunction,
				Names:           creditInternalFunctionSignatures,
				Forbidden:       []dbprivileges.Privilege{dbprivileges.PrivilegeExecute},
				PublicForbidden: []dbprivileges.Privilege{dbprivileges.PrivilegeExecute},
			},
		},
	}
}
