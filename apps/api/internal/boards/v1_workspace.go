package boards

// Workspace-rooted board collaboration store (issue #802).
//
// The legacy boards tables carry no workspace association, archive state, or
// revision history, and sqlc generated code must not be hand-edited. This
// file therefore owns a small additive WorkspaceBoardStore interface with
// context.Context-first methods plus an in-memory implementation used by
// contract tests. A postgres implementation (workspace_board_links mapping +
// immutable board_revisions table) is a documented #809 follow-up; until it
// lands, production handlers fail closed with 501 when the store is nil.
//
// Invariants owned here:
//   - Board external ids mint board_<hex>; legacy non-prefixed rows are out
//     of scope for this store (tolerated read-only by the legacy handlers).
//   - Share token bearers mint sht_<hex>; the bearer is shown once at create
//     (and on idempotent replay) and only its hash is retained for lookup.
//   - Revisions are INSERT-only: ReplaceDocument appends a new immutable
//     revision entry; entries are never mutated and are addressable by
//     revision number or entry id.
//   - Workspace members with visible membership default to viewer
//     (workspace owners default to owner); explicit board rows override
//     the default. Callers with no visible workspace membership and no
//     explicit grant stay invisible (404 at the handler edge).
//   - The last owner cannot be removed or demoted (ErrV1LastOwner).

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"ideaboard/api/internal/platform/contract"
)

var (
	// ErrV1LastOwner blocks removing or demoting the last board owner.
	ErrV1LastOwner = errors.New("board last owner cannot be removed or demoted")
	// ErrV1Archived reports a write against an archived board.
	ErrV1Archived = errors.New("board is archived")
	// ErrV1ShareTokenExpired reports a join against an expired token.
	ErrV1ShareTokenExpired = errors.New("share token expired")
)

// Board document bounds (issue #802): the v1 snapshot is a versioned
// discriminated union {"schema_version": N, "payload": {...}} persisted raw.
const (
	v1BoardNameMaxRunes = 120
	// v1ShareTokenPrefix stays local: sht_ is not a kernel prefix yet
	// (see #801); shape validation delegates to the kernel helper.
	v1ShareTokenPrefix    = "sht_"
	v1MaxDocumentBytes    = 8 * 1024 * 1024
	v1InitialSchemaVerion = int32(1)
)

