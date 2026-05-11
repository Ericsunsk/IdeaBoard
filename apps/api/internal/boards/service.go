package boards

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"api/internal/users"

	"gorm.io/gorm"
)

var ErrBoardNotFound = errors.New("board not found")
var ErrInvalidBoardInput = errors.New("invalid board input")
var ErrBoardNameConflict = errors.New("board name conflict")
var ErrBoardPermissionDenied = errors.New("board permission denied")
var ErrBoardMemberNotFound = errors.New("board member not found")
var ErrBoardMemberInvalidRole = errors.New("board member invalid role")
var ErrBoardMemberInviteSelf = errors.New("board member invite self")
var ErrBoardWorkspaceCleanupFailed = errors.New("board workspace cleanup failed")
var ErrShareTokenNotFound = errors.New("share token not found")
var ErrShareTokenInvalid = errors.New("share token invalid")
var ErrShareTokenExpired = errors.New("share token expired")
var ErrShareTokenRevoked = errors.New("share token revoked")

const maxBoardNameLen = 60
const maxBoardIDLen = 100
const maxBoardStateBytes = 8 * 1024 * 1024
const maxBoardStateDepth = 32

type BoardService struct {
	repo             *BoardRepository
	userRepo         *users.UserRepository
	workspaceCleaner BoardWorkspaceCleaner
}

type BoardWorkspaceCleaner interface {
	DeleteWorkspaceBoardByStorageUID(storageUID string, boardID string) error
}

func NewBoardService(repo *BoardRepository, userRepo *users.UserRepository, workspaceCleaner ...BoardWorkspaceCleaner) *BoardService {
	var cleaner BoardWorkspaceCleaner
	if len(workspaceCleaner) > 0 {
		cleaner = workspaceCleaner[0]
	}
	return &BoardService{repo: repo, userRepo: userRepo, workspaceCleaner: cleaner}
}

func (s *BoardService) ListBoards(userID uint64) ([]BoardWithRole, error) {
	return s.repo.ListAccessibleByUserID(userID)
}

func (s *BoardService) ResolveBoardAccess(userID uint64, boardID string) (*UserBoard, BoardAccessRole, error) {
	normalizedID := normalizeBoardID(boardID)
	if normalizedID == "" {
		return nil, "", ErrInvalidBoardInput
	}
	board, err := s.repo.GetByBoardID(normalizedID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, "", ErrBoardNotFound
		}
		return nil, "", err
	}
	if board.UserID == userID {
		return board, BoardAccessRoleOwner, nil
	}
	role, err := s.repo.GetMemberRole(board.BoardID, userID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, "", ErrBoardPermissionDenied
		}
		return nil, "", err
	}
	return board, role, nil
}

func (s *BoardService) ResolveBoardWriteAccess(userID uint64, boardID string) (*UserBoard, BoardAccessRole, error) {
	board, role, err := s.ResolveBoardAccess(userID, boardID)
	if err != nil {
		return nil, "", err
	}
	if !boardRoleCanWrite(role) {
		return nil, "", ErrBoardPermissionDenied
	}
	return board, role, nil
}

func (s *BoardService) UpsertBoard(userID uint64, boardID string, req *UpsertBoardRequest) (*BoardWithRole, error) {
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

	existing, err := s.repo.GetByBoardID(normalizedID)
	if err == nil {
		role := BoardAccessRoleOwner
		if existing.UserID != userID {
			role, err = s.repo.GetMemberRole(existing.BoardID, userID)
			if err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return nil, ErrBoardPermissionDenied
				}
				return nil, err
			}
		}

		nextName := existing.Name
		if role == BoardAccessRoleOwner {
			conflicted, err := s.repo.HasNameConflict(existing.UserID, normalizedID, name)
			if err != nil {
				return nil, err
			}
			if conflicted {
				return nil, ErrBoardNameConflict
			}
			nextName = name
		} else if role == BoardAccessRoleEditor {
			if name != normalizeBoardName(existing.Name) {
				return nil, ErrBoardPermissionDenied
			}
		} else {
			return nil, ErrBoardPermissionDenied
		}

		board := &UserBoard{
			BoardID:   existing.BoardID,
			UserID:    existing.UserID,
			Name:      nextName,
			State:     state,
			CreatedAt: existing.CreatedAt,
			UpdatedAt: updatedAt,
		}
		if err := s.repo.Upsert(board); err != nil {
			if IsBoardNameUniqueConstraintError(err) {
				return nil, ErrBoardNameConflict
			}
			return nil, err
		}
		saved, err := s.repo.GetByBoardID(normalizedID)
		if err != nil {
			return nil, err
		}
		return &BoardWithRole{Board: *saved, Role: role}, nil
	}

	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	conflicted, err := s.repo.HasNameConflict(userID, normalizedID, name)
	if err != nil {
		return nil, err
	}
	if conflicted {
		return nil, ErrBoardNameConflict
	}
	board := &UserBoard{
		BoardID:   normalizedID,
		UserID:    userID,
		Name:      name,
		State:     state,
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
	}
	if err := s.repo.Upsert(board); err != nil {
		if IsBoardNameUniqueConstraintError(err) {
			return nil, ErrBoardNameConflict
		}
		return nil, err
	}
	saved, err := s.repo.GetByBoardID(normalizedID)
	if err != nil {
		return nil, err
	}
	return &BoardWithRole{Board: *saved, Role: BoardAccessRoleOwner}, nil
}

