package httpapi

// Control Plane route registration (issue #808).
//
// RegisterContractControlRoutes mounts the full Control Plane surface on
// the group: dedicated contract-envelope handlers for users, workspaces,
// billing, credits, payments, tasks, and platform settings, plus guarded
// adapters over the legacy admin handlers for ai-gateway, storage, search
// providers, feature prompts, monitor/statistics, brand-logo overrides,
// and rewards (single-sourced validation and side effects; the
// control-facing render layer translates every proxied response into the
// contract envelope at the boundary, parent #809). Unknown control paths
// fail closed with 501. There is NO task creation route here by design.
// Legacy /api/admin/* registration is untouched.

import (
	"net/http"
	"strings"

	"ideaboard/api/internal/platform/contract"

	"github.com/gin-gonic/gin"
)

// controlProxy adapts a legacy admin handler method to a guarded control
// route with the unified contract envelope: the legacy response is
// captured and re-rendered as {data} / {error:{code,message,request_id}}
// at the same HTTP status (see translateControlProxyResponse). available
// reports whether the handler is wired; nil stays fail-closed with 501
// instead of panicking.
func (d *ContractControlDeps) controlProxy(route controlRoute, capability string, available bool, method func(*gin.Context)) gin.HandlerFunc {
	return d.guard(capability, route, func(c *gin.Context, _ controlContext) {
		if !available {
			contract.RenderError(c, http.StatusNotImplemented, contract.CodeContractNotImplemented, "控制面服务未实现")
			return
		}
		translateControlProxyResponse(c, method)
	})
}

// RegisterContractControlRoutes mounts the Control Plane handlers on
// group (exported for tests; production calls it from RegisterRoutes
// with session auth deps). The group carries session auth, tenant scope
// resolution, the operator grant, and the separate control rate-limit
// bucket; every route adds the capability + audit + idempotency guard.
func RegisterContractControlRoutes(group *gin.RouterGroup, authDeps AuthDeps, deps ContractControlDeps) {
	d := &deps
	group.Use(v1AuthMiddleware(authDeps), ResolvePrincipal())
	group.Use(controlOperatorMiddleware(d.AuditSink))
	group.Use(ControlRateLimitMiddleware(d.rateLimiter()))
	d.registerControlUserRoutes(group)
	d.registerControlWorkspaceRoutes(group)
	d.registerControlBillingRoutes(group)
	d.registerControlCreditRoutes(group)
	d.registerControlPaymentRoutes(group)
	d.registerControlTaskRoutes(group)
	d.registerControlOAuthRoutes(group)
	d.registerControlPlatformSettingsRoutes(group)
	d.registerControlProxiedRoutes(group)
	// No catch-all here: gin cannot combine a /* wildcard with the
	// :param siblings above. Unknown control paths fall through to the
	// engine NoRoute (404 for /api/*), consistent with /api/v1.
}

func (d *ContractControlDeps) registerControlUserRoutes(control *gin.RouterGroup) {
	userTarget := controlTargetParam("user_id", "user")
	control.GET("/users", d.guard(ControlCapabilityUsers,
		controlRoute{Operation: "users.list", Route: "GET /users"}, d.listControlUsers))
	control.GET("/users/:user_id", d.guard(ControlCapabilityUsers,
		controlRoute{Operation: "users.get", Route: "GET /users/{user_id}", Target: userTarget}, d.getControlUser))
	control.PUT("/users/:user_id/role", d.guard(ControlCapabilityUsers,
		controlRoute{Operation: "users.set_role", Route: "PUT /users/{user_id}/role", Mutation: true, Target: userTarget}, d.updateControlUserRole))
	control.POST("/users/:user_id/ban", d.guard(ControlCapabilityUsers,
		controlRoute{Operation: "users.ban", Route: "POST /users/{user_id}/ban", Mutation: true, Target: userTarget}, d.banControlUser))
	control.POST("/users/:user_id/unban", d.guard(ControlCapabilityUsers,
		controlRoute{Operation: "users.unban", Route: "POST /users/{user_id}/unban", Mutation: true, Target: userTarget}, d.unbanControlUser))
	control.DELETE("/users/:user_id/sessions", d.guard(ControlCapabilityUsers,
		controlRoute{Operation: "users.revoke_sessions", Route: "DELETE /users/{user_id}/sessions", Mutation: true, Target: userTarget}, d.revokeControlUserSessions))
	control.DELETE("/users/:user_id", d.guard(ControlCapabilityUsers,
		controlRoute{Operation: "users.delete", Route: "DELETE /users/{user_id}", Mutation: true, Target: userTarget}, d.deleteControlUser))
	control.POST("/users/:user_id/impersonation", d.guard(ControlCapabilityUsers,
		controlRoute{Operation: "users.impersonate", Route: "POST /users/{user_id}/impersonation", Mutation: true, Target: userTarget}, d.impersonateControlUser))
}