// V1Board is one workspace-rooted board: the revision counter doubles as the
// ETag value for optimistic document writes.
type V1Board struct {
	WorkspacePublicID string
	PublicID          string
	Name              string
	Revision          int64
	SchemaVersion     int32
	Document          json.RawMessage
	ArchivedAt        *time.Time
	OwnerUserID       int64
	CreatedBy         int64
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// V1BoardMember is one explicit board grant. The board creator/owner is an
// implicit owner and never needs a row.
type V1BoardMember struct {
	BoardPublicID string
	UserID        int64
	Role          AccessRole
	CreatedAt     time.Time
}

// V1AuditEvent carries operator/target/reason/outcome for share and member
// mutations. It is only logged today; #808 persists it as durable audit.
type V1AuditEvent struct {
	At             time.Time `json:"at"`
	OperatorUserID int64     `json:"operator_user_id"`
	Target         string    `json:"target"`
	Reason         string    `json:"reason"`
	Outcome        string    `json:"outcome"`
}

// V1ShareToken is one collaboration bearer. Bearer plaintext lives only in
// the create/replay response; the store retains the hash for join lookup.
// (Production postgres must encrypt the bearer at rest like the legacy
// CreateShareTokenIdempotent path; see the #809 migration note.)
type V1ShareToken struct {
	TokenID       string
	BoardPublicID string
	TokenHash     string
	TokenPrefix   string
	Bearer        string `json:"-"`
	Role          AccessRole
	CreatedBy     int64
	Revoked       bool
	UsageCount    int64
	RevokedAt     *time.Time
	RevokedBy     *int64
	CreatedAt     time.Time
	AuditTrail    []V1AuditEvent
}

// V1Revision is one immutable document snapshot.
type V1Revision struct {
	ID             int64
	BoardPublicID  string
	Revision       int64
	SchemaVersion  int32
	Snapshot       json.RawMessage
	SourceRevision int64
	CreatedBy      int64
	CreatedAt      time.Time
}

// WorkspaceBoardStore is the additive persistence seam for workspace-rooted
// boards. Repository methods take context.Context first.
type WorkspaceBoardStore interface {
	CreateBoard(ctx context.Context, workspacePublicID string, creatorUserID int64, name, publicID string) (*V1Board, error)
	ListBoards(ctx context.Context, workspacePublicID string, includeArchived bool) ([]*V1Board, error)
	GetBoard(ctx context.Context, workspacePublicID, boardPublicID string) (*V1Board, error)
	RenameBoard(ctx context.Context, workspacePublicID, boardPublicID, name string) (*V1Board, error)
	ArchiveBoard(ctx context.Context, workspacePublicID, boardPublicID string) (*V1Board, error)
	RestoreBoard(ctx context.Context, workspacePublicID, boardPublicID string) (*V1Board, error)
	ReplaceDocument(ctx context.Context, workspacePublicID, boardPublicID string, actorUserID, expectedRevision int64, schemaVersion int32, snapshot json.RawMessage) (*V1Board, error)
	ListRevisions(ctx context.Context, workspacePublicID, boardPublicID string) ([]*V1Revision, error)
	GetRevision(ctx context.Context, workspacePublicID, boardPublicID string, revisionOrID int64) (*V1Revision, error)
	GetBoardRole(ctx context.Context, workspacePublicID, boardPublicID string, userID int64, workspaceRole string) (AccessRole, bool, error)
	ListBoardMembers(ctx context.Context, workspacePublicID, boardPublicID string) ([]*V1BoardMember, error)
	AddBoardMember(ctx context.Context, workspacePublicID, boardPublicID string, actorUserID, targetUserID int64, role AccessRole, actorWorkspaceRole string) (*V1BoardMember, error)
	UpdateBoardMemberRole(ctx context.Context, workspacePublicID, boardPublicID string, actorUserID, targetUserID int64, newRole AccessRole, actorWorkspaceRole string) (*V1BoardMember, error)
	RemoveBoardMember(ctx context.Context, workspacePublicID, boardPublicID string, actorUserID, targetUserID int64, actorWorkspaceRole string) error
	CreateShareToken(ctx context.Context, workspacePublicID, boardPublicID string, creatorUserID int64, role AccessRole, tokenID, tokenHash, bearer string) (*V1ShareToken, error)
	ListShareTokens(ctx context.Context, workspacePublicID, boardPublicID string) ([]*V1ShareToken, error)
	RevokeShareToken(ctx context.Context, workspacePublicID, boardPublicID, tokenID string, revokedBy int64) error
	JoinByToken(ctx context.Context, workspacePublicID, bearer string, userID int64) (*V1Board, AccessRole, error)
}

// NewV1BoardPublicID mints an opaque board_ public identifier.
func NewV1BoardPublicID() (string, error) {
	return contract.MintPublicID(contract.PrefixBoard, 12)
}

// NewV1ShareTokenID mints an opaque sht_ share bearer.
func NewV1ShareTokenID() (string, error) {
	return contract.MintPublicID(v1ShareTokenPrefix, 16)
}

// IsV1ShareTokenID reports sht_ shape (opaque URL segment for {token_id}).
func IsV1ShareTokenID(id string) bool {
	return contract.IsValidPublicID(v1ShareTokenPrefix, id)
}

// V1ShareTokenHash derives the lookup hash stored for a bearer.
func V1ShareTokenHash(bearer string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(bearer)))
	return hex.EncodeToString(sum[:])
}

// V1ShareTokenPrefixOf derives the list-safe prefix shown for a bearer.
func V1ShareTokenPrefixOf(bearer string) string {
	trimmed := strings.TrimSpace(bearer)
	if len(trimmed) <= 8 {
		return trimmed
	}
	return trimmed[:8]
}

// ValidateV1BoardName validates v1 board display names (1..120 runes after
// trim; the legacy 60-rune bound stays untouched on the old surface).
func ValidateV1BoardName(name string) error {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" || utf8.RuneCountInString(trimmed) > v1BoardNameMaxRunes {
		return errors.New("boards: v1 board name must be 1..120 characters")
	}
	return nil
}

// ValidateV1BoardRole validates explicit board grant roles.
func ValidateV1BoardRole(role AccessRole) error {
	switch role {
	case RoleOwner, RoleEditor, RoleViewer:
		return nil
	default:
		return ErrMemberInvalidRole
	}
}

// DefaultBoardRoleForWorkspaceRole maps a visible workspace membership role
// to the default board role: workspace owners default to board owner,
// admins/members default to viewer. Unknown/empty roles report ok=false so
// callers stay fail-closed. Explicit board rows always override this
// default (check explicit first, then fall back).
func DefaultBoardRoleForWorkspaceRole(workspaceRole string) (AccessRole, bool) {
	switch strings.TrimSpace(workspaceRole) {
	case "owner":
		return RoleOwner, true
	case "admin", "member":
		return RoleViewer, true
	default:
		return "", false
	}
}

