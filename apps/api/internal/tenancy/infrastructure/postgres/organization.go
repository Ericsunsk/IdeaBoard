// Package postgres implements Organization lifecycle transactions behind
// tenancy.OrganizationStore. Workspace persistence shares only transaction-local
// primitives; Organization writes never call back into the composite Store.
package postgres

import (
	"context"
	"sort"
	"strings"

	"ideaboard/api/internal/db/dbutil"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/platform/database"
	"ideaboard/api/internal/tenancy"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// OrganizationStore owns Organization lifecycle and membership transactions.
// It is usable independently of Workspace content deletion and object storage.
type OrganizationStore struct {
	db *pgxpool.Pool
	q  *dbsqlc.Queries
}

var _ tenancy.OrganizationStore = (*OrganizationStore)(nil)

func NewOrganizationStore(db *pgxpool.Pool) *OrganizationStore {
	return &OrganizationStore{db: db, q: dbsqlc.New(db)}
}

func (s *OrganizationStore) dbWithContext(ctx context.Context) repositoryQueryer {
	return newRepositoryQueryer(ctx, s.db, s.q)
}

// lockActiveOrganization serializes every Organization mutation with closure.
// All state and role reads that follow use the same transaction-local queries.
func lockActiveOrganization(ctx context.Context, q *dbsqlc.Queries, publicID string) (*tenancy.Organization, error) {
	row, err := q.LockOrganizationByPublicID(ctx, strings.TrimSpace(publicID))
	if err != nil {
		return nil, notFound(err)
	}
	organization := mapOrganization(row)
	if organization.IsClosed() {
		return nil, tenancy.ErrOrganizationClosed
	}
	return organization, nil
}

func mapOrganization(row dbsqlc.Organization) *tenancy.Organization {
	return &tenancy.Organization{
		InternalID: row.InternalID,
		PublicID:   row.PublicID,
		Name:       row.Name,
		ClosedAt:   dbutil.TimePtr(row.ClosedAt),
		ClosedBy:   dbutil.Int64Ptr(row.ClosedBy),
		CreatedAt:  dbutil.TimeValue(row.CreatedAt),
		UpdatedAt:  dbutil.TimeValue(row.UpdatedAt),
	}
}

// CreateOrganization mints an org_ organization and grants the creator the
// owner membership in the same transaction. The stable organization Billing
// Account (#944) is provisioned in the same transaction with the creator as
// billing owner (wallet via trigger), so org creation/first config never
// leaves an org without its funds identity and org workspaces never fall
// back to the creator personal pool.
//
// Cardinality (ADR-0038 #951): a user creating while already holding a
// live organization fails closed with ErrOrganizationLimitReached. The
// user's row is locked first so concurrent creates serialize per user.
func (s *OrganizationStore) CreateOrganization(ctx context.Context, ownerUserID int64, name string) (*tenancy.Organization, error) {
	if err := tenancy.ValidateName(name); err != nil {
		return nil, err
	}
	var created dbsqlc.Organization
	for attempt := 0; attempt < mintAttempts; attempt++ {
		publicID, err := tenancy.NewOrganizationPublicID()
		if err != nil {
			return nil, err
		}
		err = s.dbWithContext(ctx).withTx(func(q *dbsqlc.Queries) error {
			if _, err := q.LockUserRow(ctx, ownerUserID); err != nil {
				if database.IsNotFound(err) {
					return tenancy.ErrNotFound
				}
				return err
			}
			count, err := q.CountOrganizationsByUserID(ctx, ownerUserID)
			if err != nil {
				return err
			}
			if count >= 1 {
				return tenancy.ErrOrganizationLimitReached
			}
			row, insertErr := q.CreateOrganization(ctx, dbsqlc.CreateOrganizationParams{
				PublicID: publicID, Name: strings.TrimSpace(name),
			})
			if insertErr != nil {
				return insertErr
			}
			created = row
			if err := q.UpsertOrganizationMembership(ctx, dbsqlc.UpsertOrganizationMembershipParams{
				OrganizationID: row.InternalID,
				UserID:         ownerUserID,
				Role:           tenancy.RoleOwner,
			}); err != nil {
				return err
			}
			if _, err := ensureOrganizationBillingAccountInTx(ctx, q, row.InternalID, ownerUserID); err != nil {
				return err
			}
			return nil
		})
		if isPublicIDConflict(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		return mapOrganization(created), nil
	}
	return nil, tenancy.ErrConflict
}

// GetOrganization resolves an organization by opaque public id.
func (s *OrganizationStore) GetOrganization(ctx context.Context, publicID string) (*tenancy.Organization, error) {
	row, err := s.dbWithContext(ctx).q.GetOrganizationByPublicID(ctx, strings.TrimSpace(publicID))
	if err != nil {
		return nil, notFound(err)
	}
	return mapOrganization(row), nil
}

// ListOrganizationsForUser lists membership-visible organizations.
func (s *OrganizationStore) ListOrganizationsForUser(ctx context.Context, userID int64) ([]*tenancy.Organization, error) {
	rows, err := s.dbWithContext(ctx).q.ListOrganizationsByUserID(ctx, dbsqlc.ListOrganizationsByUserIDParams{
		UserID: userID, RowLimit: maxTenancyRows,
	})
	if err != nil {
		return nil, err
	}
	out := make([]*tenancy.Organization, 0, len(rows))
	for _, row := range rows {
		out = append(out, mapOrganization(row))
	}
	return out, nil
}

// UpdateOrganization renames a live organization; closed or missing rows
// read as not found (closed) or are refused.
func (s *OrganizationStore) UpdateOrganization(ctx context.Context, publicID, name string) (*tenancy.Organization, error) {
	if err := tenancy.ValidateName(name); err != nil {
		return nil, err
	}
	var updated *tenancy.Organization
	err := s.dbWithContext(ctx).withTx(func(q *dbsqlc.Queries) error {
		if _, err := lockActiveOrganization(ctx, q, publicID); err != nil {
			return err
		}
		rows, err := q.UpdateOrganizationName(ctx, dbsqlc.UpdateOrganizationNameParams{
			Name: strings.TrimSpace(name), PublicID: strings.TrimSpace(publicID),
		})
		if err != nil {
			return err
		}
		if rows == 0 {
			return tenancy.ErrNotFound
		}
		row, err := q.GetOrganizationByPublicID(ctx, strings.TrimSpace(publicID))
		if err != nil {
			return notFound(err)
		}
		updated = mapOrganization(row)
		return nil
	})
	return updated, err
}

// GetOrganizationRole reports the caller's direct organization membership
// role, ok=false when the organization exists but the user is not a member.
func (s *OrganizationStore) GetOrganizationRole(ctx context.Context, organizationPublicID string, userID int64) (string, bool, error) {
	organization, err := s.GetOrganization(ctx, organizationPublicID)
	if err != nil {
		return "", false, err
	}
	row, err := s.dbWithContext(ctx).q.GetOrganizationMembership(ctx, dbsqlc.GetOrganizationMembershipParams{
		OrganizationID: organization.InternalID, UserID: userID,
	})
	if database.IsNotFound(err) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return row.Role, true, nil
}

// GetOrganizationRoles resolves one role per user inside an organization in
// a single ANY lookup, replacing per-row GetOrganizationRole round trips.
// Unknown organizations read as not found; users holding no membership are
// omitted (callers read them as non-members).
func (s *OrganizationStore) GetOrganizationRoles(ctx context.Context, organizationPublicID string, userIDs []int64) (map[int64]string, error) {
	out := make(map[int64]string, len(userIDs))
	seen := make(map[int64]struct{}, len(userIDs))
	deduped := make([]int64, 0, len(userIDs))
	for _, id := range userIDs {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		deduped = append(deduped, id)
	}
	if len(deduped) == 0 {
		return out, nil
	}
	rows, err := s.dbWithContext(ctx).q.ListOrganizationRolesByUserIDs(ctx, dbsqlc.ListOrganizationRolesByUserIDsParams{
		OrganizationPublicID: strings.TrimSpace(organizationPublicID), UserIds: deduped,
	})
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.UserID] = row.Role
	}
	return out, nil
}

