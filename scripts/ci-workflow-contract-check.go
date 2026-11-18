package main

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/goccy/go-yaml"
)

var externalActionPattern = regexp.MustCompile(`^[^@]+@[0-9a-f]{40}$`)

type workflow map[string]any

func main() {
	root, err := repositoryRoot()
	failIf(err)

	ci := loadWorkflow(root, "ci.yml")
	release := loadWorkflow(root, "docker-images.yml")

	checkCI(ci)
	checkReusableWorkflows(root)
	checkAIProxyVerification(root)
	checkMigrationGate(root)
	checkRelease(root, release)
	checkDeployOnly(root)
	checkDeploymentAction(root)
	checkActionPinning(root)
	checkGoCaching(root)
	checkLegacyWorkflows(root)

	fmt.Println("CI workflow contract tests passed")
}

func checkDeployOnly(root string) {
	workflow := loadWorkflow(root, "deploy-production.yml")
	triggers := object(workflow, "on")
	dispatch := object(triggers, "workflow_dispatch")
	inputs := object(dispatch, "inputs")
	releaseTag := object(inputs, "release_tag")
	requireEqual(releaseTag["required"], true, "Deploy-only workflow must require release_tag")

	jobs := object(workflow, "jobs")
	deploy := object(jobs, "deploy-production")
	requireEqual(deploy["environment"], "production", "Deploy-only workflow must target production environment")
	requireEqual(object(workflow, "concurrency")["group"], "production-deployment", "Deploy-only workflow must share the Production deployment lock")
	requireEqual(object(workflow, "concurrency")["cancel-in-progress"], false, "Deploy-only workflow must not cancel a deployment")
	checkDeploymentCaller(deploy, "${{ inputs.release_tag }}", "")

	allText := scalarText(workflow)
	reject(strings.Contains(allText, "docker/build-push-action"), "Deploy-only workflow must not build images")
	reject(strings.Contains(allText, "cosign sign"), "Deploy-only workflow must not sign images")
}

func checkDeploymentCaller(deploy map[string]any, releaseTag, sourceCommit string) {
	requireEqual(deploy["environment"], "production", "Deployment callers must retain the Production Environment gate")
	requireEqual(object(deploy, "permissions")["contents"], "read", "Deployment callers need only read access")
	step := requireStep(deploy, "Deploy immutable Production release")
	requireEqual(step["uses"], "./.github/actions/deploy-production", "Deployment callers must use the shared action")
	requireEqual(step["if"], "always()", "Deployment action must report prerequisite failures")
	reject(step["continue-on-error"] == true, "Production deployment failures must fail the workflow")
	inputs := object(step, "with")
	requireEqual(inputs["prerequisite-status"], "${{ job.status }}", "Deployment must be blocked after prerequisite failure or cancellation")
	requireEqual(inputs["manifest-path"], "release-assets/release-manifest.json", "Deployment must consume the downloaded immutable manifest")
	requireEqual(inputs["bundle-path"], "release-assets/deployment-bundle.tar", "Deployment must consume the complete signed deployment archive")
	requireEqual(inputs["bundle-signature-path"], "release-assets/deployment-bundle.sigstore.json", "Deployment must consume the archive signature evidence")
	requireEqual(inputs["release-tag"], releaseTag, "Deployment must bind the requested release tag")
	if sourceCommit == "" {
		rejectKey(inputs, "source-commit", "Existing releases must use the manifest's source commit")
	} else {
		requireEqual(inputs["source-commit"], sourceCommit, "New releases must bind the verified source commit")
	}
	for input, expected := range map[string]string{
		"vps-host":               "${{ vars.VPS_HOST }}",
		"vps-ssh-port":           "${{ vars.VPS_SSH_PORT }}",
		"vps-deploy-user":        "${{ vars.VPS_DEPLOY_USER }}",
		"vps-deploy-private-key": "${{ secrets.VPS_DEPLOY_PRIVATE_KEY }}",
		"vps-known-hosts":        "${{ secrets.VPS_KNOWN_HOSTS }}",
		"infisical-token":        "${{ secrets.INFISICAL_TOKEN }}",
	} {
		requireEqual(inputs[input], expected, "Deployment must consume Production connection input "+input)
	}
}

func checkDeploymentAction(root string) {
	data, err := os.ReadFile(filepath.Join(root, ".github", "actions", "deploy-production", "action.yml"))
	failIf(err)
	var action workflow
	failIf(yaml.Unmarshal(data, &action))
	runs := object(action, "runs")
	requireEqual(runs["using"], "composite", "Production deployment must have one composite action")
	validate := requireStep(runs, "Validate immutable release manifest")
	requireEqual(object(validate, "env")["PREREQUISITE_STATUS"], "${{ inputs.prerequisite-status }}", "Deployment must consume the caller's prerequisite status")
	requireContains(stringValue(validate["run"]), `[[ "${PREREQUISITE_STATUS}" == success ]]`, "Failed or cancelled prerequisites must never deploy")
	requireEqual(object(validate, "env")["SOURCE_COMMIT"], "${{ inputs.source-commit }}", "Manifest validation must consume the preflight commit through env")
	for _, required := range []string{`args=(validate --manifest "${RELEASE_MANIFEST}"`, `--repository "${GITHUB_REPOSITORY}" --tag "${RELEASE_TAG}"`, `[[ -z "${SOURCE_COMMIT}" ]] || args+=(--commit "${SOURCE_COMMIT}")`, `python3 scripts/release-artifacts.py "${args[@]}" > /dev/null`} {
		requireContains(stringValue(validate["run"]), required, "Deployment canonical manifest validation is missing "+required)
	}
	reject(strings.Contains(stringValue(validate["run"]), "schema_version =="), "Deployment must validate the sole canonical manifest schema unconditionally")
	verify := requireStep(runs, "Verify signed deployment bundle")
	for _, required := range []string{"scripts/deployment-bundle.py verify", `--archive "${RELEASE_BUNDLE}"`, `--signature "${RELEASE_BUNDLE_SIGNATURE}"`, `--repository "${GITHUB_REPOSITORY}"`, `--tag "${RELEASE_TAG}"`, `--commit "${commit}"`, `cmp "${RELEASE_MANIFEST}" "${verified}/release-manifest.json"`} {
		requireContains(stringValue(verify["run"]), required, "Production bundle verification is missing "+required)
	}
	rejectKey(verify, "if", "Production archive verification cannot be skipped")
	rejectKey(verify, "continue-on-error", "Production archive verification failures must block deployment")
	configure := requireStep(runs, "Configure restricted SSH transport")
	requireContains(stringValue(configure["run"]), "ssh-keygen -F", "Deployment must verify the pinned endpoint before SSH")
	stream := requireStep(runs, "Stream signed bundle to Production deploy entrypoint")
	requireEqual(stream["run"], `./scripts/run-production-deploy.sh "${RELEASE_MANIFEST}" "${RELEASE_BUNDLE}" "${RELEASE_BUNDLE_SIGNATURE}"`, "Shared deployment action must use the sole transport client")
	requireEqual(object(stream, "env")["INFISICAL_TOKEN"], "${{ inputs.infisical-token }}", "Deployment must stream the scoped Infisical token through process env")
	for _, name := range []string{"Remove SSH credentials", "Write Production deployment summary"} {
		requireEqual(requireStep(runs, name)["if"], "always()", name+" must run after deployment failure")
	}
}

