package auth

import (
	"context"
	"fmt"
	"net/http"

	"ideaboard/api/internal/identity/auth/internalcontract"

	openapi_types "github.com/oapi-codegen/runtime/types"
)

// WorkspaceInvitationMail contains only the facts needed by Auth to render and
// deliver a Workspace invitation. Auth derives the link and idempotency key.
type WorkspaceInvitationMail struct {
	Recipient     string
	InviteID      string
	WorkspaceName string
	InviterName   string
	Role          string
	Token         string
}

func (c *ServiceClient) SendWorkspaceInvitation(ctx context.Context, input WorkspaceInvitationMail) error {
	client, err := c.contractClient()
	if err != nil {
		return err
	}
	response, err := client.SendWorkspaceInvitationWithResponse(ctx, internalcontract.WorkspaceInvitationMailRequest{
		Recipient:     openapi_types.Email(input.Recipient),
		InviteID:      input.InviteID,
		WorkspaceName: input.WorkspaceName,
		InviterName:   input.InviterName,
		Role:          internalcontract.WorkspaceInvitationMailRequestRole(input.Role),
		Token:         &input.Token,
	})
	if err != nil {
		return err
	}
	if response.StatusCode() != http.StatusOK || response.JSON200 == nil {
		return mailConfigContractFailure("send workspace invitation", response.StatusCode(), response.Body)
	}
	if _, err := requireSuccessEnvelopeFields(response.Body); err != nil || response.JSON200.Code != 0 || !bool(response.JSON200.Data.Ok) {
		return fmt.Errorf("auth service send workspace invitation returned an invalid success envelope")
	}
	return nil
}
