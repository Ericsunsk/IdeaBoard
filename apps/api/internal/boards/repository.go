package boards

import (
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type BoardRepository struct {
	db *gorm.DB
}

func NewBoardRepository(db *gorm.DB) *BoardRepository {
	return &BoardRepository{db: db}
}

func (r *BoardRepository) ListByUserID(userID uint64) ([]UserBoard, error) {
	var boards []UserBoard
	err := r.db.Where("user_id = ?", userID).Order("updated_at DESC").Find(&boards).Error
	return boards, err
}

func (r *BoardRepository) GetByUserBoardID(userID uint64, boardID string) (*UserBoard, error) {
	var board UserBoard
	err := r.db.Where("user_id = ? AND board_id = ?", userID, boardID).First(&board).Error
	if err != nil {
		return nil, err
	}
	return &board, nil
}

func (r *BoardRepository) Upsert(board *UserBoard) error {
	return r.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "board_id"},
			{Name: "user_id"},
		},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"name":       board.Name,
			"state":      board.State,
			"updated_at": board.UpdatedAt,
		}),
	}).Create(board).Error
}

func (r *BoardRepository) DeleteByUserBoardID(userID uint64, boardID string) error {
	return r.db.Where("user_id = ? AND board_id = ?", userID, boardID).Delete(&UserBoard{}).Error
}
