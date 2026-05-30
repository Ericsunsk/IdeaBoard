package accounts

import (
	"fmt"
	"testing"

	"api/internal/tasks"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func newAccountsTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := fmt.Sprintf("file:task_runtime_%s?mode=memory&cache=shared", t.Name())
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite db failed: %v", err)
	}
	if err := db.AutoMigrate(&tasks.Task{}, &tasks.TaskOutput{}); err != nil {
		t.Fatalf("auto migrate task failed: %v", err)
	}
	if err := db.AutoMigrate(&TaskDailyStat{}); err != nil {
		t.Fatalf("auto migrate task_daily_stats failed: %v", err)
	}
	if err := db.AutoMigrate(&TaskStatEvent{}, &TaskDailyStatBackfill{}); err != nil {
		t.Fatalf("auto migrate stat event/backfill failed: %v", err)
	}
	return db
}
