package postgres

import (
	"slices"
	"testing"

	dbprivileges "ideaboard/api/internal/platform/database/privileges"
)

func TestRuntimePrivilegePolicyIncludesPaymentCheckoutShutdowns(t *testing.T) {
	policy := RuntimePrivilegePolicy()
	for _, rule := range policy.Objects {
		if rule.Kind != dbprivileges.KindTable || !slices.Contains(rule.Names, "payment_checkout_shutdowns") {
			continue
		}
		for _, required := range []dbprivileges.Privilege{
			dbprivileges.PrivilegeSelect,
			dbprivileges.PrivilegeInsert,
			dbprivileges.PrivilegeUpdate,
		} {
			if !slices.Contains(rule.Required, required) {
				t.Fatalf("payment checkout shutdowns policy lacks required privilege %s", required)
			}
		}
		for _, forbidden := range []dbprivileges.Privilege{
			dbprivileges.PrivilegeDelete,
			dbprivileges.PrivilegeTruncate,
			dbprivileges.PrivilegeReferences,
			dbprivileges.PrivilegeTrigger,
		} {
			if !slices.Contains(rule.Forbidden, forbidden) {
				t.Fatalf("payment checkout shutdowns policy lacks forbidden privilege %s", forbidden)
			}
		}
		return
	}
	t.Fatal("payment checkout shutdowns table is missing from the Payments runtime privilege policy")
}
