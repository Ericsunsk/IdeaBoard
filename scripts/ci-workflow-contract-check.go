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
	for _, required := range []string{".repository == $expected_repository", ".tag == $expected_tag", ".ref == $expected_ref", "$expected_commit == \"\" or .commit == $expected_commit"} {
		requireContains(stringValue(validate["run"]), required, "Deployment manifest binding is missing "+required)
	}
	configure := requireStep(runs, "Configure restricted SSH transport")
	requireContains(stringValue(configure["run"]), "ssh-keygen -F", "Deployment must verify the pinned endpoint before SSH")
	stream := requireStep(runs, "Stream manifest to Production deploy entrypoint")
	requireEqual(stream["run"], `./scripts/run-production-deploy.sh "${RELEASE_MANIFEST}"`, "Shared deployment action must use the sole transport client")
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
	requireStringIn(object(triggers, "push"), "branches", "main", "CI push trigger must include main")
	rejectKey(triggers, "merge_group", "CI must not expose merge_group")

	jobs := object(ci, "jobs")
	requireEqual(object(jobs, "web")["uses"], "./.github/workflows/web.yml", "Web job must call web.yml")
	requireEqual(object(jobs, "api")["uses"], "./.github/workflows/api.yml", "API job must call api.yml")
	requireEqual(object(jobs, "auth")["uses"], "./.github/workflows/auth.yml", "Auth job must call auth.yml")
	requireEqual(object(jobs, "realtime")["uses"], "./.github/workflows/realtime.yml", "Realtime job must call realtime.yml")
	requireEqual(object(jobs, "migration-integration")["uses"], "./.github/workflows/migration-integration.yml", "Migration job must call migration-integration.yml")
	rejectKey(jobs, "security", "Scheduled security checks must remain outside CI")

	gate := object(jobs, "ci-gate")
	requireEqual(gate["name"], "CI Gate", "Required check must remain CI Gate")
	requireStringIn(gate, "needs", "changes", "CI Gate must depend on change classification")
	requireStringIn(gate, "needs", "api", "CI Gate must depend on API verification")
	requireStringIn(gate, "needs", "realtime", "CI Gate must depend on realtime")
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
	gateEnv := object(gateStep, "env")
	requireEqual(gateEnv["API_REQUIRED"], "${{ needs.changes.outputs.api }}", "CI Gate must receive the API required flag")
	requireEqual(gateEnv["API_RESULT"], "${{ needs.api.result }}", "CI Gate must receive the aggregate API result")
	requireEqual(gateEnv["MEDIA_CONTRACT_REQUIRED"], "true", "Media contract must always be a required CI lane")
	requireEqual(gateEnv["MEDIA_CONTRACT_RESULT"], "${{ needs.media-contract.result }}", "CI Gate must receive the Media contract result")
	requireEqual(gateEnv["REALTIME_REQUIRED"], "${{ needs.changes.outputs.realtime }}", "CI Gate must receive the Realtime required flag")
	requireEqual(gateEnv["REALTIME_RESULT"], "${{ needs.realtime.result }}", "CI Gate must receive the Realtime result")
}

