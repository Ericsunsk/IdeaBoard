package events

import (
	"context"
	"fmt"

	"ideaboard/api/internal/notifications"
	"ideaboard/api/internal/tenancy"
)

type collaborationEventDispatcher struct {
	notifications notifications.Store
	emailSender   tenancy.WorkspaceInviteEmailSender
}

// NewCollaborationEventDispatcher creates a CollaborationEventDispatcher backed by
// a notification store and a transactional mail enqueuer. Callers run it
// inside the same transaction as the invitation; no write error is best-effort.
func NewCollaborationEventDispatcher(notifications notifications.Store, emailSender tenancy.WorkspaceInviteEmailSender) tenancy.CollaborationEventDispatcher {
	return &collaborationEventDispatcher{
		notifications: notifications,
		emailSender:   emailSender,
	}
}

func (d *collaborationEventDispatcher) OnWorkspaceMemberInvited(ctx context.Context, invite *tenancy.WorkspaceMemberInvite, rawToken string) error {
	if d == nil || invite == nil {
		return nil
	}
	if d.emailSender != nil && rawToken != "" {
		if err := d.emailSender.SendWorkspaceInviteEmail(ctx, invite, rawToken); err != nil {
			return err
		}
	}
	if d.notifications != nil && invite.InviteeUserID != nil && *invite.InviteeUserID > 0 {
		wsName := invite.WorkspaceName
		if wsName == "" {
			wsName = invite.WorkspacePublicID
		}
		_, err := d.notifications.CreateNotification(ctx, notifications.CreateNotificationInput{
			UserID:           *invite.InviteeUserID,
			DeduplicationKey: "invite:" + invite.PublicID,
			Type:             notifications.TypeWorkspaceInvite,
			Title:            "Workspace Invitation",
			Body:             fmt.Sprintf("You have been invited to join %s as %s", wsName, invite.Role),
			ActionPayload: notifications.InviteActionPayload{
				InviteID:      invite.PublicID,
				InviteStatus:  tenancy.MemberInviteStatusPending,
				Role:          invite.Role,
				WorkspaceID:   invite.WorkspacePublicID,
				WorkspaceName: wsName,
			}.ToActionPayload(),
		})
		return err
	}
	return nil
}

func (d *collaborationEventDispatcher) OnWorkspaceMemberInviteAccepted(ctx context.Context, invite *tenancy.WorkspaceMemberInvite, member *tenancy.WorkspaceMember, recipientEmail string) error {
	if d == nil || d.notifications == nil || invite == nil || invite.InviterUserID <= 0 {
		return nil
	}
	// Resolution and the inviter notification join the invitation transaction.
	// Any failure rolls back the membership decision as well.
	if _, err := d.notifications.MarkInviteResolved(ctx, invite.PublicID, tenancy.MemberInviteStatusAccepted); err != nil {
		return err
	}
	memberPID := ""
	if member != nil {
		memberPID = member.UserExternalID
	}
	name := recipientEmail
	if name == "" {
		name = memberPID
	}
	wsName := invite.WorkspaceName
	if wsName == "" {
		wsName = invite.WorkspacePublicID
	}
	_, err := d.notifications.CreateNotification(ctx, notifications.CreateNotificationInput{
		UserID:           invite.InviterUserID,
		DeduplicationKey: "invite:" + invite.PublicID + ":accepted",
		Type:             notifications.TypeWorkspaceMemberJoined,
		Title:            "Workspace Member Joined",
		Body:             fmt.Sprintf("%s joined %s", name, wsName),
		ActionPayload: notifications.InviteActionPayload{
			InviteID:      invite.PublicID,
			Role:          invite.Role,
			WorkspaceID:   invite.WorkspacePublicID,
			WorkspaceName: wsName,
			MemberUserID:  memberPID,
		}.ToActionPayload(),
	})
	return err
}

func (d *collaborationEventDispatcher) OnWorkspaceMemberInviteDeclined(ctx context.Context, invite *tenancy.WorkspaceMemberInvite) error {
	if d == nil || d.notifications == nil || invite == nil || invite.InviterUserID <= 0 {
		return nil
	}
	if _, err := d.notifications.MarkInviteResolved(ctx, invite.PublicID, tenancy.MemberInviteStatusDeclined); err != nil {
		return err
	}
	name := invite.InviteeEmail
	wsName := invite.WorkspaceName
	if wsName == "" {
		wsName = invite.WorkspacePublicID
	}
	_, err := d.notifications.CreateNotification(ctx, notifications.CreateNotificationInput{
		UserID:           invite.InviterUserID,
		DeduplicationKey: "invite:" + invite.PublicID + ":declined",
		Type:             notifications.TypeWorkspaceInviteDeclined,
		Title:            "Workspace Invitation Declined",
		Body:             fmt.Sprintf("%s declined the invitation to join %s", name, wsName),
		ActionPayload: notifications.InviteActionPayload{
			InviteID:      invite.PublicID,
			Role:          invite.Role,
			WorkspaceID:   invite.WorkspacePublicID,
			WorkspaceName: wsName,
			InviteeEmail:  invite.InviteeEmail,
		}.ToActionPayload(),
	})
	return err
}

func (d *collaborationEventDispatcher) OnMemberInviteCancelled(ctx context.Context, inviteID string) error {
	if d == nil || d.notifications == nil || inviteID == "" {
		return nil
	}
	if _, err := d.notifications.MarkInviteResolved(ctx, inviteID, tenancy.MemberInviteStatusCancelled); err != nil {
		return err
	}
	return nil
}