func repositoryRoot() (string, error) {
	workingDirectory, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return filepath.Clean(filepath.Join(workingDirectory, "../..")), nil
}

func loadWorkflow(root, name string) workflow {
	path := filepath.Join(root, ".github", "workflows", name)
	data, err := os.ReadFile(path)
	failIf(err)
	var parsed workflow
	failIf(yaml.Unmarshal(data, &parsed))
	return parsed
}

func checkCI(ci workflow) {
	triggers := object(ci, "on")
	requireKey(triggers, "pull_request", "CI must run on pull_request")
	requireKey(triggers, "workflow_dispatch", "CI must support workflow_dispatch")
	callInputs := object(object(triggers, "workflow_call"), "inputs")
	requireEqual(object(callInputs, "force_full")["default"], true, "Reusable CI must default to full verification")
	requireEqual(object(callInputs, "source_commit")["required"], true, "Reusable CI must verify an explicit source commit")
	requireEqual(object(callInputs, "preview_images")["default"], false, "Release verification must not duplicate preview image builds")
	requireStringIn(object(triggers, "push"), "branches", "main", "CI push trigger must include main")
	rejectKey(triggers, "merge_group", "CI must not expose merge_group")

	jobs := object(ci, "jobs")
	changes := object(jobs, "changes")
	baseline := requireStep(changes, "Select verified ancestor baseline")
	requireEqual(baseline["run"], `./scripts/ci-baseline.sh "${RUNNER_TEMP}/changed-paths.txt" | tee -a "${GITHUB_OUTPUT}"`, "Incremental CI must use the trusted ancestor resolver")
	requireEqual(object(baseline, "env")["SOURCE_COMMIT"], "${{ inputs.source_commit }}", "Baseline must validate the requested release source")
	requireContains(stringValue(object(baseline, "env")["FORCE_FULL"]), "inputs.force_full", "CI must preserve explicit full verification")
	for _, lane := range []string{"api", "web"} {
		inputs := object(object(jobs, lane), "with")
		for _, name := range []string{"base_sha", "head_sha"} {
			requireEqual(inputs[name], "${{ needs.changes.outputs."+name+" }}", lane+" must consume the verified commit range")
		}
		requireEqual(inputs["force_full"], "${{ needs.changes.outputs.force_full == 'true' }}", lane+" must consume full fallback")
	}
	for _, lane := range []string{"auth", "realtime", "ai-proxy", "ops", "migration-integration"} {
		requireEqual(object(object(jobs, lane), "with")["source_commit"], "${{ needs.changes.outputs.head_sha }}", lane+" must verify the same selected source")
	}
	for input, output := range map[string]string{"run_tests": "api_tests", "run_sqlc": "api_sqlc", "run_migrations": "api_migrations", "build_api": "api_build"} {
		requireEqual(object(object(jobs, "api"), "with")[input], "${{ needs.changes.outputs."+output+" == 'true' }}", "API must consume its precise check requirement")
	}
	requireEqual(object(jobs, "web")["uses"], "./.github/workflows/web.yml", "Web job must call web.yml")
	requireEqual(object(jobs, "api")["uses"], "./.github/workflows/api.yml", "API job must call api.yml")
	requireEqual(object(jobs, "auth")["uses"], "./.github/workflows/auth.yml", "Auth job must call auth.yml")
	requireEqual(object(jobs, "realtime")["uses"], "./.github/workflows/realtime.yml", "Realtime job must call realtime.yml")
	aiProxy := object(jobs, "ai-proxy")
	requireEqual(aiProxy["uses"], "./.github/workflows/ai-proxy.yml", "AI Proxy job must call ai-proxy.yml")
	requireEqual(aiProxy["if"], "needs.changes.outputs.ai_proxy == 'true'", "AI Proxy must use its selected scope")
	requireStringIn(aiProxy, "needs", "changes", "AI Proxy must wait for classification")
	requireEqual(object(aiProxy, "with")["audit_dependencies"], "${{ needs.changes.outputs.ai_proxy_audit == 'true' }}", "AI Proxy audit must follow dependency inputs")
	for _, output := range []string{"ai_proxy", "ai_proxy_audit"} {
		requireEqual(object(changes, "outputs")[output], "${{ steps.classify.outputs."+output+" }}", "AI Proxy outputs must come from classification")
	}
	requireEqual(object(jobs, "migration-integration")["uses"], "./.github/workflows/migration-integration.yml", "Migration job must call migration-integration.yml")
	rejectKey(jobs, "security", "Scheduled security checks must remain outside CI")

	gate := object(jobs, "ci-gate")
	requireEqual(gate["name"], "CI Gate", "Required check must remain CI Gate")
	requireEqual(gate["if"], "always()", "CI Gate must run after skipped or failed lanes")
	rejectKey(gate, "continue-on-error", "CI Gate failures must block CI")
	requireStringIn(gate, "needs", "changes", "CI Gate must depend on change classification")
	requireStringIn(gate, "needs", "web", "CI Gate must depend on aggregate Web verification")
	requireStringIn(gate, "needs", "api", "CI Gate must depend on API verification")
	requireStringIn(gate, "needs", "realtime", "CI Gate must depend on realtime")
	requireStringIn(gate, "needs", "ai-proxy", "CI Gate must depend on AI Proxy")
	requireStringIn(gate, "needs", "migration-integration", "CI Gate must depend on migration integration")
	requireStringIn(gate, "needs", "media-contract", "CI Gate must depend on Media contract")
	mediaContract := object(jobs, "media-contract")
	requireEqual(mediaContract["name"], "Media Contract", "Media contract job must remain separately visible")
	requireStringIn(mediaContract, "needs", "changes", "Media contract must wait for change classification")
	mediaBoundary := requireStep(mediaContract, "Run media package boundary gate")
	requireEqual(mediaBoundary["run"], "./scripts/ci-media-package-boundary.sh", "Media contract must enforce the package boundary")
	legacyScan := requireStep(mediaContract, "Run legacy media cutover zero-result scan")
	requireEqual(legacyScan["run"], "./scripts/ci-legacy-media-zero-scan.sh", "Media contract must enforce the clean-break scan")
	gateCheckout := requireStep(gate, "Checkout")
	requireEqual(gateCheckout["uses"], "actions/checkout@df4cb1c069e1874edd31b4311f1884172cec0e10", "CI Gate must checkout the lane policy module")
	gateStep := requireStep(gate, "Summarize and verify required lanes")
	requireEqual(gateStep["run"], "./scripts/ci-gate.sh", "CI Gate must use the lane policy module")
	requireEqual(gateStep["if"], "always()", "CI Gate policy must run after failed or skipped lanes")
	rejectKey(gateStep, "continue-on-error", "CI Gate policy failures must block CI")
	gateEnv := object(gateStep, "env")
	requireEqual(gateEnv["BROWSER_SMOKE_REQUIRED"], "${{ needs.changes.outputs.browser_smoke }}", "Browser smoke must use the explicit runtime-impact flag")
	requireEqual(gateEnv["WEB_REQUIRED"], "${{ needs.changes.outputs.web }}", "CI Gate must receive the Web required flag")
	requireEqual(gateEnv["WEB_RESULT"], "${{ needs.web.result }}", "CI Gate must receive the aggregate Web result")
	requireEqual(gateEnv["API_REQUIRED"], "${{ needs.changes.outputs.api }}", "CI Gate must receive the API required flag")
	requireEqual(gateEnv["API_RESULT"], "${{ needs.api.result }}", "CI Gate must receive the aggregate API result")
	requireEqual(gateEnv["MEDIA_CONTRACT_REQUIRED"], "true", "Media contract must always be a required CI lane")
	requireEqual(gateEnv["MEDIA_CONTRACT_RESULT"], "${{ needs.media-contract.result }}", "CI Gate must receive the Media contract result")
	requireEqual(gateEnv["REALTIME_REQUIRED"], "${{ needs.changes.outputs.realtime }}", "CI Gate must receive the Realtime required flag")
	requireEqual(gateEnv["REALTIME_RESULT"], "${{ needs.realtime.result }}", "CI Gate must receive the Realtime result")
	requireEqual(gateEnv["AI_PROXY_REQUIRED"], "${{ needs.changes.outputs.ai_proxy }}", "CI Gate must receive the AI Proxy required flag")
	requireEqual(gateEnv["AI_PROXY_RESULT"], "${{ needs.ai-proxy.result }}", "CI Gate must receive the AI Proxy result")
}

