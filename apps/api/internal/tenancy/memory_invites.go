package tenancy

// Unified in-memory invitation lifecycle (test double only).
//
// Both scopes share one memberInvites table keyed by invite public_id, with
// token_hash and raw-token indexes. Scope differences ride the
// memoryInviteScope hooks: Default protection (workspace only), closure +
// inviter gates (organization), capacity (workspace 5-member cap vs
// organization single-org limit at accept), and the member-write target.
// The public methods below are thin delegators preserving the
// WorkspaceInviteStore / OrganizationInviteStore signatures.

import (
	"context"
	"sort"
	"strings"
)

// memoryInviteScope parameterizes the unified in-memory lifecycle.
// All hooks run under the store mutex.
type memoryInviteScope struct {
	policy MemberInvitePolicy
	// resolveScope maps a scope public_id to its identity.
	resolveScope func(scopePublicID string) (internalID int64, publicID, name string, err error)
	// checkCreateGate enforces create-time scope rules (Default
	// protection for workspace; closure + inviter role gates for org).
	checkCreateGate func(scopePublicID string, inviterUserID int64, role string) error
	// activeCount counts current members for capacity checks.
	activeCount func(scopePublicID string) int
	// checkCapacity enforces create-time capacity (nil = no cap).
	checkCapacity func(active, pending int) error
	// isMember reports existing membership (accept fast-path).
	isMember func(scopePublicID string, userID int64) bool
	// grantMember writes the membership row.
	grantMember func(scopePublicID string, userID int64, role string)
	// checkAcceptGate enforces accept-time scope rules (Default/closure/
	// single-org limit). Runs after recipient attribution.
	checkAcceptGate func(scopePublicID string, recipientUserID int64) error
}

func workspaceMemoryScope(s *InMemoryStore) memoryInviteScope {
	return memoryInviteScope{
		policy: WorkspaceInvitePolicy(),
		resolveScope: func(scopePublicID string) (int64, string, string, error) {
			w, ok := s.workspaces[scopePublicID]
			if !ok {
				return 0, "", "", ErrNotFound
			}
			return w.InternalID, w.PublicID, w.Name, nil
		},
		checkCreateGate: func(scopePublicID string, _ int64, _ string) error {
			if w, ok := s.workspaces[scopePublicID]; ok && w.IsDefaultWorkspace() {
				return ErrDefaultWorkspaceProtected
			}
			return nil
		},
		activeCount: func(scopePublicID string) int {
			return len(s.wsMembers[scopePublicID])
		},
		checkCapacity: func(active, pending int) error {
			if active+pending >= MaxWorkspaceMembers {
				return ErrWorkspaceMemberLimitReached
			}
			return nil
		},
		isMember: func(scopePublicID string, userID int64) bool {
			_, ok := s.wsMembers[scopePublicID][userID]
			return ok
		},
		grantMember: func(scopePublicID string, userID int64, role string) {
			now := s.clock()
			if members := s.wsMembers[scopePublicID]; members != nil {
				if _, ok := members[userID]; ok {
					return
				}
			}
			s.grantWorkspaceMemberLocked(scopePublicID, userID, role, now)
		},
		checkAcceptGate: func(scopePublicID string, recipientUserID int64) error {
			w, ok := s.workspaces[scopePublicID]
			if !ok {
				return ErrNotFound
			}
			if w.IsDefaultWorkspace() {
				return ErrDefaultWorkspaceProtected
			}
			if w.OrganizationID != nil {
				if org := s.organizations[*w.OrganizationID]; org != nil && org.IsClosed() {
					return ErrOrganizationClosed
				}
				if _, member := s.orgMembers[*w.OrganizationID][recipientUserID]; !member {
					return ErrNotFound
				}
			}
			return nil
		},
	}
}