func (s *BoardService) DeleteBoard(userID uint64, boardID string) error {
	board, role, err := s.ResolveBoardAccess(userID, boardID)
	if err != nil {
		return err
	}
	if role != BoardAccessRoleOwner {
		return ErrBoardPermissionDenied
	}
	if s.workspaceCleaner != nil {
		owner, err := s.userRepo.GetByID(board.UserID)
		if err != nil {
			return err
		}
		if err := s.workspaceCleaner.DeleteWorkspaceBoardByStorageUID(owner.StorageUID, board.BoardID); err != nil {
			return fmt.Errorf("%w: %v", ErrBoardWorkspaceCleanupFailed, err)
		}
	}
	return s.repo.db.Transaction(func(tx *gorm.DB) error {
		repo := NewBoardRepository(tx)
		if err := repo.DeleteShareTokensByBoardID(board.BoardID); err != nil {
			return err
		}
		if err := repo.DeleteMembersByBoardID(board.BoardID); err != nil {
			return err
		}
		return repo.DeleteByBoardID(board.BoardID)
	})
}

func (s *BoardService) ListMembers(ownerUserID uint64, boardID string) ([]BoardMemberWithUser, error) {
	board, err := s.requireOwnerBoard(ownerUserID, boardID)
	if err != nil {
		return nil, err
	}
	ownerUser, err := s.userRepo.GetByID(board.UserID)
	if err != nil {
		return nil, err
	}
	result := []BoardMemberWithUser{{
		UserID:    ownerUser.ID,
		MemberUID: ownerUser.StorageUID,
		Username:  ownerUser.Username,
		Role:      BoardAccessRoleOwner,
	}}
	members, err := s.repo.ListMembers(board.BoardID)
	if err != nil {
		return nil, err
	}
	for _, member := range members {
		if member.UserID == board.UserID {
			continue
		}
		result = append(result, member)
	}
	return result, nil
}

func (s *BoardService) InviteMember(ownerUserID uint64, boardID string, memberUID string, role string) (*BoardMemberWithUser, error) {
	board, err := s.requireOwnerBoard(ownerUserID, boardID)
	if err != nil {
		return nil, err
	}
	normalizedUID := strings.TrimSpace(memberUID)
	if !users.IsValidStorageUID(normalizedUID) {
		return nil, ErrInvalidBoardInput
	}
	targetRole := normalizeBoardMemberRole(role)
	if targetRole == "" {
		return nil, ErrBoardMemberInvalidRole
	}
	memberUser, err := s.userRepo.GetByStorageUID(normalizedUID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrBoardMemberNotFound
		}
		return nil, err
	}
	if memberUser.ID == board.UserID {
		return nil, ErrBoardMemberInviteSelf
	}
	if err := s.repo.UpsertMember(board.BoardID, memberUser.ID, targetRole); err != nil {
		return nil, err
	}
	return &BoardMemberWithUser{
		UserID:    memberUser.ID,
		MemberUID: memberUser.StorageUID,
		Username:  memberUser.Username,
		Role:      targetRole,
	}, nil
}

func (s *BoardService) UpdateMemberRole(ownerUserID uint64, boardID string, memberUID string, role string) (*BoardMemberWithUser, error) {
	return s.InviteMember(ownerUserID, boardID, memberUID, role)
}

