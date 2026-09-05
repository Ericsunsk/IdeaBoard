package boards

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"

	"ideaboard/api/internal/identity/users/domain"
	"ideaboard/api/internal/platform/database"
	"ideaboard/api/internal/platform/jsonlimit"
)

var (
	ErrNotFound            = errors.New("board not found")
	ErrInvalidInput        = errors.New("invalid board input")
	ErrNameConflict        = errors.New("board name conflict")
	ErrIDConflict          = errors.New("board id conflict")
	ErrPermissionDenied    = errors.New("board permission denied")
	ErrMemberNotFound      = errors.New("board member not found")
	ErrMemberInvalidRole   = errors.New("board member invalid role")
	ErrMemberInviteSelf    = errors.New("board member invite self")
	ErrCanvasCleanupFailed = errors.New("board canvas cleanup failed")

	ErrShareTokenNotFound = errors.New("share token not found")
	ErrShareTokenInvalid  = errors.New("share token invalid")
	ErrShareTokenExpired  = errors.New("share token expired")
	ErrShareTokenRevoked  = errors.New("share token revoked")
)

const maxBoardNameLen = 60
const maxBoardIDLen = 100
const maxBoardStateBytes = 8 * 1024 * 1024
const maxBoardStateDepth = 32

type Service struct {
	repo          *Repository
	userRepo      UserReader
	canvasCleaner CanvasCleaner
}

// UserReader is the identity data this context needs to resolve board ownership.
type UserReader interface {
	GetByID(context.Context, int64) (*domain.User, error)
	GetByPublicID(context.Context, string) (*domain.User, error)
}

type CanvasCleaner interface {
	DeleteCanvasBoardByStorageUID(ctx context.Context, storageUID string, boardID string) error
}

func NewService(repo *Repository, userRepo UserReader, canvasCleaner ...CanvasCleaner) *Service {
	var cleaner CanvasCleaner
	if len(canvasCleaner) > 0 {
		cleaner = canvasCleaner[0]
	}
	return &Service{repo: repo, userRepo: userRepo, canvasCleaner: cleaner}
}

func (s *Service) ListBoards(ctx context.Context, userID int64) ([]WithRole, error) {
	return s.repo.ListAccessibleByUserID(ctx, userID)
}

func (s *Service) ResolveBoardAccess(ctx context.Context, userID int64, boardID string) (*Board, AccessRole, error) {
	access, err := s.resolveAccessibleBoard(ctx, userID, boardID)
	if err != nil {
		return nil, "", err
	}
	return &access.Board, access.Role, nil
}

func (s *Service) ResolveBoardWriteAccess(ctx context.Context, userID int64, boardID string) (*Board, AccessRole, error) {
	access, err := s.resolveAccessibleBoard(ctx, userID, boardID)
	if err != nil {
		return nil, "", err
	}
	if !access.CanWrite() {
		return nil, "", ErrPermissionDenied
	}
	return &access.Board, access.Role, nil
}

