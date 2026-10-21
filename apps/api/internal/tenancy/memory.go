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
	now         func() time.Time
}

// NewInMemoryStoreForTests builds an empty test-only store.
func NewInMemoryStoreForTests() *InMemoryStore {
	return &InMemoryStore{
		workspaces:    make(map[string]*Workspace),
		organizations: make(map[string]*Organization),
		wsMembers:     make(map[string]map[int64]string),
		orgMembers:    make(map[string]map[int64]string),
		memberSince:   make(map[string]map[int64]time.Time),
		now:           time.Now,
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
	for publicID, members := range s.wsMembers {
		if _, ok := members[userID]; !ok {
			continue
		}
		w, ok := s.workspaces[publicID]
		if !ok {
			continue
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
	s.mu.Lock()
	defer s.mu.Unlock()
	if params.IsDefault && params.OrganizationID != nil {
		return nil, ErrDefaultWorkspaceConflict
	}
	// #944 creation-time guards (parity with postgres): billing opts are
	// mutually exclusive; Default and org scopes reject explicit billing
	// choices fail-closed instead of silently picking another pool.
	if params.BillingAccountPublicID != nil && params.NewBillingAccount {
		return nil, ErrBillingConflict
	}
	if params.IsDefault && (params.BillingAccountPublicID != nil || params.NewBillingAccount) {
		return nil, ErrBillingConflict
	}
	if params.BillingAccountPublicID != nil {
		if target := strings.TrimSpace(*params.BillingAccountPublicID); !strings.HasPrefix(target, "ba_") || len(target) <= 3 {
			return nil, ErrBillingNotFound
		}
		if params.OrganizationID != nil {
			return nil, ErrBillingConflict
		}
	}
	if params.NewBillingAccount && params.OrganizationID != nil {
		return nil, ErrBillingConflict
	}
	if params.OrganizationID != nil {
		if _, ok := s.organizations[*params.OrganizationID]; !ok {
			return nil, ErrNotFound
		}
	}
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
	if _, ok := s.workspaces[workspacePublicID]; !ok {
		return "", false, ErrNotFound
	}
	members, ok := s.wsMembers[workspacePublicID]
	if !ok {
		return "", false, nil
	}
	role, ok := members[userID]
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
	if _, ok := s.workspaces[workspacePublicID]; !ok {
		return nil, ErrNotFound
	}
	if existing, ok := s.wsMembers[workspacePublicID][targetUserID]; ok {
		return &WorkspaceMember{WorkspacePublicID: workspacePublicID, UserID: targetUserID, Role: existing}, nil
	}
	if len(s.wsMembers[workspacePublicID]) >= MaxWorkspaceMembers {
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

func (s *InMemoryStore) CreateOrganization(ctx context.Context, ownerUserID int64, name string) (*Organization, error) {
	if err := ValidateName(name); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
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
		Name:       name,
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
		if !ok || org.ArchivedAt != nil {
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
	if !ok || org.ArchivedAt != nil {
		return nil, ErrNotFound
	}
	org.Name = name
	org.UpdatedAt = s.clock()
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
		Name:           name,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	s.workspaces[publicID] = w
	s.grantWorkspaceMemberLocked(publicID, ownerUserID, RoleOwner, now)
	return copyWorkspace(w), nil
}

// WithWorkspaceMembers keeps the membership snapshot stable throughout a dependent mutation.
func (s *InMemoryStore) WithWorkspaceMembers(ctx context.Context, workspacePublicID string, action func(roleFor func(int64) string) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.workspaces[workspacePublicID]; !ok {
		return ErrNotFound
	}
	return action(func(userID int64) string { return s.wsMembers[workspacePublicID][userID] })
}