func (s *BoardService) RemoveMember(ownerUserID uint64, boardID string, memberUID string) error {
	board, err := s.requireOwnerBoard(ownerUserID, boardID)
	if err != nil {
		return err
	}
	normalizedUID := strings.TrimSpace(memberUID)
	if !users.IsValidStorageUID(normalizedUID) {
		return ErrInvalidBoardInput
	}
	memberUser, err := s.userRepo.GetByStorageUID(normalizedUID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrBoardMemberNotFound
		}
		return err
	}
	if memberUser.ID == board.UserID {
		return ErrBoardMemberInviteSelf
	}
	member, err := s.repo.GetMemberByUserID(board.BoardID, memberUser.ID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrBoardMemberNotFound
		}
		return err
	}
	if member == nil {
		return ErrBoardMemberNotFound
	}
	return s.repo.DeleteMember(board.BoardID, memberUser.ID)
}

func (s *BoardService) CreateShareToken(ownerUserID uint64, boardID string, role string, expiresAt *int64) (*BoardShareToken, error) {
	board, err := s.requireOwnerBoard(ownerUserID, boardID)
	if err != nil {
		return nil, err
	}
	targetRole := normalizeBoardMemberRole(role)
	if targetRole == "" {
		return nil, ErrBoardMemberInvalidRole
	}
	var expiresAtTime *time.Time
	if expiresAt != nil && *expiresAt > 0 {
		v := time.UnixMilli(*expiresAt)
		expiresAtTime = &v
	}
	token, err := generateBoardShareToken(s.repo)
	if err != nil {
		return nil, err
	}
	shareToken := &BoardShareToken{
		BoardID:   board.BoardID,
		Token:     token,
		Role:      string(targetRole),
		CreatedBy: ownerUserID,
		Revoked:   false,
		ExpiresAt: expiresAtTime,
	}
	if err := s.repo.CreateShareToken(shareToken); err != nil {
		return nil, err
	}
	return shareToken, nil
}

func (s *BoardService) ListShareTokens(ownerUserID uint64, boardID string) ([]BoardShareToken, error) {
	board, err := s.requireOwnerBoard(ownerUserID, boardID)
	if err != nil {
		return nil, err
	}
	return s.repo.ListShareTokens(board.BoardID)
}

func (s *BoardService) RevokeShareToken(ownerUserID uint64, boardID, token string) error {
	board, err := s.requireOwnerBoard(ownerUserID, boardID)
	if err != nil {
		return err
	}
	normalizedToken := strings.TrimSpace(token)
	if normalizedToken == "" {
		return ErrInvalidBoardInput
	}
	shareToken, err := s.repo.GetShareTokenByToken(normalizedToken)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrShareTokenNotFound
		}
		return err
	}
	if shareToken.BoardID != board.BoardID {
		return ErrShareTokenNotFound
	}
	return s.repo.RevokeShareToken(board.BoardID, normalizedToken)
}

func (s *BoardService) JoinBoardByShareToken(userID uint64, token string) (*BoardWithRole, error) {
	normalizedToken := strings.TrimSpace(token)
	if normalizedToken == "" {
		return nil, ErrInvalidBoardInput
	}
	shareToken, err := s.repo.GetShareTokenByToken(normalizedToken)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrShareTokenNotFound
		}
		return nil, err
	}
	if shareToken.Revoked {
		return nil, ErrShareTokenRevoked
	}
	if shareToken.ExpiresAt != nil && shareToken.ExpiresAt.Before(time.Now()) {
		return nil, ErrShareTokenExpired
	}
	role := normalizeBoardMemberRole(shareToken.Role)
	if role == "" {
		return nil, ErrShareTokenInvalid
	}
	board, err := s.repo.GetByBoardID(shareToken.BoardID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrBoardNotFound
		}
		return nil, err
	}
	if board.UserID == userID {
		return &BoardWithRole{Board: *board, Role: BoardAccessRoleOwner}, nil
	}
	if err := s.repo.UpsertMember(board.BoardID, userID, role); err != nil {
		return nil, err
	}
	return &BoardWithRole{Board: *board, Role: role}, nil
}