func (s *Service) UpsertBoard(ctx context.Context, userID int64, boardID string, req *UpsertRequest) (*WithRole, error) {
	if req == nil {
		return nil, ErrInvalidInput
	}
	normalizedID := normalizeBoardID(boardID)
	name := normalizeBoardName(req.Name)
	if normalizedID == "" || name == "" {
		return nil, ErrInvalidInput
	}
	state, stateSchemaVersion, err := marshalState(req.State)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	createdAt := resolveClientTime(req.CreatedAt, now)
	updatedAt := resolveClientTime(req.UpdatedAt, now)

	if existing, err := s.resolveOwnerBoardByNormalizedID(ctx, userID, normalizedID); err == nil {
		if err := s.ensureBoardNameAvailable(ctx, userID, normalizedID, name); err != nil {
			return nil, err
		}
		board := existing.Board
		board.Name = name
		board.State = state
		board.StateSchemaVersion = stateSchemaVersion
		board.LastSavedBy = &userID
		board.UpdatedAt = updatedAt
		return s.saveAndReloadBoardForActor(ctx, userID, &board)
	} else if !errors.Is(err, ErrNotFound) {
		return nil, err
	}

	if accessible, err := s.resolveAccessibleBoardByNormalizedID(ctx, userID, normalizedID); err == nil {
		if accessible.Role != RoleEditor {
			return nil, ErrPermissionDenied
		}
		if name != normalizeBoardName(accessible.Board.Name) {
			return nil, ErrPermissionDenied
		}
		board := accessible.Board
		board.State = state
		board.StateSchemaVersion = stateSchemaVersion
		board.LastSavedBy = &userID
		board.UpdatedAt = updatedAt
		return s.saveAndReloadBoardForActor(ctx, userID, &board)
	} else if !errors.Is(err, ErrNotFound) {
		return nil, err
	}

	if err := s.ensureBoardNameAvailable(ctx, userID, normalizedID, name); err != nil {
		return nil, err
	}
	if err := s.ensureBoardIDAvailable(ctx, userID, normalizedID); err != nil {
		return nil, err
	}
	board := &Board{
		BoardID:            normalizedID,
		OwnerUserID:        userID,
		CreatedBy:          userID,
		Name:               name,
		State:              state,
		Revision:           1,
		StateSchemaVersion: stateSchemaVersion,
		LastSavedBy:        &userID,
		CreatedAt:          createdAt,
		UpdatedAt:          updatedAt,
	}
	return s.saveAndReloadBoardForActor(ctx, userID, board)
}

func (s *Service) DeleteBoard(ctx context.Context, userID int64, boardID string) error {
	board, err := s.requireOwnerBoard(ctx, userID, boardID)
	if err != nil {
		return err
	}
	if s.canvasCleaner != nil {
		owner, err := s.userRepo.GetByID(ctx, board.OwnerUserID)
		if err != nil {
			return err
		}
		if err := s.canvasCleaner.DeleteCanvasBoardByStorageUID(ctx, owner.StorageUID, board.BoardID); err != nil {
			return fmt.Errorf("%w: %w", ErrCanvasCleanupFailed, err)
		}
	}
	return s.repo.DeleteBoardCascade(ctx, board.OwnerUserID, board.BoardID)
}

func (s *Service) ListMembers(ctx context.Context, ownerUserID int64, boardID string) ([]MemberWithUser, error) {
	board, err := s.requireOwnerBoard(ctx, ownerUserID, boardID)
	if err != nil {
		return nil, err
	}
	ownerUser, err := s.userRepo.GetByID(ctx, board.OwnerUserID)
	if err != nil {
		return nil, err
	}
	result := []MemberWithUser{{
		UserID:      ownerUser.ID,
		PublicID:    ownerUser.PublicID,
		DisplayName: ownerUser.DisplayName,
		Role:        RoleOwner,
	}}
	members, err := s.repo.ListMembers(ctx, board.OwnerUserID, board.BoardID)
	if err != nil {
		return nil, err
	}
	for _, member := range members {
		if member.UserID == board.OwnerUserID {
			continue
		}
		result = append(result, member)
	}
	return result, nil
}

func (s *Service) InviteMember(ctx context.Context, ownerUserID int64, boardID string, memberPublicID string, role string) (*MemberWithUser, error) {
	board, err := s.requireOwnerBoard(ctx, ownerUserID, boardID)
	if err != nil {
		return nil, err
	}
	normalizedPublicID := strings.TrimSpace(memberPublicID)
	if normalizedPublicID == "" {
		return nil, ErrInvalidInput
	}
	targetRole := normalizeMemberRole(role)
	if targetRole == "" {
		return nil, ErrMemberInvalidRole
	}
	memberUser, err := s.userRepo.GetByPublicID(ctx, normalizedPublicID)
	if err != nil {
		if database.IsNotFound(err) {
			return nil, ErrMemberNotFound
		}
		return nil, err
	}
	if memberUser.ID == board.OwnerUserID {
		return nil, ErrMemberInviteSelf
	}
	if err := s.repo.UpsertMember(ctx, board.OwnerUserID, board.BoardID, memberUser.ID, targetRole); err != nil {
		return nil, err
	}
	return &MemberWithUser{
		UserID:      memberUser.ID,
		PublicID:    memberUser.PublicID,
		DisplayName: memberUser.DisplayName,
		Role:        targetRole,
	}, nil
}

