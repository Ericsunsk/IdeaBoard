package httpapi

// Control operation policy parity gate (issue #909).
//
// Compares the runtime guard policy (capability + Mutation/Detached in
// contract_control_routes.go) against the spec policy (x-capability,
// x-audit reason/outcome, Idempotency-Key/OperatorReason params in
// openapi.control.yaml). Mail test is the fixed reference: it stays a
// detached mutation with header-only Idempotency-Key and audited
// success outcome on both sides (routine probe, so the operator reason
// is optional on both sides), and the runtime must stay detached (no
// guard-owned DB transaction wraps the external Auth HTTP probe).
// Detached mutations (mail test/update/delete, Stripe
// config update) run the M1 durable winner election through short
// independent transactions, so cross-process replay holds without a
// long guard transaction. create_refund deliberately stays atomic:
// its reservation/settlement already commit in their own transactions
// (beginCommittedRefundTx) and the blocking guard claim is the
// stronger cash-path guarantee (see the route comment).
//
// Explicit exceptions (spec mutation-like, runtime POST read-guard) stay
// pinned here so future fixes must consciously update this list:
//   - testControlAIGatewayConnection: gateway reachability probe.
//   - testControlStorageProfile: storage connectivity probe.
//   - testControlSearchProvider: search provider probe.
//   - uploadControlBrandingLogo: multipart upload; the guard cannot
//     buffer bodies, so reason/key/replay are enforced in-handler.
// Mail test is deliberately NOT an exception after #909.

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

type specPolicy struct {
	operationId string
	method      string
	path        string
	capability  string
	operator    bool
	reason      bool
	outcome     string
	idempotency bool
	reasonParam bool
	hasConflict bool
}

type runtimePolicy struct {
	operation      string
	method         string
	path           string
	capability     string
	mutation       bool
	reasonOptional bool
	detached       bool
	viaProxy       bool
	viaGuard       bool
}

// policyParityExceptions documents spec-mutation vs runtime-read drift
// that #909 intentionally leaves in place. Keys are spec operationIds.
var policyParityExceptions = map[string]string{
	"testControlAIGatewayConnection": "gateway reachability probe stays a POST read-guard (no guard tx across probe)",
	"testControlStorageProfile":      "storage connectivity probe stays a POST read-guard (no guard tx across probe)",
	"testControlSearchProvider":      "search provider probe stays a POST read-guard (no guard tx across probe)",
	"uploadControlBrandingLogo":      "multipart upload stays a POST read-guard with in-handler reason/key/replay (guard cannot buffer multipart)",
}

