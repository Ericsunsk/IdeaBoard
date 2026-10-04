// Package postgres implements tenancy.Store against PostgreSQL/sqlc.
//
// This is the #801 DB-stream backend for the agreed tenancy interface owned
// by internal/tenancy. User identity translation reuses
// tenancy.FormatUserPublicID/ParseUserPublicID (usr_<uuid_hex> over the
// stored users.public_id UUID); the users table is never altered here.
// Repository methods take context.Context first and use dbWithContext(ctx).
package postgres

import (
	"context"
	"sort"
	"strings"

	"ideaboard/api/internal/db/dbutil"
	dbsqlc "ideaboard/api/internal/db/sqlc"
	"ideaboard/api/internal/platform/database"
	"ideaboard/api/internal/tenancy"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Bounded list caps: every list query carries a LIMIT so no caller can
// trigger an unbounded scan.
const maxTenancyRows = int32(500)

// mintAttempts bounds public_id regeneration after a random collision.
const mintAttempts = 5

// Store is the postgres tenancy.Store. It must never be replaced by the
// test-only InMemoryStore in production wiring.
type Store struct {
	db *pgxpool.Pool
	q  *dbsqlc.Queries
}

// Compile-time convergence with the agreed interface.
var _ tenancy.Store = (*Store)(nil)

type repositoryQueryer struct {
	ctx context.Context
	db  *pgxpool.Pool
	q   *dbsqlc.Queries
}

// NewStore builds a postgres tenancy Store over pool.
func NewStore(db *pgxpool.Pool) *Store {
	return &Store{db: db, q: dbsqlc.New(db)}
}

func (s *Store) dbWithContext(ctx context.Context) repositoryQueryer {
	return repositoryQueryer{ctx: ctx, db: s.db, q: s.q}
}

func (db repositoryQueryer) withTx(run func(*dbsqlc.Queries) error) error {
	// Ambient guard transaction (control audit atomicity, step 3): when
	// the guard staged its mutation transaction in the context, the
	// tenancy writes join it as participants. The transaction is
	// BORROWED (the guard began it and the guard commits or rolls it
	// back), so this path never begins, commits, or rolls back. With no
	// carrier the legacy owned-transaction path below runs unchanged.
	if ambient, ok := dbutil.AmbientTxFromContext(db.ctx); ok {
		return run(dbsqlc.New(ambient))
	}
	return dbutil.WithTx(db.ctx, db.db, run)
}

func notFound(err error) error {
	if database.IsNotFound(err) {
		return tenancy.ErrNotFound
	}
	return err
}

func isPublicIDConflict(err error) bool {
	if !database.IsDuplicateConstraintError(err) {
		return false
	}
	lowered := strings.ToLower(err.Error())
	return strings.Contains(lowered, "uq_workspaces_public_id") ||
		strings.Contains(lowered, "uq_organizations_public_id")
}

func nullInt64(value int64) pgtype.Int8 {
	if value <= 0 {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: value, Valid: true}
}

func mapOrganization(row dbsqlc.Organization) *tenancy.Organization {
	return &tenancy.Organization{
		InternalID: row.InternalID,
		PublicID:   row.PublicID,
		Name:       row.Name,
		ArchivedAt: dbutil.TimePtr(row.ArchivedAt),
		CreatedAt:  dbutil.TimeValue(row.CreatedAt),
		UpdatedAt:  dbutil.TimeValue(row.UpdatedAt),
	}
}

func mapWorkspace(row dbsqlc.Workspace, organizationPublicID *string) *tenancy.Workspace {
	return &tenancy.Workspace{
		InternalID:     row.InternalID,
		PublicID:       row.PublicID,
		OrganizationID: organizationPublicID,
		OwnerUserID:    dbutil.Int64Ptr(row.OwnerUserID),
		Name:           row.Name,
		ArchivedAt:     dbutil.TimePtr(row.ArchivedAt),
		CreatedAt:      dbutil.TimeValue(row.CreatedAt),
		UpdatedAt:      dbutil.TimeValue(row.UpdatedAt),
	}
}

// resolveOrgPublicID maps a nullable organization internal id to its public
// id. Callers pass the already-known public id when they have it.
func (s *Store) resolveOrgPublicID(ctx context.Context, q *dbsqlc.Queries, organizationID pgtype.Int8) (*string, error) {
	if !organizationID.Valid {
		return nil, nil
	}
	row, err := q.GetOrganizationByInternalID(ctx, organizationID.Int64)
	if err != nil {
		return nil, notFound(err)
	}
	publicID := row.PublicID
	return &publicID, nil
}

func (s *Store) mapWorkspaceRow(ctx context.Context, row dbsqlc.Workspace) (*tenancy.Workspace, error) {
	orgPublicID, err := s.resolveOrgPublicID(ctx, s.q, row.OrganizationID)
	if err != nil {
		return nil, err
	}
	return mapWorkspace(row, orgPublicID), nil
}

// userExternalIDs batch-resolves users.id to usr_<hex> external ids.
func userExternalIDs(ctx context.Context, q *dbsqlc.Queries, userIDs []int64) (map[int64]string, error) {
	external := make(map[int64]string, len(userIDs))
	if len(userIDs) == 0 {
		return external, nil
	}
	rows, err := q.GetUsersByIDs(ctx, userIDs)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		external[row.ID] = tenancy.FormatUserPublicID(uuid.UUID(row.PublicID.Bytes))
	}
	return external, nil
}

