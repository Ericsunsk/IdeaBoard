package infrastructure

import dbprivileges "ideaboard/api/internal/platform/database/privileges"

func RuntimePrivilegePolicy() dbprivileges.Policy {
	all := []dbprivileges.Privilege{
		dbprivileges.PrivilegeSelect,
		dbprivileges.PrivilegeInsert,
		dbprivileges.PrivilegeUpdate,
		dbprivileges.PrivilegeDelete,
		dbprivileges.PrivilegeTruncate,
		dbprivileges.PrivilegeReferences,
		dbprivileges.PrivilegeTrigger,
	}
	return dbprivileges.Policy{
		Name: "Search Catalog",
		Objects: []dbprivileges.ObjectRule{{
			Kind:  dbprivileges.KindTable,
			Names: []string{"ai_search_operations"},
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
			PublicForbidden: all,
		}},
	}
}
