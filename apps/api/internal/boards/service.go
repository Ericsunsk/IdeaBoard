package boards

import (
	"context"
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

	"api/internal/platform/database"
	"api/internal/platform/jsonlimit"
	"api/internal/users"
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
	DeleteWorkspaceBoardByStorageUID(ctx context.Context, storageUID string, boardID string) error
}

func NewBoardService(repo *BoardRepository, userRepo *users.UserRepository, workspaceCleaner ...BoardWorkspaceCleaner) *BoardService {
	var cleaner BoardWorkspaceCleaner
	if len(workspaceCleaner) > 0 {
		cleaner = workspaceCleaner[0]
	}
	return &BoardService{repo: repo, userRepo: userRepo, workspaceCleaner: cleaner}
}

func (s *BoardService) ListBoards(ctx context.Context, userID int64) ([]BoardWithRole, error) {
	return s.repo.ListAccessibleByUserID(ctx, userID)
}

func (s *BoardService) ResolveBoardAccess(ctx context.Context, userID int64, boardID string) (*UserBoard, BoardAccessRole, error) {
	normalizedID := normalizeBoardID(boardID)
	if normalizedID == "" {
		return nil, "", ErrInvalidBoardInput
	}
	board, err := s.repo.GetByBoardID(ctx, normalizedID)
	if err != nil {
		if database.IsNotFound(err) {
			return nil, "", ErrBoardNotFound
		}
		return nil, "", err
	}
	if board.UserID == userID {
		return board, BoardAccessRoleOwner, nil
	}
	role, err := s.repo.GetMemberRole(ctx, board.BoardID, userID)
	if err != nil {
		if database.IsNotFound(err) {
			return nil, "", ErrBoardPermissionDenied
		}
		return nil, "", err
	}
	return board, role, nil
}

func (s *BoardService) ResolveBoardWriteAccess(ctx context.Context, userID int64, boardID string) (*UserBoard, BoardAccessRole, error) {
	board, role, err := s.ResolveBoardAccess(ctx, userID, boardID)
	if err != nil {
		return nil, "", err
	}
	if !boardRoleCanWrite(role) {
		return nil, "", ErrBoardPermissionDenied
	}
	return board, role, nil
}