// FormatV1ETag renders the revision counter as the ETag validator value.
func FormatV1ETag(revision int64) string {
	return fmt.Sprintf("%d", revision)
}

// ParseV1IfMatch parses an If-Match header into the expected revision.
// Weak validators (W/), quotes, and surrounding spaces are tolerated; "*"
// and empty values report ErrInvalidInput so callers render 400.
func ParseV1IfMatch(raw string) (int64, error) {
	trimmed := strings.TrimSpace(raw)
	trimmed = strings.TrimPrefix(trimmed, "W/")
	trimmed = strings.Trim(trimmed, `"`)
	trimmed = strings.TrimSpace(trimmed)
	if trimmed == "" || trimmed == "*" {
		return 0, ErrInvalidInput
	}
	var revision int64
	if _, err := fmt.Sscanf(trimmed, "%d", &revision); err != nil || revision <= 0 {
		return 0, ErrInvalidInput
	}
	return revision, nil
}

// v1DocumentEnvelope is the versioned discriminated union persisted as the
// board snapshot. Unknown fields are rejected by the strict decoder.
type v1DocumentEnvelope struct {
	SchemaVersion int32           `json:"schema_version"`
	Payload       json.RawMessage `json:"payload"`
}

// ValidateV1DocumentStrict validates a raw snapshot: schema_version (>=1) +
// object payload, unknown fields and trailing payloads rejected.
func ValidateV1DocumentStrict(raw []byte) (int32, json.RawMessage, error) {
	if len(raw) == 0 || len(raw) > v1MaxDocumentBytes {
		return 0, nil, ErrInvalidInput
	}
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	var envelope v1DocumentEnvelope
	if err := decoder.Decode(&envelope); err != nil {
		return 0, nil, ErrInvalidInput
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return 0, nil, ErrInvalidInput
	}
	if envelope.SchemaVersion < 1 {
		return 0, nil, ErrInvalidInput
	}
	payload := envelope.Payload
	if len(payload) == 0 {
		return 0, nil, ErrInvalidInput
	}
	var object map[string]any
	if err := json.Unmarshal(payload, &object); err != nil || object == nil {
		return 0, nil, ErrInvalidInput
	}
	return envelope.SchemaVersion, payload, nil
}

// V1InitialDocument renders the empty revision-1 snapshot.
func V1InitialDocument() json.RawMessage {
	return json.RawMessage(`{"schema_version":1,"payload":{}}`)
}

func v1BoardKey(workspacePublicID, boardPublicID string) string {
	return workspacePublicID + "\x00" + boardPublicID
}

// InMemoryWorkspaceBoardStore is a mutex-guarded WorkspaceBoardStore for
// tests ONLY. It must never be wired in production.
type InMemoryWorkspaceBoardStore struct {
	mu         sync.Mutex
	boards     map[string]map[string]*V1Board
	members    map[string]map[int64]*V1BoardMember
	shares     map[string]map[string]*V1ShareToken
	shareIndex map[string]*V1ShareToken
	revisions  map[string][]*V1Revision
	nextRevID  int64
	now        func() time.Time
}

// NewInMemoryWorkspaceBoardStoreForTests builds an empty test-only store.
func NewInMemoryWorkspaceBoardStoreForTests() *InMemoryWorkspaceBoardStore {
	return &InMemoryWorkspaceBoardStore{
		boards:     make(map[string]map[string]*V1Board),
		members:    make(map[string]map[int64]*V1BoardMember),
		shares:     make(map[string]map[string]*V1ShareToken),
		shareIndex: make(map[string]*V1ShareToken),
		revisions:  make(map[string][]*V1Revision),
		now:        time.Now,
	}
}

func (s *InMemoryWorkspaceBoardStore) clock() time.Time {
	if s.now != nil {
		return s.now().UTC()
	}
	return time.Now().UTC()
}

func copyV1Board(board *V1Board) *V1Board {
	if board == nil {
		return nil
	}
	out := *board
	if board.ArchivedAt != nil {
		archived := *board.ArchivedAt
		out.ArchivedAt = &archived
	}
	if len(board.Document) > 0 {
		document := make(json.RawMessage, len(board.Document))
		copy(document, board.Document)
		out.Document = document
	}
	return &out
}