func organizationMemoryScope(s *InMemoryStore) memoryInviteScope {
	return memoryInviteScope{
		policy: OrganizationInvitePolicy(),
		resolveScope: func(scopePublicID string) (int64, string, string, error) {
			org, ok := s.organizations[scopePublicID]
			if !ok {
				return 0, "", "", ErrNotFound
			}
			return org.InternalID, org.PublicID, org.Name, nil
		},
		checkCreateGate: func(scopePublicID string, inviterUserID int64, role string) error {
			org, ok := s.organizations[scopePublicID]
			if !ok {
				return ErrNotFound
			}
			if org.IsClosed() {
				return ErrOrganizationClosed
			}
			// Fidelity with postgres lockActiveOrganization + actor gate:
			// only owner/admin may invite, and only owner may grant admin.
			actorRole := s.orgMembers[scopePublicID][inviterUserID]
			if actorRole != RoleOwner && actorRole != RoleAdmin {
				return ErrForbidden
			}
			if role == RoleAdmin && actorRole != RoleOwner {
				return ErrForbidden
			}
			return nil
		},
		activeCount:   func(_ string) int { return 0 },
		checkCapacity: nil,
		isMember: func(scopePublicID string, userID int64) bool {
			_, ok := s.orgMembers[scopePublicID][userID]
			return ok
		},
		grantMember: func(scopePublicID string, userID int64, role string) {
			if s.orgMembers[scopePublicID] == nil {
				s.orgMembers[scopePublicID] = make(map[int64]string)
			}
			s.orgMembers[scopePublicID][userID] = role
		},
		checkAcceptGate: func(scopePublicID string, recipientUserID int64) error {
			if org, ok := s.organizations[scopePublicID]; ok && org.IsClosed() {
				return ErrOrganizationClosed
			}
			// Live single-organization limit (closed orgs excluded).
			for orgID, members := range s.orgMembers {
				if orgID == scopePublicID {
					continue
				}
				if _, member := members[recipientUserID]; member {
					if org, ok := s.organizations[orgID]; ok && org.IsClosed() {
						continue
					}
					return ErrOrganizationLimitReached
				}
			}
			return nil
		},
	}
}

// resolveMemberInviteLocked resolves an invite by public id or raw token.
// Caller holds the mutex.
func (s *InMemoryStore) resolveMemberInviteLocked(policy MemberInvitePolicy, ref string) *MemberInvite {
	if IsMemberInviteRef(ref, policy.PublicIDPrefix) {
		if inv, ok := s.memberInvites[ref]; ok && inv.Scope == policy.Scope {
			return inv
		}
		return nil
	}
	if pubID, ok := s.memberInviteByRaw[ref]; ok {
		if inv, ok := s.memberInvites[pubID]; ok && inv.Scope == policy.Scope {
			return inv
		}
	}
	if pubID, ok := s.memberInviteByHash[MemberInviteTokenHash(ref)]; ok {
		if inv, ok := s.memberInvites[pubID]; ok && inv.Scope == policy.Scope {
			return inv
		}
	}
	return nil
}