func checkReusableWorkflows(root string) {
	for _, name := range []string{"web.yml", "api.yml", "auth.yml", "realtime.yml", "ai-proxy.yml", "migration-integration.yml", "ops.yml"} {
		parsed := loadWorkflow(root, name)
		triggers := object(parsed, "on")
		requireKey(triggers, "workflow_call", name+" must expose workflow_call")
		checkoutRef := "${{ inputs.source_commit || github.sha }}"
		if name == "api.yml" || name == "web.yml" {
			checkoutRef = "${{ inputs.head_sha || github.sha }}"
		}
		for _, rawJob := range object(parsed, "jobs") {
			job, ok := rawJob.(map[string]any)
			if !ok {
				continue
			}
			for _, step := range steps(job) {
				if strings.HasPrefix(stringValue(step["uses"]), "actions/checkout@") {
					requireEqual(object(step, "with")["ref"], checkoutRef, name+" must checkout the exact verified source")
				}
			}
		}
		for _, forbidden := range []string{"pull_request", "push", "merge_group"} {
			rejectKey(triggers, forbidden, name+" must not expose event trigger "+forbidden)
		}
	}

	api := loadWorkflow(root, "api.yml")
	rejectKey(object(api, "jobs"), "schema", "Schema semantics must run once in the migration gate")
	rejectKey(object(object(object(api, "on"), "workflow_call"), "inputs"), "run_schema", "Schema verification must not require an optional input")
	requireEqual(object(api, "env")["DOCKER_BUILD_RECORD_UPLOAD"], "false", "API workflow must not upload Docker build records")
	apiInputs := object(object(object(api, "on"), "workflow_call"), "inputs")
	for _, name := range []string{"force_full", "run_tests", "run_sqlc", "run_migrations", "build_api"} {
		requireEqual(object(apiInputs, name)["default"], true, "Standalone API checks must safely default to full coverage")
	}
	for _, name := range []string{"sqlc-diff", "sqlc-vet"} {
		requireEqual(object(object(api, "jobs"), name)["if"], "inputs.run_sqlc", "SQL checks must follow SQL inputs")
	}
	checkAPITestMatrix(api)
	migrationGate := object(object(api, "jobs"), "migration-gate")
	requireEqual(migrationGate["if"], "inputs.run_migrations", "Migration verification must run when its inputs change")
	migrationEnv := object(migrationGate, "env")
	requireEqual(migrationEnv["ATLAS_BASE_SHA"], "${{ inputs.base_sha }}", "Migration review must use the cumulative verified baseline")
	requireEqual(migrationEnv["ATLAS_HEAD_SHA"], "${{ inputs.head_sha || github.sha }}", "Migration review must bind the selected source")
	migrationRange := requireStep(migrationGate, "Verify trusted migration review range")
	for _, required := range []string{`[[ "${ATLAS_BASE_SHA}" =~ ^[0-9a-f]{40}$ ]]`, `git rev-parse HEAD`, `git merge-base --is-ancestor "${ATLAS_BASE_SHA}" "${ATLAS_HEAD_SHA}"`, "exit 1"} {
		requireContains(stringValue(migrationRange["run"]), required, "Migration review must reject missing or invalid comparison history")
	}
	rejectKey(migrationRange, "continue-on-error", "Invalid migration history must fail verification")
	rejectKey(migrationRange, "if", "Migration history validation must not be skipped")
	openapiRange := requireStep(object(object(api, "jobs"), "openapi-breaking"), "Check for breaking OpenAPI changes")
	requireEqual(object(openapiRange, "env")["BASE_SHA"], "${{ inputs.base_sha }}", "OpenAPI review must use the cumulative verified baseline")
	requireEqual(object(openapiRange, "env")["HEAD_SHA"], "${{ inputs.head_sha || github.sha }}", "OpenAPI review must bind the selected source")
	for _, required := range []string{`[[ "${BASE_SHA}" =~ ^[0-9a-f]{40}$ ]]`, `git rev-parse HEAD`, `git merge-base --is-ancestor "${BASE_SHA}" "${HEAD_SHA}"`, "exit 1", `./scripts/openapi-breaking-check.sh "${BASE_SHA}"`} {
		requireContains(stringValue(openapiRange["run"]), required, "OpenAPI review must reject missing or invalid comparison history")
	}
	rejectKey(openapiRange, "continue-on-error", "Invalid OpenAPI history must fail verification")
	rejectKey(openapiRange, "if", "OpenAPI history validation must not be skipped")
	reject(strings.Contains(scalarText(api), "github.event.before"), "API review cannot narrow to the last potentially failed commit")
	installAPIAtlas := requireStep(migrationGate, "Install Atlas")
	installAPIAtlasRun := stringValue(installAPIAtlas["run"])
	requireContains(installAPIAtlasRun, "./apps/api/install-atlas.sh /tmp/atlas", "API migration gate must use the verified Atlas installer")
	semanticCheck := requireStep(migrationGate, "Compare migration replay with schema snapshot")
	requireEqual(semanticCheck["working-directory"], "apps/api", "Migration semantics must use the API project root")
	requireContains(stringValue(semanticCheck["run"]), `go run ./cmd/schemarelease --base-sha "${ATLAS_BASE_SHA}"`, "Migration gate must compare migration replay and snapshot semantics")
	requireStep(migrationGate, "Check destructive migrations and validate the chain")
	requireStep(migrationGate, "Apply API migrations up")
	requireStep(migrationGate, "Finalize migration (River schema and runtime privileges)")
	for _, step := range steps(migrationGate) {
		reject(strings.Contains(stringValue(step["run"]), "ci-parity.sh api-test"), "Migration gate must not repeat snapshot-backed API tests")
	}
	staticContracts := object(object(api, "jobs"), "static-contracts")
	sqlScope := requireStep(staticContracts, "Check workspace SQL scope predicates")
	requireEqual(stringValue(sqlScope["run"]), "bash scripts/sql-workspace-scope-check.sh", "API contracts must run the workspace scope guard")
	openapiPattern := requireStep(staticContracts, "Check OpenAPI scalar pattern consistency")
	requireEqual(stringValue(openapiPattern["run"]), "bash scripts/openapi-pattern-check.sh", "API contracts must run the OpenAPI pattern guard")
	backendCopy := requireStep(staticContracts, "Check backend copy vocabulary")
	requireEqual(stringValue(backendCopy["run"]), "bash scripts/check-backend-copy.sh", "API contracts must run the backend copy guard")

	auth := loadWorkflow(root, "auth.yml")
	requireEqual(object(auth, "env")["DOCKER_BUILD_RECORD_UPLOAD"], "false", "Auth workflow must not upload Docker build records")
	authQuality := object(object(auth, "jobs"), "quality")
	prepareGo := requireStep(authQuality, "Prepare Go contract test")
	requireEqual(prepareGo["working-directory"], "apps/api", "Auth contract preparation must use the API module")
	requireEqual(prepareGo["run"], "go test -run '^$' ./internal/identity/auth", "Auth must compile the Go contract before its runtime deadline")
	rejectKey(prepareGo, "if", "Auth contract preparation must not be skipped")
	rejectKey(prepareGo, "continue-on-error", "Auth contract preparation failures must fail verification")
	goSetupIndex, prepareIndex, authTestIndex := -1, -1, -1
	for index, step := range steps(authQuality) {
		if step["uses"] == "./.github/actions/setup-go" {
			reject(goSetupIndex != -1, "Auth must configure Go once")
			goSetupIndex = index
			requireEqual(object(step, "with")["build-cache-scope"], "auth-contract", "Auth must isolate its Go contract build cache")
			rejectKey(step, "if", "Auth Go setup must not be skipped")
			rejectKey(step, "continue-on-error", "Auth Go setup failures must fail verification")
		}
		if step["name"] == "Prepare Go contract test" {
			prepareIndex = index
		}
		if step["name"] == "Test Auth" {
			authTestIndex = index
		}
	}
	reject(goSetupIndex < 0, "Auth must use the canonical Go setup")
	reject(!(goSetupIndex < prepareIndex && prepareIndex < authTestIndex), "Auth must set up and compile Go before running contract assertions")

	realtime := loadWorkflow(root, "realtime.yml")
	requireEqual(object(realtime, "env")["DOCKER_BUILD_RECORD_UPLOAD"], "false", "Realtime workflow must not upload Docker build records")

	web := loadWorkflow(root, "web.yml")
	checkWebVerification(root, web)
	webGenerated := requireStep(object(object(web, "jobs"), "build"), "Generated API drift")
	requireEqual(webGenerated["working-directory"], "apps/web", "Web generated drift must run from the canonical Web root")
	requireEqual(webGenerated["run"], "npm run api:check-generated", "Web generated drift must use the isolated canonical API client checker")
	webSmokeContract := requireStep(object(object(web, "jobs"), "smoke"), "Anonymous smoke API contract")
	requireEqual(webSmokeContract["working-directory"], "apps/web", "Anonymous smoke API contract must run from the Web root")
	requireEqual(webSmokeContract["run"], "npx tsx --test scripts/smokeSupport.contract.spec.ts", "Web smoke must execute the standalone anonymous API contract")
	webAudit := requireStep(object(object(web, "jobs"), "build"), "Production dependency audit")
	requireEqual(webAudit["run"], "npm audit --omit=dev --audit-level=high", "Web audit must reject high production vulnerabilities")

	migration := loadWorkflow(root, "migration-integration.yml")
	migrationInputs := object(object(migration, "on"), "workflow_call")
	requireKey(migrationInputs, "inputs", "Migration integration must accept commit range inputs")
	verify := object(object(migration, "jobs"), "verify")
	verifyEnv := object(verify, "env")
	requireEqual(verifyEnv["ATLAS_BASE_SHA"], "${{ inputs.base_sha }}", "Migration base SHA must be passed through job env")
	requireEqual(verifyEnv["ATLAS_HEAD_SHA"], "${{ inputs.head_sha }}", "Migration head SHA must be passed through job env")
	requireStepRun(verify, "bash scripts/ci-migration-integration-test.sh")
	requireStepRun(verify, "bash scripts/atlas-migration-check.sh")
	requireStepRun(verify, "./scripts/ci-migration-integration.sh")
	installMigrationAtlas := requireStep(verify, "Install Atlas")
	installMigrationAtlasRun := stringValue(installMigrationAtlas["run"])
	requireContains(installMigrationAtlasRun, "./apps/api/install-atlas.sh /tmp/atlas", "Migration integration must use the verified Atlas installer")
	for _, step := range steps(verify) {
		run, _ := step["run"].(string)
		reject(strings.Contains(run, "npm run build") || strings.Contains(run, "npm run migrate:dist"), "Migration implementation must stay inside ci-migration-integration.sh")
		reject(strings.Contains(run, "inputs.base_sha") || strings.Contains(run, "inputs.head_sha"), "Migration inputs must not be interpolated into shell source")
	}

	ops := loadWorkflow(root, "ops.yml")
	ciContracts := requireStep(object(object(ops, "jobs"), "verify"), "Verify CI orchestration contracts")
	ciContractRun := stringValue(ciContracts["run"])
	requireContains(ciContractRun, "bash scripts/run-all-self-tests.sh", "Ops must run the self-test manifest")
	manifestBytes, err := os.ReadFile(filepath.Join(root, "scripts", "run-all-self-tests.sh"))
	failIf(err)
	requireContains(string(manifestBytes), "*-test.sh", "Self-test manifest must glob scripts/*-test.sh")
}

