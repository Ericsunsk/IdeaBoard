package httpapi

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

func TestControlMutationRequestSchemas(t *testing.T) {
	doc, err := openapi3.NewLoader().LoadFromFile(filepath.Join("..", "..", "..", "..", "openapi.control.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name    string
		payload string
	}{
		{"ControlBanRequest", `{"expires_in_days":7}`},
		{"ControlCreditCommandRequest", `{"user_id":"usr_test","amount_credits":1,"business_reference_id":"test"}`},
		{"ControlCreditRefundRequest", `{"user_id":"usr_test","amount_credits":1,"business_reference_id":"test","charge_transaction_id":"charge_test"}`},
		{"ControlCreditCorrectionRequest", `{"user_id":"usr_test","amount_credits":1,"business_reference_id":"test","original_transaction_id":"charge_test"}`},
		{"ControlPaymentRefundRequest", `{"idea_quantity":1}`},
		{"ControlMailConfigRequest", `{"from_name":"IdeaBoard","from_email":"mail@example.com","reply_to":"support@example.com","enabled":false,"api_key":"test-key"}`},
		{"ControlMailTestRequest", `{"recipient":"recipient@example.com"}`},
		{"ControlOAuthProviderUpdateRequest", `{"enabled":true,"client_id":"test-client","client_secret":"test-secret"}`},
		{"ControlRewardCodeCreateRequest", `{"display_name":"Test","code":"TEST","credit_amount":1,"max_redemptions":10,"expires_at":"2030-01-01T00:00:00Z"}`},
		{"ControlRewardCodeUpdateRequest", `{"display_name":"Test","code":"TEST","credit_amount":1,"max_redemptions":10,"expires_at":null}`},
		{"ControlRewardCodeEnabledRequest", `{"enabled":false}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ref, ok := doc.Components.Schemas[tt.name]
			if !ok || ref.Value == nil {
				t.Fatalf("missing schema %s", tt.name)
			}
			var payload map[string]any
			if err := json.Unmarshal([]byte(tt.payload), &payload); err != nil {
				t.Fatal(err)
			}
			if err := ref.Value.VisitJSON(payload); err != nil {
				t.Fatalf("valid business payload rejected: %v", err)
			}
			for _, field := range []string{"unknown_field", "reason", "operator_note"} {
				payload[field] = "text"
				if err := ref.Value.VisitJSON(payload); err == nil {
					t.Fatalf("unknown field %s accepted", field)
				}
				delete(payload, field)
			}
		})
	}
}