func (s *Service) UpdateMemberRole(ctx context.Context, ownerUserID int64, boardID string, memberPublicID string, role string) (*MemberWithUser, error) {
	return s.InviteMember(ctx, ownerUserID, boardID, memberPublicID, role)
}

func (s *Service) RemoveMember(ctx context.Context, ownerUserID int64, boardID string, memberPublicID string) error {
	board, err := s.requireOwnerBoard(ctx, ownerUserID, boardID)
	if err != nil {
		return err
	}
	normalizedPublicID := strings.TrimSpace(memberPublicID)
	if normalizedPublicID == "" {
		return ErrInvalidInput
	}
	memberUser, err := s.userRepo.GetByPublicID(ctx, normalizedPublicID)
	if err != nil {
		if database.IsNotFound(err) {
			return ErrMemberNotFound
		}
		return err
	}
	if memberUser.ID == board.OwnerUserID {
		return ErrMemberInviteSelf
	}
	member, err := s.repo.GetMemberByUserID(ctx, board.OwnerUserID, board.BoardID, memberUser.ID)
	if err != nil {
		if database.IsNotFound(err) {
			return ErrMemberNotFound
		}
		return err
	}
	if member == nil {
		return ErrMemberNotFound
	}
	return s.repo.DeleteMember(ctx, board.OwnerUserID, board.BoardID, memberUser.ID)
}