func checkAIProxyVerification(root string) {
	aiProxy := loadWorkflow(root, "ai-proxy.yml")
	requireEqual(object(aiProxy, "permissions")["contents"], "read", "AI Proxy verification needs only read access")
	quality := object(object(aiProxy, "jobs"), "quality")
	rejectKey(quality, "environment", "AI Proxy CI must not enter a deployment environment")
	for name, command := range map[string]string{
		"Build AI Proxy":                 "npm run build",
		"Test AI Proxy":                  "npm test",
		"Verify Go Worker wire contract": "npm run test:contract",
	} {
		step := requireStep(quality, name)
		requireEqual(step["run"], command, "AI Proxy verification must execute "+command)
		requireEqual(step["working-directory"], "apps/ai-proxy", "AI Proxy checks must use the Worker root")
		rejectKey(step, "if", "AI Proxy checks must not be skipped")
		rejectKey(step, "continue-on-error", "AI Proxy check failures must fail verification")
	}
	requireContains(scalarText(quality), "./.github/actions/setup-go", "AI Proxy wire contract must use the canonical Go setup")
	requireContains(scalarText(quality), "./.github/actions/setup-node", "AI Proxy must use the canonical Node setup")
	reject(strings.Contains(scalarText(aiProxy), "wrangler deploy"), "AI Proxy CI must not deploy the Worker")
}