// createMemberInviteLocked runs the shared creation sequencing.
func (s *InMemoryStore) createMemberInviteLocked(scope memoryInviteScope, scopePublicID string, inviterUserID int64, email, role string, inviteeUserID int64) (*MemberInvite, string, error) {
	policy := scope.policy
	scopePublicID = strings.TrimSpace(scopePublicID)
	internalID, publicID, name, err := scope.resolveScope(scopePublicID)
	if err != nil {
		return nil, "", err
	}
	email, role, err = ValidateMemberInviteCreateInput(email, role)
	if err != nil {
		return nil, "", err
	}
	if err := scope.checkCreateGate(scopePublicID, inviterUserID, role); err != nil {
		return nil, "", err
	}
	now := s.clock()
	pending := 0
	for _, inv := range s.memberInvites {
		if inv.Scope != policy.Scope || inv.ScopePublicID != scopePublicID || inv.Status != MemberInviteStatusPending {
			continue
		}
		if MemberInviteExpired(inv.ExpiresAt, now) {
			inv.Status = MemberInviteStatusExpired
			continue
		}
		pending++
		if strings.EqualFold(inv.InviteeEmail, email) {
			return nil, "", policy.ConflictErr
		}
	}
	if scope.checkCapacity != nil {
		if err := scope.checkCapacity(scope.activeCount(scopePublicID), pending); err != nil {
			return nil, "", err
		}
	}
	rawToken, tokenHash, err := MintMemberInviteToken()
	if err != nil {
		return nil, "", err
	}
	pubID, err := policy.NewPublicID()
	if err != nil {
		return nil, "", err
	}
	inv := &MemberInvite{
		Scope:           policy.Scope,
		InternalID:      s.nextInternalIDLocked(),
		PublicID:        pubID,
		ScopeInternalID: internalID,
		ScopePublicID:   publicID,
		ScopeName:       name,
		InviterUserID:   inviterUserID,
		InviteeEmail:    email,
		Role:            role,
		Status:          MemberInviteStatusPending,
		TokenHash:       tokenHash,
		ExpiresAt:       now.Add(MemberInviteTTL),
		CreatedAt:       now,
		UpdatedAt:       now,
	}
	WithMemberInviteInviteeUserID(inv, inviteeUserID)
	s.memberInvites[pubID] = inv
	s.memberInviteByHash[tokenHash] = pubID
	s.memberInviteByRaw[rawToken] = pubID
	return CopyMemberInvite(inv), rawToken, nil
}

