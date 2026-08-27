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
	checkRelease(root, release)
	checkDeployOnly(root)
	checkActionPinning(root)
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
	step := requireStep(deploy, "Deploy existing immutable release")
	requireEqual(step["run"], "./scripts/run-production-deploy.sh release-assets/release-manifest.json", "Deploy-only workflow must use the shared deploy client")

	allText := scalarText(workflow)
	reject(strings.Contains(allText, "docker/build-push-action"), "Deploy-only workflow must not build images")
	reject(strings.Contains(allText, "cosign sign"), "Deploy-only workflow must not sign images")
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
	requireEqual(object(jobs, "migration-integration")["uses"], "./.github/workflows/migration-integration.yml", "Migration job must call migration-integration.yml")
	rejectKey(jobs, "security", "Scheduled security checks must remain outside CI")

	gate := object(jobs, "ci-gate")
	requireEqual(gate["name"], "CI Gate", "Required check must remain CI Gate")
	requireStringIn(gate, "needs", "changes", "CI Gate must depend on change classification")
	requireStringIn(gate, "needs", "migration-integration", "CI Gate must depend on migration integration")
	gateCheckout := requireStep(gate, "Checkout")
	requireEqual(gateCheckout["uses"], "actions/checkout@df4cb1c069e1874edd31b4311f1884172cec0e10", "CI Gate must checkout the lane policy module")
	gateStep := requireStep(gate, "Summarize and verify required lanes")
	requireEqual(gateStep["run"], "./scripts/ci-gate.sh", "CI Gate must use the lane policy module")
	requireEqual(gateStep["if"], "always()", "CI Gate policy must run after failed or skipped lanes")
}

func checkReusableWorkflows(root string) {
	for _, name := range []string{"web.yml", "api.yml", "auth.yml", "migration-integration.yml", "ops.yml"} {
		parsed := loadWorkflow(root, name)
		triggers := object(parsed, "on")
		requireKey(triggers, "workflow_call", name+" must expose workflow_call")
		for _, forbidden := range []string{"pull_request", "push", "merge_group"} {
			rejectKey(triggers, forbidden, name+" must not expose event trigger "+forbidden)
		}
	}

	api := loadWorkflow(root, "api.yml")
	requireEqual(object(api, "env")["DOCKER_BUILD_RECORD_UPLOAD"], "false", "API workflow must not upload Docker build records")
	apiTestStep := requireStep(object(object(api, "jobs"), "tests"), "Test race shard")
	requireEqual(object(apiTestStep, "env")["TEST_MODE"], "race", "API tests must run in race mode")

	auth := loadWorkflow(root, "auth.yml")
	requireEqual(object(auth, "env")["DOCKER_BUILD_RECORD_UPLOAD"], "false", "Auth workflow must not upload Docker build records")

	web := loadWorkflow(root, "web.yml")
	webAudit := requireStep(object(object(web, "jobs"), "quality"), "Production dependency audit")
	requireEqual(webAudit["run"], "npm audit --omit=dev --audit-level=high", "Web audit must reject high production vulnerabilities")

	migration := loadWorkflow(root, "migration-integration.yml")
	migrationInputs := object(object(migration, "on"), "workflow_call")
	requireKey(migrationInputs, "inputs", "Migration integration must accept commit range inputs")
	verify := object(object(migration, "jobs"), "verify")
	requireStepRun(verify, "bash scripts/ci-migration-integration-test.sh")
	requireStepRun(verify, "bash scripts/atlas-migration-check.sh")
	requireStepRun(verify, "./scripts/ci-migration-integration.sh")
	for _, step := range steps(verify) {
		run, _ := step["run"].(string)
		reject(strings.Contains(run, "npm run build") || strings.Contains(run, "npm run migrate:dist"), "Migration implementation must stay inside ci-migration-integration.sh")
	}

	ops := loadWorkflow(root, "ops.yml")
	ciContracts := requireStep(object(object(ops, "jobs"), "verify"), "Verify CI orchestration contracts")
	ciContractRun := stringValue(ciContracts["run"])
	for _, command := range []string{
		"bash scripts/ci-change-classifier-test.sh",
		"bash scripts/ci-gate-test.sh",
		"bash scripts/ci-workflow-contract-test.sh",
	} {
		requireContains(ciContractRun, command, "Ops must run "+command)
	}
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
	requireContains(stringValue(prepareImage["run"]), "immutable_tag=ghcr.io/${repository}/ideaboard-${{ matrix.image.name }}:sha-${{ needs.preflight.outputs.source_commit }}", "Images must use immutable commit tags")

	deploy := object(jobs, "deploy-production")
	requireEqual(object(deploy, "concurrency")["cancel-in-progress"], false, "Production deployments must not cancel in progress")
	stream := requireStep(deploy, "Stream manifest to Production deploy entrypoint")
	streamRun := stringValue(stream["run"])
	requireContains(streamRun, "./scripts/run-production-deploy.sh release-assets/release-manifest.json", "Production deployment must use the shared deploy client")
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