func (s *Service) CreateShareToken(ctx context.Context, ownerUserID int64, boardID string, role string, expiresAt *int64) (*ShareToken, error) {
	board, err := s.requireOwnerBoard(ctx, ownerUserID, boardID)
	if err != nil {
		return nil, err
	}
	targetRole := normalizeMemberRole(role)
	if targetRole == "" {
		return nil, ErrMemberInvalidRole
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
	tokenHash := boardShareTokenHash(token)
	shareToken := &ShareToken{
		BoardID:     board.BoardID,
		Token:       token,
		TokenHash:   tokenHash,
		TokenPrefix: boardShareTokenPrefix(token),
		Role:        string(targetRole),
		CreatedBy:   ownerUserID,
		Revoked:     false,
		ExpiresAt:   expiresAtTime,
	}
	if err := s.repo.CreateShareToken(ctx, shareToken); err != nil {
		return nil, err
	}
	return shareToken, nil
}

func (s *Service) ListShareTokens(ctx context.Context, ownerUserID int64, boardID string) ([]ShareToken, error) {
	board, err := s.requireOwnerBoard(ctx, ownerUserID, boardID)
	if err != nil {
		return nil, err
	}
	return s.repo.ListShareTokens(ctx, board.OwnerUserID, board.BoardID)
}

func (s *Service) RevokeShareToken(ctx context.Context, ownerUserID int64, boardID string, tokenID int64) error {
	board, err := s.requireOwnerBoard(ctx, ownerUserID, boardID)
	if err != nil {
		return err
	}
	if tokenID <= 0 {
		return ErrInvalidInput
	}
	if err := s.repo.RevokeShareTokenByID(ctx, ownerUserID, board.BoardID, tokenID); err != nil {
		if database.IsNotFound(err) {
			return ErrShareTokenNotFound
		}
		return err
	}
	return nil
}

func (s *Service) JoinBoardByShareToken(ctx context.Context, userID int64, token string) (*WithRole, error) {
	normalizedToken := strings.TrimSpace(token)
	if normalizedToken == "" {
		return nil, ErrInvalidInput
	}
	tokenHash := boardShareTokenHash(normalizedToken)
	shareToken, err := s.repo.GetShareTokenByHash(ctx, tokenHash)
	if err != nil {
		if database.IsNotFound(err) {
			return nil, ErrShareTokenNotFound
		}
		return nil, err
	}
	if err := validateShareTokenForJoin(*shareToken); err != nil {
		return nil, err
	}
	role := normalizeMemberRole(shareToken.Role)
	if role == "" {
		return nil, ErrShareTokenInvalid
	}
	board, err := s.resolveBoardByNormalizedID(ctx, shareToken.BoardID)
	if err != nil {
		return nil, err
	}
	if board.OwnerUserID == userID {
		return &WithRole{Board: board, Role: RoleOwner}, nil
	}
	consumedToken, err := s.repo.ConsumeShareTokenAndUpsertMember(ctx, tokenHash, userID)
	if err != nil {
		if database.IsNotFound(err) {
			return nil, s.classifyShareTokenConsumeError(ctx, tokenHash)
		}
		return nil, err
	}
	role = normalizeMemberRole(consumedToken.Role)
	if role == "" {
		return nil, ErrShareTokenInvalid
	}
	return &WithRole{Board: board, Role: role}, nil
}

func validateShareTokenForJoin(shareToken ShareToken) error {
	if shareToken.Revoked {
		return ErrShareTokenRevoked
	}
	if shareToken.ExpiresAt != nil && shareToken.ExpiresAt.Before(time.Now()) {
		return ErrShareTokenExpired
	}
	if shareToken.MaxUses != nil && shareToken.UsageCount >= int64(*shareToken.MaxUses) {
		return ErrShareTokenExpired
	}
	return nil
}

func (s *Service) classifyShareTokenConsumeError(ctx context.Context, tokenHash string) error {
	shareToken, err := s.repo.GetShareTokenByHash(ctx, tokenHash)
	if err != nil {
		if database.IsNotFound(err) {
			return ErrShareTokenNotFound
		}
		return err
	}
	if err := validateShareTokenForJoin(*shareToken); err != nil {
		return err
	}
	return ErrShareTokenExpired
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

func normalizeAccessRole(role string) AccessRole {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case string(RoleOwner):
		return RoleOwner
	case string(RoleEditor):
		return RoleEditor
	case string(RoleViewer):
		return RoleViewer
	default:
		return ""
	}
}

func normalizeMemberRole(role string) AccessRole {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case string(RoleEditor):
		return RoleEditor
	case string(RoleViewer):
		return RoleViewer
	default:
		return ""
	}
}

func marshalState(state interface{}) (string, int32, error) {
	if state == nil {
		return "", 0, ErrInvalidInput
	}
	if jsonlimit.ValueDepth(state) > maxBoardStateDepth {
		return "", 0, ErrInvalidInput
	}
	payload, err := json.Marshal(state)
	if err != nil {
		return "", 0, err
	}
	if len(payload) > maxBoardStateBytes {
		return "", 0, ErrInvalidInput
	}
	envelope, err := validateBoardStateEnvelopeJSON(payload)
	if err != nil {
		return "", 0, err
	}
	return string(payload), int32(envelope.SchemaVersion), nil
}

func resolveClientTime(raw *int64, fallback time.Time) time.Time {
	if raw == nil || *raw <= 0 {
		return fallback
	}
	return time.UnixMilli(*raw)
}

func (s *Service) resolveAccessibleBoard(ctx context.Context, userID int64, boardID string) (*WithRole, error) {
	normalizedID := normalizeBoardID(boardID)
	if normalizedID == "" {
		return nil, ErrInvalidInput
	}
	return s.resolveAccessibleBoardByNormalizedID(ctx, userID, normalizedID)
}

