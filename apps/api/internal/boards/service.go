package boards

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
)

var ErrBoardNotFound = errors.New("board not found")
var ErrInvalidBoardInput = errors.New("invalid board input")

type BoardService struct {
	repo *BoardRepository
}

func NewBoardService(repo *BoardRepository) *BoardService {
	return &BoardService{repo: repo}
}

func (s *BoardService) ListBoards(userID uint64) ([]UserBoard, error) {
	return s.repo.ListByUserID(userID)
}

func (s *BoardService) UpsertBoard(userID uint64, boardID string, req *UpsertBoardRequest) (*UserBoard, error) {
	normalizedID := normalizeBoardID(boardID)
	name := normalizeBoardName(req.Name)
	if normalizedID == "" || name == "" {
		return nil, ErrInvalidBoardInput
	}
	state, err := marshalState(req.State)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	createdAt := resolveClientTime(req.CreatedAt, now)
	updatedAt := resolveClientTime(req.UpdatedAt, now)
	board := &UserBoard{
		BoardID:   normalizedID,
		UserID:    userID,
		Name:      name,
		State:     state,
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
	}
	if err := s.repo.Upsert(board); err != nil {
		return nil, err
	}
	return s.repo.GetByUserBoardID(userID, normalizedID)
}

func (s *BoardService) DeleteBoard(userID uint64, boardID string) error {
	boardID = normalizeBoardID(boardID)
	if boardID == "" {
		return ErrInvalidBoardInput
	}
	_, err := s.repo.GetByUserBoardID(userID, boardID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrBoardNotFound
		}
		return err
	}
	return s.repo.DeleteByUserBoardID(userID, boardID)
}

func normalizeBoardID(boardID string) string {
	return strings.TrimSpace(boardID)
}

func normalizeBoardName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	return name
}

func marshalState(state interface{}) (string, error) {
	if state == nil {
		return "null", nil
	}
	payload, err := json.Marshal(state)
	if err != nil {
		return "", err
	}
	return string(payload), nil
}

func resolveClientTime(raw *int64, fallback time.Time) time.Time {
	if raw == nil || *raw <= 0 {
		return fallback
	}
	return time.UnixMilli(*raw)
}