func (d *collaborationEventDispatcher) OnOrganizationMemberInvited(ctx context.Context, invite *tenancy.OrganizationMemberInvite) error {
	if d == nil || invite == nil {
		return nil
	}
	if d.notifications != nil && invite.InviteeUserID != nil && *invite.InviteeUserID > 0 {
		orgName := invite.OrganizationName
		if orgName == "" {
			orgName = invite.OrganizationPublicID
		}
		_, err := d.notifications.CreateNotification(ctx, notifications.CreateNotificationInput{
			UserID:           *invite.InviteeUserID,
			DeduplicationKey: "invite:" + invite.PublicID,
			Type:             notifications.TypeOrganizationInvite,
			Title:            "Organization Invitation",
			Body:             fmt.Sprintf("You have been invited to join %s as %s", orgName, invite.Role),
			ActionPayload: notifications.InviteActionPayload{
				InviteID:         invite.PublicID,
				InviteStatus:     tenancy.MemberInviteStatusPending,
				Role:             invite.Role,
				OrganizationID:   invite.OrganizationPublicID,
				OrganizationName: orgName,
			}.ToActionPayload(),
		})
		return err
	}
	return nil
}

func (d *collaborationEventDispatcher) OnOrganizationMemberInviteAccepted(ctx context.Context, invite *tenancy.OrganizationMemberInvite, member *tenancy.OrganizationMember, recipientEmail string) error {
	if d == nil || d.notifications == nil || invite == nil || invite.InviterUserID <= 0 {
		return nil
	}
	if _, err := d.notifications.MarkInviteResolved(ctx, invite.PublicID, tenancy.MemberInviteStatusAccepted); err != nil {
		return err
	}
	memberPID := ""
	if member != nil {
		memberPID = member.UserExternalID
	}
	name := recipientEmail
	if name == "" {
		name = memberPID
	}
	orgName := invite.OrganizationName
	if orgName == "" {
		orgName = invite.OrganizationPublicID
	}
	_, err := d.notifications.CreateNotification(ctx, notifications.CreateNotificationInput{
		UserID:           invite.InviterUserID,
		DeduplicationKey: "invite:" + invite.PublicID + ":accepted",
		Type:             notifications.TypeOrganizationMemberJoined,
		Title:            "Organization Member Joined",
		Body:             fmt.Sprintf("%s joined %s", name, orgName),
		ActionPayload: notifications.InviteActionPayload{
			InviteID:         invite.PublicID,
			Role:             invite.Role,
			OrganizationID:   invite.OrganizationPublicID,
			OrganizationName: orgName,
			MemberUserID:     memberPID,
			MemberEmail:      recipientEmail,
		}.ToActionPayload(),
	})
	return err
}

func (d *collaborationEventDispatcher) OnOrganizationMemberInviteDeclined(ctx context.Context, invite *tenancy.OrganizationMemberInvite) error {
	if d == nil || d.notifications == nil || invite == nil || invite.InviterUserID <= 0 {
		return nil
	}
	if _, err := d.notifications.MarkInviteResolved(ctx, invite.PublicID, tenancy.MemberInviteStatusDeclined); err != nil {
		return err
	}
	name := invite.InviteeEmail
	orgName := invite.OrganizationName
	if orgName == "" {
		orgName = invite.OrganizationPublicID
	}
	_, err := d.notifications.CreateNotification(ctx, notifications.CreateNotificationInput{
		UserID:           invite.InviterUserID,
		DeduplicationKey: "invite:" + invite.PublicID + ":declined",
		Type:             notifications.TypeOrganizationInviteDeclined,
		Title:            "Organization Invitation Declined",
		Body:             fmt.Sprintf("%s declined the invitation to join %s", name, orgName),
		ActionPayload: notifications.InviteActionPayload{
			InviteID:         invite.PublicID,
			Role:             invite.Role,
			OrganizationID:   invite.OrganizationPublicID,
			OrganizationName: orgName,
			InviteeEmail:     invite.InviteeEmail,
		}.ToActionPayload(),
	})
	return err
}

func (d *collaborationEventDispatcher) OnBillingTransferInitiated(ctx context.Context, transfer *tenancy.BillingTransferInvite, params tenancy.BillingTransferInitiatedParams) error {
	if d == nil || d.notifications == nil || transfer == nil || transfer.ToUserID <= 0 {
		return nil
	}
	wsName := params.WorkspaceName
	if wsName == "" {
		wsName = params.WorkspacePublicID
	}
	payload := notifications.HandoverActionPayload{
		TransferInviteID: transfer.PublicID,
		WorkspaceID:      params.WorkspacePublicID,
		WorkspaceName:    wsName,
		FromUserID:       params.FromUserPublicID,
	}
	if params.BillingAccountPublicID != "" {
		payload.BillingAccountID = params.BillingAccountPublicID
	}
	_, err := d.notifications.CreateNotification(ctx, notifications.CreateNotificationInput{
		UserID:           transfer.ToUserID,
		DeduplicationKey: params.NotificationKey,
		Type:             notifications.TypeStudioHandover,
		Title:            "Studio Handover Invitation",
		Body:             fmt.Sprintf("You have been invited to take over ownership of the studio billing account for workspace %s", wsName),
		ActionPayload:    payload.ToActionPayload(),
	})
	return err
}