func (s *BoardService) UpsertBoard(ctx context.Context, userID int64, boardID string, req *UpsertBoardRequest) (*BoardWithRole, error) {
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

	existing, err := s.repo.GetByBoardID(ctx, normalizedID)
	if err == nil {
		role := BoardAccessRoleOwner
		if existing.UserID != userID {
			role, err = s.repo.GetMemberRole(ctx, existing.BoardID, userID)
			if err != nil {
				if database.IsNotFound(err) {
					return nil, ErrBoardPermissionDenied
				}
				return nil, err
			}
		}

		nextName := existing.Name
		switch role {
		case BoardAccessRoleOwner:
			conflicted, err := s.repo.HasNameConflict(ctx, existing.UserID, normalizedID, name)
			if err != nil {
				return nil, err
			}
			if conflicted {
				return nil, ErrBoardNameConflict
			}
			nextName = name
		case BoardAccessRoleEditor:
			if name != normalizeBoardName(existing.Name) {
				return nil, ErrBoardPermissionDenied
			}
		default:
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
		if err := s.repo.Upsert(ctx, board); err != nil {
			if IsBoardNameUniqueConstraintError(err) {
				return nil, ErrBoardNameConflict
			}
			return nil, err
		}
		saved, err := s.repo.GetByBoardID(ctx, normalizedID)
		if err != nil {
			return nil, err
		}
		return &BoardWithRole{Board: *saved, Role: role}, nil
	}

	if !database.IsNotFound(err) {
		return nil, err
	}

	conflicted, err := s.repo.HasNameConflict(ctx, userID, normalizedID, name)
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
	if err := s.repo.Upsert(ctx, board); err != nil {
		if IsBoardNameUniqueConstraintError(err) {
			return nil, ErrBoardNameConflict
		}
		return nil, err
	}
	saved, err := s.repo.GetByBoardID(ctx, normalizedID)
	if err != nil {
		return nil, err
	}
	return &BoardWithRole{Board: *saved, Role: BoardAccessRoleOwner}, nil
}

func (s *BoardService) DeleteBoard(ctx context.Context, userID int64, boardID string) error {
	board, role, err := s.ResolveBoardAccess(ctx, userID, boardID)
	if err != nil {
		return err
	}
	if role != BoardAccessRoleOwner {
		return ErrBoardPermissionDenied
	}
	if s.workspaceCleaner != nil {
		owner, err := s.userRepo.GetByID(ctx, board.UserID)
		if err != nil {
			return err
		}
		if err := s.workspaceCleaner.DeleteWorkspaceBoardByStorageUID(ctx, owner.StorageUID, board.BoardID); err != nil {
			return fmt.Errorf("%w: %v", ErrBoardWorkspaceCleanupFailed, err)
		}
	}
	return s.repo.DeleteBoardCascade(ctx, board.BoardID)
}

func (s *BoardService) ListMembers(ctx context.Context, ownerUserID int64, boardID string) ([]BoardMemberWithUser, error) {
	board, err := s.requireOwnerBoard(ctx, ownerUserID, boardID)
	if err != nil {
		return nil, err
	}
	ownerUser, err := s.userRepo.GetByID(ctx, board.UserID)
	if err != nil {
		return nil, err
	}
	result := []BoardMemberWithUser{{
		UserID:    ownerUser.ID,
		MemberUID: ownerUser.StorageUID,
		Username:  ownerUser.Username,
		Role:      BoardAccessRoleOwner,
	}}
	members, err := s.repo.ListMembers(ctx, board.BoardID)
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

func (s *BoardService) InviteMember(ctx context.Context, ownerUserID int64, boardID string, memberUID string, role string) (*BoardMemberWithUser, error) {
	board, err := s.requireOwnerBoard(ctx, ownerUserID, boardID)
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
	memberUser, err := s.userRepo.GetByStorageUID(ctx, normalizedUID)
	if err != nil {
		if database.IsNotFound(err) {
			return nil, ErrBoardMemberNotFound
		}
		return nil, err
	}
	if memberUser.ID == board.UserID {
		return nil, ErrBoardMemberInviteSelf
	}
	if err := s.repo.UpsertMember(ctx, board.BoardID, memberUser.ID, targetRole); err != nil {
		return nil, err
	}
	return &BoardMemberWithUser{
		UserID:    memberUser.ID,
		MemberUID: memberUser.StorageUID,
		Username:  memberUser.Username,
		Role:      targetRole,
	}, nil
}

func (s *BoardService) UpdateMemberRole(ctx context.Context, ownerUserID int64, boardID string, memberUID string, role string) (*BoardMemberWithUser, error) {
	return s.InviteMember(ctx, ownerUserID, boardID, memberUID, role)
}

func (s *BoardService) RemoveMember(ctx context.Context, ownerUserID int64, boardID string, memberUID string) error {
	board, err := s.requireOwnerBoard(ctx, ownerUserID, boardID)
	if err != nil {
		return err
	}
	normalizedUID := strings.TrimSpace(memberUID)
	if !users.IsValidStorageUID(normalizedUID) {
		return ErrInvalidBoardInput
	}
	memberUser, err := s.userRepo.GetByStorageUID(ctx, normalizedUID)
	if err != nil {
		if database.IsNotFound(err) {
			return ErrBoardMemberNotFound
		}
		return err
	}
	if memberUser.ID == board.UserID {
		return ErrBoardMemberInviteSelf
	}
	member, err := s.repo.GetMemberByUserID(ctx, board.BoardID, memberUser.ID)
	if err != nil {
		if database.IsNotFound(err) {
			return ErrBoardMemberNotFound
		}
		return err
	}
	if member == nil {
		return ErrBoardMemberNotFound
	}
	return s.repo.DeleteMember(ctx, board.BoardID, memberUser.ID)
}

func (s *BoardService) CreateShareToken(ctx context.Context, ownerUserID int64, boardID string, role string, expiresAt *int64) (*BoardShareToken, error) {
	board, err := s.requireOwnerBoard(ctx, ownerUserID, boardID)
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
	token, err := generateBoardShareToken(ctx, s.repo)
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
	if err := s.repo.CreateShareToken(ctx, shareToken); err != nil {
		return nil, err
	}
	return shareToken, nil
}

func (s *BoardService) ListShareTokens(ctx context.Context, ownerUserID int64, boardID string) ([]BoardShareToken, error) {
	board, err := s.requireOwnerBoard(ctx, ownerUserID, boardID)
	if err != nil {
		return nil, err
	}
	return s.repo.ListShareTokens(ctx, board.BoardID)
}

func (s *BoardService) RevokeShareToken(ctx context.Context, ownerUserID int64, boardID, token string) error {
	board, err := s.requireOwnerBoard(ctx, ownerUserID, boardID)
	if err != nil {
		return err
	}
	normalizedToken := strings.TrimSpace(token)
	if normalizedToken == "" {
		return ErrInvalidBoardInput
	}
	shareToken, err := s.repo.GetShareTokenByToken(ctx, normalizedToken)
	if err != nil {
		if database.IsNotFound(err) {
			return ErrShareTokenNotFound
		}
		return err
	}
	if shareToken.BoardID != board.BoardID {
		return ErrShareTokenNotFound
	}
	return s.repo.RevokeShareToken(ctx, board.BoardID, normalizedToken)
}

func (s *BoardService) JoinBoardByShareToken(ctx context.Context, userID int64, token string) (*BoardWithRole, error) {
	normalizedToken := strings.TrimSpace(token)
	if normalizedToken == "" {
		return nil, ErrInvalidBoardInput
	}
	shareToken, err := s.repo.GetShareTokenByToken(ctx, normalizedToken)
	if err != nil {
		if database.IsNotFound(err) {
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
	board, err := s.repo.GetByBoardID(ctx, shareToken.BoardID)
	if err != nil {
		if database.IsNotFound(err) {
			return nil, ErrBoardNotFound
		}
		return nil, err
	}
	if board.UserID == userID {
		return &BoardWithRole{Board: *board, Role: BoardAccessRoleOwner}, nil
	}
	if err := s.repo.UpsertMember(ctx, board.BoardID, userID, role); err != nil {
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
	if jsonlimit.ValueDepth(state) > maxBoardStateDepth {
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

func resolveClientTime(raw *int64, fallback time.Time) time.Time {
	if raw == nil || *raw <= 0 {
		return fallback
	}
	return time.UnixMilli(*raw)
}

func (s *BoardService) requireOwnerBoard(ctx context.Context, userID int64, boardID string) (*UserBoard, error) {
	board, role, err := s.ResolveBoardAccess(ctx, userID, boardID)
	if err != nil {
		return nil, err
	}
	if role != BoardAccessRoleOwner {
		return nil, ErrBoardPermissionDenied
	}
	return board, nil
}

func generateBoardShareToken(ctx context.Context, repo *BoardRepository) (string, error) {
	for i := 0; i < 64; i++ {
		buf := make([]byte, 24)
		if _, err := rand.Read(buf); err != nil {
			return "", fmt.Errorf("生成分享令牌失败: %v", err)
		}
		token := base64.RawURLEncoding.EncodeToString(buf)
		exists, err := repo.ShareTokenExists(ctx, token)
		if err != nil {
			return "", err
		}
		if !exists {
			return token, nil
		}
	}
	return "", fmt.Errorf("生成分享令牌失败")
}
