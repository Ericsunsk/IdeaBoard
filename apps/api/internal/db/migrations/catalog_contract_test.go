package migrations

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestVerifySchemaSnapshotContractMatchesMigrationChain(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()

	snapshot, err := os.ReadFile("../schema/schema.sql")
	if err != nil {
		t.Fatal(err)
	}
	if err := VerifySchemaSnapshotContract(
		ctx,
		openIsolatedMigrationConn(t, ctx),
		openIsolatedMigrationConn(t, ctx),
		snapshot,
	); err != nil {
		t.Fatal(err)
	}
}

func TestCompareSchemaContractsReportsObjectAndPropertyDrift(t *testing.T) {
	leftContract := schemaContract{properties: map[catalogProperty]string{
		{kind: "column", object: "example_items.name", property: "nullable"}: "false",
	}}
	rightContract := schemaContract{properties: map[catalogProperty]string{
		{kind: "column", object: "example_items.name", property: "nullable"}: "true",
	}}
	err := compareSchemaContracts(leftContract, rightContract)
	if err == nil {
		t.Fatal("compareSchemaContracts unexpectedly succeeded")
	}
	if got := err.Error(); !strings.Contains(got, "column example_items.name nullable") || !strings.Contains(got, `snapshot="false"`) || !strings.Contains(got, `migration="true"`) {
		t.Fatalf("drift error=%q", got)
	}
}