func BuildBoardShareURL(baseURL, token string) string {
	trimmedToken := strings.TrimSpace(token)
	if trimmedToken == "" {
		return ""
	}
	trimmedBase := strings.TrimSpace(baseURL)
	if trimmedBase == "" {
		return ""
	}
	u, err := url.Parse(trimmedBase)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/share"
	q := u.Query()
	q.Set("token", trimmedToken)
	u.RawQuery = q.Encode()
	return u.String()
}

func ResolvePublicBaseURL(requestScheme, requestHost string) string {
	for _, key := range []string{"APP_PUBLIC_BASE_URL", "APP_BASE_URL", "PUBLIC_BASE_URL"} {
		if value := strings.TrimSpace(os.Getenv(key)); value != "" {
			return strings.TrimRight(value, "/")
		}
	}
	host := strings.TrimSpace(requestHost)
	if host == "" {
		return ""
	}
	scheme := strings.TrimSpace(requestScheme)
	if scheme == "" {
		scheme = "http"
	}
	return scheme + "://" + host
}

func normalizeBoardID(boardID string) string {
	normalized := strings.TrimSpace(boardID)
	if normalized == "" || len([]byte(normalized)) > maxBoardIDLen {
		return ""
	}
	for _, r := range normalized {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
			continue
		}
		return ""
	}
	return normalized
}

func normalizeBoardName(name string) string {
	parts := strings.Fields(name)
	if len(parts) == 0 {
		return ""
	}
	normalized := strings.Join(parts, " ")
	if utf8.RuneCountInString(normalized) <= maxBoardNameLen {
		return normalized
	}
	runes := []rune(normalized)
	return string(runes[:maxBoardNameLen])
}

func normalizeBoardAccessRole(role string) BoardAccessRole {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case string(BoardAccessRoleOwner):
		return BoardAccessRoleOwner
	case string(BoardAccessRoleEditor):
		return BoardAccessRoleEditor
	case string(BoardAccessRoleViewer):
		return BoardAccessRoleViewer
	default:
		return ""
	}
}

func normalizeBoardMemberRole(role string) BoardAccessRole {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case string(BoardAccessRoleEditor):
		return BoardAccessRoleEditor
	case string(BoardAccessRoleViewer):
		return BoardAccessRoleViewer
	default:
		return ""
	}
}

func boardRoleCanWrite(role BoardAccessRole) bool {
	return role == BoardAccessRoleOwner || role == BoardAccessRoleEditor
}

func marshalState(state interface{}) (string, error) {
	if state == nil {
		return "null", nil
	}
	if boardJSONValueDepth(state) > maxBoardStateDepth {
		return "", ErrInvalidBoardInput
	}
	payload, err := json.Marshal(state)
	if err != nil {
		return "", err
	}
	if len(payload) > maxBoardStateBytes {
		return "", ErrInvalidBoardInput
	}
	return string(payload), nil
}

func boardJSONValueDepth(value interface{}) int {
	switch v := value.(type) {
	case map[string]interface{}:
		maxDepth := 1
		for _, child := range v {
			if depth := 1 + boardJSONValueDepth(child); depth > maxDepth {
				maxDepth = depth
			}
		}
		return maxDepth
	case []interface{}:
		maxDepth := 1
		for _, child := range v {
			if depth := 1 + boardJSONValueDepth(child); depth > maxDepth {
				maxDepth = depth
			}
		}
		return maxDepth
	default:
		return 1
	}
}

func resolveClientTime(raw *int64, fallback time.Time) time.Time {
	if raw == nil || *raw <= 0 {
		return fallback
	}
	return time.UnixMilli(*raw)
}

func (s *BoardService) requireOwnerBoard(userID uint64, boardID string) (*UserBoard, error) {
	board, role, err := s.ResolveBoardAccess(userID, boardID)
	if err != nil {
		return nil, err
	}
	if role != BoardAccessRoleOwner {
		return nil, ErrBoardPermissionDenied
	}
	return board, nil
}

func generateBoardShareToken(repo *BoardRepository) (string, error) {
	for i := 0; i < 64; i++ {
		buf := make([]byte, 24)
		if _, err := rand.Read(buf); err != nil {
			return "", fmt.Errorf("生成分享令牌失败: %v", err)
		}
		token := base64.RawURLEncoding.EncodeToString(buf)
		exists, err := repo.ShareTokenExists(token)
		if err != nil {
			return "", err
		}
		if !exists {
			return token, nil
		}
	}
	return "", fmt.Errorf("生成分享令牌失败")
}