func mapWorkspaceMember(publicID string, row dbsqlc.WorkspaceMembership, externalIDs map[int64]string) *tenancy.WorkspaceMember {
	return &tenancy.WorkspaceMember{
		WorkspacePublicID: publicID,
		UserID:            row.UserID,
		UserExternalID:    externalIDs[row.UserID],
		Role:              row.Role,
		CreatedAt:         dbutil.TimeValue(row.CreatedAt),
	}
}

// GetWorkspace resolves a workspace by opaque public id. Archived rows stay
// visible here (parity with the in-memory Store); lists filter them.
func (s *Store) GetWorkspace(ctx context.Context, publicID string) (*tenancy.Workspace, error) {
	row, err := s.dbWithContext(ctx).q.GetWorkspaceByPublicID(ctx, strings.TrimSpace(publicID))
	if err != nil {
		return nil, notFound(err)
	}
	return s.mapWorkspaceRow(ctx, row)
}

// ListWorkspacesForUser lists membership-visible workspaces. Owners always
// hold an owner membership row (granted at create/ensure time), so a
// membership join matches the in-memory Store exactly.
func (s *Store) ListWorkspacesForUser(ctx context.Context, userID int64) ([]*tenancy.Workspace, error) {
	rows, err := s.dbWithContext(ctx).q.ListWorkspacesByUserID(ctx, dbsqlc.ListWorkspacesByUserIDParams{
		UserID: userID, RowLimit: maxTenancyRows,
	})
	if err != nil {
		return nil, err
	}
	return s.mapWorkspaceRows(ctx, rows)
}

func (s *Store) mapWorkspaceRows(ctx context.Context, rows []dbsqlc.Workspace) ([]*tenancy.Workspace, error) {
	orgIDs := make(map[int64]struct{})
	for _, row := range rows {
		if row.OrganizationID.Valid {
			orgIDs[row.OrganizationID.Int64] = struct{}{}
		}
	}
	publicIDs := make(map[int64]string, len(orgIDs))
	for internalID := range orgIDs {
		row, err := s.q.GetOrganizationByInternalID(ctx, internalID)
		if err != nil {
			return nil, notFound(err)
		}
		publicIDs[internalID] = row.PublicID
	}
	out := make([]*tenancy.Workspace, 0, len(rows))
	for _, row := range rows {
		var orgPublicID *string
		if row.OrganizationID.Valid {
			publicID := publicIDs[row.OrganizationID.Int64]
			orgPublicID = &publicID
		}
		out = append(out, mapWorkspace(row, orgPublicID))
	}
	return out, nil
}

