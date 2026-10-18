package webhook

import (
	"bytes"
	"encoding/json"
	"strings"

	"ideaboard/api/internal/events"
)

// IsWorkspaceWebhookEventType identifies supported Workspace webhook types.
func IsWorkspaceWebhookEventType(typ string) bool {
	return strings.TrimSpace(typ) == events.EventV1BillingSettled
}

// Task-linked settlements, like Task, Media and Board content, require a
// user principal unavailable to Workspace webhooks.
func workspaceWebhookVisible(envelope events.V1Envelope) bool {
	if !IsWorkspaceWebhookEventType(envelope.Type) {
		return false
	}
	var data events.V1BillingSettledData
	decoder := json.NewDecoder(bytes.NewReader(envelope.Data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&data); err != nil {
		return false
	}
	return data.ReferenceType == events.V1ResourceCheckoutSession &&
		envelope.Resource.Type == events.V1ResourceCheckoutSession &&
		data.ReferenceID != "" && data.ReferenceID == envelope.Resource.ID
}
