package catalog

import dbprivileges "ideaboard/api/internal/platform/database/privileges"

// serviceModelPoolTableNames lists the generation catalog tables the API
// runtime reads and writes directly. Both ListPool (admin reads) and
// ReplacePool (admin writes, optimistic version check) run as the runtime
// role, so both SELECT and write privileges are required here — unlike
// Billing, which routes writes through SECURITY DEFINER functions.
var serviceModelPoolTableNames = []string{
	"service_model_pool_entry",
	"service_model_pool_resolution",
	"service_model_pool_version",
}

// serviceModelPoolSequenceNames lists the identity backing sequences the API
// runtime needs USAGE on: inserts omit the id column and take the generated
// value, so the table grants alone are not sufficient.
var serviceModelPoolSequenceNames = []string{
	"service_model_pool_entry_id_seq",
}

func RuntimePrivilegePolicy() dbprivileges.Policy {
	return dbprivileges.Policy{
		Name: "GenerationCatalog",
		Objects: []dbprivileges.ObjectRule{
			{
				Kind:  dbprivileges.KindTable,
				Names: serviceModelPoolTableNames,
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
				PublicForbidden: []dbprivileges.Privilege{
					dbprivileges.PrivilegeSelect,
					dbprivileges.PrivilegeInsert,
					dbprivileges.PrivilegeUpdate,
					dbprivileges.PrivilegeDelete,
					dbprivileges.PrivilegeTruncate,
					dbprivileges.PrivilegeReferences,
					dbprivileges.PrivilegeTrigger,
				},
			},
			{
				Kind:  dbprivileges.KindSequence,
				Names: serviceModelPoolSequenceNames,
				Required: []dbprivileges.Privilege{
					dbprivileges.PrivilegeUsage,
					dbprivileges.PrivilegeSelect,
					dbprivileges.PrivilegeUpdate,
				},
				PublicForbidden: []dbprivileges.Privilege{
					dbprivileges.PrivilegeUsage,
					dbprivileges.PrivilegeSelect,
					dbprivileges.PrivilegeUpdate,
				},
			},
		},
	}
}