// ListOrganizationRolesForUser resolves one user's role per organization in
// a single ANY lookup, replacing per-organization GetOrganizationRole round
// trips. Organizations holding no membership are omitted.
func (s *OrganizationStore) ListOrganizationRolesForUser(ctx context.Context, organizationPublicIDs []string, userID int64) (map[string]string, error) {
	out := make(map[string]string, len(organizationPublicIDs))
	seen := make(map[string]struct{}, len(organizationPublicIDs))
	deduped := make([]string, 0, len(organizationPublicIDs))
	for _, id := range organizationPublicIDs {
		trimmed := strings.TrimSpace(id)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		deduped = append(deduped, trimmed)
	}
	if len(deduped) == 0 || userID <= 0 {
		return out, nil
	}
	rows, err := s.dbWithContext(ctx).q.ListOrganizationRolesForUser(ctx, dbsqlc.ListOrganizationRolesForUserParams{
		OrganizationPublicIds: deduped, UserID: userID,
	})
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.OrganizationPublicID] = row.Role
	}
	return out, nil
}

// ListOrganizationWorkspaceCounts reports visible workspace counts per
// organization in a single GROUP BY, replacing per-organization
// ListWorkspacesByOrganizationID round trips. Closed organizations report
// zero; unknown organizations are omitted (callers read them as zero).
func (s *OrganizationStore) ListOrganizationWorkspaceCounts(ctx context.Context, organizationPublicIDs []string) (map[string]int64, error) {
	out := make(map[string]int64, len(organizationPublicIDs))
	seen := make(map[string]struct{}, len(organizationPublicIDs))
	deduped := make([]string, 0, len(organizationPublicIDs))
	for _, id := range organizationPublicIDs {
		trimmed := strings.TrimSpace(id)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		deduped = append(deduped, trimmed)
	}
	if len(deduped) == 0 {
		return out, nil
	}
	rows, err := s.dbWithContext(ctx).q.CountWorkspacesByOrganizationPublicIDs(ctx, deduped)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		out[row.OrganizationPublicID] = row.WorkspaceCount
	}
	return out, nil
}