func checkAPITestMatrix(api workflow) {
	jobs := object(api, "jobs")
	rejectKey(jobs, "standard-tests", "API test modes must share one matrix")
	plan := object(jobs, "test-plan")
	requireEqual(plan["if"], "inputs.run_tests", "API planning must follow the selected test requirement")
	planStep := requireStep(plan, "Select impacted API packages and source contracts")
	planEnv := object(planStep, "env")
	requireEqual(planEnv["BASE_SHA"], "${{ inputs.base_sha }}", "API planner must use the trusted baseline")
	requireEqual(planEnv["HEAD_SHA"], "${{ inputs.head_sha || github.sha }}", "API planner must bind the checkout")
	requireEqual(planEnv["FORCE_FULL"], "${{ inputs.force_full }}", "API planner must preserve full-release mode")
	rejectKey(planStep, "continue-on-error", "API planning failures must fail verification")
	job := object(jobs, "tests")
	requireStringIn(job, "needs", "test-plan", "API shards must use one canonical test plan")
	requireContains(stringValue(job["if"]), "inputs.run_tests", "API tests must follow the explicit test requirement")
	rejectKey(job, "continue-on-error", "API test failures must fail verification")
	requireEqual(job["timeout-minutes"], "${{ matrix.mode == 'race' && 30 || 20 }}", "API tests must preserve the standard and race deadlines")
	strategy := object(job, "strategy")
	requireEqual(strategy["fail-fast"], false, "API tests must complete all mode/shard combinations")
	requireEqual(strategy["max-parallel"], 4, "API tests must preserve four concurrent shards")
	matrix := object(strategy, "matrix")
	requireEqual(matrix["mode"], []any{"standard", "race"}, "API tests must cover standard and race modes")
	requireEqual(matrix["shard"], []any{0, 1}, "API tests must cover shards 0 and 1")
	rejectKey(matrix, "exclude", "API tests must not exclude mode/shard combinations")
	rejectKey(matrix, "include", "API tests must not add duplicate mode/shard combinations")
	postgres := object(object(job, "services"), "postgres")
	requireEqual(postgres["image"], "postgres:17-alpine", "Each API test shard must have isolated PostgreSQL 17")
	requireContains(stringValue(postgres["options"]), "--health-cmd", "API tests must wait for PostgreSQL health")
	env := object(job, "env")
	requireEqual(env["API_TEST_DATABASE_DSN"], "postgres://ideaboard:ideaboard_test@127.0.0.1:5432/ideaboard_test?sslmode=disable", "API tests must use their PostgreSQL service")
	requireEqual(env["API_TEST_DATABASE_REQUIRED"], "1", "API tests must require PostgreSQL integration")
	requireEqual(env["API_TEST_NATS_URL"], "nats://ideaboard:ideaboard_test@127.0.0.1:4222", "API tests must require authenticated NATS integration")
	nats := requireStep(job, "Start NATS with JetStream")
	rejectKey(nats, "if", "API tests must always start NATS")
	rejectKey(nats, "continue-on-error", "NATS readiness failures must fail API tests")
	natsRun := stringValue(nats["run"])
	for _, required := range []string{"nats:2.11-alpine", "-js", "--user ideaboard --pass ideaboard_test", "/healthz", `[[ "${nats_ready}" == 1 ]]`} {
		requireContains(natsRun, required, "NATS setup contract is missing "+required)
	}
	requireEqual(requireStep(job, "Tear down NATS")["if"], "always()", "API tests must clean up NATS after failure")
	step := requireStep(job, "Test API shard")
	requireEqual(step["run"], "./scripts/ci-parity.sh api-test", "API tests must use the parity runner")
	rejectKey(step, "if", "API tests must execute every shard")
	rejectKey(step, "continue-on-error", "API tests must propagate shard failures")
	testEnv := object(step, "env")
	requireEqual(testEnv["API_TEST_PLAN"], "${{ runner.temp }}/api-test-plan/api-test-plan.json", "API shards must consume the single verified plan")
	requireEqual(testEnv["TEST_MODE"], "${{ matrix.mode }}", "API tests must select their matrix mode")
	requireEqual(testEnv["TEST_SHARD"], "${{ matrix.shard }}", "API tests must select their matrix shard")
	requireEqual(testEnv["TEST_SHARD_COUNT"], "2", "API tests must partition each mode into two shards")
	setupCount := 0
	for _, step := range steps(job) {
		if step["uses"] == "./.github/actions/setup-go" {
			setupCount++
			requireEqual(object(step, "with")["build-cache-scope"], "api-${{ matrix.mode }}-${{ matrix.shard }}", "API tests must isolate compiled caches per mode and shard")
		}
	}
	requireEqual(setupCount, 1, "API tests must use the canonical Go setup")
	quality := object(jobs, "quality")
	rejectKey(quality, "needs", "Quality must run independently of API test shards")
	requireStepRun(quality, "bash scripts/ci-parity-test.sh")
	requireStep(quality, "Test nested test infrastructure modules")
	requireStep(quality, "Go vulnerability check")
	requireStepRun(quality, "go build ./...")
	for _, step := range steps(quality) {
		reject(step["run"] == "./scripts/ci-parity.sh api-test", "Quality must not repeat the standard test suite")
	}
	for _, dependency := range []string{"quality", "tests"} {
		requireStringIn(object(jobs, "image"), "needs", dependency, "API image must wait for "+dependency)
	}
}

func checkMigrationGate(root string) {
	installer, err := os.ReadFile(filepath.Join(root, "apps/api/install-atlas.sh"))
	failIf(err)
	for _, required := range []string{"ARG ATLAS_VERSION=", "atlas-checksums", "sha256sum", `[[ "$actual" == "$expected" ]]`} {
		requireContains(string(installer), required, "Atlas installer must verify the canonical version and checksum")
	}
	dockerfile, err := os.ReadFile(filepath.Join(root, "apps/api/Dockerfile"))
	failIf(err)
	requireContains(string(dockerfile), `./install-atlas.sh /out/atlas "${TARGETARCH}"`, "Migration image must use the verified Atlas installer")
	// Atlas lint-equivalent gate: Community `migrate validate` plus the local
	// static checks in atlas-migration-check.sh (Atlas Pro `migrate lint` is
	// intentionally not a CI dependency). The checker owns the destructive
	// pattern with the `-- destructive: approved` + ATLAS_ALLOW_DESTRUCTIVE
	// escape hatch, the no-bypass risky patterns, the --dev-url Fresh replay,
	// and linear-version checks. History policy is delegated to the Go owner.
	checkerBytes, err := os.ReadFile(filepath.Join(root, "scripts", "atlas-migration-check.sh"))
	failIf(err)
	checkerText := string(checkerBytes)
	for _, required := range []string{
		"migrate validate",
		"--dev-url",
		"-- destructive: approved",
		"ATLAS_ALLOW_DESTRUCTIVE",
		"column type change is blocked",
		"SET NOT NULL is blocked",
		"NOT NULL without DEFAULT is blocked",
		"go run ./cmd/migrationhistory",
		"history --list-changed",
		"check_migration_conflicts() {\n  history\n",
		"duplicate migration version",
		"does not sort after base max",
		"atlas.sum is missing entry",
	} {
		requireContains(checkerText, required, "Atlas migration checker contract is missing "+required)
	}
	historyCommandBytes, err := os.ReadFile(filepath.Join(root, "apps", "api", "cmd", "migrationhistory", "main.go"))
	failIf(err)
	for _, required := range []string{"migrationhistory.Inspect(", "history.Validate()"} {
		requireContains(string(historyCommandBytes), required, "Migration history command must delegate to the Go policy: "+required)
	}

	// Workflows install the canonical Atlas pin once; the integration runner
	// consumes that binary for both apply passes without another download.
	integrationBytes, err := os.ReadFile(filepath.Join(root, "scripts", "ci-migration-integration.sh"))
	failIf(err)
	integrationText := string(integrationBytes)
	requireContains(integrationText, `atlas_binary="${ATLAS_BINARY:-atlas}"`, "Migration integration must consume the caller's Atlas binary")
	requireContains(integrationText, `command -v "${atlas_binary}"`, "Migration integration must fail closed without Atlas")
	reject(strings.Contains(integrationText, "release.ariga.io"), "Migration integration must not download Atlas again")

	// Classifier must route migration SQL and atlas.sum to the migration lanes.
	classifierBytes, err := os.ReadFile(filepath.Join(root, "scripts", "ci-change-classifier.sh"))
	failIf(err)
	requireContains(string(classifierBytes), "apps/api/internal/db/migrations/", "Classifier must route migration files")
	requireContains(string(classifierBytes), "atlas.sum", "Classifier must route atlas.sum")
	requireContains(string(classifierBytes), "migration_integration=true", "Classifier must set the migration integration lane")
}