func (d *ContractControlDeps) registerControlWorkspaceRoutes(control *gin.RouterGroup) {
	workspaceTarget := controlTargetParam("workspace_id", "workspace")
	memberTarget := controlTargetParam("member_id", "user")
	control.GET("/workspaces/:workspace_id", d.guard(ControlCapabilityWorkspaces,
		controlRoute{Operation: "workspaces.get", Route: "GET /workspaces/{workspace_id}", Target: workspaceTarget}, d.getControlWorkspace))
	control.GET("/workspaces/:workspace_id/members", d.guard(ControlCapabilityWorkspaces,
		controlRoute{Operation: "workspaces.list_members", Route: "GET /workspaces/{workspace_id}/members", Target: workspaceTarget}, d.listControlWorkspaceMembers))
	control.POST("/workspaces/:workspace_id/members", d.guard(ControlCapabilityWorkspaces,
		controlRoute{Operation: "workspaces.add_member", Route: "POST /workspaces/{workspace_id}/members", Mutation: true, Target: workspaceTarget}, d.addControlWorkspaceMember))
	control.PATCH("/workspaces/:workspace_id/members/:member_id", d.guard(ControlCapabilityWorkspaces,
		controlRoute{Operation: "workspaces.update_member", Route: "PATCH /workspaces/{workspace_id}/members/{member_id}", Mutation: true, Target: memberTarget}, d.updateControlWorkspaceMember))
	control.DELETE("/workspaces/:workspace_id/members/:member_id", d.guard(ControlCapabilityWorkspaces,
		controlRoute{Operation: "workspaces.remove_member", Route: "DELETE /workspaces/{workspace_id}/members/{member_id}", Mutation: true, Target: memberTarget}, d.removeControlWorkspaceMember))
}

func (d *ContractControlDeps) registerControlBillingRoutes(control *gin.RouterGroup) {
	skuTarget := controlTargetParam("sku", "billing_sku")
	control.GET("/billing/accounts", d.guard(ControlCapabilityBilling,
		controlRoute{Operation: "billing.get_account", Route: "GET /billing/accounts"}, d.listControlBillingAccounts))
	control.GET("/billing/catalog", d.guard(ControlCapabilityBilling,
		controlRoute{Operation: "billing.list_catalog", Route: "GET /billing/catalog"}, d.listControlBillingCatalog))
	control.POST("/billing/catalog/:sku/prices", d.guard(ControlCapabilityBilling,
		controlRoute{Operation: "billing.publish_prices", Route: "POST /billing/catalog/{sku}/prices", Mutation: true, Target: skuTarget}, d.publishControlCatalogPrices))
	control.PUT("/billing/catalog/:sku/availability", d.guard(ControlCapabilityBilling,
		controlRoute{Operation: "billing.set_availability", Route: "PUT /billing/catalog/{sku}/availability", Mutation: true, Target: skuTarget}, d.setControlCatalogAvailability))
	control.DELETE("/billing/catalog/:sku", d.guard(ControlCapabilityBilling,
		controlRoute{Operation: "billing.delete_item", Route: "DELETE /billing/catalog/{sku}", Mutation: true, Target: skuTarget}, d.deleteControlCatalogItem))
}