// ListOrganizationWorkspaces lists workspaces owned by an organization.
func (s *OrganizationStore) ListOrganizationWorkspaces(ctx context.Context, organizationPublicID string) ([]*tenancy.Workspace, error) {
	organization, err := s.GetOrganization(ctx, organizationPublicID)
	if err != nil {
		return nil, err
	}
	rows, err := s.dbWithContext(ctx).q.ListWorkspacesByOrganizationID(ctx, dbsqlc.ListWorkspacesByOrganizationIDParams{
		OrganizationID: pgtype.Int8{Int64: organization.InternalID, Valid: true},
		RowLimit:       maxTenancyRows,
	})
	if err != nil {
		return nil, err
	}
	out := make([]*tenancy.Workspace, 0, len(rows))
	for _, row := range rows {
		orgPublicID := organization.PublicID
		out = append(out, mapWorkspace(row, &orgPublicID))
	}
	return out, nil
}

// CreateOrganizationWorkspace mints a ws_ workspace under an organization.
// Only org Owner/Admin may create (fail closed otherwise); the shared org
// pool is ensured in the same transaction (never the creator personal
// pool). The creator gains an owner membership row.
func (s *OrganizationStore) CreateOrganizationWorkspace(ctx context.Context, organizationPublicID string, ownerUserID int64, name string) (*tenancy.Workspace, error) {
	if err := tenancy.ValidateName(name); err != nil {
		return nil, err
	}
	for attempt := 0; attempt < mintAttempts; attempt++ {
		publicID, err := tenancy.NewWorkspacePublicID()
		if err != nil {
			return nil, err
		}
		var created *tenancy.Workspace
		err = s.dbWithContext(ctx).withTx(func(q *dbsqlc.Queries) error {
			organization, err := lockActiveOrganization(ctx, q, organizationPublicID)
			if err != nil {
				return err
			}
			membership, err := q.GetOrganizationMembership(ctx, dbsqlc.GetOrganizationMembershipParams{
				OrganizationID: organization.InternalID, UserID: ownerUserID,
			})
			if err != nil {
				return notFound(err)
			}
			if membership.Role != tenancy.RoleOwner && membership.Role != tenancy.RoleAdmin {
				return tenancy.ErrForbidden
			}
			row, err := createWorkspaceInTx(ctx, q, publicID,
				pgtype.Int8{Int64: organization.InternalID, Valid: true},
				tenancy.CreateWorkspaceParams{Name: name, OrganizationID: &organization.PublicID, OwnerUserID: ownerUserID})
			if err != nil {
				return err
			}
			created = mapWorkspace(row, &organization.PublicID)
			return nil
		})
		if isPublicIDConflict(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		return created, nil
	}
	return nil, tenancy.ErrConflict
}

func (s *OrganizationStore) mapOrganizationMembers(ctx context.Context, publicID string, rows []dbsqlc.OrganizationMembership) ([]*tenancy.OrganizationMember, error) {
	userIDs := make([]int64, 0, len(rows))
	for _, row := range rows {
		userIDs = append(userIDs, row.UserID)
	}
	externalIDs, err := userExternalIDs(ctx, s.dbWithContext(ctx).q, userIDs)
	if err != nil {
		return nil, err
	}
	out := make([]*tenancy.OrganizationMember, 0, len(rows))
	for _, row := range rows {
		out = append(out, &tenancy.OrganizationMember{
			OrganizationPublicID: publicID,
			UserID:               row.UserID,
			UserExternalID:       externalIDs[row.UserID],
			Role:                 row.Role,
			CreatedAt:            dbutil.TimeValue(row.CreatedAt),
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].UserID < out[j].UserID })
	return out, nil
}

func (s *OrganizationStore) orgMutationContext(ctx context.Context, q *dbsqlc.Queries, organizationInternalID, actorUserID int64) (string, int, error) {
	actor, err := q.GetOrganizationMembership(ctx, dbsqlc.GetOrganizationMembershipParams{
		OrganizationID: organizationInternalID, UserID: actorUserID,
	})
	actorRole := ""
	if err == nil {
		actorRole = actor.Role
	} else if !database.IsNotFound(err) {
		return "", 0, err
	}
	owners, err := q.CountOrganizationOwners(ctx, organizationInternalID)
	if err != nil {
		return "", 0, err
	}
	return actorRole, int(owners), nil
}

// ListOrganizationMembers lists all direct organization members ordered by user id.
func (s *OrganizationStore) ListOrganizationMembers(ctx context.Context, organizationPublicID string) ([]*tenancy.OrganizationMember, error) {
	organization, err := s.GetOrganization(ctx, organizationPublicID)
	if err != nil {
		return nil, err
	}
	rows, err := s.dbWithContext(ctx).q.ListOrganizationMembers(ctx, organization.InternalID)
	if err != nil {
		return nil, err
	}
	return s.mapOrganizationMembers(ctx, organization.PublicID, rows)
}

// AddOrganizationMember invites a user into the organization idempotently:
// re-adding keeps the existing role. Only Owner/Admin may invite and only
// Owner may grant Admin; Owner grants never flow here (handover only). The
// target joining a second organization fails closed with
// ErrOrganizationLimitReached.
func (s *OrganizationStore) AddOrganizationMember(ctx context.Context, organizationPublicID string, targetUserID int64, role string) (*tenancy.OrganizationMember, error) {
	role = strings.TrimSpace(role)
	if !tenancy.IsValidRole(role) || role == tenancy.RoleOwner {
		return nil, tenancy.ErrForbidden
	}
	var added *tenancy.OrganizationMember
	err := s.dbWithContext(ctx).withTx(func(q *dbsqlc.Queries) error {
		organization, err := lockActiveOrganization(ctx, q, organizationPublicID)
		if err != nil {
			return err
		}
		if _, err := q.LockUserRow(ctx, targetUserID); err != nil {
			if database.IsNotFound(err) {
				return tenancy.ErrNotFound
			}
			return err
		}
		if existing, err := q.GetOrganizationMembership(ctx, dbsqlc.GetOrganizationMembershipParams{
			OrganizationID: organization.InternalID, UserID: targetUserID,
		}); err == nil {
			externalIDs, err := userExternalIDs(ctx, q, []int64{targetUserID})
			if err != nil {
				return err
			}
			added = &tenancy.OrganizationMember{
				OrganizationPublicID: organization.PublicID,
				UserID:               targetUserID,
				UserExternalID:       externalIDs[targetUserID],
				Role:                 existing.Role,
				CreatedAt:            dbutil.TimeValue(existing.CreatedAt),
			}
			return nil
		} else if !database.IsNotFound(err) {
			return err
		}
		count, err := q.CountOrganizationsByUserID(ctx, targetUserID)
		if err != nil {
			return err
		}
		if count >= 1 {
			return tenancy.ErrOrganizationLimitReached
		}
		if err := q.UpsertOrganizationMembership(ctx, dbsqlc.UpsertOrganizationMembershipParams{
			OrganizationID: organization.InternalID, UserID: targetUserID, Role: role,
		}); err != nil {
			return err
		}
		row, err := q.GetOrganizationMembership(ctx, dbsqlc.GetOrganizationMembershipParams{
			OrganizationID: organization.InternalID, UserID: targetUserID,
		})
		if err != nil {
			return notFound(err)
		}
		externalIDs, err := userExternalIDs(ctx, q, []int64{targetUserID})
		if err != nil {
			return err
		}
		added = &tenancy.OrganizationMember{
			OrganizationPublicID: organization.PublicID,
			UserID:               targetUserID,
			UserExternalID:       externalIDs[targetUserID],
			Role:                 row.Role,
			CreatedAt:            dbutil.TimeValue(row.CreatedAt),
		}
		return nil
	})
	return added, err
}

// UpdateOrganizationMemberRole applies org role-transition rules: only
// Owner changes Admin roles; Owner rows are immutable here (handover only).
func (s *OrganizationStore) UpdateOrganizationMemberRole(ctx context.Context, organizationPublicID string, actorUserID, targetUserID int64, newRole string) (*tenancy.OrganizationMember, error) {
	var updated *tenancy.OrganizationMember
	err := s.dbWithContext(ctx).withTx(func(q *dbsqlc.Queries) error {
		organization, err := lockActiveOrganization(ctx, q, organizationPublicID)
		if err != nil {
			return err
		}
		target, err := q.GetOrganizationMembership(ctx, dbsqlc.GetOrganizationMembershipParams{
			OrganizationID: organization.InternalID, UserID: targetUserID,
		})
		if err != nil {
			return notFound(err)
		}
		actorRole, owners, err := s.orgMutationContext(ctx, q, organization.InternalID, actorUserID)
		if err != nil {
			return err
		}
		if err := tenancy.CheckOrganizationMemberMutation(actorRole, actorUserID, targetUserID, target.Role, newRole, owners, false); err != nil {
			return err
		}
		if err := q.UpsertOrganizationMembership(ctx, dbsqlc.UpsertOrganizationMembershipParams{
			OrganizationID: organization.InternalID, UserID: targetUserID, Role: strings.TrimSpace(newRole),
		}); err != nil {
			return err
		}
		row, err := q.GetOrganizationMembership(ctx, dbsqlc.GetOrganizationMembershipParams{
			OrganizationID: organization.InternalID, UserID: targetUserID,
		})
		if err != nil {
			return notFound(err)
		}
		externalIDs, err := userExternalIDs(ctx, q, []int64{targetUserID})
		if err != nil {
			return err
		}
		updated = &tenancy.OrganizationMember{
			OrganizationPublicID: organization.PublicID,
			UserID:               targetUserID,
			UserExternalID:       externalIDs[targetUserID],
			Role:                 row.Role,
			CreatedAt:            dbutil.TimeValue(row.CreatedAt),
		}
		return nil
	})
	return updated, err
}

// RemoveOrganizationMember removes an org membership and every explicit
// workspace grant of that user across the organization's workspaces in one
// transaction (leave = immediate loss of all workspace access; created
// boards/tasks/media stay org-owned and are never deleted here).
func (s *OrganizationStore) RemoveOrganizationMember(ctx context.Context, organizationPublicID string, actorUserID, targetUserID int64) error {
	return s.dbWithContext(ctx).withTx(func(q *dbsqlc.Queries) error {
		organization, err := lockActiveOrganization(ctx, q, organizationPublicID)
		if err != nil {
			return err
		}
		target, err := q.GetOrganizationMembership(ctx, dbsqlc.GetOrganizationMembershipParams{
			OrganizationID: organization.InternalID, UserID: targetUserID,
		})
		if err != nil {
			return notFound(err)
		}
		actorRole, owners, err := s.orgMutationContext(ctx, q, organization.InternalID, actorUserID)
		if err != nil {
			return err
		}
		if err := tenancy.CheckOrganizationMemberMutation(actorRole, actorUserID, targetUserID, target.Role, "", owners, true); err != nil {
			return err
		}
		if err := q.DeleteOrgWorkspaceMembershipsForUser(ctx, dbsqlc.DeleteOrgWorkspaceMembershipsForUserParams{
			OrganizationID: pgtype.Int8{Int64: organization.InternalID, Valid: true}, UserID: targetUserID,
		}); err != nil {
			return err
		}
		rows, err := q.DeleteOrganizationMembership(ctx, dbsqlc.DeleteOrganizationMembershipParams{
			OrganizationID: organization.InternalID, UserID: targetUserID,
		})
		if err != nil {
			return err
		}
		if rows == 0 {
			return tenancy.ErrNotFound
		}
		return nil
	})
}
