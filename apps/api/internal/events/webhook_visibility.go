package events

import (
	"bytes"
	"encoding/json"
	"strings"
)

// IsWorkspaceWebhookEventType identifies supported Workspace webhook types.
func IsWorkspaceWebhookEventType(typ string) bool {
	return strings.TrimSpace(typ) == EventV1BillingSettled
}

// Task-linked settlements, like Task, Media and Board content, require a
// user principal unavailable to Workspace webhooks.
func workspaceWebhookVisible(envelope V1Envelope) bool {
	if !IsWorkspaceWebhookEventType(envelope.Type) {
		return false
	}
	var data V1BillingSettledData
	decoder := json.NewDecoder(bytes.NewReader(envelope.Data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&data); err != nil {
		return false
	}
	return data.ReferenceType == V1ResourceCheckoutSession &&
		envelope.Resource.Type == V1ResourceCheckoutSession &&
		data.ReferenceID != "" && data.ReferenceID == envelope.Resource.ID
}
