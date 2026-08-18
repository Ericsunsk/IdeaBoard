package schemacontract

import (
	"strings"
	"testing"
)

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
