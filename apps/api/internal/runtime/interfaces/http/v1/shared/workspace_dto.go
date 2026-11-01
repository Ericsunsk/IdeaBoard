package shared

import (
	"strings"
	"time"

	"ideaboard/api/internal/platform/contract"
	"ideaboard/api/internal/tenancy"
)

// FormatTime formats a time.Time to RFC3339 UTC string, returning empty string for zero time.
func FormatTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// WorkspaceDTO represents a workspace on the wire.
type WorkspaceDTO struct {
	ID               string  `json:"id"`
	OrganizationID   *string `json:"organization_id"`
	Name             string  `json:"name"`
	IsDefault        bool    `json:"is_default"`
	CreatedAt        string  `json:"created_at"`
	BillingAccountID *string `json:"billing_account_id,omitempty"`
}

// ToWorkspaceDTO converts a tenancy.Workspace to WorkspaceDTO.
func ToWorkspaceDTO(w *tenancy.Workspace) WorkspaceDTO {
	dto := WorkspaceDTO{
		ID:        w.PublicID,
		Name:      w.Name,
		IsDefault: w.IsDefaultWorkspace(),
		CreatedAt: FormatTime(w.CreatedAt),
	}
	if w.OrganizationID != nil {
		org := *w.OrganizationID
		dto.OrganizationID = &org
	}
	return dto
}

// DeletedDTO represents a standard deletion confirmation.
type DeletedDTO struct {
	Deleted bool   `json:"deleted"`
	ID      string `json:"id,omitempty"`
}

// WithWorkspaceBillingAccount attaches a billing account ID to the workspace DTO.
func WithWorkspaceBillingAccount(dto WorkspaceDTO, billingAccountPublicID string) WorkspaceDTO {
	if trimmed := strings.TrimSpace(billingAccountPublicID); contract.IsBillingAccountID(trimmed) {
		dto.BillingAccountID = &trimmed
	}
	return dto
}