func checkWebVerification(root string, web workflow) {
	ci := loadWorkflow(root, "ci.yml")
	caller := object(object(ci, "jobs"), "web")
	requireEqual(object(caller, "permissions")["pull-requests"], "write", "Web caller must authorize bundle comment permissions")
	jobs := object(web, "jobs")
	planStep := requireStep(object(jobs, "unit-plan"), "Plan unit tests")
	requireEqual(planStep["run"], "node scripts/ciUnitPlan.mjs", "Web must use the native Vitest plan boundary")
	planEnv := object(planStep, "env")
	requireEqual(planEnv["CI_BASE_SHA"], "${{ inputs.base_sha }}", "Web planner must consume the trusted baseline")
	requireEqual(planEnv["CI_FORCE_FULL"], "${{ inputs.force_full }}", "Web planner must preserve full-release mode")
	rejectKey(planStep, "continue-on-error", "Web planning failures must fail verification")
	rejectKey(planStep, "if", "Web planning must not be skipped")
	unit := object(jobs, "unit")
	requireStringIn(unit, "needs", "unit-plan", "Web unit shards must depend on the canonical plan")
	requireEqual(requireStep(unit, "Unit tests (shard)")["run"], "node scripts/ciUnitPlan.mjs run", "Full and related Web tests must share one runner")

	comment := object(jobs, "bundle-comment")
	requireContains(stringValue(comment["if"]), "github.event.pull_request.head.repo.full_name == github.repository", "Bundle comments must skip read-only fork PRs")
	for _, name := range []string{"unit-plan", "unit", "browser", "typecheck-app", "typecheck-node", "build"} {
		requireKey(jobs, name, "Missing required Web verification job: "+name)
		job := object(jobs, name)
		rejectKey(job, "if", "Required Web verification jobs must not be skipped")
		rejectKey(job, "continue-on-error", "Required Web verification jobs must propagate failures")
	}
	for name, command := range map[string]string{
		"typecheck-app":  "npm run typecheck:app",
		"typecheck-node": "npm run typecheck:node",
	} {
		requireStepRun(object(jobs, name), command)
	}
	requireStepRun(object(jobs, "typecheck-node"), "npm run test:typecheck")
	build := requireStep(object(jobs, "build"), "Build")
	requireEqual(build["run"], "npx vite build && npm run bundle:size:check", "Web CI build must bundle and enforce its budget without repeating typechecks")
	for _, name := range []string{"typecheck-app", "typecheck-node", "build"} {
		for _, step := range steps(object(jobs, name)) {
			if step["name"] != "Build" && !strings.HasPrefix(stringValue(step["run"]), "npm run typecheck:") && step["run"] != "npm run test:typecheck" {
				continue
			}
			rejectKey(step, "if", "Web typecheck and build steps must not be skipped")
			rejectKey(step, "continue-on-error", "Web typecheck and build steps must propagate failures")
		}
	}
	for _, name := range []string{"unit", "browser"} {
		job := object(jobs, name)
		strategy := object(job, "strategy")
		requireEqual(strategy["fail-fast"], false, "Web shards must all complete")
		testName := "Unit tests (shard)"
		if name == "unit" {
			requireEqual(strategy["matrix"], "${{ fromJSON(needs.unit-plan.outputs.matrix) }}", "Web unit shards must consume the canonical matrix")
		} else {
			testName = "Browser tests (shard)"
			requireEqual(object(strategy, "matrix")["shard"], []any{1, 2}, "Web browser must run both shards")
			rejectKey(object(strategy, "matrix"), "exclude", "Web browser must not omit shards")
			rejectKey(object(strategy, "matrix"), "include", "Web browser must not duplicate shards")
			requireEqual(requireStep(job, testName)["run"], `npm run test:browser -- --shard="${{ matrix.shard }}/2" --reporter=blob --reporter=default "--outputFile.blob=./.vitest-reports/blob-shard-${{ matrix.shard }}.json"`, "Web browser must execute each shard and propagate failures")
		}
		test := requireStep(job, testName)
		rejectKey(test, "if", "Web test shards must not be skipped")
		rejectKey(test, "continue-on-error", "Web test shards must propagate failures")
		upload := requireStep(job, "Upload shard blob report")
		requireEqual(upload["if"], "always()", "Web shard reports must survive failed tests")
		requireEqual(upload["uses"], "actions/upload-artifact@ea165f8d65b6e75b540449e92b4886f43607fa02", "Web shard reports must use the pinned artifact uploader")
		rejectKey(upload, "continue-on-error", "Missing Web shard reports must fail verification")
		uploadInputs := object(upload, "with")
		requireEqual(uploadInputs["include-hidden-files"], true, name+" reports must include the hidden report directory")
		requireEqual(uploadInputs["if-no-files-found"], "error", "Missing Web shard reports must fail verification")
		requireEqual(uploadInputs["path"], "apps/web/.vitest-reports", "Web shard reports must retain the blob directory")
		reportName := "vitest-blob-unit-${{ matrix.shard }}-of-${{ matrix.shard_count }}"
		if name == "browser" {
			reportName = "vitest-blob-browser-${{ matrix.shard }}-of-2"
		}
		requireEqual(uploadInputs["name"], reportName, "Web shard report artifacts must have unique names")
	}
}

