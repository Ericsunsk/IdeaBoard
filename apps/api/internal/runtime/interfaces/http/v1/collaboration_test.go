package v1

import (
	"context"

	"github.com/google/uuid"

	"ideaboard/api/internal/notifications"
	runtimehttp "ideaboard/api/internal/runtime/interfaces/http"
	"ideaboard/api/internal/tenancy"
	tenancyevents "ideaboard/api/internal/tenancy/events"
	"ideaboard/api/internal/tenancy/invitations"
)

// HTTP memory fixtures exercise transport behavior; atomicity and rollback are
// tested against PostgreSQL in the invitations application package.
func testInvitations(store tenancy.Store, transfers invitations.TransferStore, events tenancy.CollaborationEventDispatcher, identities invitations.IdentityReader) *invitations.Service {
	if events == nil {
		events = tenancyevents.NewCollaborationEventDispatcher(notifications.NewMemoryStore(), nil)
	}
	return &invitations.Service{
		Store: store, Transfers: transfers, Events: events, Identities: identities,
		Transaction: func(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) },
	}
}

// Invitation displays are optional; this test adapter mirrors only the public
// identity/display facts supplied by transport fixtures.
func testInvitationReader(dir runtimehttp.V1UserDirectory, display V1MemberDisplayProvider) invitations.Reader {
	return invitations.Reader{People: func(ctx context.Context, ids []int64) map[int64]invitations.Person {
		people := make(map[int64]invitations.Person, len(ids))
		publicIDs, _ := dir.GetUserPublicIDs(ctx, ids)
		displays := batchDisplayFacts(ctx, display, ids)
		for _, id := range ids {
			person := invitations.Person{Email: displays[id].Email, Avatar: displays[id].Avatar}
			if publicID := publicIDs[id]; publicID != uuid.Nil {
				person.PublicID = tenancy.FormatUserPublicID(publicID)
			}
			people[id] = person
		}
		return people
	}}
}
