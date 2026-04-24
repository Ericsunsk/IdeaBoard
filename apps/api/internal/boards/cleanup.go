package boards

import (
	"errors"
	"fmt"
	"strings"

	mysqlDriver "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
)

func CleanupDuplicateBoardNames(db *gorm.DB) error {
	if db == nil {
		return nil
	}
	if !db.Migrator().HasTable(&UserBoard{}) {
		return nil
	}

	stmt := &gorm.Statement{DB: db}
	if err := stmt.Parse(&UserBoard{}); err != nil {
		return err
	}
	table := stmt.Schema.Table
	sql := fmt.Sprintf(
		"DELETE b1 FROM `%s` b1 JOIN `%s` b2 ON b1.user_id = b2.user_id AND b1.name = b2.name AND (b1.updated_at < b2.updated_at OR (b1.updated_at = b2.updated_at AND b1.id < b2.id))",
		table,
		table,
	)
	if err := db.Exec(sql).Error; err != nil {
		if isTableNotExistsErr(err) {
			return nil
		}
		return err
	}

	dedupBoardIDSQL := fmt.Sprintf(
		"DELETE b1 FROM `%s` b1 JOIN `%s` b2 ON b1.board_id = b2.board_id AND (b1.updated_at < b2.updated_at OR (b1.updated_at = b2.updated_at AND b1.id < b2.id))",
		table,
		table,
	)
	if err := db.Exec(dedupBoardIDSQL).Error; err != nil {
		if isTableNotExistsErr(err) {
			return nil
		}
		return err
	}
	return nil
}

func isTableNotExistsErr(err error) bool {
	var mysqlErr *mysqlDriver.MySQLError
	if errors.As(err, &mysqlErr) {
		return mysqlErr.Number == 1146
	}
	return strings.Contains(strings.ToLower(err.Error()), "doesn't exist")
}