func checkRelease(root string, release workflow) {
	requireEqual(object(release, "env")["DOCKER_BUILD_RECORD_UPLOAD"], "false", "Release workflow must not upload Docker build records")
	jobs := object(release, "jobs")
	preflight := object(jobs, "preflight")
	requireEqual(object(preflight, "permissions")["actions"], "read", "Release preflight must read trusted CI run provenance")
	checkout := requireStep(preflight, "Checkout requested ref")
	requireEqual(object(checkout, "with")["persist-credentials"], false, "Release checkout must not persist credentials")
	validate := requireStep(preflight, "Verify source commit and CI Gate")
	requireEqual(object(validate, "env")["GH_TOKEN"], "${{ github.token }}", "Release fetch must use the scoped GitHub token")
	requireEqual(validate["run"], `./scripts/release-eligibility-check.sh >> "${GITHUB_OUTPUT}"`, "Release must verify exact-source canonical CI provenance")
	requireStringIn(object(jobs, "publish-images"), "needs", "preflight", "Image publication must depend on preflight")
	rejectKey(jobs, "verify-source", "Release must reuse same-commit CI instead of rerunning source suites")
	rejectKey(preflight, "continue-on-error", "Trusted CI verification must block publication on failure")
	rejectKey(preflight, "if", "Trusted CI provenance cannot be conditionally skipped")
	rejectKey(object(jobs, "publish-images"), "if", "Publication must retain default success-only dependency gating")

	build := requireStep(object(jobs, "publish-images"), "Build and push image")
	requireEqual(build["uses"], "docker/build-push-action@10e90e3645eae34f1e60eeb005ba3a3d33f178e8", "Image build action must remain pinned")
	buildInputs := object(build, "with")
	requireEqual(buildInputs["context"], "${{ matrix.context }}", "Image build context must match the component plan")
	requireEqual(buildInputs["file"], "${{ matrix.file }}", "Image Dockerfile must match the component plan")
	requireEqual(buildInputs["target"], "${{ matrix.target }}", "Image target must match the component plan")
	requireEqual(buildInputs["platforms"], "linux/arm64", "Image platform must match component provenance")
	requireEqual(buildInputs["provenance"], "mode=max", "Images must include max provenance")
	requireEqual(buildInputs["sbom"], true, "Images must include SBOM attestations")
	requireEqual(buildInputs["tags"], "${{ steps.image.outputs.immutable_tag }}", "Image build must consume the prepared immutable tag")
	prepareImage := requireStep(object(jobs, "publish-images"), "Prepare image name")
	prepareImageEnv := object(prepareImage, "env")
	requireEqual(prepareImageEnv["IMAGE_NAME"], "${{ matrix.name }}", "Image name must be passed through step env")
	requireEqual(prepareImageEnv["SOURCE_COMMIT"], "${{ needs.preflight.outputs.source_commit }}", "Image commit must be passed through step env")
	prepareImageRun := stringValue(prepareImage["run"])
	requireContains(prepareImageRun, "immutable_tag=ghcr.io/${repository}/ideaboard-${IMAGE_NAME}:sha-${SOURCE_COMMIT}", "Images must use immutable commit tags")
	plan := requireStep(preflight, "Plan component images")
	requireContains(stringValue(plan["run"]), "./scripts/prepare-release-plan.sh release-plan.json", "Release must authenticate prior component provenance before planning")
	requireEqual(object(plan, "env")["RELEASE_REF"], "${{ github.ref }}", "Component signer identity must bind the actual workflow ref")
	publish := object(jobs, "publish-images")
	requireEqual(object(publish, "strategy")["matrix"], "${{ fromJSON(needs.preflight.outputs.matrix) }}", "Release must consume the verified component plan")
	for _, name := range []string{"Build and push image", "Sign image digest"} {
		requireEqual(requireStep(publish, name)["if"], "matrix.action == 'build'", "Only rebuilt components may be built and signed")
	}
	reusedSignature := requireStep(publish, "Verify reused component signature")
	requireEqual(reusedSignature["if"], "matrix.action == 'reuse'", "Reused components require signature verification")
	for _, required := range []string{`--certificate-identity "${CERTIFICATE_IDENTITY}"`, `--certificate-oidc-issuer "${CERTIFICATE_ISSUER}"`, `"${IMAGE}"`} {
		requireContains(stringValue(reusedSignature["run"]), required, "Reused signature must bind the exact signed digest and identity")
	}
	for _, name := range []string{"Update Grype vulnerability database", "Scan linux/arm64 image"} {
		step := requireStep(publish, name)
		rejectKey(step, "if", "Every built or reused image must pass the current vulnerability scan")
		rejectKey(step, "continue-on-error", "Image security failures must block release")
	}
	rejectKey(reusedSignature, "continue-on-error", "Image security failures must block release")
	cache := requireStep(publish, "Cache Grype vulnerability database")
	reject(fmt.Sprint(object(cache, "with")["lookup-only"]) == "true", "Grype cache must restore database bytes")
	update := requireStep(publish, "Update Grype vulnerability database")
	requireEqual(update["run"], "grype db update", "Every digest must refresh the vulnerability database even after a cache hit")
	scan := requireStep(publish, "Scan linux/arm64 image")
	requireEqual(scan["run"], `grype "registry:${IMAGE}" --platform linux/arm64 --config .grype.yaml --fail-on high --output table`, "Every digest must pass the High/Critical scan")
	requireEqual(object(cache, "with")["path"], "${{ runner.temp }}/grype-db", "Grype cache and scans must share the database directory")
	for _, step := range []map[string]any{update, scan} {
		requireEqual(object(step, "env")["GRYPE_DB_CACHE_DIR"], "${{ runner.temp }}/grype-db", "Grype cache and scans must share the database directory")
	}
	updateIndex, scanIndex, signIndex := -1, -1, -1
	for index, step := range steps(publish) {
		switch step["name"] {
		case "Update Grype vulnerability database":
			updateIndex = index
		case "Scan linux/arm64 image":
			scanIndex = index
		case "Sign image digest":
			signIndex = index
		}
	}
	reject(!(updateIndex < scanIndex && scanIndex < signIndex), "Image signing must follow a fresh database update and successful scan")
	requireEqual(object(scan, "env")["IMAGE"], "${{ steps.resolved.outputs.ref }}", "Scanning must consume the resolved immutable digest")
	aggregate := object(jobs, "aggregate-release-manifest")
	requireEqual(object(requireStep(aggregate, "Checkout manifest implementation"), "with")["fetch-depth"], uint64(0), "Manifest validation needs source tags and ancestry")
	requireContains(stringValue(requireStep(aggregate, "Generate release manifest")["run"]), "scripts/release-artifacts.py manifest", "Complete manifest must use the provenance owner")
	// Source provenance is validated in CI; the runtime host verifies only signed artifacts.
	requireEqual(object(aggregate, "permissions")["id-token"], "write", "Deployment bundle signing must use scoped GitHub OIDC")
	bundleBuild := requireStep(aggregate, "Verify component signatures and build deployment bundle")
	bundleSign := requireStep(aggregate, "Sign deployment bundle")
	for _, step := range []map[string]any{bundleBuild, bundleSign} {
		requireEqual(step["if"], "steps.production-release.outputs.is-production-release == 'true'", "Only strict Production tags may produce deployable bundles")
		rejectKey(step, "continue-on-error", "Bundle provenance or signing failures must block publication")
	}
	for _, required := range []string{"cosign verify --certificate-identity", "--certificate-oidc-issuer", "scripts/deployment-bundle.py build", "--manifest release-assets/release-manifest.json", "--output release-assets/deployment-bundle.tar"} {
		requireContains(stringValue(bundleBuild["run"]), required, "Deployment bundle must bind verified component provenance: "+required)
	}
	requireContains(stringValue(bundleSign["run"]), "cosign sign-blob --yes --bundle release-assets/deployment-bundle.sigstore.json release-assets/deployment-bundle.tar", "Complete deployment archive must have a keyless blob signature")
	assets := object(jobs, "publish-release-assets")
	files := stringValue(object(requireStep(assets, "Upload image metadata to GitHub Release"), "with")["files"])
	for _, file := range []string{"release-assets/release-manifest.json", "release-assets/deployment-bundle.tar", "release-assets/deployment-bundle.sigstore.json"} {
		requireContains(files, file, "Release assets must include "+file)
	}

	deploy := object(jobs, "deploy-production")
	requireEqual(object(deploy, "concurrency")["cancel-in-progress"], false, "Production deployments must not cancel in progress")
	requireEqual(object(deploy, "concurrency")["group"], "production-deployment", "New releases must share the Production deployment lock")
	checkDeploymentCaller(deploy, "${{ github.ref_name }}", "${{ needs.preflight.outputs.source_commit }}")
	clientBytes, err := os.ReadFile(filepath.Join(root, "scripts", "run-production-deploy.sh"))
	failIf(err)
	clientText := string(clientBytes)
	for _, required := range []string{
		"StrictHostKeyChecking=yes",
		"UserKnownHostsFile=\"${ssh_known_hosts_file}\"",
		"PasswordAuthentication=no",
		"IDEABOARD_DEPLOY_V2",
		"production-deploy-error.raw",
	} {
		requireContains(clientText, required, "Production transport contract is missing "+required)
	}

	allText := scalarText(release)
	reject(strings.Contains(allText, "Publish verified image aliases"), "Release must not publish mutable aliases")
	reject(strings.Contains(allText, "uses: ./.github/workflows/backend.yml"), "Release must not call the retired backend workflow")
}