func copyV1Revision(revision *V1Revision) *V1Revision {
	if revision == nil {
		return nil
	}
	out := *revision
	if len(revision.Snapshot) > 0 {
		snapshot := make(json.RawMessage, len(revision.Snapshot))
		copy(snapshot, revision.Snapshot)
		out.Snapshot = snapshot
	}
	return &out
}

func copyV1ShareToken(token *V1ShareToken) *V1ShareToken {
	if token == nil {
		return nil
	}
	out := *token
	out.Bearer = ""
	if token.RevokedAt != nil {
		revoked := *token.RevokedAt
		out.RevokedAt = &revoked
	}
	if token.RevokedBy != nil {
		revokedBy := *token.RevokedBy
		out.RevokedBy = &revokedBy
	}
	out.AuditTrail = append([]V1AuditEvent(nil), token.AuditTrail...)
	return &out
}

func (s *InMemoryWorkspaceBoardStore) getBoardLocked(key, boardPublicID string) (*V1Board, error) {
	byBoard, ok := s.boards[key]
	if !ok {
		// Distinguish unknown workspace (also not found: callers already
		// converged workspace visibility to 404 before reaching here).
		return nil, ErrNotFound
	}
	// key is the workspace prefix; boards are stored per workspace map.
	board, ok := byBoard[boardPublicID]
	if !ok {
		return nil, ErrNotFound
	}
	return board, nil
}

// boardRoleLocked reports the effective board role: the creator/owner row is
// an implicit owner; explicit member rows override; visible workspace
// members fall back to viewer (owners to owner). Unknown/empty workspace
// roles stay fail-closed (no default).
func (s *InMemoryWorkspaceBoardStore) boardRoleLocked(key string, board *V1Board, userID int64, workspaceRole string) (AccessRole, bool) {
	if board.OwnerUserID == userID {
		return RoleOwner, true
	}
	if member, ok := s.members[key][userID]; ok {
		return member.Role, true
	}
	if role, ok := DefaultBoardRoleForWorkspaceRole(workspaceRole); ok {
		return role, true
	}
	return "", false
}

func (s *InMemoryWorkspaceBoardStore) ownerCountLocked(key string, board *V1Board) int {
	count := 1 // creator/owner row is always an owner.
	for userID, member := range s.members[key] {
		if userID != board.OwnerUserID && member.Role == RoleOwner {
			count++
		}
	}
	return count
}