func (d *ContractControlDeps) registerControlCreditRoutes(control *gin.RouterGroup) {
	walletTarget := controlTargetParam("user_id", "user")
	control.POST("/credits/grants", d.guard(ControlCapabilityCredits,
		controlRoute{Operation: "credits.grant", Route: "POST /credits/grants", Mutation: true, Target: controlCreditBodyUserTarget}, d.grantControlCredits))
	control.POST("/credits/deductions", d.guard(ControlCapabilityCredits,
		controlRoute{Operation: "credits.deduct", Route: "POST /credits/deductions", Mutation: true, Target: controlCreditBodyUserTarget}, d.deductControlCredits))
	control.POST("/credits/refunds", d.guard(ControlCapabilityCredits,
		controlRoute{Operation: "credits.refund", Route: "POST /credits/refunds", Mutation: true, Target: controlCreditBodyUserTarget}, d.refundControlCredits))
	control.POST("/credits/corrections", d.guard(ControlCapabilityCredits,
		controlRoute{Operation: "credits.correct", Route: "POST /credits/corrections", Mutation: true, Target: controlCreditBodyUserTarget}, d.correctControlCredits))
	control.GET("/credits/wallets/:user_id", d.guard(ControlCapabilityCredits,
		controlRoute{Operation: "credits.get_wallet", Route: "GET /credits/wallets/{user_id}", Target: walletTarget}, d.getControlCreditWallet))
	control.GET("/credits/wallets/:user_id/ledger", d.guard(ControlCapabilityCredits,
		controlRoute{Operation: "credits.list_ledger", Route: "GET /credits/wallets/{user_id}/ledger", Target: walletTarget}, d.listControlCreditLedger))
	control.GET("/credits/health", d.guard(ControlCapabilityCredits,
		controlRoute{Operation: "credits.get_health", Route: "GET /credits/health"}, d.getControlCreditHealth))
	control.POST("/credits/reconciliation/runs", d.guard(ControlCapabilityCredits,
		controlRoute{Operation: "credits.start_reconciliation", Route: "POST /credits/reconciliation/runs", Mutation: true}, d.startControlReconciliation))
	control.GET("/credits/reconciliation/runs", d.guard(ControlCapabilityCredits,
		controlRoute{Operation: "credits.list_reconciliation_runs", Route: "GET /credits/reconciliation/runs"}, d.listControlReconciliationRuns))
	control.GET("/credits/reconciliation/runs/:run_id", d.guard(ControlCapabilityCredits,
		controlRoute{Operation: "credits.get_reconciliation_run", Route: "GET /credits/reconciliation/runs/{run_id}", Target: controlTargetParam("run_id", "reconciliation_run")}, d.getControlReconciliationRun))
	control.GET("/credits/reconciliation/findings", d.guard(ControlCapabilityCredits,
		controlRoute{Operation: "credits.list_reconciliation_findings", Route: "GET /credits/reconciliation/findings"}, d.listControlReconciliationFindings))
}

// controlCreditBodyUserTarget extracts the audit target from the credit
// mutation body (user_id is usr_-shaped; unparseable means no target,
// the handler still fails closed with 400).
func controlCreditBodyUserTarget(c *gin.Context, body []byte) (string, string) {
	if len(body) == 0 {
		return "", ""
	}
	var payload map[string]any
	if err := contract.DecodeStrict(body, &payload); err != nil {
		return "", ""
	}
	raw, _ := payload["user_id"].(string)
	if raw == "" {
		return "", ""
	}
	return "user", raw
}

func (d *ContractControlDeps) registerControlPaymentRoutes(control *gin.RouterGroup) {
	orderTarget := controlTargetParam("order_number", "payment_order")
	control.GET("/payments/orders", d.guard(ControlCapabilityPayments,
		controlRoute{Operation: "payments.list_orders", Route: "GET /payments/orders"}, d.listControlPaymentOrders))
	control.GET("/payments/orders/:order_number", d.guard(ControlCapabilityPayments,
		controlRoute{Operation: "payments.get_order", Route: "GET /payments/orders/{order_number}", Target: orderTarget}, d.getControlPaymentOrder))
	control.POST("/payments/orders/:order_number/refunds", d.guard(ControlCapabilityPayments,
		controlRoute{Operation: "payments.create_refund", Route: "POST /payments/orders/{order_number}/refunds", Mutation: true, Target: orderTarget}, d.createControlPaymentRefund))
	control.GET("/payments/stripe-config", d.guard(ControlCapabilityPayments,
		controlRoute{Operation: "payments.get_stripe_config", Route: "GET /payments/stripe-config"}, d.getControlStripeConfig))
	control.PUT("/payments/stripe-config", d.guard(ControlCapabilityPayments,
		controlRoute{Operation: "payments.update_stripe_config", Route: "PUT /payments/stripe-config", Mutation: true}, d.updateControlStripeConfig))
}