func (s *Service) resolveAccessibleBoardByNormalizedID(ctx context.Context, userID int64, normalizedBoardID string) (*WithRole, error) {
	boardWithRole, err := s.repo.GetAccessibleByBoardID(ctx, userID, normalizedBoardID)
	if err != nil {
		if database.IsNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if boardWithRole == nil || boardWithRole.Role == "" {
		return nil, ErrNotFound
	}
	return boardWithRole, nil
}

func (s *Service) resolveBoardByNormalizedID(ctx context.Context, normalizedBoardID string) (Board, error) {
	board, err := s.repo.GetByBoardID(ctx, normalizedBoardID)
	if err != nil {
		if database.IsNotFound(err) {
			return Board{}, ErrNotFound
		}
		return Board{}, err
	}
	if board == nil {
		return Board{}, ErrNotFound
	}
	return *board, nil
}

func (s *Service) resolveOwnerBoardByNormalizedID(ctx context.Context, userID int64, normalizedBoardID string) (*WithRole, error) {
	board, err := s.repo.GetByOwnerAndBoardID(ctx, userID, normalizedBoardID)
	if err != nil {
		if database.IsNotFound(err) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if board == nil {
		return nil, ErrNotFound
	}
	if board.OwnerUserID != userID {
		return nil, ErrPermissionDenied
	}
	return &WithRole{Board: *board, Role: RoleOwner}, nil
}

func (s *Service) ensureBoardNameAvailable(ctx context.Context, ownerUserID int64, boardID string, name string) error {
	conflicted, err := s.repo.HasNameConflict(ctx, ownerUserID, boardID, name)
	if err != nil {
		return err
	}
	if conflicted {
		return ErrNameConflict
	}
	return nil
}

func (s *Service) ensureBoardIDAvailable(ctx context.Context, ownerUserID int64, boardID string) error {
	conflicted, err := s.repo.BoardIDExists(ctx, boardID)
	if err != nil {
		return err
	}
	if conflicted {
		return ErrIDConflict
	}
	return nil
}

func (s *Service) saveAndReloadBoardForActor(ctx context.Context, actorUserID int64, board *Board) (*WithRole, error) {
	if board == nil {
		return nil, ErrInvalidInput
	}
	if err := s.repo.Upsert(ctx, board); err != nil {
		if IsBoardNameUniqueConstraintError(err) {
			return nil, ErrNameConflict
		}
		if IsBoardIDUniqueConstraintError(err) {
			return nil, ErrIDConflict
		}
		return nil, err
	}
	if board.OwnerUserID == actorUserID {
		return s.resolveOwnerBoardByNormalizedID(ctx, actorUserID, board.BoardID)
	}
	return s.resolveAccessibleBoardByNormalizedID(ctx, actorUserID, board.BoardID)
}

func (s *Service) requireOwnerBoard(ctx context.Context, userID int64, boardID string) (*Board, error) {
	access, err := s.resolveAccessibleBoard(ctx, userID, boardID)
	if err != nil {
		return nil, err
	}
	if access.Role != RoleOwner || access.Board.OwnerUserID != userID {
		return nil, ErrPermissionDenied
	}
	return &access.Board, nil
}

func generateBoardShareToken(ctx context.Context, repo *Repository) (string, error) {
	for i := 0; i < 64; i++ {
		buf := make([]byte, 24)
		if _, err := rand.Read(buf); err != nil {
			return "", fmt.Errorf("生成分享令牌失败: %w", err)
		}
		token := base64.RawURLEncoding.EncodeToString(buf)
		exists, err := repo.ShareTokenHashExists(ctx, boardShareTokenHash(token))
		if err != nil {
			return "", err
		}
		if !exists {
			return token, nil
		}
	}
	return "", fmt.Errorf("生成分享令牌失败")
}

func boardShareTokenHash(token string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(token)))
	return hex.EncodeToString(sum[:])
}

func boardShareTokenPrefix(token string) string {
	trimmed := strings.TrimSpace(token)
	if len(trimmed) <= 8 {
		return trimmed
	}
	return trimmed[:8]
}
