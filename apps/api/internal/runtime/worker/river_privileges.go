package worker

import dbprivileges "ideaboard/api/internal/platform/database/privileges"

var riverRuntimeTableNames = []string{
	"river_client",
	"river_client_queue",
	"river_job",
	"river_leader",
	"river_queue",
}

var riverRuntimeSequenceNames = []string{"river_job_id_seq"}

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
		Name: "River",
		Objects: []dbprivileges.ObjectRule{
			{
				Kind:  dbprivileges.KindTable,
				Names: riverRuntimeTableNames,
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
				PublicForbidden: allTablePrivileges,
			},
			{
				Kind:     dbprivileges.KindTable,
				Names:    []string{"river_migration"},
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
				Names:           riverRuntimeSequenceNames,
				Required:        []dbprivileges.Privilege{dbprivileges.PrivilegeUsage, dbprivileges.PrivilegeSelect, dbprivileges.PrivilegeUpdate},
				PublicForbidden: []dbprivileges.Privilege{dbprivileges.PrivilegeUsage, dbprivileges.PrivilegeSelect, dbprivileges.PrivilegeUpdate},
			},
		},
	}
}
