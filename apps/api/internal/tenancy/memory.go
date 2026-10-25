package tenancy

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"
)

// InMemoryStore is a mutex-guarded Store implementation for tests ONLY.
// It must never be wired in production; the DB sibling swaps it out with
// a postgres/sqlc implementation of Store.
//
// The board membership scope below is a separate seam
// (boards.WorkspaceMembershipScope), not part of Store.
var _ Store = (*InMemoryStore)(nil)

type InMemoryStore struct {
	mu            sync.Mutex
	nextID        int64
	workspaces    map[string]*Workspace
	organizations map[string]*Organization
	// wsMembers maps workspace public_id -> userID -> role.
	wsMembers map[string]map[int64]string
	// orgMembers maps organization public_id -> userID -> role.
	orgMembers map[string]map[int64]string
	// memberSince tracks first-seen timestamps for member projections.
	memberSince map[string]map[int64]time.Time
	// memberInvites is the unified invitation table for both scopes
	// (workspace_member_invites / organization_member_invites, keyed by
	// invite public_id). Scope-specific behavior rides the
	// MemberInvitePolicy hooks in memory_invites.go.
	memberInvites map[string]*MemberInvite
	// memberInviteByHash indexes token_hash -> invite public_id.
	memberInviteByHash map[string]string
	// memberInviteByRaw indexes raw token -> invite public_id for
	// raw-capability lookups that bypass hashing.
	memberInviteByRaw map[string]string
	now               func() time.Time
}

// NewInMemoryStoreForTests builds an empty test-only store.
func NewInMemoryStoreForTests() *InMemoryStore {
	return &InMemoryStore{
		workspaces:         make(map[string]*Workspace),
		organizations:      make(map[string]*Organization),
		wsMembers:          make(map[string]map[int64]string),
		orgMembers:         make(map[string]map[int64]string),
		memberSince:        make(map[string]map[int64]time.Time),
		memberInvites:      make(map[string]*MemberInvite),
		memberInviteByHash: make(map[string]string),
		memberInviteByRaw:  make(map[string]string),
		now:                time.Now,
	}
}

func (s *InMemoryStore) clock() time.Time {
	if s.now != nil {
		return s.now().UTC()
	}
	return time.Now().UTC()
}

func (s *InMemoryStore) nextInternalIDLocked() int64 {
	s.nextID++
	return s.nextID
}

func copyWorkspace(w *Workspace) *Workspace {
	if w == nil {
		return nil
	}
	out := *w
	if w.OrganizationID != nil {
		org := *w.OrganizationID
		out.OrganizationID = &org
	}
	if w.OwnerUserID != nil {
		owner := *w.OwnerUserID
		out.OwnerUserID = &owner
	}
	return &out
}

func copyOrganization(o *Organization) *Organization {
	if o == nil {
		return nil
	}
	out := *o
	return &out
}

// SeedWorkspaceMember grants a membership without transition checks. It
// exists so authorization-matrix tests can set up owner/admin/member/
// outsider fixtures directly.
func (s *InMemoryStore) SeedWorkspaceMember(workspacePublicID string, userID int64, role string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	members, ok := s.wsMembers[workspacePublicID]
	if !ok {
		members = make(map[int64]string)
		s.wsMembers[workspacePublicID] = members
	}
	members[userID] = role
	since, ok := s.memberSince[workspacePublicID]
	if !ok {
		since = make(map[int64]time.Time)
		s.memberSince[workspacePublicID] = since
	}
	if _, ok := since[userID]; !ok {
		since[userID] = s.clock()
	}
}

// SeedOrganizationMember grants an org membership without checks (test setup).
func (s *InMemoryStore) SeedOrganizationMember(organizationPublicID string, userID int64, role string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	members, ok := s.orgMembers[organizationPublicID]
	if !ok {
		members = make(map[int64]string)
		s.orgMembers[organizationPublicID] = members
	}
	members[userID] = role
}

func (s *InMemoryStore) GetWorkspace(ctx context.Context, publicID string) (*Workspace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w, ok := s.workspaces[publicID]
	if !ok {
		return nil, ErrNotFound
	}
	return copyWorkspace(w), nil
}