// CreateWorkspace mints a ws_ workspace under an organization (by public id)
// or a personal owner. The owner invariant (organization OR owner) is
// enforced by the database CHECK; the owner gains an owner membership row.
func (s *Store) CreateWorkspace(ctx context.Context, params tenancy.CreateWorkspaceParams) (*tenancy.Workspace, error) {
	if err := tenancy.ValidateName(params.Name); err != nil {
		return nil, err
	}
	var orgInternal pgtype.Int8
	var orgPublicID *string
	if params.OrganizationID != nil {
		org, err := s.GetOrganization(ctx, *params.OrganizationID)
		if err != nil {
			return nil, err
		}
		orgInternal = pgtype.Int8{Int64: org.InternalID, Valid: true}
		orgPublicID = &org.PublicID
	}
	if !orgInternal.Valid && params.OwnerUserID <= 0 {
		return nil, tenancy.ErrWorkspaceOwnerRequired
	}
	var created dbsqlc.Workspace
	for attempt := 0; attempt < mintAttempts; attempt++ {
		publicID, err := tenancy.NewWorkspacePublicID()
		if err != nil {
			return nil, err
		}
		err = s.dbWithContext(ctx).withTx(func(q *dbsqlc.Queries) error {
			row, insertErr := q.CreateWorkspace(ctx, dbsqlc.CreateWorkspaceParams{
				PublicID:       publicID,
				OrganizationID: orgInternal,
				OwnerUserID:    nullInt64(params.OwnerUserID),
				Name:           strings.TrimSpace(params.Name),
			})
			if insertErr != nil {
				return insertErr
			}
			created = row
			if params.OwnerUserID > 0 {
				return q.UpsertWorkspaceMembership(ctx, dbsqlc.UpsertWorkspaceMembershipParams{
					WorkspaceID: row.InternalID,
					UserID:      params.OwnerUserID,
					Role:        tenancy.RoleOwner,
				})
			}
			return nil
		})
		if isPublicIDConflict(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		return mapWorkspace(created, orgPublicID), nil
	}
	return nil, tenancy.ErrConflict
}

// UpdateWorkspace renames a live workspace; archived or missing rows read
// as not found.
func (s *Store) UpdateWorkspace(ctx context.Context, publicID, name string) (*tenancy.Workspace, error) {
	if err := tenancy.ValidateName(name); err != nil {
		return nil, err
	}
	rows, err := s.dbWithContext(ctx).q.UpdateWorkspaceName(ctx, dbsqlc.UpdateWorkspaceNameParams{
		Name: strings.TrimSpace(name), PublicID: strings.TrimSpace(publicID),
	})
	if err != nil {
		return nil, err
	}
	if rows == 0 {
		return nil, tenancy.ErrNotFound
	}
	return s.GetWorkspace(ctx, publicID)
}

// ArchiveWorkspace archives a live workspace and returns it with ArchivedAt
// set; already-archived or missing rows read as not found.
func (s *Store) ArchiveWorkspace(ctx context.Context, publicID string) (*tenancy.Workspace, error) {
	rows, err := s.dbWithContext(ctx).q.ArchiveWorkspace(ctx, strings.TrimSpace(publicID))
	if err != nil {
		return nil, err
	}
	if rows == 0 {
		return nil, tenancy.ErrNotFound
	}
	return s.GetWorkspace(ctx, publicID)
}

// EnsurePersonalWorkspace idempotently provisions the user's personal
// workspace. Concurrent callers collapse on ux_workspaces_personal_owner
// (INSERT ... ON CONFLICT DO NOTHING) and all read back the same row.
func (s *Store) EnsurePersonalWorkspace(ctx context.Context, userID int64) (*tenancy.Workspace, error) {
	if userID <= 0 {
		return nil, tenancy.ErrPersonalWorkspaceUserRequired
	}
	for attempt := 0; attempt < mintAttempts; attempt++ {
		publicID, err := tenancy.NewWorkspacePublicID()
		if err != nil {
			return nil, err
		}
		var row dbsqlc.Workspace
		err = s.dbWithContext(ctx).withTx(func(q *dbsqlc.Queries) error {
			if _, insertErr := q.InsertPersonalWorkspaceIgnore(ctx, dbsqlc.InsertPersonalWorkspaceIgnoreParams{
				PublicID:    publicID,
				OwnerUserID: pgtype.Int8{Int64: userID, Valid: true},
				Name:        "My Workspace",
			}); insertErr != nil {
				return insertErr
			}
			selected, selectErr := q.GetPersonalWorkspaceByOwner(ctx, pgtype.Int8{Int64: userID, Valid: true})
			if selectErr != nil {
				return selectErr
			}
			row = selected
			return q.UpsertWorkspaceMembership(ctx, dbsqlc.UpsertWorkspaceMembershipParams{
				WorkspaceID: row.InternalID,
				UserID:      userID,
				Role:        tenancy.RoleOwner,
			})
		})
		if isPublicIDConflict(err) {
			continue
		}
		if err != nil {
			return nil, notFound(err)
		}
		return mapWorkspace(row, nil), nil
	}
	return nil, tenancy.ErrConflict
}

// GetWorkspaceRole reports the caller's direct workspace membership role.
// It returns ok=false when the workspace exists but the user is not a
// member, and ErrNotFound when the workspace does not exist.
func (s *Store) GetWorkspaceRole(ctx context.Context, workspacePublicID string, userID int64) (string, bool, error) {
	workspace, err := s.GetWorkspace(ctx, workspacePublicID)
	if err != nil {
		return "", false, err
	}
	row, err := s.dbWithContext(ctx).q.GetWorkspaceMembership(ctx, dbsqlc.GetWorkspaceMembershipParams{
		WorkspaceID: workspace.InternalID, UserID: userID,
	})
	if database.IsNotFound(err) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return row.Role, true, nil
}

// ListWorkspaceMembers lists direct workspace members ordered by user id
// (parity with the in-memory Store ordering).
func (s *Store) ListWorkspaceMembers(ctx context.Context, workspacePublicID string) ([]*tenancy.WorkspaceMember, error) {
	workspace, err := s.GetWorkspace(ctx, workspacePublicID)
	if err != nil {
		return nil, err
	}
	rows, err := s.dbWithContext(ctx).q.ListWorkspaceMembers(ctx, dbsqlc.ListWorkspaceMembersParams{
		WorkspaceID: workspace.InternalID, RowLimit: maxTenancyRows,
	})
	if err != nil {
		return nil, err
	}
	return s.mapWorkspaceMembers(ctx, workspace.PublicID, rows)
}

func (s *Store) mapWorkspaceMembers(ctx context.Context, publicID string, rows []dbsqlc.WorkspaceMembership) ([]*tenancy.WorkspaceMember, error) {
	userIDs := make([]int64, 0, len(rows))
	for _, row := range rows {
		userIDs = append(userIDs, row.UserID)
	}
	externalIDs, err := userExternalIDs(ctx, s.q, userIDs)
	if err != nil {
		return nil, err
	}
	out := make([]*tenancy.WorkspaceMember, 0, len(rows))
	for _, row := range rows {
		out = append(out, mapWorkspaceMember(publicID, row, externalIDs))
	}
	sortMembersByUserID(out)
	return out, nil
}

func sortMembersByUserID(members []*tenancy.WorkspaceMember) {
	sort.Slice(members, func(i, j int) bool { return members[i].UserID < members[j].UserID })
}

// AddWorkspaceMember grants a membership idempotently: re-adding keeps the
// existing role (parity with the in-memory Store).
func (s *Store) AddWorkspaceMember(ctx context.Context, workspacePublicID string, targetUserID int64, role string) (*tenancy.WorkspaceMember, error) {
	role = strings.TrimSpace(role)
	if !tenancy.IsValidRole(role) {
		return nil, tenancy.ErrForbidden
	}
	workspace, err := s.GetWorkspace(ctx, workspacePublicID)
	if err != nil {
		return nil, err
	}
	dbCtx := s.dbWithContext(ctx)
	if _, err := dbCtx.q.InsertWorkspaceMembershipIgnore(ctx, dbsqlc.InsertWorkspaceMembershipIgnoreParams{
		WorkspaceID: workspace.InternalID, UserID: targetUserID, Role: role,
	}); err != nil {
		return nil, err
	}
	row, err := dbCtx.q.GetWorkspaceMembership(ctx, dbsqlc.GetWorkspaceMembershipParams{
		WorkspaceID: workspace.InternalID, UserID: targetUserID,
	})
	if err != nil {
		return nil, notFound(err)
	}
	externalIDs, err := userExternalIDs(ctx, dbCtx.q, []int64{targetUserID})
	if err != nil {
		return nil, err
	}
	return mapWorkspaceMember(workspace.PublicID, row, externalIDs), nil
}

// UpdateWorkspaceMemberRole applies the agreed role-transition rules before
// persisting the new role.
func (s *Store) UpdateWorkspaceMemberRole(ctx context.Context, workspacePublicID string, actorUserID, targetUserID int64, newRole string) (*tenancy.WorkspaceMember, error) {
	workspace, err := s.GetWorkspace(ctx, workspacePublicID)
	if err != nil {
		return nil, err
	}
	dbCtx := s.dbWithContext(ctx)
	target, err := dbCtx.q.GetWorkspaceMembership(ctx, dbsqlc.GetWorkspaceMembershipParams{
		WorkspaceID: workspace.InternalID, UserID: targetUserID,
	})
	if err != nil {
		return nil, notFound(err)
	}
	actorRole, owners, err := s.mutationContext(ctx, dbCtx.q, workspace.InternalID, actorUserID)
	if err != nil {
		return nil, err
	}
	if err := tenancy.CheckMemberMutation(actorRole, actorUserID, targetUserID, target.Role, newRole, owners, false); err != nil {
		return nil, err
	}
	if _, err := dbCtx.q.UpdateWorkspaceMemberRole(ctx, dbsqlc.UpdateWorkspaceMemberRoleParams{
		WorkspaceID: workspace.InternalID, UserID: targetUserID, Role: strings.TrimSpace(newRole),
	}); err != nil {
		return nil, err
	}
	updated, err := dbCtx.q.GetWorkspaceMembership(ctx, dbsqlc.GetWorkspaceMembershipParams{
		WorkspaceID: workspace.InternalID, UserID: targetUserID,
	})
	if err != nil {
		return nil, notFound(err)
	}
	externalIDs, err := userExternalIDs(ctx, dbCtx.q, []int64{targetUserID})
	if err != nil {
		return nil, err
	}
	return mapWorkspaceMember(workspace.PublicID, updated, externalIDs), nil
}

// RemoveWorkspaceMember applies the agreed removal rules (last-owner guard,
// self-removal) before deleting the membership row.
func (s *Store) RemoveWorkspaceMember(ctx context.Context, workspacePublicID string, actorUserID, targetUserID int64) error {
	workspace, err := s.GetWorkspace(ctx, workspacePublicID)
	if err != nil {
		return err
	}
	dbCtx := s.dbWithContext(ctx)
	target, err := dbCtx.q.GetWorkspaceMembership(ctx, dbsqlc.GetWorkspaceMembershipParams{
		WorkspaceID: workspace.InternalID, UserID: targetUserID,
	})
	if err != nil {
		return notFound(err)
	}
	actorRole, owners, err := s.mutationContext(ctx, dbCtx.q, workspace.InternalID, actorUserID)
	if err != nil {
		return err
	}
	if err := tenancy.CheckMemberMutation(actorRole, actorUserID, targetUserID, target.Role, "", owners, true); err != nil {
		return err
	}
	rows, err := dbCtx.q.DeleteWorkspaceMembership(ctx, dbsqlc.DeleteWorkspaceMembershipParams{
		WorkspaceID: workspace.InternalID, UserID: targetUserID,
	})
	if err != nil {
		return err
	}
	if rows == 0 {
		return tenancy.ErrNotFound
	}
	return nil
}

func (s *Store) mutationContext(ctx context.Context, q *dbsqlc.Queries, workspaceInternalID, actorUserID int64) (string, int, error) {
	actor, err := q.GetWorkspaceMembership(ctx, dbsqlc.GetWorkspaceMembershipParams{
		WorkspaceID: workspaceInternalID, UserID: actorUserID,
	})
	actorRole := ""
	if err == nil {
		actorRole = actor.Role
	} else if !database.IsNotFound(err) {
		return "", 0, err
	}
	owners, err := q.CountWorkspaceOwners(ctx, workspaceInternalID)
	if err != nil {
		return "", 0, err
	}
	return actorRole, int(owners), nil
}

// CreateOrganization mints an org_ organization and grants the creator the
// owner membership in the same transaction.
func (s *Store) CreateOrganization(ctx context.Context, ownerUserID int64, name string) (*tenancy.Organization, error) {
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
			row, insertErr := q.CreateOrganization(ctx, dbsqlc.CreateOrganizationParams{
				PublicID: publicID, Name: strings.TrimSpace(name),
			})
			if insertErr != nil {
				return insertErr
			}
			created = row
			return q.UpsertOrganizationMembership(ctx, dbsqlc.UpsertOrganizationMembershipParams{
				OrganizationID: row.InternalID,
				UserID:         ownerUserID,
				Role:           tenancy.RoleOwner,
			})
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
func (s *Store) GetOrganization(ctx context.Context, publicID string) (*tenancy.Organization, error) {
	row, err := s.dbWithContext(ctx).q.GetOrganizationByPublicID(ctx, strings.TrimSpace(publicID))
	if err != nil {
		return nil, notFound(err)
	}
	return mapOrganization(row), nil
}

// ListOrganizationsForUser lists membership-visible organizations.
func (s *Store) ListOrganizationsForUser(ctx context.Context, userID int64) ([]*tenancy.Organization, error) {
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

// UpdateOrganization renames a live organization; archived or missing rows
// read as not found.
func (s *Store) UpdateOrganization(ctx context.Context, publicID, name string) (*tenancy.Organization, error) {
	if err := tenancy.ValidateName(name); err != nil {
		return nil, err
	}
	rows, err := s.dbWithContext(ctx).q.UpdateOrganizationName(ctx, dbsqlc.UpdateOrganizationNameParams{
		Name: strings.TrimSpace(name), PublicID: strings.TrimSpace(publicID),
	})
	if err != nil {
		return nil, err
	}
	if rows == 0 {
		return nil, tenancy.ErrNotFound
	}
	return s.GetOrganization(ctx, publicID)
}

// GetOrganizationRole reports the caller's direct organization membership
// role, ok=false when the organization exists but the user is not a member.
func (s *Store) GetOrganizationRole(ctx context.Context, organizationPublicID string, userID int64) (string, bool, error) {
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

// ListOrganizationWorkspaces lists live workspaces owned by an organization.
func (s *Store) ListOrganizationWorkspaces(ctx context.Context, organizationPublicID string) ([]*tenancy.Workspace, error) {
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
// The creator gains an owner membership row.
func (s *Store) CreateOrganizationWorkspace(ctx context.Context, organizationPublicID string, ownerUserID int64, name string) (*tenancy.Workspace, error) {
	if err := tenancy.ValidateName(name); err != nil {
		return nil, err
	}
	organization, err := s.GetOrganization(ctx, organizationPublicID)
	if err != nil {
		return nil, err
	}
	params := tenancy.CreateWorkspaceParams{
		Name:           name,
		OrganizationID: &organization.PublicID,
		OwnerUserID:    ownerUserID,
	}
	return s.CreateWorkspace(ctx, params)
}
