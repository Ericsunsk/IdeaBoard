package invitations

import (
	"context"
	"time"

	"ideaboard/api/internal/tenancy"
)

// Person contains optional public identity and display facts, never credentials.
type Person struct {
	PublicID string
	Name     string
	Email    string
	Avatar   string
}

// PeopleReader enriches a deduplicated participant set in one batch. Missing
// facts are omitted: invitation display is not a required membership identity.
type PeopleReader func(context.Context, []int64) map[int64]Person

type View struct {
	ID                string
	ScopePublicID     string
	ScopeName         string
	Inviter           Person
	InviteeEmail      string
	InviteeUserID     string
	Role              string
	Status            string
	ExpiresAt         time.Time
	CreatedAt         time.Time
	RespondedAt       *time.Time
	RespondedByUserID string
}

// Reader owns participant collection, enrichment and the shared invitation
// projection. Transport adapters only add scope field names and wire formats.
type Reader struct {
	People PeopleReader
}

func (r Reader) Project(ctx context.Context, invites []*tenancy.MemberInvite) []View {
	views := make([]View, 0, len(invites))
	ids := make([]int64, 0, len(invites)*3)
	seen := make(map[int64]struct{}, len(invites)*3)
	add := func(id int64) {
		if id > 0 {
			if _, exists := seen[id]; !exists {
				seen[id] = struct{}{}
				ids = append(ids, id)
			}
		}
	}
	for _, invite := range invites {
		if invite == nil {
			continue
		}
		add(invite.InviterUserID)
		if invite.InviteeUserID != nil {
			add(*invite.InviteeUserID)
		}
		if invite.RespondedByUserID != nil {
			add(*invite.RespondedByUserID)
		}
	}
	people := make(map[int64]Person)
	if len(ids) > 0 {
		people = r.People(ctx, ids)
	}
	for _, invite := range invites {
		if invite == nil {
			continue
		}
		view := View{
			ID: invite.PublicID, ScopePublicID: invite.ScopePublicID, ScopeName: invite.ScopeName,
			Inviter: people[invite.InviterUserID], InviteeEmail: invite.InviteeEmail,
			Role: invite.Role, Status: invite.Status, ExpiresAt: invite.ExpiresAt, CreatedAt: invite.CreatedAt,
		}
		if invite.InviteeUserID != nil {
			view.InviteeUserID = people[*invite.InviteeUserID].PublicID
		}
		if invite.RespondedByUserID != nil {
			view.RespondedByUserID = people[*invite.RespondedByUserID].PublicID
		}
		if invite.RespondedAt != nil {
			responded := *invite.RespondedAt
			view.RespondedAt = &responded
		}
		views = append(views, view)
	}
	return views
}