func loadSpecPolicies(t *testing.T) map[string]specPolicy {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "openapi.control.yaml"))
	if err != nil {
		t.Fatalf("read openapi.control.yaml: %v", err)
	}
	lines := strings.Split(string(raw), "\n")
	out := map[string]specPolicy{}
	var current specPolicy
	var haveOperation bool
	var inAudit, inParameters bool
	flush := func() {
		if !haveOperation {
			return
		}
		if current.operationId == "" {
			t.Fatalf("spec operation %s %s has no operationId", current.method, current.path)
		}
		if _, exists := out[current.operationId]; exists {
			t.Fatalf("duplicate spec operationId %s", current.operationId)
		}
		out[current.operationId] = current
		haveOperation = false
	}
	for _, line := range lines {
		if len(line) > 0 && line[0] != ' ' && strings.HasSuffix(strings.TrimSpace(line), ":") {
			flush()
			continue
		}
		if strings.HasPrefix(line, "  /") && strings.HasSuffix(strings.TrimSpace(line), ":") {
			flush()
			current = specPolicy{path: strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(line), ":"))}
			continue
		}
		method := ""
		for _, candidate := range []string{"get", "post", "put", "patch", "delete"} {
			if strings.HasPrefix(line, "    "+candidate+":") {
				method = strings.ToUpper(candidate)
				break
			}
		}
		if method != "" {
			flush()
			path := current.path
			current = specPolicy{path: path}
			current.method = method
			haveOperation = true
			inAudit, inParameters = false, false
			continue
		}
		if !haveOperation {
			continue
		}
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(line, "      ") && len(line) > 6 && line[6] != ' ' {
			inAudit = trimmed == "x-audit:"
			inParameters = trimmed == "parameters:"
		}
		if strings.HasPrefix(trimmed, "operationId:") {
			current.operationId = strings.TrimSpace(strings.TrimPrefix(trimmed, "operationId:"))
		}
		if strings.HasPrefix(trimmed, "x-capability:") {
			current.capability = strings.TrimSpace(strings.TrimPrefix(trimmed, "x-capability:"))
		}
		if inAudit && trimmed == "reason: true" {
			current.reason = true
		}
		if inAudit && trimmed == "operator: true" {
			current.operator = true
		}
		if inAudit && strings.HasPrefix(trimmed, "outcome:") {
			current.outcome = strings.TrimSpace(strings.TrimPrefix(trimmed, "outcome:"))
		}
		if inParameters && strings.Contains(trimmed, "#/components/parameters/IdempotencyKey") {
			current.idempotency = true
		}
		if inParameters && strings.Contains(trimmed, "#/components/parameters/OperatorReason") {
			current.reasonParam = true
		}
		if strings.HasPrefix(trimmed, "'409':") {
			current.hasConflict = true
		}
	}
	flush()
	if len(out) == 0 {
		t.Fatal("no operationIds parsed from openapi.control.yaml")
	}
	return out
}

var runtimeRouteRe = regexp.MustCompile(`control\.(GET|POST|PUT|PATCH|DELETE)\("([^"]+)",`)

func normalizeControlPath(path string) string {
	return regexp.MustCompile(`:[^/]+`).ReplaceAllStringFunc(path, func(segment string) string {
		return "{" + strings.TrimPrefix(segment, ":") + "}"
	})
}

func compareControlPolicyMetadata(sp specPolicy, rp runtimePolicy, exception bool) error {
	if !sp.operator {
		return fmt.Errorf("x-audit operator must be true")
	}
	if exception && rp.mutation {
		return fmt.Errorf("explicit exception must stay a runtime read-guard")
	}
	wantMutation := rp.mutation || exception
	// Routine config mutations stay mutations (key + audited success
	// outcome) with an exempt reason: only x-audit reason flips false.
	wantReason := wantMutation && !rp.reasonOptional
	if sp.reason != wantReason {
		return fmt.Errorf("x-audit reason = %v, want %v", sp.reason, wantReason)
	}
	for _, field := range []struct {
		name string
		got  bool
	}{
		{"IdempotencyKey parameter", sp.idempotency},
		{"OperatorReason parameter", sp.reasonParam},
	} {
		if field.got != wantMutation {
			return fmt.Errorf("%s = %v, want %v", field.name, field.got, wantMutation)
		}
	}
	wantOutcome := "read"
	if wantMutation {
		wantOutcome = "success"
	}
	if sp.outcome != wantOutcome {
		return fmt.Errorf("x-audit outcome = %q, want %q", sp.outcome, wantOutcome)
	}
	return nil
}