// CreateBoard mints a workspace-rooted board at revision 1 with the empty
// discriminated-union snapshot; the creator is the implicit owner.
func (s *InMemoryWorkspaceBoardStore) CreateBoard(ctx context.Context, workspacePublicID string, creatorUserID int64, name, publicID string) (*V1Board, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(workspacePublicID) == "" || creatorUserID <= 0 {
		return nil, ErrInvalidInput
	}
	if err := ValidateV1BoardName(name); err != nil {
		return nil, err
	}
	trimmedID := strings.TrimSpace(publicID)
	if trimmedID == "" {
		return nil, ErrInvalidInput
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.boards[workspacePublicID] == nil {
		s.boards[workspacePublicID] = make(map[string]*V1Board)
	}
	if _, exists := s.boards[workspacePublicID][trimmedID]; exists {
		return nil, ErrIDConflict
	}
	now := s.clock()
	document := V1InitialDocument()
	board := &V1Board{
		WorkspacePublicID: workspacePublicID,
		PublicID:          trimmedID,
		Name:              strings.TrimSpace(name),
		Revision:          1,
		SchemaVersion:     v1InitialSchemaVerion,
		Document:          document,
		OwnerUserID:       creatorUserID,
		CreatedBy:         creatorUserID,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	s.boards[workspacePublicID][trimmedID] = board
	key := v1BoardKey(workspacePublicID, trimmedID)
	s.nextRevID++
	s.revisions[key] = append(s.revisions[key], &V1Revision{
		ID:             s.nextRevID,
		BoardPublicID:  trimmedID,
		Revision:       1,
		SchemaVersion:  v1InitialSchemaVerion,
		Snapshot:       document,
		SourceRevision: 0,
		CreatedBy:      creatorUserID,
		CreatedAt:      now,
	})
	return copyV1Board(board), nil
}

// ListBoards lists workspace boards ordered by updated_at desc (stable by
// public id); archived rows are excluded unless includeArchived.
func (s *InMemoryWorkspaceBoardStore) ListBoards(ctx context.Context, workspacePublicID string, includeArchived bool) ([]*V1Board, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*V1Board
	for _, board := range s.boards[workspacePublicID] {
		if board.ArchivedAt != nil && !includeArchived {
			continue
		}
		out = append(out, copyV1Board(board))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].UpdatedAt.Equal(out[j].UpdatedAt) {
			return out[i].PublicID < out[j].PublicID
		}
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	return out, nil
}

// GetBoard reads one board (archived rows stay visible here; callers decide
// whether the operation is allowed on them).
func (s *InMemoryWorkspaceBoardStore) GetBoard(ctx context.Context, workspacePublicID, boardPublicID string) (*V1Board, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	board, err := s.getBoardLocked(workspacePublicID, strings.TrimSpace(boardPublicID))
	if err != nil {
		return nil, err
	}
	return copyV1Board(board), nil
}

// RenameBoard renames a live board; the revision counter is untouched.
func (s *InMemoryWorkspaceBoardStore) RenameBoard(ctx context.Context, workspacePublicID, boardPublicID, name string) (*V1Board, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := ValidateV1BoardName(name); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	board, err := s.getBoardLocked(workspacePublicID, strings.TrimSpace(boardPublicID))
	if err != nil {
		return nil, err
	}
	if board.ArchivedAt != nil {
		return nil, ErrV1Archived
	}
	board.Name = strings.TrimSpace(name)
	board.UpdatedAt = s.clock()
	return copyV1Board(board), nil
}

// ArchiveBoard archives a live board; already-archived rows read as not
// found so archive stays idempotent-safe at the handler layer.
func (s *InMemoryWorkspaceBoardStore) ArchiveBoard(ctx context.Context, workspacePublicID, boardPublicID string) (*V1Board, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	board, err := s.getBoardLocked(workspacePublicID, strings.TrimSpace(boardPublicID))
	if err != nil {
		return nil, err
	}
	if board.ArchivedAt != nil {
		return nil, ErrNotFound
	}
	now := s.clock()
	board.ArchivedAt = &now
	board.UpdatedAt = now
	return copyV1Board(board), nil
}

// RestoreBoard clears the archive timestamp; live boards read as not found.
func (s *InMemoryWorkspaceBoardStore) RestoreBoard(ctx context.Context, workspacePublicID, boardPublicID string) (*V1Board, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	board, err := s.getBoardLocked(workspacePublicID, strings.TrimSpace(boardPublicID))
	if err != nil {
		return nil, err
	}
	if board.ArchivedAt == nil {
		return nil, ErrNotFound
	}
	board.ArchivedAt = nil
	board.UpdatedAt = s.clock()
	return copyV1Board(board), nil
}

// ReplaceDocument appends an immutable revision when the expected revision
// matches the current counter; stale writers get ErrRevisionConflict.
func (s *InMemoryWorkspaceBoardStore) ReplaceDocument(ctx context.Context, workspacePublicID, boardPublicID string, actorUserID, expectedRevision int64, schemaVersion int32, snapshot json.RawMessage) (*V1Board, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if actorUserID <= 0 || expectedRevision <= 0 || schemaVersion < 1 || len(snapshot) == 0 {
		return nil, ErrInvalidInput
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	board, err := s.getBoardLocked(workspacePublicID, strings.TrimSpace(boardPublicID))
	if err != nil {
		return nil, err
	}
	if board.ArchivedAt != nil {
		return nil, ErrV1Archived
	}
	if board.Revision != expectedRevision {
		return nil, ErrRevisionConflict
	}
	document := make(json.RawMessage, len(snapshot))
	copy(document, snapshot)
	board.Revision++
	board.SchemaVersion = schemaVersion
	board.Document = document
	board.UpdatedAt = s.clock()
	key := v1BoardKey(workspacePublicID, board.PublicID)
	s.nextRevID++
	s.revisions[key] = append(s.revisions[key], &V1Revision{
		ID:             s.nextRevID,
		BoardPublicID:  board.PublicID,
		Revision:       board.Revision,
		SchemaVersion:  schemaVersion,
		Snapshot:       document,
		SourceRevision: expectedRevision,
		CreatedBy:      actorUserID,
		CreatedAt:      board.UpdatedAt,
	})
	return copyV1Board(board), nil
}

// ListRevisions returns immutable snapshots oldest-first (copies; the stored
// entries are never mutated after append).
func (s *InMemoryWorkspaceBoardStore) ListRevisions(ctx context.Context, workspacePublicID, boardPublicID string) ([]*V1Revision, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.getBoardLocked(workspacePublicID, strings.TrimSpace(boardPublicID)); err != nil {
		return nil, err
	}
	entries := s.revisions[v1BoardKey(workspacePublicID, strings.TrimSpace(boardPublicID))]
	out := make([]*V1Revision, 0, len(entries))
	for _, entry := range entries {
		out = append(out, copyV1Revision(entry))
	}
	return out, nil
}

// GetRevision resolves an entry by revision number first, then by entry id.
func (s *InMemoryWorkspaceBoardStore) GetRevision(ctx context.Context, workspacePublicID, boardPublicID string, revisionOrID int64) (*V1Revision, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.getBoardLocked(workspacePublicID, strings.TrimSpace(boardPublicID)); err != nil {
		return nil, err
	}
	entries := s.revisions[v1BoardKey(workspacePublicID, strings.TrimSpace(boardPublicID))]
	for _, entry := range entries {
		if entry.Revision == revisionOrID {
			return copyV1Revision(entry), nil
		}
	}
	for _, entry := range entries {
		if entry.ID == revisionOrID {
			return copyV1Revision(entry), nil
		}
	}
	return nil, ErrNotFound
}

// GetBoardRole reports the effective board role (creator counts as owner;
// explicit rows override; visible workspace members fall back to viewer,
// workspace owners to owner; unknown/empty workspace roles stay denied).
func (s *InMemoryWorkspaceBoardStore) GetBoardRole(ctx context.Context, workspacePublicID, boardPublicID string, userID int64, workspaceRole string) (AccessRole, bool, error) {
	if err := ctx.Err(); err != nil {
		return "", false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	board, err := s.getBoardLocked(workspacePublicID, strings.TrimSpace(boardPublicID))
	if err != nil {
		return "", false, err
	}
	role, ok := s.boardRoleLocked(v1BoardKey(workspacePublicID, board.PublicID), board, userID, workspaceRole)
	return role, ok, nil
}

// ListBoardMembers lists explicit grants ordered by user id (the implicit
// owner row is not listed here; handlers prepend it from the board row).
func (s *InMemoryWorkspaceBoardStore) ListBoardMembers(ctx context.Context, workspacePublicID, boardPublicID string) ([]*V1BoardMember, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	board, err := s.getBoardLocked(workspacePublicID, strings.TrimSpace(boardPublicID))
	if err != nil {
		return nil, err
	}
	key := v1BoardKey(workspacePublicID, board.PublicID)
	out := make([]*V1BoardMember, 0, len(s.members[key]))
	for _, member := range s.members[key] {
		copied := *member
		out = append(out, &copied)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UserID < out[j].UserID })
	return out, nil
}

// AddBoardMember grants a role idempotently: re-adding keeps the existing
// role. Only board owners may grant; only owners may grant owner.
// The actor's effective role includes the workspace default (workspace
// owners count as board owners); explicit rows still override.
func (s *InMemoryWorkspaceBoardStore) AddBoardMember(ctx context.Context, workspacePublicID, boardPublicID string, actorUserID, targetUserID int64, role AccessRole, actorWorkspaceRole string) (*V1BoardMember, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := ValidateV1BoardRole(role); err != nil {
		return nil, err
	}
	if actorUserID <= 0 || targetUserID <= 0 {
		return nil, ErrInvalidInput
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	board, err := s.getBoardLocked(workspacePublicID, strings.TrimSpace(boardPublicID))
	if err != nil {
		return nil, err
	}
	key := v1BoardKey(workspacePublicID, board.PublicID)
	actorRole, ok := s.boardRoleLocked(key, board, actorUserID, actorWorkspaceRole)
	if !ok || actorRole != RoleOwner {
		return nil, ErrPermissionDenied
	}
	if targetUserID == board.OwnerUserID {
		return nil, ErrMemberInviteSelf
	}
	if existing, ok := s.members[key][targetUserID]; ok {
		copied := *existing
		return &copied, nil
	}
	if role == RoleOwner && actorRole != RoleOwner {
		return nil, ErrPermissionDenied
	}
	now := s.clock()
	member := &V1BoardMember{BoardPublicID: board.PublicID, UserID: targetUserID, Role: role, CreatedAt: now}
	if s.members[key] == nil {
		s.members[key] = make(map[int64]*V1BoardMember)
	}
	s.members[key][targetUserID] = member
	copied := *member
	return &copied, nil
}

// UpdateBoardMemberRole applies owner-only transitions with last-owner
// protection. The actor's effective role includes the workspace default.
func (s *InMemoryWorkspaceBoardStore) UpdateBoardMemberRole(ctx context.Context, workspacePublicID, boardPublicID string, actorUserID, targetUserID int64, newRole AccessRole, actorWorkspaceRole string) (*V1BoardMember, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := ValidateV1BoardRole(newRole); err != nil {
		return nil, err
	}
	if actorUserID <= 0 || targetUserID <= 0 {
		return nil, ErrInvalidInput
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	board, err := s.getBoardLocked(workspacePublicID, strings.TrimSpace(boardPublicID))
	if err != nil {
		return nil, err
	}
	key := v1BoardKey(workspacePublicID, board.PublicID)
	actorRole, ok := s.boardRoleLocked(key, board, actorUserID, actorWorkspaceRole)
	if !ok || actorRole != RoleOwner {
		return nil, ErrPermissionDenied
	}
	if targetUserID == board.OwnerUserID {
		if newRole != RoleOwner && s.ownerCountLocked(key, board) <= 1 {
			return nil, ErrV1LastOwner
		}
		if newRole != RoleOwner {
			return nil, ErrPermissionDenied
		}
		return &V1BoardMember{BoardPublicID: board.PublicID, UserID: targetUserID, Role: RoleOwner}, nil
	}
	member, ok := s.members[key][targetUserID]
	if !ok {
		return nil, ErrMemberNotFound
	}
	if member.Role == RoleOwner && newRole != RoleOwner && s.ownerCountLocked(key, board) <= 1 {
		return nil, ErrV1LastOwner
	}
	member.Role = newRole
	copied := *member
	return &copied, nil
}

// RemoveBoardMember removes a grant: owners may remove anyone, anyone may
// remove themselves. The last owner is protected. The actor's effective
// role includes the workspace default.
func (s *InMemoryWorkspaceBoardStore) RemoveBoardMember(ctx context.Context, workspacePublicID, boardPublicID string, actorUserID, targetUserID int64, actorWorkspaceRole string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if actorUserID <= 0 || targetUserID <= 0 {
		return ErrInvalidInput
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	board, err := s.getBoardLocked(workspacePublicID, strings.TrimSpace(boardPublicID))
	if err != nil {
		return err
	}
	key := v1BoardKey(workspacePublicID, board.PublicID)
	if targetUserID == board.OwnerUserID {
		return ErrMemberInviteSelf
	}
	member, ok := s.members[key][targetUserID]
	if !ok {
		return ErrMemberNotFound
	}
	if member.Role == RoleOwner && s.ownerCountLocked(key, board) <= 1 {
		return ErrV1LastOwner
	}
	if actorUserID != targetUserID {
		actorRole, ok := s.boardRoleLocked(key, board, actorUserID, actorWorkspaceRole)
		if !ok || actorRole != RoleOwner {
			return ErrPermissionDenied
		}
	}
	delete(s.members[key], targetUserID)
	return nil
}

// CreateShareToken stores one bearer (plaintext in memory only; production
// postgres must encrypt at rest). Bearer uniqueness rides the hash unique
// index; collisions return ErrIDConflict so callers mint again.
func (s *InMemoryWorkspaceBoardStore) CreateShareToken(ctx context.Context, workspacePublicID, boardPublicID string, creatorUserID int64, role AccessRole, tokenID, tokenHash, bearer string) (*V1ShareToken, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if role != RoleEditor && role != RoleViewer {
		return nil, ErrMemberInvalidRole
	}
	if !IsV1ShareTokenID(strings.TrimSpace(tokenID)) || strings.TrimSpace(tokenHash) == "" || strings.TrimSpace(bearer) == "" {
		return nil, ErrInvalidInput
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	board, err := s.getBoardLocked(workspacePublicID, strings.TrimSpace(boardPublicID))
	if err != nil {
		return nil, err
	}
	key := v1BoardKey(workspacePublicID, board.PublicID)
	if _, exists := s.shareIndex[strings.TrimSpace(tokenHash)]; exists {
		return nil, ErrIDConflict
	}
	now := s.clock()
	token := &V1ShareToken{
		TokenID:       strings.TrimSpace(tokenID),
		BoardPublicID: board.PublicID,
		TokenHash:     strings.TrimSpace(tokenHash),
		TokenPrefix:   V1ShareTokenPrefixOf(bearer),
		Bearer:        strings.TrimSpace(bearer),
		Role:          role,
		CreatedBy:     creatorUserID,
		CreatedAt:     now,
		AuditTrail: []V1AuditEvent{{
			At:             now,
			OperatorUserID: creatorUserID,
			Target:         strings.TrimSpace(tokenID),
			Reason:         "share_create",
			Outcome:        "created",
		}},
	}
	if s.shares[key] == nil {
		s.shares[key] = make(map[string]*V1ShareToken)
	}
	s.shares[key][token.TokenID] = token
	s.shareIndex[token.TokenHash] = token
	out := copyV1ShareToken(token)
	out.Bearer = token.Bearer
	return out, nil
}

// ListShareTokens lists bearers newest-first without secrets.
func (s *InMemoryWorkspaceBoardStore) ListShareTokens(ctx context.Context, workspacePublicID, boardPublicID string) ([]*V1ShareToken, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	board, err := s.getBoardLocked(workspacePublicID, strings.TrimSpace(boardPublicID))
	if err != nil {
		return nil, err
	}
	key := v1BoardKey(workspacePublicID, board.PublicID)
	out := make([]*V1ShareToken, 0, len(s.shares[key]))
	for _, token := range s.shares[key] {
		out = append(out, copyV1ShareToken(token))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

// RevokeShareToken revokes a bearer with audit fields (idempotent: revoking
// twice succeeds).
func (s *InMemoryWorkspaceBoardStore) RevokeShareToken(ctx context.Context, workspacePublicID, boardPublicID, tokenID string, revokedBy int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	board, err := s.getBoardLocked(workspacePublicID, strings.TrimSpace(boardPublicID))
	if err != nil {
		return err
	}
	key := v1BoardKey(workspacePublicID, board.PublicID)
	token, ok := s.shares[key][strings.TrimSpace(tokenID)]
	if !ok {
		return ErrShareTokenNotFound
	}
	now := s.clock()
	if !token.Revoked {
		token.Revoked = true
		token.RevokedAt = &now
		token.RevokedBy = &revokedBy
	}
	token.AuditTrail = append(token.AuditTrail, V1AuditEvent{
		At:             now,
		OperatorUserID: revokedBy,
		Target:         token.TokenID,
		Reason:         "share_revoke",
		Outcome:        "revoked",
	})
	return nil
}

// JoinByToken consumes a bearer idempotently: repeat joins by the same user
// converge to the same role (outcome "rejoined"); the usage counter and
// audit trail still advance so joins stay auditable for #808.
func (s *InMemoryWorkspaceBoardStore) JoinByToken(ctx context.Context, workspacePublicID, bearer string, userID int64) (*V1Board, AccessRole, error) {
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	if userID <= 0 || strings.TrimSpace(bearer) == "" {
		return nil, "", ErrInvalidInput
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	token, ok := s.shareIndex[V1ShareTokenHash(bearer)]
	if !ok || token == nil {
		return nil, "", ErrShareTokenNotFound
	}
	board, err := s.getBoardLocked(workspacePublicID, token.BoardPublicID)
	if err != nil {
		return nil, "", ErrShareTokenNotFound
	}
	if token.Revoked {
		return nil, "", ErrShareTokenRevoked
	}
	if token.Role != RoleEditor && token.Role != RoleViewer {
		return nil, "", ErrShareTokenInvalid
	}
	if board.ArchivedAt != nil {
		return nil, "", ErrV1Archived
	}
	now := s.clock()
	key := v1BoardKey(workspacePublicID, board.PublicID)
	token.UsageCount++
	outcome := "joined"
	if board.OwnerUserID == userID {
		outcome = "owner_rejoined"
	} else if existing, ok := s.members[key][userID]; ok && existing.Role == token.Role {
		outcome = "rejoined"
	} else {
		if s.members[key] == nil {
			s.members[key] = make(map[int64]*V1BoardMember)
		}
		if existing, ok := s.members[key][userID]; ok {
			existing.Role = token.Role
		} else {
			s.members[key][userID] = &V1BoardMember{BoardPublicID: board.PublicID, UserID: userID, Role: token.Role, CreatedAt: now}
		}
	}
	token.AuditTrail = append(token.AuditTrail, V1AuditEvent{
		At:             now,
		OperatorUserID: userID,
		Target:         token.TokenID,
		Reason:         "share_join",
		Outcome:        outcome,
	})
	role, _ := s.boardRoleLocked(key, board, userID, "")
	return copyV1Board(board), role, nil
}

// Compile-time convergence: the in-memory store implements the seam.
var _ WorkspaceBoardStore = (*InMemoryWorkspaceBoardStore)(nil)
