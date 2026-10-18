package boards

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"ideaboard/api/internal/boards/previewscene"
	"ideaboard/api/internal/events"
	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/tenancy"

	"github.com/google/uuid"
)

var (
	// ErrV1Archived reports a write against an archived board.
	ErrV1Archived      = errors.New("board is archived")
	ErrV1CatalogPage   = errors.New("invalid board catalog page")
	ErrV1CatalogOffset = errors.New("invalid board catalog offset")
)

// Board document bounds (issue #802): the v1 snapshot is a versioned
// discriminated union {"schema_version": N, "payload": {...}} persisted raw.
const (
	v1BoardNameMaxRunes   = 120
	v1MaxDocumentBytes    = 8 * 1024 * 1024
	v1InitialSchemaVerion = int32(1)
)

// V1Board is one workspace-rooted board: the revision counter doubles as the
// ETag value for optimistic document writes. ContentVersion is the opaque
// identity of the accepted persisted content (hex SHA-256 of the canonical
// document envelope, computed in the accepting transaction; metadata
// renames and archive transitions never change it). PreviewScene is the
// stored versioned scene projection (nil when the document schema is not
// extractable); the Gallery catalog reads it without fetching documents.
// WorkspaceAccess is the common role for selected members; AccessVersion is the
// independent monotonic access counter (ETag access-<version>), bumped only
// by access mutations and never by document writes.
type V1Board struct {
	WorkspacePublicID string
	PublicID          string
	Name              string
	Revision          int64
	SchemaVersion     int32
	Document          json.RawMessage
	ContentVersion    string
	PreviewScene      json.RawMessage
	ArchivedAt        *time.Time
	OwnerUserID       int64
	CreatedBy         int64
	WorkspaceAccess   WorkspaceAccess
	AccessVersion     int64
	CreatedAt         time.Time
	UpdatedAt         time.Time
}