func (s *InMemoryStore) ListWorkspacesForUser(ctx context.Context, userID int64) ([]*Workspace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*Workspace
	for publicID, w := range s.workspaces {
		directRole, directMember := "", false
		if members, ok := s.wsMembers[publicID]; ok {
			directRole, directMember = members[userID]
		}
		var orgRole string
		var orgMember bool
		if !w.IsPersonal() && w.OrganizationID != nil {
			orgRole, orgMember = s.orgMembers[*w.OrganizationID][userID]
		}
		if _, ok := EffectiveWorkspaceRole(w, orgRole, orgMember, directRole, directMember); !ok {
			continue
		}
		// Closed Organizations suspend Workspace listing (#955).
		if !w.IsPersonal() && w.OrganizationID != nil {
			if org, ok := s.organizations[*w.OrganizationID]; ok && org.IsClosed() {
				continue
			}
		}
		out = append(out, copyWorkspace(w))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].PublicID < out[j].PublicID
		}
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out, nil
}

func (s *InMemoryStore) CreateWorkspace(ctx context.Context, params CreateWorkspaceParams) (*Workspace, error) {
	if err := ValidateName(params.Name); err != nil {
		return nil, err
	}
	if params.OrganizationID != nil {
		if params.IsDefault {
			return nil, ErrDefaultWorkspaceConflict
		}
		return s.CreateOrganizationWorkspace(ctx, *params.OrganizationID, params.OwnerUserID, params.Name)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if params.IsDefault && params.OwnerUserID <= 0 {
		return nil, ErrPersonalWorkspaceUserRequired
	}
	if params.IsDefault && params.OrganizationID == nil {
		for _, w := range s.workspaces {
			if w.IsDefaultWorkspace() && w.OwnerUserID != nil && *w.OwnerUserID == params.OwnerUserID {
				return nil, ErrDefaultWorkspaceConflict
			}
		}
	}
	publicID, err := NewWorkspacePublicID()
	if err != nil {
		return nil, err
	}
	if _, exists := s.workspaces[publicID]; exists {
		return nil, ErrConflict
	}
	now := s.clock()
	w := &Workspace{
		InternalID:     s.nextInternalIDLocked(),
		PublicID:       publicID,
		OrganizationID: params.OrganizationID,
		Name:           strings.TrimSpace(params.Name),
		IsDefault:      params.IsDefault && params.OrganizationID == nil,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	if params.OwnerUserID > 0 {
		owner := params.OwnerUserID
		w.OwnerUserID = &owner
		s.grantWorkspaceMemberLocked(publicID, owner, RoleOwner, now)
	}
	s.workspaces[publicID] = w
	return copyWorkspace(w), nil
}

func (s *InMemoryStore) grantWorkspaceMemberLocked(publicID string, userID int64, role string, now time.Time) {
	members, ok := s.wsMembers[publicID]
	if !ok {
		members = make(map[int64]string)
		s.wsMembers[publicID] = members
	}
	members[userID] = role
	since, ok := s.memberSince[publicID]
	if !ok {
		since = make(map[int64]time.Time)
		s.memberSince[publicID] = since
	}
	if _, ok := since[userID]; !ok {
		since[userID] = now
	}
}

func (s *InMemoryStore) UpdateWorkspace(ctx context.Context, publicID, name string) (*Workspace, error) {
	if err := ValidateName(name); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	w, ok := s.workspaces[publicID]
	if !ok {
		return nil, ErrNotFound
	}
	if w.IsDefaultWorkspace() {
		return nil, ErrDefaultWorkspaceProtected
	}
	w.Name = strings.TrimSpace(name)
	w.UpdatedAt = s.clock()
	return copyWorkspace(w), nil
}

func (s *InMemoryStore) DeleteWorkspace(ctx context.Context, publicID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	w, ok := s.workspaces[publicID]
	if !ok {
		return ErrNotFound
	}
	if w.IsDefaultWorkspace() {
		return ErrDefaultWorkspaceProtected
	}
	delete(s.workspaces, publicID)
	delete(s.wsMembers, publicID)
	delete(s.memberSince, publicID)
	return nil
}

func (s *InMemoryStore) EnsureDefaultWorkspace(ctx context.Context, userID int64) (*Workspace, error) {
	if userID <= 0 {
		return nil, ErrPersonalWorkspaceUserRequired
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, w := range s.workspaces {
		if w.IsDefaultWorkspace() && w.OwnerUserID != nil && *w.OwnerUserID == userID {
			return copyWorkspace(w), nil
		}
	}
	publicID, err := NewWorkspacePublicID()
	if err != nil {
		return nil, err
	}
	if _, exists := s.workspaces[publicID]; exists {
		return nil, ErrConflict
	}
	now := s.clock()
	owner := userID
	w := &Workspace{
		InternalID:  s.nextInternalIDLocked(),
		PublicID:    publicID,
		OwnerUserID: &owner,
		Name:        "My Workspace",
		IsDefault:   true,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	s.workspaces[publicID] = w
	s.grantWorkspaceMemberLocked(publicID, userID, RoleOwner, now)
	return copyWorkspace(w), nil
}

func (s *InMemoryStore) GetWorkspaceRole(ctx context.Context, workspacePublicID string, userID int64) (string, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w, ok := s.workspaces[workspacePublicID]
	if !ok {
		return "", false, ErrNotFound
	}
	// Closed Organizations suspend all Workspace content access (#955).
	if !w.IsPersonal() && w.OrganizationID != nil {
		if org, ok := s.organizations[*w.OrganizationID]; ok && org.IsClosed() {
			return "", false, nil
		}
	}
	directRole, directMember := "", false
	if members, ok := s.wsMembers[workspacePublicID]; ok {
		directRole, directMember = members[userID]
	}
	if w.IsPersonal() {
		return directRole, directMember, nil
	}
	orgID := ""
	if w.OrganizationID != nil {
		orgID = *w.OrganizationID
	}
	orgRole, orgMember := "", false
	if orgID != "" {
		orgRole, orgMember = s.orgMembers[orgID][userID]
	}
	role, ok := EffectiveWorkspaceRole(w, orgRole, orgMember, directRole, directMember)
	return role, ok, nil
}

func (s *InMemoryStore) ownerCountLocked(workspacePublicID string) int {
	count := 0
	for _, role := range s.wsMembers[workspacePublicID] {
		if role == RoleOwner {
			count++
		}
	}
	return count
}

func (s *InMemoryStore) ListWorkspaceMembers(ctx context.Context, workspacePublicID string) ([]*WorkspaceMember, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.workspaces[workspacePublicID]; !ok {
		return nil, ErrNotFound
	}
	members := s.wsMembers[workspacePublicID]
	out := make([]*WorkspaceMember, 0, len(members))
	for userID, role := range members {
		created := s.clock()
		if since, ok := s.memberSince[workspacePublicID][userID]; ok {
			created = since
		}
		out = append(out, &WorkspaceMember{
			WorkspacePublicID: workspacePublicID,
			UserID:            userID,
			Role:              role,
			CreatedAt:         created,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UserID < out[j].UserID })
	return out, nil
}

func (s *InMemoryStore) AddWorkspaceMember(ctx context.Context, workspacePublicID string, targetUserID int64, role string) (*WorkspaceMember, error) {
	role = strings.TrimSpace(role)
	if !IsValidRole(role) {
		return nil, ErrForbidden
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	w, ok := s.workspaces[workspacePublicID]
	if !ok {
		return nil, ErrNotFound
	}
	// Personal Default is private (#952): it never gains collaborators.
	// Non-Default Personal workspaces invite freely (absent quota reads
	// as unlimited); Organization workspaces still require prior org
	// membership below.
	if w.IsDefaultWorkspace() {
		return nil, ErrDefaultWorkspaceProtected
	}
	if !w.IsPersonal() && w.OrganizationID != nil {
		if _, ok := s.orgMembers[*w.OrganizationID][targetUserID]; !ok {
			return nil, ErrNotFound
		}
		if org, ok := s.organizations[*w.OrganizationID]; ok && org.IsClosed() {
			return nil, ErrOrganizationClosed
		}
	}
	if existing, ok := s.wsMembers[workspacePublicID][targetUserID]; ok {
		return &WorkspaceMember{WorkspacePublicID: workspacePublicID, UserID: targetUserID, Role: existing}, nil
	}
	pending := 0
	for _, inv := range s.memberInvites {
		if inv.Scope == InviteScopeWorkspace && inv.ScopePublicID == workspacePublicID &&
			inv.Status == MemberInviteStatusPending && !MemberInviteExpired(inv.ExpiresAt, s.clock()) {
			pending++
		}
	}
	if len(s.wsMembers[workspacePublicID])+pending >= MaxWorkspaceMembers {
		return nil, ErrWorkspaceMemberLimitReached
	}
	now := s.clock()
	s.grantWorkspaceMemberLocked(workspacePublicID, targetUserID, role, now)
	return &WorkspaceMember{WorkspacePublicID: workspacePublicID, UserID: targetUserID, Role: role, CreatedAt: now}, nil
}

func (s *InMemoryStore) UpdateWorkspaceMemberRole(ctx context.Context, workspacePublicID string, actorUserID, targetUserID int64, newRole string) (*WorkspaceMember, error) {
	newRole = strings.TrimSpace(newRole)
	if !IsValidRole(newRole) {
		return nil, ErrForbidden
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.workspaces[workspacePublicID]; !ok {
		return nil, ErrNotFound
	}
	if w := s.workspaces[workspacePublicID]; !w.IsPersonal() && w.OrganizationID != nil {
		if org, ok := s.organizations[*w.OrganizationID]; ok && org.IsClosed() {
			return nil, ErrOrganizationClosed
		}
	}
	members := s.wsMembers[workspacePublicID]
	current, ok := members[targetUserID]
	if !ok {
		return nil, ErrNotFound
	}
	actorRole := members[actorUserID]
	if err := CheckMemberMutation(actorRole, actorUserID, targetUserID, current, newRole, s.ownerCountLocked(workspacePublicID), false); err != nil {
		return nil, err
	}
	members[targetUserID] = newRole
	return &WorkspaceMember{WorkspacePublicID: workspacePublicID, UserID: targetUserID, Role: newRole}, nil
}

func (s *InMemoryStore) RemoveWorkspaceMember(ctx context.Context, workspacePublicID string, actorUserID, targetUserID int64, grants WorkspaceMemberGrantStore) ([]MemberGrantCleanup, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.workspaces[workspacePublicID]; !ok {
		return nil, ErrNotFound
	}
	if w := s.workspaces[workspacePublicID]; !w.IsPersonal() && w.OrganizationID != nil {
		if org, ok := s.organizations[*w.OrganizationID]; ok && org.IsClosed() {
			return nil, ErrOrganizationClosed
		}
	}
	members := s.wsMembers[workspacePublicID]
	current, ok := members[targetUserID]
	if !ok {
		return nil, ErrNotFound
	}
	actorRole := members[actorUserID]
	if err := CheckMemberMutation(actorRole, actorUserID, targetUserID, current, "", s.ownerCountLocked(workspacePublicID), true); err != nil {
		return nil, err
	}
	if grants == nil {
		return nil, ErrConflict
	}
	cleaned, err := grants.RemoveWorkspaceMemberGrants(ctx, workspacePublicID, targetUserID)
	if err != nil {
		return nil, err
	}

	delete(members, targetUserID)
	return cleaned, nil
}

func (s *InMemoryStore) orgCountForUserLocked(userID int64) int {
	count := 0
	for publicID, members := range s.orgMembers {
		if _, ok := members[userID]; !ok {
			continue
		}
		if org, ok := s.organizations[publicID]; !ok || org.IsClosed() {
			continue
		}
		count++
	}
	return count
}

func (s *InMemoryStore) CreateOrganization(ctx context.Context, ownerUserID int64, name string) (*Organization, error) {
	if err := ValidateName(name); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.orgCountForUserLocked(ownerUserID) >= 1 {
		return nil, ErrOrganizationLimitReached
	}
	publicID, err := NewOrganizationPublicID()
	if err != nil {
		return nil, err
	}
	if _, exists := s.organizations[publicID]; exists {
		return nil, ErrConflict
	}
	now := s.clock()
	org := &Organization{
		InternalID: s.nextInternalIDLocked(),
		PublicID:   publicID,
		Name:       strings.TrimSpace(name),
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	s.organizations[publicID] = org
	members, ok := s.orgMembers[publicID]
	if !ok {
		members = make(map[int64]string)
		s.orgMembers[publicID] = members
	}
	members[ownerUserID] = RoleOwner
	return copyOrganization(org), nil
}

func (s *InMemoryStore) GetOrganization(ctx context.Context, publicID string) (*Organization, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	org, ok := s.organizations[publicID]
	if !ok {
		return nil, ErrNotFound
	}
	return copyOrganization(org), nil
}

func (s *InMemoryStore) ListOrganizationsForUser(ctx context.Context, userID int64) ([]*Organization, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var out []*Organization
	for publicID, members := range s.orgMembers {
		if _, ok := members[userID]; !ok {
			continue
		}
		org, ok := s.organizations[publicID]
		if !ok || org.IsClosed() {
			continue
		}
		out = append(out, copyOrganization(org))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].PublicID < out[j].PublicID
		}
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out, nil
}

func (s *InMemoryStore) UpdateOrganization(ctx context.Context, publicID, name string) (*Organization, error) {
	if err := ValidateName(name); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	org, ok := s.organizations[publicID]
	if !ok {
		return nil, ErrNotFound
	}
	if org.IsClosed() {
		return nil, ErrOrganizationClosed
	}
	org.Name = name
	org.UpdatedAt = s.clock()
	return copyOrganization(org), nil
}

// CloseOrganization permanently closes a live Organization in memory. The actor must
// be the Owner; non-members read as not found, non-owners as forbidden.
// The in-memory fixture holds no Task/payment state, so the in-flight
// gate trivially passes; the postgres implementation enforces it.
func (s *InMemoryStore) CloseOrganization(ctx context.Context, organizationPublicID string, actorUserID int64, reason string) (*Organization, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	org, ok := s.organizations[organizationPublicID]
	if !ok {
		return nil, ErrNotFound
	}
	if org.IsClosed() {
		return nil, ErrOrganizationClosed
	}
	role, ok := s.orgMembers[organizationPublicID][actorUserID]
	if !ok {
		return nil, ErrNotFound
	}
	if role != RoleOwner {
		return nil, ErrForbidden
	}
	now := s.clock()
	org.ClosedAt = &now
	closedBy := actorUserID
	org.ClosedBy = &closedBy
	org.UpdatedAt = now
	return copyOrganization(org), nil
}

func (s *InMemoryStore) GetOrganizationRole(ctx context.Context, organizationPublicID string, userID int64) (string, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.organizations[organizationPublicID]; !ok {
		return "", false, ErrNotFound
	}
	role, ok := s.orgMembers[organizationPublicID][userID]
	return role, ok, nil
}

// GetOrganizationRoles resolves one role per user in a single locked pass.
func (s *InMemoryStore) GetOrganizationRoles(ctx context.Context, organizationPublicID string, userIDs []int64) (map[int64]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[int64]string, len(userIDs))
	if _, ok := s.organizations[organizationPublicID]; !ok {
		return nil, ErrNotFound
	}
	members := s.orgMembers[organizationPublicID]
	for _, id := range userIDs {
		if id <= 0 {
			continue
		}
		if role, ok := members[id]; ok {
			out[id] = role
		}
	}
	return out, nil
}

// ListOrganizationRolesForUser resolves one user's role per organization in
// a single locked pass.
func (s *InMemoryStore) ListOrganizationRolesForUser(ctx context.Context, organizationPublicIDs []string, userID int64) (map[string]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]string, len(organizationPublicIDs))
	if userID <= 0 {
		return out, nil
	}
	for _, id := range organizationPublicIDs {
		trimmed := strings.TrimSpace(id)
		if trimmed == "" {
			continue
		}
		if _, ok := s.organizations[trimmed]; !ok {
			continue
		}
		if role, ok := s.orgMembers[trimmed][userID]; ok {
			out[trimmed] = role
		}
	}
	return out, nil
}

// ListOrganizationWorkspaceCounts counts visible workspaces per organization
// in a single locked pass. Closed organizations report zero, matching
// ListWorkspacesForUser visibility; unknown organizations are omitted.
func (s *InMemoryStore) ListOrganizationWorkspaceCounts(ctx context.Context, organizationPublicIDs []string) (map[string]int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make(map[string]int64, len(organizationPublicIDs))
	for _, id := range organizationPublicIDs {
		trimmed := strings.TrimSpace(id)
		if trimmed == "" {
			continue
		}
		org, ok := s.organizations[trimmed]
		if !ok {
			continue
		}
		if org.IsClosed() {
			out[trimmed] = 0
			continue
		}
		var count int64
		for _, w := range s.workspaces {
			if w.OrganizationID != nil && *w.OrganizationID == trimmed {
				count++
			}
		}
		out[trimmed] = count
	}
	return out, nil
}

func (s *InMemoryStore) ListOrganizationWorkspaces(ctx context.Context, organizationPublicID string) ([]*Workspace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.organizations[organizationPublicID]; !ok {
		return nil, ErrNotFound
	}
	var out []*Workspace
	for _, w := range s.workspaces {
		if w.OrganizationID != nil && *w.OrganizationID == organizationPublicID {
			out = append(out, copyWorkspace(w))
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].PublicID < out[j].PublicID
		}
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out, nil
}

func (s *InMemoryStore) CreateOrganizationWorkspace(ctx context.Context, organizationPublicID string, ownerUserID int64, name string) (*Workspace, error) {
	if err := ValidateName(name); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.organizations[organizationPublicID]; !ok {
		return nil, ErrNotFound
	}
	if s.organizations[organizationPublicID].IsClosed() {
		return nil, ErrOrganizationClosed
	}
	if role := s.orgMembers[organizationPublicID][ownerUserID]; role != RoleOwner && role != RoleAdmin {
		if _, ok := s.orgMembers[organizationPublicID][ownerUserID]; !ok {
			return nil, ErrNotFound
		}
		return nil, ErrForbidden
	}
	publicID, err := NewWorkspacePublicID()
	if err != nil {
		return nil, err
	}
	if _, exists := s.workspaces[publicID]; exists {
		return nil, ErrConflict
	}
	now := s.clock()
	orgID := organizationPublicID
	owner := ownerUserID
	w := &Workspace{
		InternalID:     s.nextInternalIDLocked(),
		PublicID:       publicID,
		OrganizationID: &orgID,
		OwnerUserID:    &owner,
		Name:           strings.TrimSpace(name),
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	s.workspaces[publicID] = w
	s.grantWorkspaceMemberLocked(publicID, ownerUserID, RoleOwner, now)
	return copyWorkspace(w), nil
}

func (s *InMemoryStore) orgOwnerCountLocked(organizationPublicID string) int {
	count := 0
	for _, role := range s.orgMembers[organizationPublicID] {
		if role == RoleOwner {
			count++
		}
	}
	return count
}

func (s *InMemoryStore) ListOrganizationMembers(ctx context.Context, organizationPublicID string) ([]*OrganizationMember, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.organizations[organizationPublicID]; !ok {
		return nil, ErrNotFound
	}
	members := s.orgMembers[organizationPublicID]
	out := make([]*OrganizationMember, 0, len(members))
	for userID, role := range members {
		out = append(out, &OrganizationMember{
			OrganizationPublicID: organizationPublicID,
			UserID:               userID,
			Role:                 role,
			CreatedAt:            s.clock(),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UserID < out[j].UserID })
	return out, nil
}

func (s *InMemoryStore) AddOrganizationMember(ctx context.Context, organizationPublicID string, targetUserID int64, role string) (*OrganizationMember, error) {
	role = strings.TrimSpace(role)
	if !IsValidRole(role) || role == RoleOwner {
		return nil, ErrForbidden
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.organizations[organizationPublicID]; !ok {
		return nil, ErrNotFound
	}
	if s.organizations[organizationPublicID].IsClosed() {
		return nil, ErrOrganizationClosed
	}
	if existing, ok := s.orgMembers[organizationPublicID][targetUserID]; ok {
		return &OrganizationMember{OrganizationPublicID: organizationPublicID, UserID: targetUserID, Role: existing}, nil
	}
	if s.orgCountForUserLocked(targetUserID) >= 1 {
		return nil, ErrOrganizationLimitReached
	}
	members, ok := s.orgMembers[organizationPublicID]
	if !ok {
		members = make(map[int64]string)
		s.orgMembers[organizationPublicID] = members
	}
	members[targetUserID] = role
	return &OrganizationMember{OrganizationPublicID: organizationPublicID, UserID: targetUserID, Role: role, CreatedAt: s.clock()}, nil
}

func (s *InMemoryStore) UpdateOrganizationMemberRole(ctx context.Context, organizationPublicID string, actorUserID, targetUserID int64, newRole string) (*OrganizationMember, error) {
	newRole = strings.TrimSpace(newRole)
	if !IsValidRole(newRole) {
		return nil, ErrForbidden
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.organizations[organizationPublicID]; !ok {
		return nil, ErrNotFound
	}
	if s.organizations[organizationPublicID].IsClosed() {
		return nil, ErrOrganizationClosed
	}
	members := s.orgMembers[organizationPublicID]
	current, ok := members[targetUserID]
	if !ok {
		return nil, ErrNotFound
	}
	actorRole := members[actorUserID]
	if err := CheckOrganizationMemberMutation(actorRole, actorUserID, targetUserID, current, newRole, s.orgOwnerCountLocked(organizationPublicID), false); err != nil {
		return nil, err
	}
	members[targetUserID] = newRole
	return &OrganizationMember{OrganizationPublicID: organizationPublicID, UserID: targetUserID, Role: newRole}, nil
}

func (s *InMemoryStore) RemoveOrganizationMember(ctx context.Context, organizationPublicID string, actorUserID, targetUserID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.organizations[organizationPublicID]; !ok {
		return ErrNotFound
	}
	if s.organizations[organizationPublicID].IsClosed() {
		return ErrOrganizationClosed
	}
	members := s.orgMembers[organizationPublicID]
	current, ok := members[targetUserID]
	if !ok {
		return ErrNotFound
	}
	actorRole := members[actorUserID]
	if err := CheckOrganizationMemberMutation(actorRole, actorUserID, targetUserID, current, "", s.orgOwnerCountLocked(organizationPublicID), true); err != nil {
		return err
	}
	delete(members, targetUserID)
	for publicID, w := range s.workspaces {
		if w.OrganizationID != nil && *w.OrganizationID == organizationPublicID {
			if wsMembers, ok := s.wsMembers[publicID]; ok {
				delete(wsMembers, targetUserID)
			}
		}
	}
	return nil
}

// WithWorkspaceMembers keeps the membership snapshot stable throughout a dependent mutation.
// The role function reports the effective workspace role (#951): org
// Owner/Admin via their org role, others via the direct row.
func (s *InMemoryStore) WithWorkspaceMembers(ctx context.Context, workspacePublicID string, action func(roleFor func(int64) string) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	w, ok := s.workspaces[workspacePublicID]
	if !ok {
		return ErrNotFound
	}
	return action(func(userID int64) string {
		direct := s.wsMembers[workspacePublicID][userID]
		directMember := false
		if _, ok := s.wsMembers[workspacePublicID][userID]; ok {
			directMember = true
		}
		if w.IsPersonal() {
			return direct
		}
		var orgRole string
		var orgMember bool
		if w.OrganizationID != nil {
			orgRole, orgMember = s.orgMembers[*w.OrganizationID][userID]
		}
		role, ok := EffectiveWorkspaceRole(w, orgRole, orgMember, direct, directMember)
		if !ok {
			return ""
		}
		return role
	})
}