func (d *ContractControlDeps) registerControlTaskRoutes(control *gin.RouterGroup) {
	taskTarget := controlTargetParam("task_id", "task")
	control.GET("/tasks", d.guard(ControlCapabilityTasks,
		controlRoute{Operation: "tasks.list", Route: "GET /tasks"}, d.listControlTasks))
	control.GET("/tasks/:task_id", d.guard(ControlCapabilityTasks,
		controlRoute{Operation: "tasks.get", Route: "GET /tasks/{task_id}", Target: taskTarget}, d.getControlTask))
	control.POST("/tasks/:task_id/cancel", d.guard(ControlCapabilityTasks,
		controlRoute{Operation: "tasks.cancel", Route: "POST /tasks/{task_id}/cancel", Mutation: true, Target: taskTarget}, d.cancelControlTask))
}

func (d *ContractControlDeps) registerControlOAuthRoutes(control *gin.RouterGroup) {
	providerTarget := controlTargetParam("provider", "oauth_provider")
	control.GET("/users/oauth-providers/:provider", d.guard(ControlCapabilityUsers,
		controlRoute{Operation: "users.get_oauth_provider", Route: "GET /users/oauth-providers/{provider}", Target: providerTarget}, d.getControlOAuthProvider))
	control.PUT("/users/oauth-providers/:provider", d.guard(ControlCapabilityUsers,
		controlRoute{Operation: "users.update_oauth_provider", Route: "PUT /users/oauth-providers/{provider}", Mutation: true, Target: providerTarget}, d.updateControlOAuthProvider))
}

func (d *ContractControlDeps) registerControlPlatformSettingsRoutes(control *gin.RouterGroup) {
	control.GET("/platform/settings/registration", d.guard(ControlCapabilityPlatform,
		controlRoute{Operation: "platform.get_registration", Route: "GET /platform/settings/registration"}, d.getControlRegistrationSetting))
	control.PUT("/platform/settings/registration", d.guard(ControlCapabilityPlatform,
		controlRoute{Operation: "platform.update_registration", Route: "PUT /platform/settings/registration", Mutation: true}, d.updateControlRegistrationSetting))
	control.GET("/platform/settings/rate-limit", d.guard(ControlCapabilityPlatform,
		controlRoute{Operation: "platform.get_rate_limit", Route: "GET /platform/settings/rate-limit"}, d.getControlRateLimitSetting))
	control.PUT("/platform/settings/rate-limit", d.guard(ControlCapabilityPlatform,
		controlRoute{Operation: "platform.update_rate_limit", Route: "PUT /platform/settings/rate-limit", Mutation: true}, d.updateControlRateLimitSetting))
	control.GET("/platform/settings/branding", d.guard(ControlCapabilityPlatform,
		controlRoute{Operation: "platform.get_branding", Route: "GET /platform/settings/branding"}, d.getControlBrandingSetting))
	control.PUT("/platform/settings/branding", d.guard(ControlCapabilityPlatform,
		controlRoute{Operation: "platform.update_branding", Route: "PUT /platform/settings/branding", Mutation: true}, d.updateControlBrandingSetting))
	// Branding logo upload is a POST read-guard (the guard cannot buffer
	// multipart bodies): reason + Idempotency-Key are enforced in-handler
	// with content-hash replay, and every call is audited.
	control.POST("/platform/settings/branding/logo", d.guard(ControlCapabilityPlatform,
		controlRoute{Operation: "platform.upload_branding_logo", Route: "POST /platform/settings/branding/logo"}, d.uploadControlBrandingLogo))
	control.GET("/platform/settings/media-upload", d.guard(ControlCapabilityPlatform,
		controlRoute{Operation: "platform.get_media_upload", Route: "GET /platform/settings/media-upload"}, d.getControlMediaUploadSetting))
	control.PUT("/platform/settings/media-upload", d.guard(ControlCapabilityPlatform,
		controlRoute{Operation: "platform.update_media_upload", Route: "PUT /platform/settings/media-upload", Mutation: true}, d.updateControlMediaUploadSetting))
	control.GET("/platform/settings/outbound-policy", d.guard(ControlCapabilityPlatform,
		controlRoute{Operation: "platform.get_outbound_policy", Route: "GET /platform/settings/outbound-policy"}, d.getControlOutboundPolicy))
	control.PUT("/platform/settings/outbound-policy", d.guard(ControlCapabilityPlatform,
		controlRoute{Operation: "platform.update_outbound_policy", Route: "PUT /platform/settings/outbound-policy", Mutation: true}, d.updateControlOutboundPolicy))
	control.GET("/platform/settings/mail", d.guard(ControlCapabilityPlatform,
		controlRoute{Operation: "platform.get_mail_setting", Route: "GET /platform/settings/mail"}, d.getControlMailSetting))
	control.PUT("/platform/settings/mail", d.guard(ControlCapabilityPlatform,
		controlRoute{Operation: "platform.update_mail_setting", Route: "PUT /platform/settings/mail", Mutation: true}, d.updateControlMailSetting))
	control.DELETE("/platform/settings/mail", d.guard(ControlCapabilityPlatform,
		controlRoute{Operation: "platform.delete_mail_setting", Route: "DELETE /platform/settings/mail", Mutation: true}, d.deleteControlMailSetting))
	control.POST("/platform/settings/mail/test", d.guard(ControlCapabilityPlatform,
		controlRoute{Operation: "platform.test_mail_setting", Route: "POST /platform/settings/mail/test", Mutation: true}, d.testControlMailSetting))
}