func checkReusableWorkflows(root string) {
	for _, name := range []string{"web.yml", "api.yml", "auth.yml", "realtime.yml", "migration-integration.yml", "ops.yml"} {
		parsed := loadWorkflow(root, name)
		triggers := object(parsed, "on")
		requireKey(triggers, "workflow_call", name+" must expose workflow_call")
		for _, forbidden := range []string{"pull_request", "push", "merge_group"} {
			rejectKey(triggers, forbidden, name+" must not expose event trigger "+forbidden)
		}
	}

	api := loadWorkflow(root, "api.yml")
	rejectKey(object(api, "jobs"), "schema", "Schema semantics must run once in the migration gate")
	rejectKey(object(object(object(api, "on"), "workflow_call"), "inputs"), "run_schema", "Schema verification must not require an optional input")
	requireEqual(object(api, "env")["DOCKER_BUILD_RECORD_UPLOAD"], "false", "API workflow must not upload Docker build records")
	checkAPITestMatrix(api)
	migrationGate := object(object(api, "jobs"), "migration-gate")
	rejectKey(migrationGate, "if", "API migration verification must run on every invocation")
	installAPIAtlas := requireStep(migrationGate, "Install Atlas")
	installAPIAtlasRun := stringValue(installAPIAtlas["run"])
	requireContains(installAPIAtlasRun, "apps/api/Dockerfile", "API migration gate must derive Atlas from the Dockerfile pin")
	requireContains(installAPIAtlasRun, "ARG ATLAS_VERSION", "API migration gate must derive Atlas from the Dockerfile pin")
	reject(strings.Contains(installAPIAtlasRun, "v1.3.2"), "API migration gate must not hardcode the Atlas version")
	semanticCheck := requireStep(migrationGate, "Compare migration replay with schema snapshot")
	requireEqual(semanticCheck["working-directory"], "apps/api", "Migration semantics must use the API project root")
	requireContains(stringValue(semanticCheck["run"]), `go run ./cmd/schemarelease --base-sha "${ATLAS_BASE_SHA:-HEAD}"`, "Migration gate must compare migration replay and snapshot semantics")
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
	requireContains(installMigrationAtlasRun, "apps/api/Dockerfile", "Migration integration must derive Atlas from the Dockerfile pin")
	requireContains(installMigrationAtlasRun, "ARG ATLAS_VERSION", "Migration integration must derive Atlas from the Dockerfile pin")
	reject(strings.Contains(installMigrationAtlasRun, "v1.3.2"), "Migration integration must not hardcode the Atlas version")
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

func checkAPITestMatrix(api workflow) {
	jobs := object(api, "jobs")
	rejectKey(jobs, "standard-tests", "API test modes must share one matrix")
	job := object(jobs, "tests")
	rejectKey(job, "needs", "API tests must run independently")
	rejectKey(job, "if", "API tests must always run")
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
	comment := object(jobs, "bundle-comment")
	requireContains(stringValue(comment["if"]), "github.event.pull_request.head.repo.full_name == github.repository", "Bundle comments must skip read-only fork PRs")
	for _, name := range []string{"unit", "browser"} {
		upload := requireStep(object(jobs, name), "Upload shard blob report")
		requireEqual(object(upload, "with")["include-hidden-files"], true, name+" reports must include the hidden report directory")
	}
	requireStepRun(object(jobs, "typecheck-node"), "npm run test:typecheck")
}

func checkRelease(root string, release workflow) {
	requireEqual(object(release, "env")["DOCKER_BUILD_RECORD_UPLOAD"], "false", "Release workflow must not upload Docker build records")
	jobs := object(release, "jobs")
	preflight := object(jobs, "preflight")
	requireEqual(object(preflight, "permissions")["checks"], "read", "Release preflight must read CI checks")
	requireStringIn(object(jobs, "publish-images"), "needs", "preflight", "Image publication must depend on preflight")

	build := requireStep(object(jobs, "publish-images"), "Build and push image")
	requireEqual(build["uses"], "docker/build-push-action@10e90e3645eae34f1e60eeb005ba3a3d33f178e8", "Image build action must remain pinned")
	buildInputs := object(build, "with")
	requireEqual(buildInputs["provenance"], "mode=max", "Images must include max provenance")
	requireEqual(buildInputs["sbom"], true, "Images must include SBOM attestations")
	requireEqual(buildInputs["tags"], "${{ steps.image.outputs.immutable_tag }}", "Image build must consume the prepared immutable tag")
	prepareImage := requireStep(object(jobs, "publish-images"), "Prepare image name")
	prepareImageEnv := object(prepareImage, "env")
	requireEqual(prepareImageEnv["IMAGE_NAME"], "${{ matrix.image.name }}", "Image name must be passed through step env")
	requireEqual(prepareImageEnv["SOURCE_COMMIT"], "${{ needs.preflight.outputs.source_commit }}", "Image commit must be passed through step env")
	prepareImageRun := stringValue(prepareImage["run"])
	requireContains(prepareImageRun, "immutable_tag=ghcr.io/${repository}/ideaboard-${IMAGE_NAME}:sha-${SOURCE_COMMIT}", "Images must use immutable commit tags")

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
		"< \"${manifest}\"",
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
