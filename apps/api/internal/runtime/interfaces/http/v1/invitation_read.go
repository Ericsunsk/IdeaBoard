package v1

import (
	"context"

	identityports "ideaboard/api/internal/identity/users/ports"
	"ideaboard/api/internal/tenancy"
	"ideaboard/api/internal/tenancy/invitations"

	"github.com/google/uuid"
)

// NewInvitationReader joins public identity and names from one local batch;
// optional Auth display failure cannot discard those local facts.
func NewInvitationReader(users identityports.UserRepository, emails identityports.UserEmailReader) invitations.Reader {
	return invitations.Reader{People: func(ctx context.Context, ids []int64) map[int64]invitations.Person {
		people := make(map[int64]invitations.Person, len(ids))
		rows, err := users.GetByIDs(ctx, ids)
		if err != nil {
			return people
		}
		displays, _ := memberDisplayForUsers(ctx, rows, emails)
		for id, row := range rows {
			if row == nil {
				continue
			}
			person := invitations.Person{Name: row.DisplayName}
			if row.PublicID != uuid.Nil {
				person.PublicID = tenancy.FormatUserPublicID(row.PublicID)
			}
			person.Email, person.Avatar = displays[id].Email, displays[id].Avatar
			people[id] = person
		}
		return people
	}}
}

type v1MemberInviteDTO struct {
	ID                string `json:"id"`
	InviterUserID     string `json:"inviter_user_id"`
	InviterEmail      string `json:"inviter_email,omitempty"`
	InviterName       string `json:"inviter_name,omitempty"`
	InviterAvatar     string `json:"inviter_avatar,omitempty"`
	InviteeEmail      string `json:"invitee_email"`
	InviteeUserID     string `json:"invitee_user_id,omitempty"`
	Role              string `json:"role"`
	Status            string `json:"status"`
	ExpiresAt         string `json:"expires_at"`
	CreatedAt         string `json:"created_at"`
	RespondedAt       string `json:"responded_at,omitempty"`
	RespondedByUserID string `json:"responded_by_user_id,omitempty"`
	Token             string `json:"token,omitempty"`
}

func memberInviteDTO(view invitations.View) v1MemberInviteDTO {
	dto := v1MemberInviteDTO{
		ID: view.ID, InviterUserID: view.Inviter.PublicID, InviterEmail: view.Inviter.Email,
		InviterName: view.Inviter.Name, InviterAvatar: view.Inviter.Avatar,
		InviteeEmail: view.InviteeEmail, InviteeUserID: view.InviteeUserID,
		Role: view.Role, Status: view.Status, ExpiresAt: v1FormatTime(view.ExpiresAt),
		CreatedAt: v1FormatTime(view.CreatedAt), RespondedByUserID: view.RespondedByUserID,
	}
	if view.RespondedAt != nil {
		dto.RespondedAt = v1FormatTime(*view.RespondedAt)
	}
	return dto
}