func checkActionPinning(root string) {
	paths := []string{filepath.Join(root, ".github", "workflows"), filepath.Join(root, ".github", "actions")}
	for _, base := range paths {
		failIf(filepath.WalkDir(base, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() || (filepath.Ext(path) != ".yml" && filepath.Ext(path) != ".yaml") {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			var parsed any
			if err := yaml.Unmarshal(data, &parsed); err != nil {
				return fmt.Errorf("parse %s: %w", path, err)
			}
			for _, action := range valuesForKey(parsed, "uses") {
				if strings.HasPrefix(action, "./") {
					continue
				}
				reject(!externalActionPattern.MatchString(action), "External action must use full SHA: "+action)
			}
			return nil
		}))
	}

	workflowRoot := filepath.Join(root, ".github", "workflows")
	failIf(filepath.WalkDir(workflowRoot, func(path string, entry os.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		text := string(data)
		reject(strings.Contains(text, "uses: actions/setup-node@") || strings.Contains(text, "uses: actions/setup-go@"), "Workflows must use repository setup actions")
		reject(strings.Contains(text, "node-version:"), "Node version must come from .node-version")
		return nil
	}))
}

func checkGoCaching(root string) {
	data, err := os.ReadFile(filepath.Join(root, ".github", "actions", "setup-go", "action.yml"))
	failIf(err)
	var action workflow
	failIf(yaml.Unmarshal(data, &action))
	run := object(action, "runs")
	setup := requireStep(run, "Set up Go")
	requireEqual(object(setup, "with")["cache"], false, "Built-in Go caching must not overlap the explicit caches")
	modules := object(requireStep(run, "Cache Go modules"), "with")
	build := object(requireStep(run, "Cache compiled Go packages"), "with")
	reject(modules["path"] == build["path"], "Module and compiled package caches must have separate paths")
	for _, key := range []string{"${{ runner.os }}", "${{ runner.arch }}", "${{ steps.go.outputs.go-version }}", "${{ hashFiles(inputs.cache-dependency-path) }}"} {
		requireContains(stringValue(modules["key"]), key, "Module cache must include platform, Go and dependency identity")
		requireContains(stringValue(build["key"]), key, "Build cache must include platform, Go and dependency identity")
	}
	requireContains(stringValue(build["key"]), "${{ inputs.build-cache-scope }}", "Build cache must isolate jobs and modes")
	requireContains(stringValue(build["key"]), "${{ github.sha }}", "New commits must save updated compilation work")
	restore := strings.TrimSpace(stringValue(build["restore-keys"]))
	requireEqual(restore, strings.TrimSuffix(stringValue(build["key"]), "${{ github.sha }}"), "Build cache must restore only from the same platform, Go, dependency and job scope")

	// Reusable workflows both name their job verify; they must not race to save one cache.
	for workflowName, scope := range map[string]string{"ops.yml": "ops", "migration-integration.yml": "migration-integration"} {
		job := object(object(loadWorkflow(root, workflowName), "jobs"), "verify")
		for _, step := range steps(job) {
			if step["uses"] == "./.github/actions/setup-go" {
				requireEqual(object(step, "with")["build-cache-scope"], scope, "Reusable verify jobs must use distinct compilation caches")
			}
		}
	}
}

func checkLegacyWorkflows(root string) {
	for _, name := range []string{"backend.yml", "release.yml"} {
		_, err := os.Stat(filepath.Join(root, ".github", "workflows", name))
		reject(err == nil, "Legacy workflow must remain deleted: "+name)
		failIfNonNotExist(err)
	}
}

func object(value map[string]any, key string) map[string]any {
	child, ok := value[key].(map[string]any)
	reject(!ok, "Expected object at "+key)
	return child
}

func steps(job map[string]any) []map[string]any {
	raw, ok := job["steps"].([]any)
	reject(!ok, "Expected job steps")
	result := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		step, ok := item.(map[string]any)
		reject(!ok, "Expected step object")
		result = append(result, step)
	}
	return result
}

func requireStep(job map[string]any, name string) map[string]any {
	for _, step := range steps(job) {
		if step["name"] == name {
			return step
		}
	}
	reject(true, "Missing step: "+name)
	return nil
}

func requireStepRun(job map[string]any, command string) {
	for _, step := range steps(job) {
		if step["run"] == command {
			return
		}
	}
	reject(true, "Missing workflow command: "+command)
}

func requireKey(value map[string]any, key, message string) {
	_, ok := value[key]
	reject(!ok, message)
}

func rejectKey(value map[string]any, key, message string) {
	_, ok := value[key]
	reject(ok, message)
}

func requireStringIn(value map[string]any, key, expected, message string) {
	raw, ok := value[key].([]any)
	reject(!ok, message)
	for _, item := range raw {
		if item == expected {
			return
		}
	}
	reject(true, message)
}

func requireEqual(actual any, expected any, message string) {
	reject(fmt.Sprint(actual) != fmt.Sprint(expected), fmt.Sprintf("%s: got %v, want %v", message, actual, expected))
}

func requireContains(actual, expected, message string) {
	reject(!strings.Contains(actual, expected), message)
}

func valuesForKey(value any, key string) []string {
	var result []string
	var walk func(any)
	walk = func(current any) {
		switch typed := current.(type) {
		case map[string]any:
			keys := make([]string, 0, len(typed))
			for childKey := range typed {
				keys = append(keys, childKey)
			}
			sort.Strings(keys)
			for _, childKey := range keys {
				child := typed[childKey]
				if childKey == key {
					if text, ok := child.(string); ok {
						result = append(result, text)
					}
				}
				walk(child)
			}
		case []any:
			for _, child := range typed {
				walk(child)
			}
		}
	}
	walk(value)
	return result
}

func scalarText(value any) string {
	var values []string
	var walk func(any)
	walk = func(current any) {
		switch typed := current.(type) {
		case string:
			values = append(values, typed)
		case map[string]any:
			for _, child := range typed {
				walk(child)
			}
		case []any:
			for _, child := range typed {
				walk(child)
			}
		}
	}
	walk(value)
	return strings.Join(values, "\n")
}

func stringValue(value any) string {
	text, _ := value.(string)
	return text
}

func reject(condition bool, message string) {
	if condition {
		fmt.Fprintln(os.Stderr, "ci-workflow-contract:", message)
		os.Exit(1)
	}
}

func failIf(err error) {
	if err != nil {
		fmt.Fprintln(os.Stderr, "ci-workflow-contract:", err)
		os.Exit(1)
	}
}

func failIfNonNotExist(err error) {
	if err != nil && !os.IsNotExist(err) {
		failIf(err)
	}
}