func TestControlPolicyMetadataRejectsIndividualFieldDrift(t *testing.T) {
	validMutation := specPolicy{
		operator: true, reason: true, idempotency: true, reasonParam: true, outcome: "success",
	}
	cases := []struct {
		name   string
		change func(*specPolicy)
	}{
		{"x-audit operator", func(p *specPolicy) { p.operator = false }},
		{"x-audit reason", func(p *specPolicy) { p.reason = false }},
		{"IdempotencyKey parameter", func(p *specPolicy) { p.idempotency = false }},
		{"OperatorReason parameter", func(p *specPolicy) { p.reasonParam = false }},
		{"x-audit outcome", func(p *specPolicy) { p.outcome = "" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			changed := validMutation
			tc.change(&changed)
			for _, exception := range []bool{false, true} {
				err := compareControlPolicyMetadata(changed, runtimePolicy{mutation: !exception}, exception)
				if err == nil || !strings.Contains(err.Error(), tc.name) {
					t.Fatalf("missing %s (exception=%v) must fail, got %v", tc.name, exception, err)
				}
			}
		})
	}
	if err := compareControlPolicyMetadata(validMutation, runtimePolicy{mutation: true}, false); err != nil {
		t.Fatalf("valid mutation must pass: %v", err)
	}
	optionalMutation := specPolicy{
		operator: true, reason: false, idempotency: true, reasonParam: true, outcome: "success",
	}
	if err := compareControlPolicyMetadata(optionalMutation, runtimePolicy{mutation: true, reasonOptional: true}, false); err != nil {
		t.Fatalf("valid reason-optional mutation must pass: %v", err)
	}
	if err := compareControlPolicyMetadata(optionalMutation, runtimePolicy{mutation: true}, false); err == nil ||
		!strings.Contains(err.Error(), "x-audit reason") {
		t.Fatalf("optional spec against required runtime must fail on reason, got %v", err)
	}
	if err := compareControlPolicyMetadata(validMutation, runtimePolicy{mutation: true, reasonOptional: true}, false); err == nil ||
		!strings.Contains(err.Error(), "x-audit reason") {
		t.Fatalf("required spec against optional runtime must fail on reason, got %v", err)
	}
	if err := compareControlPolicyMetadata(validMutation, runtimePolicy{}, true); err != nil {
		t.Fatalf("valid explicit exception must pass: %v", err)
	}
	if err := compareControlPolicyMetadata(specPolicy{operator: true, outcome: "read"}, runtimePolicy{}, false); err != nil {
		t.Fatalf("valid read must pass: %v", err)
	}
}

func loadRuntimePolicies(t *testing.T) map[string]runtimePolicy {
	t.Helper()
	raw, err := os.ReadFile("contract_control_routes.go")
	if err != nil {
		t.Fatalf("read contract_control_routes.go: %v", err)
	}
	src := string(raw)
	out := map[string]runtimePolicy{}
	matches := runtimeRouteRe.FindAllStringSubmatchIndex(src, -1)
	opRe := regexp.MustCompile(`Operation:\s*"([^"]+)"`)
	capRe := regexp.MustCompile(`ControlCapability(\w+)`)
	for _, m := range matches {
		method := src[m[2]:m[3]]
		path := src[m[4]:m[5]]
		blockStart := m[1]
		// The guard/proxy wrapper (capability, controlRoute literal,
		// Mutation/Detached flags) follows within ~1200 bytes.
		end := blockStart + 1200
		if end > len(src) {
			end = len(src)
		}
		window := src[blockStart:end]
		opMatch := opRe.FindStringSubmatch(window)
		if opMatch == nil {
			t.Fatalf("route %s %s has no Operation", method, path)
		}
		op := opMatch[1]
		capMatch := capRe.FindStringSubmatch(window)
		if capMatch == nil {
			t.Fatalf("route %s %s has no ControlCapability", method, path)
		}
		capShort := capMatch[1]
		// The controlRoute literal is the first controlRoute{...} after
		// the route registration. Route templates contain {param}
		// braces inside quoted strings, so the literal cannot end at
		// the first closing brace: it ends at the "}," that closes
		// the literal (", d." for guarded routes, ", Control..." for
		// proxied ones). Inner template braces are always followed by
		// "/" inside the quoted string, never by ",", so the first
		// "}," after the literal start is the literal end. Flags must
		// be read from inside it only, never from a neighbour route's
		// literal further down the window.
		routeLit := ""
		if litStart := strings.Index(window, "controlRoute{"); litStart >= 0 {
			rest := window[litStart:]
			if litEnd := strings.Index(rest, "},"); litEnd >= 0 {
				routeLit = rest[:litEnd+1]
			} else {
				t.Fatalf("route %s %s has no controlRoute literal end", method, path)
			}
		} else {
			t.Fatalf("route %s %s has no controlRoute literal", method, path)
		}
		viaProxy := strings.Contains(window[:400], "controlProxy(") || strings.Contains(window[:400], "controlProxyRaw(")
		viaGuard := strings.Contains(window[:400], ".guard(")
		p := runtimePolicy{
			operation:      op,
			method:         method,
			path:           path,
			capability:     "control:" + strings.ToLower(capShort),
			mutation:       strings.Contains(routeLit, "Mutation: true"),
			reasonOptional: strings.Contains(routeLit, "ReasonOptional: true"),
			detached:       strings.Contains(routeLit, "Detached: true"),
			viaProxy:       viaProxy,
			viaGuard:       viaGuard,
		}
		// Capability short names map: Users->users, Workspaces->workspaces,
		// Billing->billing, Credits->credits, Payments->payments,
		// Platform->platform, Tasks->tasks.
		out[op] = p
	}
	if len(out) == 0 {
		t.Fatal("no control routes parsed from contract_control_routes.go")
	}
	return out
}