// registerControlProxiedRoutes mounts the platform-config families that
// reuse the legacy admin handler methods under the control guard
// (capability control:platform, mandatory audit, header idempotency,
// operator reason). Every proxied response is translated into the unified
// contract envelope at the boundary (parent #809); the binary brand-logo
// and the SSE monitor stream stay raw protocol exceptions. Every other
// control guarantee holds uniformly.
func (d *ContractControlDeps) registerControlProxiedRoutes(control *gin.RouterGroup) {
	gatewayAvailable := d != nil && d.AIGateway != nil
	storageAvailable := d != nil && d.Storage != nil
	searchAvailable := d != nil && d.SearchProviders != nil
	promptsAvailable := d != nil && d.FeaturePrompts != nil
	monitorAvailable := d != nil && d.Monitor != nil

	{
		h := d.AIGateway
		control.GET("/platform/ai-gateway", d.controlProxy(
			controlRoute{Operation: "platform.get_ai_gateway", Route: "GET /platform/ai-gateway"}, ControlCapabilityPlatform, gatewayAvailable, h.Get))
		control.PUT("/platform/ai-gateway", d.controlProxy(
			controlRoute{Operation: "platform.update_ai_gateway", Route: "PUT /platform/ai-gateway", Mutation: true}, ControlCapabilityPlatform, gatewayAvailable, h.Update))
		control.GET("/platform/ai-gateway/models", d.controlProxy(
			controlRoute{Operation: "platform.list_ai_gateway_models", Route: "GET /platform/ai-gateway/models"}, ControlCapabilityPlatform, gatewayAvailable, h.ListMappings))
		control.PUT("/platform/ai-gateway/models/:feature", d.controlProxy(
			controlRoute{Operation: "platform.save_ai_gateway_model", Route: "PUT /platform/ai-gateway/models/{feature}", Mutation: true, Target: controlTargetParam("feature", "ai_gateway_model")}, ControlCapabilityPlatform, gatewayAvailable, h.SaveMapping))
		control.GET("/platform/ai-gateway/catalog", d.controlProxy(
			controlRoute{Operation: "platform.list_ai_gateway_catalog", Route: "GET /platform/ai-gateway/catalog"}, ControlCapabilityPlatform, gatewayAvailable, h.ListCatalog))
		control.PUT("/platform/ai-gateway/catalog", d.controlProxy(
			controlRoute{Operation: "platform.save_ai_gateway_catalog", Route: "PUT /platform/ai-gateway/catalog", Mutation: true}, ControlCapabilityPlatform, gatewayAvailable, h.SaveCatalog))
		control.GET("/platform/ai-gateway/providers", d.controlProxy(
			controlRoute{Operation: "platform.list_ai_gateway_providers", Route: "GET /platform/ai-gateway/providers"}, ControlCapabilityPlatform, gatewayAvailable, h.ListProviders))
		control.GET("/platform/ai-gateway/detect-models", d.controlProxy(
			controlRoute{Operation: "platform.detect_ai_gateway_models", Route: "GET /platform/ai-gateway/detect-models"}, ControlCapabilityPlatform, gatewayAvailable, h.DetectModels))
		control.GET("/platform/ai-gateway/models-reference", d.controlProxy(
			controlRoute{Operation: "platform.get_ai_gateway_model_reference", Route: "GET /platform/ai-gateway/models-reference"}, ControlCapabilityPlatform, gatewayAvailable, h.GetModelReference))
		control.GET("/platform/ai-gateway/brand-logo", d.controlProxyRaw(
			controlRoute{Operation: "platform.get_ai_gateway_brand_logo", Route: "GET /platform/ai-gateway/brand-logo"}, ControlCapabilityPlatform, gatewayAvailable, h.GetBrandLogo))
		control.GET("/platform/ai-gateway/models-observed", d.controlProxy(
			controlRoute{Operation: "platform.list_ai_gateway_models_observed", Route: "GET /platform/ai-gateway/models-observed"}, ControlCapabilityPlatform, gatewayAvailable, h.ListModelsObserved))
		// Route preview is a pure routing dry-run (no configuration read
		// or write), so it registers as a POST read-guard without reason
		// or idempotency requirements.
		control.POST("/platform/ai-gateway/route-preview", d.controlProxy(
			controlRoute{Operation: "platform.preview_ai_gateway_route", Route: "POST /platform/ai-gateway/route-preview"}, ControlCapabilityPlatform, gatewayAvailable, h.PreviewGatewayRoute))
		control.POST("/platform/ai-gateway/test-connection", d.controlProxy(
			controlRoute{Operation: "platform.test_ai_gateway_connection", Route: "POST /platform/ai-gateway/test-connection", Mutation: true}, ControlCapabilityPlatform, gatewayAvailable, h.TestConnection))
	}
	{
		h := d.Storage
		control.GET("/platform/storage/providers", d.controlProxy(
			controlRoute{Operation: "platform.list_storage_providers", Route: "GET /platform/storage/providers"}, ControlCapabilityPlatform, storageAvailable, h.ListProviders))
		control.GET("/platform/storage/profiles", d.controlProxy(
			controlRoute{Operation: "platform.list_storage_profiles", Route: "GET /platform/storage/profiles"}, ControlCapabilityPlatform, storageAvailable, h.ListProfiles))
		control.POST("/platform/storage/profiles", d.controlProxy(
			controlRoute{Operation: "platform.create_storage_profile", Route: "POST /platform/storage/profiles", Mutation: true}, ControlCapabilityPlatform, storageAvailable, h.CreateProfile))
		control.PUT("/platform/storage/profiles/:id", d.controlProxy(
			controlRoute{Operation: "platform.update_storage_profile", Route: "PUT /platform/storage/profiles/{id}", Mutation: true, Target: controlTargetParam("id", "storage_profile")}, ControlCapabilityPlatform, storageAvailable, h.UpdateProfile))
		control.DELETE("/platform/storage/profiles/:id", d.controlProxy(
			controlRoute{Operation: "platform.delete_storage_profile", Route: "DELETE /platform/storage/profiles/{id}", Mutation: true, Target: controlTargetParam("id", "storage_profile")}, ControlCapabilityPlatform, storageAvailable, h.DeleteProfile))
		control.POST("/platform/storage/profiles/:id/apply", d.controlProxy(
			controlRoute{Operation: "platform.apply_storage_profile", Route: "POST /platform/storage/profiles/{id}/apply", Mutation: true, Target: controlTargetParam("id", "storage_profile")}, ControlCapabilityPlatform, storageAvailable, h.ApplyProfile))
		control.POST("/platform/storage/profiles/:id/test", d.controlProxy(
			controlRoute{Operation: "platform.test_storage_profile", Route: "POST /platform/storage/profiles/{id}/test", Mutation: true, Target: controlTargetParam("id", "storage_profile")}, ControlCapabilityPlatform, storageAvailable, h.TestProfile))
		control.GET("/platform/storage/profiles/:id/bucket-info", d.controlProxy(
			controlRoute{Operation: "platform.get_storage_bucket_info", Route: "GET /platform/storage/profiles/{id}/bucket-info", Target: controlTargetParam("id", "storage_profile")}, ControlCapabilityPlatform, storageAvailable, h.GetBucketInfo))
	}
	{
		h := d.SearchProviders
		control.GET("/platform/search/providers", d.controlProxy(
			controlRoute{Operation: "platform.list_search_providers", Route: "GET /platform/search/providers"}, ControlCapabilityPlatform, searchAvailable, h.List))
		control.PUT("/platform/search/providers/:provider", d.controlProxy(
			controlRoute{Operation: "platform.update_search_provider", Route: "PUT /platform/search/providers/{provider}", Mutation: true, Target: controlTargetParam("provider", "search_provider")}, ControlCapabilityPlatform, searchAvailable, h.Update))
		control.POST("/platform/search/providers/:provider/test", d.controlProxy(
			controlRoute{Operation: "platform.test_search_provider", Route: "POST /platform/search/providers/{provider}/test", Mutation: true, Target: controlTargetParam("provider", "search_provider")}, ControlCapabilityPlatform, searchAvailable, h.Test))
	}
	{
		h := d.FeaturePrompts
		control.GET("/platform/feature-prompts", d.controlProxy(
			controlRoute{Operation: "platform.list_feature_prompts", Route: "GET /platform/feature-prompts"}, ControlCapabilityPlatform, promptsAvailable, h.List))
		control.GET("/platform/feature-prompts/:slug", d.controlProxy(
			controlRoute{Operation: "platform.get_feature_prompt", Route: "GET /platform/feature-prompts/{slug}", Target: controlTargetParam("slug", "feature_prompt")}, ControlCapabilityPlatform, promptsAvailable, h.Get))
		control.PUT("/platform/feature-prompts/:slug", d.controlProxy(
			controlRoute{Operation: "platform.save_feature_prompt", Route: "PUT /platform/feature-prompts/{slug}", Mutation: true, Target: controlTargetParam("slug", "feature_prompt")}, ControlCapabilityPlatform, promptsAvailable, h.Save))
	}
	{
		h := d.Monitor
		control.GET("/platform/monitor", d.controlProxy(
			controlRoute{Operation: "platform.get_monitor", Route: "GET /platform/monitor"}, ControlCapabilityPlatform, monitorAvailable, h.GetMonitor))
		// Global monitor stream (SSE, snapshot every 10s): read-guard so
		// the long-lived stream is never body-buffered; the audit record
		// emits on disconnect with the terminal status. The stream is a
		// protocol exception and bypasses envelope translation.
		control.GET("/platform/monitor/stream", d.controlProxyRaw(
			controlRoute{Operation: "platform.stream_monitor", Route: "GET /platform/monitor/stream"}, ControlCapabilityPlatform, monitorAvailable, h.StreamMonitor))
		control.GET("/platform/statistics", d.controlProxy(
			controlRoute{Operation: "platform.get_statistics", Route: "GET /platform/statistics"}, ControlCapabilityPlatform, monitorAvailable, h.GetStatistics))
		control.POST("/platform/statistics/backfill", d.controlProxy(
			controlRoute{Operation: "platform.backfill_statistics", Route: "POST /platform/statistics/backfill", Mutation: true}, ControlCapabilityPlatform, monitorAvailable, h.BackfillStatistics))
	}
	{
		h := d.BrandLogos
		brandLogosAvailable := d != nil && d.BrandLogos != nil
		control.GET("/platform/brand-logo-overrides", d.controlProxy(
			controlRoute{Operation: "platform.list_brand_logo_overrides", Route: "GET /platform/brand-logo-overrides"}, ControlCapabilityPlatform, brandLogosAvailable, h.List))
		control.PUT("/platform/brand-logo-overrides/:scope/:target_key", d.controlProxy(
			controlRoute{Operation: "platform.upsert_brand_logo_override", Route: "PUT /platform/brand-logo-overrides/{scope}/{target_key}", Mutation: true, Target: controlBrandLogoOverrideTarget}, ControlCapabilityPlatform, brandLogosAvailable, h.Upsert))
		control.DELETE("/platform/brand-logo-overrides/:scope/:target_key", d.controlProxy(
			controlRoute{Operation: "platform.delete_brand_logo_override", Route: "DELETE /platform/brand-logo-overrides/{scope}/{target_key}", Mutation: true, Target: controlBrandLogoOverrideTarget}, ControlCapabilityPlatform, brandLogosAvailable, h.Delete))
	}
	{
		cmdAvailable := d != nil && d.RewardCommands != nil
		readAvailable := d != nil && d.RewardReads != nil
		cmd := d.RewardCommands
		read := d.RewardReads
		// Reward search is a POST-body read (fuzzy name / full-code HMAC
		// lookup cannot ride a query string), so it registers as a POST
		// read-guard without reason or idempotency requirements.
		control.POST("/rewards/codes/search", d.controlProxy(
			controlRoute{Operation: "rewards.search_codes", Route: "POST /rewards/codes/search"}, ControlCapabilityCredits, readAvailable, read.SearchRewardCodes))
		control.POST("/rewards/codes", d.controlProxy(
			controlRoute{Operation: "rewards.create_code", Route: "POST /rewards/codes", Mutation: true}, ControlCapabilityCredits, cmdAvailable, cmd.CreateRewardCode))
		control.GET("/rewards/codes/:id/redemptions", d.controlProxy(
			controlRoute{Operation: "rewards.list_redemptions", Route: "GET /rewards/codes/{id}/redemptions", Target: controlTargetParam("id", "reward_code")}, ControlCapabilityCredits, readAvailable, read.ListAdminRedemptions))
		control.GET("/rewards/codes/:id/redemptions/:redemption_id", d.controlProxy(
			controlRoute{Operation: "rewards.get_redemption", Route: "GET /rewards/codes/{id}/redemptions/{redemption_id}", Target: controlTargetParam("redemption_id", "reward_redemption")}, ControlCapabilityCredits, readAvailable, read.GetAdminRedemption))
		control.PATCH("/rewards/codes/:id", d.controlProxy(
			controlRoute{Operation: "rewards.update_code", Route: "PATCH /rewards/codes/{id}", Mutation: true, Target: controlTargetParam("id", "reward_code")}, ControlCapabilityCredits, cmdAvailable, cmd.UpdateRewardCode))
		control.PATCH("/rewards/codes/:id/enabled", d.controlProxy(
			controlRoute{Operation: "rewards.set_code_enabled", Route: "PATCH /rewards/codes/{id}/enabled", Mutation: true, Target: controlTargetParam("id", "reward_code")}, ControlCapabilityCredits, cmdAvailable, cmd.SetRewardCodeEnabled))
		control.DELETE("/rewards/codes/:id", d.controlProxy(
			controlRoute{Operation: "rewards.delete_code", Route: "DELETE /rewards/codes/{id}", Mutation: true, Target: controlTargetParam("id", "reward_code")}, ControlCapabilityCredits, cmdAvailable, cmd.DeleteRewardCode))
	}
}

// controlBrandLogoOverrideTarget audits brand-logo overrides by their
// scope-qualified key (scope + target_key are operator-facing ids).
func controlBrandLogoOverrideTarget(c *gin.Context, _ []byte) (string, string) {
	scope := strings.TrimSpace(c.Param("scope"))
	target := strings.TrimSpace(c.Param("target_key"))
	if scope == "" || target == "" {
		return "", ""
	}
	return "brand_logo_override", scope + ":" + target
}

// RegisterContractControlRoutesOnEngine mounts the Control Plane group on
// the engine, replacing the #800 stub group. Legacy routes are untouched.
func RegisterContractControlRoutesOnEngine(r *gin.Engine, authDeps AuthDeps, deps ContractControlDeps) {
	control := r.Group(ContractControlGroupPath)
	control.Use(ContractRequestIDMiddleware())
	RegisterContractControlRoutes(control, authDeps, deps)
}
