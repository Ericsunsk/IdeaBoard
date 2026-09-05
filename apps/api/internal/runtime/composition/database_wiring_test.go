package composition

import (
	"strings"
	"testing"

	"ideaboard/api/internal/platform/config"
)

func TestSeparatedMigrationDatabaseConfigRequiresDistinctOwnerCredentials(t *testing.T) {
	_, err := separatedMigrationDatabaseConfig(config.DatabaseConfig{User: "runtime"})
	if err == nil || !strings.Contains(err.Error(), "required") {
		t.Fatalf("missing migration credentials error = %v", err)
	}

	_, err = separatedMigrationDatabaseConfig(config.DatabaseConfig{
		User:              "same-role",
		MigrationUser:     "same-role",
		MigrationPassword: "secret",
	})
	if err == nil || !strings.Contains(err.Error(), "must differ") {
		t.Fatalf("same-role migration credentials error = %v", err)
	}

	result, err := separatedMigrationDatabaseConfig(config.DatabaseConfig{
		User:              "runtime",
		Password:          "runtime-secret",
		MigrationUser:     "owner",
		MigrationPassword: "owner-secret",
	})
	if err != nil {
		t.Fatalf("separatedMigrationDatabaseConfig() error = %v", err)
	}
	if result.User != "owner" || result.Password != "owner-secret" {
		t.Fatalf("migration config = %+v", result)
	}
}
