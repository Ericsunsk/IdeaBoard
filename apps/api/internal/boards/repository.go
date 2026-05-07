package boards

import (
	"errors"
	"strings"
	"time"

	mysqlDriver "github.com/go-sql-driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type BoardRepository struct {
	db *gorm.DB
}

type boardWithRoleRow struct {
	UserBoard
	AccessRole string `gorm:"column:access_role"`
}

type boardMemberWithUserRow struct {
	UserID    uint64 `gorm:"column:user_id"`
	MemberUID string `gorm:"column:member_uid"`
	Username  string `gorm:"column:username"`
	Role      string `gorm:"column:role"`
}

func NewBoardRepository(db *gorm.DB) *BoardRepository {
	return &BoardRepository{db: db}
}

func (r *BoardRepository) ListAccessibleByUserID(userID uint64) ([]BoardWithRole, error) {
	var rows []boardWithRoleRow
	err := r.db.Table("user_boards AS b").
		Select(
			"b.id, b.board_id, b.user_id, b.name, b.state, b.created_at, b.updated_at, "+
				"CASE WHEN b.user_id = ? THEN ? ELSE m.role END AS access_role",
			userID,
			string(BoardAccessRoleOwner),
		).
		Joins("LEFT JOIN board_members AS m ON m.board_id = b.board_id AND m.user_id = ?", userID).
		Where("b.user_id = ? OR m.user_id = ?", userID, userID).
		Order("b.updated_at DESC").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}

	result := make([]BoardWithRole, 0, len(rows))
	for _, row := range rows {
		role := normalizeBoardAccessRole(row.AccessRole)
		if role == "" {
			continue
		}
		result = append(result, BoardWithRole{
			Board: row.UserBoard,
			Role:  role,
		})
	}
	return result, nil
}

func (r *BoardRepository) GetByBoardID(boardID string) (*UserBoard, error) {
	var board UserBoard
	tx := r.db.Where("board_id = ?", boardID).Limit(1).Find(&board)
	if tx.Error != nil {
		return nil, tx.Error
	}
	if tx.RowsAffected == 0 {
		return nil, gorm.ErrRecordNotFound
	}
	return &board, nil
}

func (r *BoardRepository) Upsert(board *UserBoard) error {
	return r.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "board_id"},
		},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"name":       board.Name,
			"state":      board.State,
			"updated_at": board.UpdatedAt,
		}),
	}).Create(board).Error
}

func (r *BoardRepository) HasNameConflict(userID uint64, boardID, name string) (bool, error) {
	query := r.db.Model(&UserBoard{}).Where("user_id = ? AND LOWER(name) = LOWER(?)", userID, name)
	if strings.TrimSpace(boardID) != "" {
		query = query.Where("board_id <> ?", boardID)
	}
	var count int64
	if err := query.Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *BoardRepository) GetMemberRole(boardID string, userID uint64) (BoardAccessRole, error) {
	var member BoardMember
	if err := r.db.Select("role").Where("board_id = ? AND user_id = ?", boardID, userID).First(&member).Error; err != nil {
		return "", err
	}
	role := normalizeBoardAccessRole(member.Role)
	if role == "" || role == BoardAccessRoleOwner {
		return "", gorm.ErrRecordNotFound
	}
	return role, nil
}

func (r *BoardRepository) DeleteByBoardID(boardID string) error {
	return r.db.Where("board_id = ?", boardID).Delete(&UserBoard{}).Error
}

func (r *BoardRepository) DeleteMembersByBoardID(boardID string) error {
	return r.db.Where("board_id = ?", boardID).Delete(&BoardMember{}).Error
}

func (r *BoardRepository) DeleteShareTokensByBoardID(boardID string) error {
	return r.db.Where("board_id = ?", boardID).Delete(&BoardShareToken{}).Error
}

func (r *BoardRepository) ListMembers(boardID string) ([]BoardMemberWithUser, error) {
	var rows []boardMemberWithUserRow
	err := r.db.Table("board_members AS m").
		Select("m.user_id, u.storage_uid AS member_uid, u.username, m.role").
		Joins("INNER JOIN users AS u ON u.id = m.user_id").
		Where("m.board_id = ?", boardID).
		Order("m.created_at ASC").
		Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	result := make([]BoardMemberWithUser, 0, len(rows))
	for _, row := range rows {
		role := normalizeBoardAccessRole(row.Role)
		if role == "" {
			continue
		}
		result = append(result, BoardMemberWithUser{
			UserID:    row.UserID,
			MemberUID: row.MemberUID,
			Username:  row.Username,
			Role:      role,
		})
	}
	return result, nil
}

func (r *BoardRepository) UpsertMember(boardID string, userID uint64, role BoardAccessRole) error {
	return r.db.Clauses(clause.OnConflict{
		Columns: []clause.Column{
			{Name: "board_id"},
			{Name: "user_id"},
		},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"role":       string(role),
			"updated_at": time.Now(),
		}),
	}).Create(&BoardMember{
		BoardID: boardID,
		UserID:  userID,
		Role:    string(role),
	}).Error
}

func (r *BoardRepository) DeleteMember(boardID string, userID uint64) error {
	return r.db.Where("board_id = ? AND user_id = ?", boardID, userID).Delete(&BoardMember{}).Error
}

func (r *BoardRepository) GetMemberByUserID(boardID string, userID uint64) (*BoardMember, error) {
	var member BoardMember
	if err := r.db.Where("board_id = ? AND user_id = ?", boardID, userID).First(&member).Error; err != nil {
		return nil, err
	}
	return &member, nil
}

func (r *BoardRepository) CreateShareToken(token *BoardShareToken) error {
	return r.db.Create(token).Error
}

func (r *BoardRepository) ListShareTokens(boardID string) ([]BoardShareToken, error) {
	var list []BoardShareToken
	if err := r.db.Where("board_id = ?", boardID).Order("created_at DESC").Find(&list).Error; err != nil {
		return nil, err
	}
	return list, nil
}

func (r *BoardRepository) GetShareTokenByToken(token string) (*BoardShareToken, error) {
	var shareToken BoardShareToken
	if err := r.db.Where("token = ?", token).First(&shareToken).Error; err != nil {
		return nil, err
	}
	return &shareToken, nil
}

func (r *BoardRepository) RevokeShareToken(boardID, token string) error {
	return r.db.Model(&BoardShareToken{}).
		Where("board_id = ? AND token = ?", boardID, token).
		Update("revoked", true).Error
}

func (r *BoardRepository) ShareTokenExists(token string) (bool, error) {
	var count int64
	if err := r.db.Model(&BoardShareToken{}).Where("token = ?", token).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

func IsBoardNameUniqueConstraintError(err error) bool {
	var mysqlErr *mysqlDriver.MySQLError
	if !errors.As(err, &mysqlErr) {
		return false
	}
	if mysqlErr.Number != 1062 {
		return false
	}
	return strings.Contains(strings.ToLower(mysqlErr.Message), "idx_user_board_name")
}