func TestControlOperationPolicyParity(t *testing.T) {
	spec := loadSpecPolicies(t)
	runtime := loadRuntimePolicies(t)

	t.Run("mail test stays detached with optional reason on both sides", func(t *testing.T) {
		sp, ok := spec["testControlMailSetting"]
		if !ok {
			t.Fatal("spec missing testControlMailSetting")
		}
		if sp.capability != "control:platform" {
			t.Fatalf("spec capability = %q, want control:platform", sp.capability)
		}
		if sp.reason {
			t.Fatal("spec mail test must have x-audit reason: false (routine probe, reason optional)")
		}
		if sp.outcome != "success" {
			t.Fatalf("spec outcome = %q, want success", sp.outcome)
		}
		if !sp.idempotency || !sp.reasonParam {
			t.Fatalf("spec mail test must carry IdempotencyKey + OperatorReason params (idempotency=%v reasonParam=%v)", sp.idempotency, sp.reasonParam)
		}
		if !sp.hasConflict {
			t.Fatal("spec mail test must document 409 IdempotencyConflict")
		}
		rp, ok := runtime["platform.test_mail_setting"]
		if !ok {
			t.Fatal("runtime missing platform.test_mail_setting")
		}
		if rp.capability != "control:platform" {
			t.Fatalf("runtime capability = %q, want control:platform", rp.capability)
		}
		if !rp.mutation {
			t.Fatal("runtime mail test must be Mutation:true (key enforced by the guard, reason optional)")
		}
		if !rp.reasonOptional {
			t.Fatal("runtime mail test must be ReasonOptional:true (routine probe)")
		}
		if !rp.detached {
			t.Fatal("runtime mail test must be Detached:true (external Auth probe never runs inside the guard DB transaction)")
		}
		if !rp.viaGuard || rp.viaProxy {
			t.Fatal("runtime mail test must run through d.guard, not controlProxy")
		}
		if _, isException := policyParityExceptions["testControlMailSetting"]; isException {
			t.Fatal("mail test must NOT be a policy parity exception after #909")
		}
	})

	t.Run("external precheck mutations stay detached", func(t *testing.T) {
		// H1: mail update/delete and Stripe config update perform the
		// external precheck first, then publish locally. They must stay
		// Detached:true so no long guard transaction ever wraps the
		// Auth/Stripe call; M1 durable short-claim keeps replay
		// without resending.
		for _, op := range []string{
			"platform.update_mail_setting",
			"platform.delete_mail_setting",
			"payments.update_stripe_config",
		} {
			rp, ok := runtime[op]
			if !ok {
				t.Fatalf("runtime missing %s", op)
			}
			if !rp.mutation {
				t.Fatalf("runtime %s must be Mutation:true", op)
			}
			if !rp.detached {
				t.Fatalf("runtime %s must be Detached:true (external precheck before local publish)", op)
			}
		}
		// Cash path deliberately stays atomic: the guard transaction
		// covering the Stripe call is the blocking single-winner
		// election for cash (see the route comment); the cash facts
		// themselves commit in their own transactions.
		if rp, ok := runtime["payments.create_refund"]; !ok {
			t.Fatal("runtime missing payments.create_refund")
		} else {
			if !rp.mutation {
				t.Fatal("runtime payments.create_refund must be Mutation:true")
			}
			if rp.detached {
				t.Fatal("runtime payments.create_refund must stay atomic (blocking cash winner election)")
			}
		}
	})

	t.Run("capabilities match exactly", func(t *testing.T) {
		seenSpec := map[string]string{}
		for id, sp := range spec {
			key := sp.method + " " + sp.path
			if sp.method == "" || sp.path == "" {
				t.Fatalf("spec %s has incomplete route: method=%q path=%q", id, sp.method, sp.path)
			}
			if previous, exists := seenSpec[key]; exists {
				t.Fatalf("spec route %s is claimed by both %s and %s", key, previous, id)
			}
			seenSpec[key] = id
		}
		seenRuntime := map[string]string{}
		for operation, rp := range runtime {
			key := rp.method + " " + normalizeControlPath(rp.path)
			if previous, exists := seenRuntime[key]; exists {
				t.Fatalf("runtime route %s is claimed by both %s and %s", key, previous, operation)
			}
			seenRuntime[key] = operation
			specID, ok := seenSpec[key]
			if !ok {
				t.Fatalf("runtime route %s (%s) has no spec operation", key, operation)
			}
			sp := spec[specID]
			if rp.capability != sp.capability {
				t.Fatalf("capability drift for %s/%s: runtime=%q spec=%q", operation, specID, rp.capability, sp.capability)
			}
		}
		for key, specID := range seenSpec {
			if _, ok := seenRuntime[key]; !ok {
				t.Fatalf("spec operation %s (%s) has no runtime route", specID, key)
			}
		}
	})

	t.Run("mutation classification matches except explicit exceptions", func(t *testing.T) {
		seenSpec := map[string]string{}
		for id, sp := range spec {
			seenSpec[sp.method+" "+sp.path] = id
		}
		seenRuntime := map[string]runtimePolicy{}
		for _, rp := range runtime {
			seenRuntime[rp.method+" "+normalizeControlPath(rp.path)] = rp
		}
		for key, operationId := range seenSpec {
			rp, ok := seenRuntime[key]
			if !ok {
				t.Fatalf("spec operation %s (%s) has no runtime route", operationId, key)
			}
			_, exception := policyParityExceptions[operationId]
			if err := compareControlPolicyMetadata(spec[operationId], rp, exception); err != nil {
				t.Errorf("policy drift for %s/%s: %v", operationId, rp.operation, err)
			}
		}
	})

	t.Run("exceptions stay documented and minimal", func(t *testing.T) {
		if len(policyParityExceptions) != 4 {
			t.Fatalf("exceptions = %d, want 4 (gateway/storage/search probes + multipart upload); mail test must not be listed", len(policyParityExceptions))
		}
		for id, why := range policyParityExceptions {
			if strings.TrimSpace(why) == "" {
				t.Fatalf("exception %s needs a justification", id)
			}
			if _, ok := spec[id]; !ok {
				t.Fatalf("exception %s missing from spec", id)
			}
		}
		if _, ok := policyParityExceptions["testControlMailSetting"]; ok {
			t.Fatal("testControlMailSetting must not be an exception after #909")
		}
	})
}
