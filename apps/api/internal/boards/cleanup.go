package boards

import (
	"context"
	"database/sql"

	"api/internal/platform/database"
)

func CleanupDuplicateBoardNames(db *sql.DB) error {
	if db == nil {
		return nil
	}

	_, err := db.ExecContext(context.Background(), `
DELETE b1
FROM user_boards b1
JOIN user_boards b2
  ON b1.user_id = b2.user_id
 AND b1.name = b2.name
 AND (b1.updated_at < b2.updated_at OR (b1.updated_at = b2.updated_at AND b1.id < b2.id))`)
	if err != nil {
		if database.IsMissingTableError(err) {
			return nil
		}
		return err
	}

	_, err = db.ExecContext(context.Background(), `
DELETE b1
FROM user_boards b1
JOIN user_boards b2
  ON b1.board_id = b2.board_id
 AND (b1.updated_at < b2.updated_at OR (b1.updated_at = b2.updated_at AND b1.id < b2.id))`)
	if err != nil {
		if database.IsMissingTableError(err) {
			return nil
		}
		return err
	}
	return nil
}