// listMemberInvitesLocked lists pending unexpired invites for a scope.
// Decided/expired rows are excluded (actionable set); expired rows are
// lazily persisted. Read-tolerant: still listable when closed.
func (s *InMemoryStore) listMemberInvitesLocked(scope memoryInviteScope, scopePublicID string) ([]*MemberInvite, error) {
	policy := scope.policy
	scopePublicID = strings.TrimSpace(scopePublicID)
	if _, _, _, err := scope.resolveScope(scopePublicID); err != nil {
		return nil, err
	}
	now := s.clock()
	var out []*MemberInvite
	for _, inv := range s.memberInvites {
		if inv.Scope != policy.Scope || inv.ScopePublicID != scopePublicID || inv.Status != MemberInviteStatusPending {
			continue
		}
		if MemberInviteExpired(inv.ExpiresAt, now) {
			inv.Status = MemberInviteStatusExpired
			continue
		}
		out = append(out, CopyMemberInvite(inv))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

// cancelMemberInviteLocked cancels a pending invite.
func (s *InMemoryStore) cancelMemberInviteLocked(scope memoryInviteScope, scopePublicID, invitePublicID string, callerUserID int64) error {
	policy := scope.policy
	inv, ok := s.memberInvites[strings.TrimSpace(invitePublicID)]
	if !ok || inv.Scope != policy.Scope || inv.ScopePublicID != strings.TrimSpace(scopePublicID) {
		return ErrNotFound
	}
	now := s.clock()
	if inv.Status != MemberInviteStatusPending {
		return ErrConflict
	}
	if MemberInviteExpired(inv.ExpiresAt, now) {
		inv.Status = MemberInviteStatusExpired
		return policy.ExpiredErr
	}
	inv.Status = MemberInviteStatusCancelled
	inv.RespondedAt = &now
	inv.RespondedByUserID = &callerUserID
	inv.UpdatedAt = now
	return nil
}

// listPendingMemberInvitesForUserLocked lists pending unexpired invites
// matching the user's email or id.
func (s *InMemoryStore) listPendingMemberInvitesForUserLocked(scope memoryInviteScope, userID int64, userEmail string) ([]*MemberInvite, error) {
	policy := scope.policy
	now := s.clock()
	userEmail = NormalizeMemberInviteEmail(userEmail)
	var out []*MemberInvite
	seen := make(map[string]bool)
	for _, inv := range s.memberInvites {
		if inv.Scope != policy.Scope || inv.Status != MemberInviteStatusPending {
			continue
		}
		if MemberInviteExpired(inv.ExpiresAt, now) {
			inv.Status = MemberInviteStatusExpired
			continue
		}
		match := false
		if userEmail != "" && strings.EqualFold(inv.InviteeEmail, userEmail) {
			match = true
		}
		if userID > 0 && inv.InviteeUserID != nil && *inv.InviteeUserID == userID {
			match = true
		}
		if match && !seen[inv.PublicID] {
			seen[inv.PublicID] = true
			out = append(out, CopyMemberInvite(inv))
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

// getMemberInviteByTokenLocked looks up an invite by raw token (copy-only
// expiry marking).
func (s *InMemoryStore) getMemberInviteByTokenLocked(scope memoryInviteScope, rawToken string) (*MemberInvite, error) {
	rawToken = strings.TrimSpace(rawToken)
	if rawToken == "" {
		return nil, ErrNotFound
	}
	inv := s.resolveMemberInviteLocked(scope.policy, rawToken)
	if inv == nil {
		return nil, ErrNotFound
	}
	return MarkMemberInviteExpiredCopy(CopyMemberInvite(inv), s.clock()), nil
}

// getMemberInviteLocked returns an invite by public id (copy-only expiry).
func (s *InMemoryStore) getMemberInviteLocked(scope memoryInviteScope, invitePublicID string) (*MemberInvite, error) {
	invitePublicID = strings.TrimSpace(invitePublicID)
	inv, ok := s.memberInvites[invitePublicID]
	if !ok || inv.Scope != scope.policy.Scope {
		return nil, ErrNotFound
	}
	return MarkMemberInviteExpiredCopy(CopyMemberInvite(inv), s.clock()), nil
}

// acceptMemberInviteLocked runs the shared accept sequencing and returns the
// granted role; callers project the scope member type.
func (s *InMemoryStore) acceptMemberInviteLocked(scope memoryInviteScope, ref string, recipientUserID int64, recipientEmail string) (*MemberInvite, error) {
	policy := scope.policy
	ref = strings.TrimSpace(ref)
	if err := ValidateMemberInviteDecisionInput(ref, recipientUserID); err != nil {
		return nil, err
	}
	inv := s.resolveMemberInviteLocked(policy, ref)
	if inv == nil {
		return nil, ErrNotFound
	}
	now := s.clock()
	if inv.Status != MemberInviteStatusPending {
		return nil, ErrConflict
	}
	if MemberInviteExpired(inv.ExpiresAt, now) {
		inv.Status = MemberInviteStatusExpired
		return nil, policy.ExpiredErr
	}
	if err := CheckMemberInviteRecipient(inv.InviteeEmail, inv.InviteeUserID, recipientUserID, recipientEmail); err != nil {
		return nil, err
	}
	if err := scope.checkAcceptGate(inv.ScopePublicID, recipientUserID); err != nil {
		return nil, err
	}
	if scope.policy.Scope == InviteScopeWorkspace && !scope.isMember(inv.ScopePublicID, recipientUserID) {
		if scope.checkCapacity != nil {
			if err := scope.checkCapacity(scope.activeCount(inv.ScopePublicID), 0); err != nil {
				return nil, err
			}
		}
	}
	scope.grantMember(inv.ScopePublicID, recipientUserID, inv.Role)
	inv.Status = MemberInviteStatusAccepted
	inv.RespondedAt = &now
	inv.RespondedByUserID = &recipientUserID
	inv.UpdatedAt = now
	return CopyMemberInvite(inv), nil
}

// declineMemberInviteLocked runs the shared decline sequencing and returns
// the decided authoritative invite.
func (s *InMemoryStore) declineMemberInviteLocked(scope memoryInviteScope, ref string, recipientUserID int64, recipientEmail string) (*MemberInvite, error) {
	policy := scope.policy
	ref = strings.TrimSpace(ref)
	if err := ValidateMemberInviteDecisionInput(ref, recipientUserID); err != nil {
		return nil, err
	}
	inv := s.resolveMemberInviteLocked(policy, ref)
	if inv == nil {
		return nil, ErrNotFound
	}
	now := s.clock()
	if inv.Status != MemberInviteStatusPending {
		return nil, ErrConflict
	}
	if MemberInviteExpired(inv.ExpiresAt, now) {
		inv.Status = MemberInviteStatusExpired
		return nil, policy.ExpiredErr
	}
	if err := CheckMemberInviteRecipient(inv.InviteeEmail, inv.InviteeUserID, recipientUserID, recipientEmail); err != nil {
		return nil, err
	}
	inv.Status = MemberInviteStatusDeclined
	inv.RespondedAt = &now
	inv.RespondedByUserID = &recipientUserID
	inv.UpdatedAt = now
	return CopyMemberInvite(inv), nil
}

// Workspace invite delegators (thin adapters over the unified core).

func (s *InMemoryStore) CreateWorkspaceMemberInvite(ctx context.Context, workspacePublicID string, inviterUserID int64, inviteeEmail, role string, inviteeUserID int64) (*WorkspaceMemberInvite, string, error) {
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	inv, raw, err := s.createMemberInviteLocked(workspaceMemoryScope(s), workspacePublicID, inviterUserID, inviteeEmail, role, inviteeUserID)
	if err != nil {
		return nil, "", err
	}
	return WorkspaceInviteFromMember(inv), raw, nil
}

func (s *InMemoryStore) ListWorkspaceMemberInvites(ctx context.Context, workspacePublicID string) ([]*WorkspaceMemberInvite, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	list, err := s.listMemberInvitesLocked(workspaceMemoryScope(s), workspacePublicID)
	if err != nil {
		return nil, err
	}
	out := make([]*WorkspaceMemberInvite, 0, len(list))
	for _, inv := range list {
		out = append(out, WorkspaceInviteFromMember(inv))
	}
	return out, nil
}

func (s *InMemoryStore) CancelWorkspaceMemberInvite(ctx context.Context, workspacePublicID, invitePublicID string, callerUserID int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cancelMemberInviteLocked(workspaceMemoryScope(s), workspacePublicID, invitePublicID, callerUserID)
}

func (s *InMemoryStore) ListPendingWorkspaceInvitesForUser(ctx context.Context, userID int64, userEmail string) ([]*WorkspaceMemberInvite, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	list, err := s.listPendingMemberInvitesForUserLocked(workspaceMemoryScope(s), userID, userEmail)
	if err != nil {
		return nil, err
	}
	out := make([]*WorkspaceMemberInvite, 0, len(list))
	for _, inv := range list {
		out = append(out, WorkspaceInviteFromMember(inv))
	}
	return out, nil
}

func (s *InMemoryStore) GetWorkspaceInviteByToken(ctx context.Context, rawToken string) (*WorkspaceMemberInvite, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	inv, err := s.getMemberInviteByTokenLocked(workspaceMemoryScope(s), rawToken)
	if err != nil {
		return nil, err
	}
	return WorkspaceInviteFromMember(inv), nil
}

func (s *InMemoryStore) GetWorkspaceMemberInvite(ctx context.Context, invitePublicID string) (*WorkspaceMemberInvite, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	inv, err := s.getMemberInviteLocked(workspaceMemoryScope(s), invitePublicID)
	if err != nil {
		return nil, err
	}
	return WorkspaceInviteFromMember(inv), nil
}

func (s *InMemoryStore) AcceptWorkspaceInvite(ctx context.Context, rawTokenOrPublicID string, recipientUserID int64, recipientEmail string) (*WorkspaceMember, *WorkspaceMemberInvite, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	inv, err := s.acceptMemberInviteLocked(workspaceMemoryScope(s), rawTokenOrPublicID, recipientUserID, recipientEmail)
	if err != nil {
		return nil, nil, err
	}
	now := s.clock()
	member := &WorkspaceMember{
		WorkspacePublicID: inv.ScopePublicID,
		UserID:            recipientUserID,
		Role:              inv.Role,
		CreatedAt:         now,
	}
	return member, WorkspaceInviteFromMember(inv), nil
}

func (s *InMemoryStore) DeclineWorkspaceInvite(ctx context.Context, rawTokenOrPublicID string, recipientUserID int64, recipientEmail string) (*WorkspaceMemberInvite, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	inv, err := s.declineMemberInviteLocked(workspaceMemoryScope(s), rawTokenOrPublicID, recipientUserID, recipientEmail)
	if err != nil {
		return nil, err
	}
	return WorkspaceInviteFromMember(inv), nil
}

// Organization invite delegators (thin adapters over the unified core).

func (s *InMemoryStore) CreateOrganizationMemberInvite(ctx context.Context, organizationPublicID string, inviterUserID int64, inviteeEmail, role string, inviteeUserID int64) (*OrganizationMemberInvite, string, error) {
	if err := ctx.Err(); err != nil {
		return nil, "", err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	inv, raw, err := s.createMemberInviteLocked(organizationMemoryScope(s), organizationPublicID, inviterUserID, inviteeEmail, role, inviteeUserID)
	if err != nil {
		return nil, "", err
	}
	return OrganizationInviteFromMember(inv), raw, nil
}

func (s *InMemoryStore) ListOrganizationMemberInvites(ctx context.Context, organizationPublicID string) ([]*OrganizationMemberInvite, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	list, err := s.listMemberInvitesLocked(organizationMemoryScope(s), organizationPublicID)
	if err != nil {
		return nil, err
	}
	out := make([]*OrganizationMemberInvite, 0, len(list))
	for _, inv := range list {
		out = append(out, OrganizationInviteFromMember(inv))
	}
	return out, nil
}

func (s *InMemoryStore) CancelOrganizationMemberInvite(ctx context.Context, organizationPublicID, invitePublicID string, callerUserID int64) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cancelMemberInviteLocked(organizationMemoryScope(s), organizationPublicID, invitePublicID, callerUserID)
}

func (s *InMemoryStore) ListPendingOrganizationInvitesForUser(ctx context.Context, userID int64, userEmail string) ([]*OrganizationMemberInvite, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	list, err := s.listPendingMemberInvitesForUserLocked(organizationMemoryScope(s), userID, userEmail)
	if err != nil {
		return nil, err
	}
	out := make([]*OrganizationMemberInvite, 0, len(list))
	for _, inv := range list {
		out = append(out, OrganizationInviteFromMember(inv))
	}
	return out, nil
}

func (s *InMemoryStore) GetOrganizationInviteByToken(ctx context.Context, rawToken string) (*OrganizationMemberInvite, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	inv, err := s.getMemberInviteByTokenLocked(organizationMemoryScope(s), rawToken)
	if err != nil {
		return nil, err
	}
	return OrganizationInviteFromMember(inv), nil
}

func (s *InMemoryStore) GetOrganizationMemberInvite(ctx context.Context, invitePublicID string) (*OrganizationMemberInvite, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	inv, err := s.getMemberInviteLocked(organizationMemoryScope(s), invitePublicID)
	if err != nil {
		return nil, err
	}
	return OrganizationInviteFromMember(inv), nil
}

func (s *InMemoryStore) AcceptOrganizationInvite(ctx context.Context, rawTokenOrPublicID string, recipientUserID int64, recipientEmail string) (*OrganizationMember, *OrganizationMemberInvite, error) {
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	inv, err := s.acceptMemberInviteLocked(organizationMemoryScope(s), rawTokenOrPublicID, recipientUserID, recipientEmail)
	if err != nil {
		return nil, nil, err
	}
	now := s.clock()
	member := &OrganizationMember{
		OrganizationPublicID: inv.ScopePublicID,
		UserID:               recipientUserID,
		Role:                 inv.Role,
		CreatedAt:            now,
	}
	return member, OrganizationInviteFromMember(inv), nil
}

func (s *InMemoryStore) DeclineOrganizationInvite(ctx context.Context, rawTokenOrPublicID string, recipientUserID int64, recipientEmail string) (*OrganizationMemberInvite, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	inv, err := s.declineMemberInviteLocked(organizationMemoryScope(s), rawTokenOrPublicID, recipientUserID, recipientEmail)
	if err != nil {
		return nil, err
	}
	return OrganizationInviteFromMember(inv), nil
}