// V1BoardCatalogEntry is one Gallery catalog card: board metadata plus the
// stored scene projection, WITHOUT the full document. Served by
// ListBoardCatalog in a single query that never reads snapshot_data.
type V1BoardCatalogEntry struct {
	WorkspacePublicID string
	PublicID          string
	Name              string
	Revision          int64
	SchemaVersion     int32
	ContentVersion    string
	PreviewScene      json.RawMessage
	ArchivedAt        *time.Time
	OwnerUserID       int64
	CreatedBy         int64
	WorkspaceAccess   WorkspaceAccess
	AccessVersion     int64
	// EffectiveRole is the caller's resolved role for this card
	// (catalog SQL computes the set-form of ResolveRole; the in-memory
	// scan resolves it per board), so handlers render capabilities
	// without per-row lookups. HasDirectGrant marks the caller's own
	// grant row; AccessState is the board-level posture.
	EffectiveRole  AccessRole
	HasDirectGrant bool
	AccessState    AccessState
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type V1BoardCatalogPageRequest struct {
	Offset int
	Limit  int
}

type V1BoardCatalogPage struct {
	Entries []*V1BoardCatalogEntry
	HasMore bool
}

const maxBoardCatalogRows = 500

func (page V1BoardCatalogPageRequest) validate() error {
	if page.Offset < 0 || page.Offset > maxBoardCatalogRows {
		return ErrV1CatalogOffset
	}
	if page.Limit < 1 || page.Limit > 100 {
		return ErrV1CatalogPage
	}
	return nil
}

// V1BoardMember records a selected Workspace member. The Board creator is an
// implicit owner and never needs a row.
type V1BoardMember struct {
	BoardPublicID string
	UserID        int64
	CreatedAt     time.Time
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

// V1BoardAccess is the access-resource view of one board: the workspace
// common role, selected members, monotonic version, and sharing posture. It
// carries no document bytes, revision, or preview payload.
type V1BoardAccess struct {
	WorkspacePublicID string
	BoardPublicID     string
	WorkspaceAccess   WorkspaceAccess
	AccessVersion     int64
	AccessState       AccessState
	MemberUserIDs     []int64
	ArchivedAt        *time.Time
	AccessSignals     []events.BoardAccessOutboxRecord
}

// WorkspaceBoardStore is the additive persistence seam for workspace-rooted
// boards. Repository methods take context.Context first.
type WorkspaceBoardStore interface {
	CreateBoard(ctx context.Context, workspacePublicID string, creatorUserID int64, name, publicID string) (*V1Board, error)
	ListBoardCatalog(ctx context.Context, workspacePublicID string, includeArchived bool, userID int64, workspaceRole string, page V1BoardCatalogPageRequest) (V1BoardCatalogPage, error)
	GetBoard(ctx context.Context, workspacePublicID, boardPublicID string) (*V1Board, error)
	RenameBoard(ctx context.Context, workspacePublicID, boardPublicID, name string, actorUserID int64) (*V1Board, error)
	ArchiveBoard(ctx context.Context, workspacePublicID, boardPublicID string, actorUserID int64) (*V1Board, error)
	RestoreBoard(ctx context.Context, workspacePublicID, boardPublicID string, actorUserID int64) (*V1Board, error)
	ReplaceDocument(ctx context.Context, workspacePublicID, boardPublicID string, actorUserID, expectedRevision int64, schemaVersion int32, snapshot json.RawMessage) (*V1Board, error)
	ListRevisions(ctx context.Context, workspacePublicID, boardPublicID string) ([]*V1Revision, error)
	GetRevision(ctx context.Context, workspacePublicID, boardPublicID string, revisionOrID int64) (*V1Revision, error)
	GetBoardRole(ctx context.Context, workspacePublicID, boardPublicID string, userID int64, workspaceRole string) (AccessRole, bool, error)
	ListBoardsReferencingAsset(ctx context.Context, workspacePublicID, assetPublicID string) ([]string, error)
	ListBoardMembers(ctx context.Context, workspacePublicID, boardPublicID string) ([]*V1BoardMember, error)
	GetBoardAccess(ctx context.Context, workspacePublicID, boardPublicID string) (*V1BoardAccess, error)
	ReplaceBoardAccess(ctx context.Context, workspacePublicID, boardPublicID string, actorUserID, expectedVersion int64, access WorkspaceAccess, selectedUserIDs []int64) (*V1BoardAccess, error)
	RemoveWorkspaceMemberGrants(ctx context.Context, workspacePublicID string, userID int64) ([]tenancy.MemberGrantCleanup, error)
}

// NewV1BoardPublicID mints an opaque board_ public identifier.
func NewV1BoardPublicID() (string, error) {
	return contract.MintPublicID(contract.PrefixBoard, 12)
}

// ValidateV1BoardName validates v1 board display names (1..120 runes after
// trim).
func ValidateV1BoardName(name string) error {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" || utf8.RuneCountInString(trimmed) > v1BoardNameMaxRunes {
		return errors.New("boards: v1 board name must be 1..120 characters")
	}
	return nil
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

// splitV1BoardKey recovers the workspace + board public ids from a board
// key. ok is false when the key carries no separator (never for keys minted
// by v1BoardKey).
func splitV1BoardKey(key string) (workspacePublicID, boardPublicID string, ok bool) {
	workspacePublicID, boardPublicID, ok = strings.Cut(key, "\x00")
	return workspacePublicID, boardPublicID, ok
}

// WorkspaceMembershipScope holds membership facts stable while a Board mutation is accepted.
type WorkspaceMembershipScope interface {
	WithWorkspaceMembers(ctx context.Context, workspacePublicID string, action func(roleFor func(int64) string) error) error
}

func withWorkspaceMembers[T any](ctx context.Context, scope WorkspaceMembershipScope, workspacePublicID string, action func(roleFor func(int64) string) (T, error)) (T, error) {
	var result T
	if scope == nil {
		return result, ErrNotFound
	}
	err := scope.WithWorkspaceMembers(ctx, workspacePublicID, func(roleFor func(int64) string) error {
		var err error
		result, err = action(roleFor)
		return err
	})
	return result, err
}

type AccessSignalRecipientDirectory interface {
	GetUserPublicID(context.Context, int64) (uuid.UUID, error)
}

// InMemoryWorkspaceBoardStore is a mutex-guarded WorkspaceBoardStore for
// tests ONLY. It must never be wired in production.
type InMemoryWorkspaceBoardStore struct {
	accessRecipients AccessSignalRecipientDirectory
	workspaceMembers WorkspaceMembershipScope
	mu               sync.Mutex
	boards           map[string]map[string]*V1Board
	members          map[string]map[int64]*V1BoardMember
	revisions        map[string][]*V1Revision
	// mediaRefs tracks accepted document -> media asset references per
	// board key (#936, in-memory twin of board_document_media_refs).
	// Reconciled on every ReplaceDocument; read through the narrow
	// BoardMediaRefReader port only.
	mediaRefs map[string]map[string]struct{}
	nextRevID int64
	now       func() time.Time
}

// NewInMemoryWorkspaceBoardStoreForTests builds an empty test-only store.
func NewInMemoryWorkspaceBoardStoreForTests(workspaceMembers WorkspaceMembershipScope, accessRecipients AccessSignalRecipientDirectory) *InMemoryWorkspaceBoardStore {
	return &InMemoryWorkspaceBoardStore{
		workspaceMembers: workspaceMembers,
		accessRecipients: accessRecipients,
		boards:           make(map[string]map[string]*V1Board),
		members:          make(map[string]map[int64]*V1BoardMember),
		revisions:        make(map[string][]*V1Revision),
		mediaRefs:        make(map[string]map[string]struct{}),
		now:              time.Now,
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
	if len(board.PreviewScene) > 0 {
		scene := make(json.RawMessage, len(board.PreviewScene))
		copy(scene, board.PreviewScene)
		out.PreviewScene = scene
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

// v1BoardCatalogEntryOf projects one board to its Gallery catalog card:
// metadata plus the stored scene projection, never the full document. The
// caller passes the already-resolved role facts (never recomputed here).
func v1BoardCatalogEntryOf(board *V1Board, role AccessRole, hasDirectGrant bool, hasAnyGrants bool) *V1BoardCatalogEntry {
	if board == nil {
		return nil
	}
	out := &V1BoardCatalogEntry{
		WorkspacePublicID: board.WorkspacePublicID,
		PublicID:          board.PublicID,
		Name:              board.Name,
		Revision:          board.Revision,
		SchemaVersion:     board.SchemaVersion,
		ContentVersion:    board.ContentVersion,
		OwnerUserID:       board.OwnerUserID,
		CreatedBy:         board.CreatedBy,
		WorkspaceAccess:   board.WorkspaceAccess,
		AccessVersion:     board.AccessVersion,
		EffectiveRole:     role,
		HasDirectGrant:    hasDirectGrant,
		AccessState:       DeriveAccessState(board.WorkspaceAccess, hasAnyGrants),
		CreatedAt:         board.CreatedAt,
		UpdatedAt:         board.UpdatedAt,
	}
	if board.ArchivedAt != nil {
		archived := *board.ArchivedAt
		out.ArchivedAt = &archived
	}
	if len(board.PreviewScene) > 0 {
		scene := make(json.RawMessage, len(board.PreviewScene))
		copy(scene, board.PreviewScene)
		out.PreviewScene = scene
	}
	return out
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

// boardRoleLocked reports the effective board role. Thin adapter over the
// unified ResolveRole policy (see policy.go): the only in-memory-specific
// steps are fetching the selected member row and the workspace_access fact
// from memory.
func (s *InMemoryWorkspaceBoardStore) boardRoleLocked(key string, board *V1Board, userID int64, workspaceRole string) (AccessRole, bool) {
	_, selected := s.members[key][userID]
	return ResolveRole(board.OwnerUserID, userID, selected, board.WorkspaceAccess, IsWorkspaceMemberRole(workspaceRole))
}

// CreateBoard mints a workspace-rooted board at revision 1 with the empty
// discriminated-union snapshot; the creator is the implicit owner. The
// sharing role is always none: creation requests cannot widen it.
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
	return withWorkspaceMembers(ctx, s.workspaceMembers, workspacePublicID, func(roleFor func(int64) string) (*V1Board, error) {
		if !IsWorkspaceMemberRole(roleFor(creatorUserID)) {
			return nil, ErrNotFound
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
		version, scene, _ := previewscene.DeriveCatalog(document)
		board := &V1Board{
			WorkspacePublicID: workspacePublicID,
			PublicID:          trimmedID,
			Name:              strings.TrimSpace(name),
			Revision:          1,
			SchemaVersion:     v1InitialSchemaVerion,
			Document:          document,
			ContentVersion:    version,
			PreviewScene:      scene,
			OwnerUserID:       creatorUserID,
			CreatedBy:         creatorUserID,
			WorkspaceAccess:   WorkspaceAccessNone,
			AccessVersion:     1,
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
	})
}

// ListBoardCatalog selects authorized cards before pagination, ordered by
// updated_at descending and public id ascending.
func (s *InMemoryWorkspaceBoardStore) ListBoardCatalog(ctx context.Context, workspacePublicID string, includeArchived bool, userID int64, workspaceRole string, page V1BoardCatalogPageRequest) (V1BoardCatalogPage, error) {
	if err := ctx.Err(); err != nil {
		return V1BoardCatalogPage{}, err
	}
	if err := page.validate(); err != nil {
		return V1BoardCatalogPage{}, err
	}
	if !IsWorkspaceMemberRole(workspaceRole) {
		if page.Offset > 0 {
			return V1BoardCatalogPage{}, ErrV1CatalogOffset
		}
		return V1BoardCatalogPage{Entries: []*V1BoardCatalogEntry{}}, nil
	}
	return withWorkspaceMembers(ctx, s.workspaceMembers, workspacePublicID, func(roleFor func(int64) string) (V1BoardCatalogPage, error) {
		s.mu.Lock()
		defer s.mu.Unlock()
		var eligible []*V1Board
		type eligibleFacts struct {
			role           AccessRole
			hasDirectGrant bool
		}
		facts := map[string]eligibleFacts{}
		for _, board := range s.boards[workspacePublicID] {
			if board.ArchivedAt != nil && !includeArchived {
				continue
			}
			if board.OwnerUserID <= 0 {
				// Owner-less rows stay invisible (fail-closed).
				continue
			}
			key := v1BoardKey(workspacePublicID, board.PublicID)
			role, ok := s.boardRoleLocked(key, board, userID, roleFor(userID))
			if !ok {
				continue
			}
			_, hasDirectGrant := s.members[key][userID]
			eligible = append(eligible, board)
			facts[board.PublicID] = eligibleFacts{role: role, hasDirectGrant: hasDirectGrant}
		}
		sort.Slice(eligible, func(i, j int) bool {
			if eligible[i].UpdatedAt.Equal(eligible[j].UpdatedAt) {
				return eligible[i].PublicID < eligible[j].PublicID
			}
			return eligible[i].UpdatedAt.After(eligible[j].UpdatedAt)
		})
		if len(eligible) > maxBoardCatalogRows {
			eligible = eligible[:maxBoardCatalogRows]
		}
		if page.Offset > len(eligible) {
			return V1BoardCatalogPage{}, ErrV1CatalogOffset
		}
		end := min(page.Offset+page.Limit, len(eligible))
		out := V1BoardCatalogPage{Entries: make([]*V1BoardCatalogEntry, 0, end-page.Offset), HasMore: end < len(eligible)}
		for _, board := range eligible[page.Offset:end] {
			fact := facts[board.PublicID]
			out.Entries = append(out.Entries, v1BoardCatalogEntryOf(board, fact.role, fact.hasDirectGrant, len(s.members[v1BoardKey(workspacePublicID, board.PublicID)]) > 0))
		}
		return out, nil
	})
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
func (s *InMemoryWorkspaceBoardStore) RenameBoard(ctx context.Context, workspacePublicID, boardPublicID, name string, actorUserID int64) (*V1Board, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := ValidateV1BoardName(name); err != nil {
		return nil, err
	}
	return withWorkspaceMembers(ctx, s.workspaceMembers, workspacePublicID, func(roleFor func(int64) string) (*V1Board, error) {
		s.mu.Lock()
		defer s.mu.Unlock()
		board, err := s.getBoardLocked(workspacePublicID, strings.TrimSpace(boardPublicID))
		if err != nil {
			return nil, err
		}
		role, visible := s.boardRoleLocked(v1BoardKey(workspacePublicID, board.PublicID), board, actorUserID, roleFor(actorUserID))
		if err := CheckCapability(role, visible, CapWrite); err != nil {
			return nil, err
		}

		if board.ArchivedAt != nil {
			return nil, ErrV1Archived
		}
		board.Name = strings.TrimSpace(name)
		board.UpdatedAt = s.clock()
		return copyV1Board(board), nil
	})
}

// ArchiveBoard archives a live board; already-archived rows read as not
// found so archive stays idempotent-safe at the handler layer.
func (s *InMemoryWorkspaceBoardStore) ArchiveBoard(ctx context.Context, workspacePublicID, boardPublicID string, actorUserID int64) (*V1Board, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return withWorkspaceMembers(ctx, s.workspaceMembers, workspacePublicID, func(roleFor func(int64) string) (*V1Board, error) {
		s.mu.Lock()
		defer s.mu.Unlock()
		board, err := s.getBoardLocked(workspacePublicID, strings.TrimSpace(boardPublicID))
		if err != nil {
			return nil, err
		}
		role, visible := s.boardRoleLocked(v1BoardKey(workspacePublicID, board.PublicID), board, actorUserID, roleFor(actorUserID))
		if err := CheckCapability(role, visible, CapWrite); err != nil {
			return nil, err
		}

		if board.ArchivedAt != nil {
			return nil, ErrNotFound
		}
		now := s.clock()
		board.ArchivedAt = &now
		board.UpdatedAt = now
		return copyV1Board(board), nil
	})
}

// RestoreBoard clears the archive timestamp; live boards read as not found.
func (s *InMemoryWorkspaceBoardStore) RestoreBoard(ctx context.Context, workspacePublicID, boardPublicID string, actorUserID int64) (*V1Board, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return withWorkspaceMembers(ctx, s.workspaceMembers, workspacePublicID, func(roleFor func(int64) string) (*V1Board, error) {
		s.mu.Lock()
		defer s.mu.Unlock()
		board, err := s.getBoardLocked(workspacePublicID, strings.TrimSpace(boardPublicID))
		if err != nil {
			return nil, err
		}
		role, visible := s.boardRoleLocked(v1BoardKey(workspacePublicID, board.PublicID), board, actorUserID, roleFor(actorUserID))
		if err := CheckCapability(role, visible, CapWrite); err != nil {
			return nil, err
		}

		if board.ArchivedAt == nil {
			return nil, ErrNotFound
		}
		board.ArchivedAt = nil
		board.UpdatedAt = s.clock()
		return copyV1Board(board), nil
	})
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
	return withWorkspaceMembers(ctx, s.workspaceMembers, workspacePublicID, func(roleFor func(int64) string) (*V1Board, error) {
		s.mu.Lock()
		defer s.mu.Unlock()
		board, err := s.getBoardLocked(workspacePublicID, strings.TrimSpace(boardPublicID))
		if err != nil {
			return nil, err
		}
		role, visible := s.boardRoleLocked(v1BoardKey(workspacePublicID, board.PublicID), board, actorUserID, roleFor(actorUserID))
		if err := CheckCapability(role, visible, CapWrite); err != nil {
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
		version, scene, _ := previewscene.DeriveCatalog(document)
		board.Revision++
		board.SchemaVersion = schemaVersion
		board.Document = document
		board.ContentVersion = version
		board.PreviewScene = scene
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
		// #936: reconcile the accepted document -> media references (twin of
		// the postgres accepting transaction above: removed ids stop
		// authorizing, unknown shapes match nothing).
		accepted := previewscene.ExtractMediaAssetIDs(document)
		refs := make(map[string]struct{}, len(accepted))
		for _, assetPublicID := range accepted {
			refs[assetPublicID] = struct{}{}
		}
		s.mediaRefs[key] = refs
		return copyV1Board(board), nil
	})
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

// GetBoardRole resolves the role against current Workspace membership.
func (s *InMemoryWorkspaceBoardStore) GetBoardRole(ctx context.Context, workspacePublicID, boardPublicID string, userID int64, workspaceRole string) (AccessRole, bool, error) {
	if err := ctx.Err(); err != nil {
		return "", false, err
	}
	if !IsWorkspaceMemberRole(workspaceRole) || s.workspaceMembers == nil {
		return "", false, nil
	}
	var role AccessRole
	var visible bool
	err := s.workspaceMembers.WithWorkspaceMembers(ctx, workspacePublicID, func(roleFor func(int64) string) error {
		s.mu.Lock()
		defer s.mu.Unlock()
		board, err := s.getBoardLocked(workspacePublicID, strings.TrimSpace(boardPublicID))
		if err != nil {
			return err
		}
		role, visible = s.boardRoleLocked(v1BoardKey(workspacePublicID, board.PublicID), board, userID, roleFor(userID))
		return nil
	})
	return role, visible, err
}

// ListBoardsReferencingAsset serves the narrow BoardMediaRefReader port
// (#936, in-memory twin): board public ids in this workspace whose accepted
// document references one media asset, stably sorted. Unknown shapes list
// empty (fail-closed, never a distinct status).
func (s *InMemoryWorkspaceBoardStore) ListBoardsReferencingAsset(ctx context.Context, workspacePublicID, assetPublicID string) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	trimmedAsset := strings.TrimSpace(assetPublicID)
	if !contract.IsAssetID(trimmedAsset) {
		return nil, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []string
	for key, refs := range s.mediaRefs {
		if _, ok := refs[trimmedAsset]; !ok {
			continue
		}
		workspaceID, boardPublicID, ok := splitV1BoardKey(key)
		if !ok || workspaceID != strings.TrimSpace(workspacePublicID) {
			continue
		}
		out = append(out, boardPublicID)
	}
	sort.Strings(out)
	return out, nil
}

// ListBoardMembers lists selected members ordered by user id (the implicit
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

func (s *InMemoryWorkspaceBoardStore) boardAccessLocked(board *V1Board) *V1BoardAccess {
	ids := make([]int64, 0, len(s.members[v1BoardKey(board.WorkspacePublicID, board.PublicID)]))
	for userID := range s.members[v1BoardKey(board.WorkspacePublicID, board.PublicID)] {
		ids = append(ids, userID)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return &V1BoardAccess{WorkspacePublicID: board.WorkspacePublicID, BoardPublicID: board.PublicID, WorkspaceAccess: board.WorkspaceAccess, AccessVersion: board.AccessVersion, AccessState: DeriveAccessState(board.WorkspaceAccess, len(ids) > 0), MemberUserIDs: ids, ArchivedAt: board.ArchivedAt}
}

func (s *InMemoryWorkspaceBoardStore) GetBoardAccess(ctx context.Context, workspacePublicID, boardPublicID string) (*V1BoardAccess, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	board, err := s.getBoardLocked(workspacePublicID, boardPublicID)
	if err != nil {
		return nil, err
	}
	return s.boardAccessLocked(board), nil
}

// ReplaceBoardAccess accepts the complete sharing configuration under one membership lock.
func (s *InMemoryWorkspaceBoardStore) ReplaceBoardAccess(ctx context.Context, workspacePublicID, boardPublicID string, actorUserID, expectedVersion int64, access WorkspaceAccess, selectedUserIDs []int64) (*V1BoardAccess, error) {
	if !ValidateWorkspaceAccess(access) || actorUserID <= 0 || expectedVersion <= 0 {
		return nil, ErrInvalidInput
	}
	return withWorkspaceMembers(ctx, s.workspaceMembers, workspacePublicID, func(roleFor func(int64) string) (*V1BoardAccess, error) {
		s.mu.Lock()
		defer s.mu.Unlock()
		board, err := s.getBoardLocked(workspacePublicID, boardPublicID)
		if err != nil {
			return nil, err
		}
		if !IsWorkspaceMemberRole(roleFor(actorUserID)) {
			return nil, ErrNotFound
		}
		if actorUserID != board.OwnerUserID {
			return nil, ErrPermissionDenied
		}
		if board.ArchivedAt != nil {
			return nil, ErrV1Archived
		}
		if board.AccessVersion != expectedVersion {
			return nil, ErrAccessConflict
		}
		ids, err := selectedBoardMembers(access, selectedUserIDs, board.OwnerUserID)
		if err != nil {
			return nil, err
		}
		for _, id := range ids {
			if !IsWorkspaceMemberRole(roleFor(id)) {
				return nil, ErrMemberNotFound
			}
		}
		before := s.boardAccessLocked(board)
		if board.WorkspaceAccess == access && equalMemberIDs(before.MemberUserIDs, ids) {
			return before, nil
		}
		signals, err := s.accessSignals(ctx, workspacePublicID, boardPublicID, changedBoardMembers(before.MemberUserIDs, ids, board.OwnerUserID), board.AccessVersion+1, s.clock())
		if err != nil {
			return nil, err
		}
		key := v1BoardKey(workspacePublicID, boardPublicID)
		members := make(map[int64]*V1BoardMember, len(ids))
		for _, id := range ids {
			member := s.members[key][id]
			if member == nil {
				member = &V1BoardMember{BoardPublicID: boardPublicID, UserID: id, CreatedAt: s.clock()}
			}
			members[id] = member
		}
		s.members[key] = members
		board.WorkspaceAccess = access
		board.AccessVersion++
		board.UpdatedAt = s.clock()
		out := s.boardAccessLocked(board)
		out.AccessSignals = signals
		return out, nil
	})
}

func selectedBoardMembers(access WorkspaceAccess, ids []int64, creatorID int64) ([]int64, error) {
	out := make([]int64, 0, len(ids))
	if access == WorkspaceAccessNone {
		return out, nil
	}
	seen := make(map[int64]bool, len(ids))
	for _, id := range ids {
		if id <= 0 || id == creatorID {
			return nil, ErrInvalidInput
		}
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	if len(out) > tenancy.MaxWorkspaceMembers-1 {
		return nil, ErrInvalidInput
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out, nil
}

func equalMemberIDs(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// RemoveWorkspaceMemberGrants removes selections and advances every affected
// Board access version while the caller holds the Workspace membership lock.
func (s *InMemoryWorkspaceBoardStore) RemoveWorkspaceMemberGrants(ctx context.Context, workspacePublicID string, userID int64) ([]tenancy.MemberGrantCleanup, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if userID <= 0 {
		return nil, ErrInvalidInput
	}
	workspacePublicID = strings.TrimSpace(workspacePublicID)
	if workspacePublicID == "" {
		return nil, ErrInvalidInput
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	byBoard, ok := s.boards[workspacePublicID]
	if !ok {
		return nil, nil
	}
	ids := make([]string, 0, len(byBoard))
	for boardID := range byBoard {
		key := v1BoardKey(workspacePublicID, boardID)
		if _, selected := s.members[key][userID]; byBoard[boardID].OwnerUserID == userID || selected && byBoard[boardID].WorkspaceAccess != WorkspaceAccessNone {
			ids = append(ids, boardID)
		}
	}
	if len(ids) == 0 {
		return nil, nil
	}
	sort.Strings(ids)
	out := make([]tenancy.MemberGrantCleanup, 0, len(ids))
	for _, boardID := range ids {
		board := byBoard[boardID]
		signals, err := s.accessSignals(ctx, workspacePublicID, boardID, changedBoardMembers(nil, []int64{userID}, board.OwnerUserID), board.AccessVersion+1, s.clock())
		if err != nil {
			return nil, err
		}
		out = append(out, tenancy.MemberGrantCleanup{BoardPublicID: boardID, AccessVersion: board.AccessVersion + 1, AccessSignals: signals})
	}
	for _, boardID := range ids {
		key := v1BoardKey(workspacePublicID, boardID)
		delete(s.members[key], userID)
		if len(s.members[key]) == 0 {
			delete(s.members, key)
		}
		board := byBoard[boardID]
		board.AccessVersion++
		board.UpdatedAt = s.clock()
	}
	return out, nil
}

func (s *InMemoryWorkspaceBoardStore) accessSignals(ctx context.Context, workspaceID, boardID string, recipients []int64, version int64, at time.Time) ([]events.BoardAccessOutboxRecord, error) {
	if s.accessRecipients == nil {
		return nil, fmt.Errorf("boards: access signal recipient directory unavailable")
	}
	signals := make([]events.BoardAccessOutboxRecord, 0, len(recipients))
	for _, id := range recipients {
		publicID, err := s.accessRecipients.GetUserPublicID(ctx, id)
		if err != nil {
			return nil, err
		}
		signals = append(signals, events.NewBoardAccessOutboxRecord(workspaceID, boardID, tenancy.FormatUserPublicID(publicID), version, at))
	}
	return signals, nil
}

// Compile-time convergence: the in-memory store implements the seam.
var _ WorkspaceBoardStore = (*InMemoryWorkspaceBoardStore)(nil)

func changedBoardMembers(before, after []int64, creatorID int64) []int64 {
	seen := map[int64]bool{creatorID: true}
	for _, id := range before {
		seen[id] = true
	}
	for _, id := range after {
		seen[id] = true
	}
	ids := make([]int64, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}
