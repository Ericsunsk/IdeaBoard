#!/usr/bin/env bash
set -euo pipefail
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PYTHONDONTWRITEBYTECODE=1 python3 - "$root/scripts/release-artifacts.py" <<'PY'
import copy
import importlib.util
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

ENGINE = Path(sys.argv.pop()).resolve()
spec = importlib.util.spec_from_file_location("release_artifacts", ENGINE)
engine = importlib.util.module_from_spec(spec)
spec.loader.exec_module(engine)
REPOSITORY = "Ericsunsk/IdeaBoard"
DIGEST = "sha256:" + "a" * 64


class ReleaseArtifactsTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)
        self.git("init", "-q")
        self.git("config", "user.name", "Fixture")
        self.git("config", "user.email", "fixture@example.invalid")
        self.workflow = (ENGINE.parent.parent / ".github/workflows/docker-images.yml").read_text()
        self.write(".github/workflows/docker-images.yml", self.workflow)
        for name in ("release-artifacts.py", "release-eligibility-check.sh", "prepare-release-plan.sh"):
            self.write(f"scripts/{name}", (ENGINE.parent / name).read_text())
        self.write(".node-version", "22.23.2\n")
        for context in {value[1] for value in engine.COMPONENTS.values()}:
            self.write(f"{context}/Dockerfile", "FROM scratch AS runtime\nFROM scratch AS migrator\n")
            self.write(f"{context}/.dockerignore", "node_modules\n")
            self.write(f"{context}/source", context)
        self.first = self.commit("v1.0.0")
        self.first_plan = self.plan(self.first, "v1.0.0")
        self.previous = self.manifest(self.first_plan)

    def git(self, *args):
        return subprocess.check_output(["git", "-C", str(self.root), *args], stderr=subprocess.DEVNULL).decode().strip()

    def write(self, name, text):
        path = self.root / name
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(text)

    def commit(self, tag):
        self.git("add", ".")
        self.git("commit", "-qm", tag, "--allow-empty")
        self.git("tag", tag)
        return self.git("rev-parse", "HEAD")

    def plan(self, commit, tag, previous=None):
        return engine.make_plan(self.root, REPOSITORY, commit, tag, "123", previous)

    def manifest(self, plan):
        metadata = {name: engine.component_metadata(plan, name, DIGEST if item["action"] == "build" else None)
                    for name, item in plan["components"].items()}
        return engine.make_manifest(plan, metadata, self.root)

    def actions(self, plan):
        return {name: item["action"] for name, item in plan["components"].items()}

    def test_first_release_and_unchanged_reuse_complete_manifest(self):
        self.assertEqual(set(self.actions(self.first_plan).values()), {"build"})
        self.write("scripts/deploy-only.sh", "echo new\n")
        second = self.commit("v1.0.1")
        plan = self.plan(second, "v1.0.1", self.previous)
        self.assertEqual(set(self.actions(plan).values()), {"reuse"})
        manifest = self.manifest(plan)
        self.assertEqual(manifest["images"], self.previous["images"])
        self.assertEqual(manifest["commit"], second)
        self.assertEqual({x["source_commit"] for x in manifest["components"].values()}, {self.first})
        self.assertEqual(len(plan["matrix"]["include"]), 5)

    def test_changed_api_builds_only_its_two_targets(self):
        self.write("apps/api/source", "changed API")
        second = self.commit("v1.0.1")
        plan = self.plan(second, "v1.0.1", self.previous)
        self.assertEqual(self.actions(plan), {name: "build" if name.startswith("api") else "reuse" for name in engine.COMPONENTS})
        manifest = self.manifest(plan)
        self.assertEqual(manifest["components"]["api"]["source_commit"], second)
        self.assertEqual(manifest["components"]["auth"]["source_commit"], self.first)
        with self.assertRaises(ValueError):
            engine.validate_manifest(self.previous, REPOSITORY, commit=second)

    def test_node_pin_and_build_recipe_invalidate_real_consumers(self):
        self.write(".node-version", "24.0.0\n")
        second = self.commit("v1.0.1")
        self.assertEqual(self.actions(self.plan(second, "v1.0.1", self.previous)),
                         {name: "reuse" if name.startswith("api") else "build" for name in engine.COMPONENTS})
        workflow = self.root / ".github/workflows/docker-images.yml"
        workflow.write_text(workflow.read_text().replace("provenance: mode=max", "provenance: mode=min"))
        third = self.commit("v1.0.2")
        self.assertEqual(set(self.actions(self.plan(third, "v1.0.2", self.previous)).values()), {"build"})

    def test_deployment_only_workflow_change_does_not_change_inputs(self):
        workflow = self.root / ".github/workflows/docker-images.yml"
        workflow.write_text(workflow.read_text().replace("  deploy-production:\n", "  deploy-production:\n    name: Changed deploy display name\n"))
        second = self.commit("v1.0.1")
        self.assertEqual(set(self.actions(self.plan(second, "v1.0.1", self.previous)).values()), {"reuse"})

    def test_actual_postbuild_scan_cache_sign_and_metadata_changes_reuse(self):
        workflow = self.root / ".github/workflows/docker-images.yml"
        mutations = [
            ("name: Scan linux/arm64 image\n        timeout-minutes: 10", "name: Scan linux/arm64 image\n        timeout-minutes: 11"),
            ("key: grype-db-v6-", "key: grype-db-v7-"),
            ("run: cosign sign --yes", "run: cosign sign --yes --timeout 2m"),
            ("name: ideaboard-${{ matrix.name }}-release-metadata", "name: changed-${{ matrix.name }}-release-metadata"),
            ("name: Set up Grype\n        timeout-minutes: 5", "name: Set up Grype\n        timeout-minutes: 6"),
        ]
        for index, (before, after) in enumerate(mutations, 1):
            with self.subTest(change=before):
                self.assertIn(before, self.workflow)
                workflow.write_text(self.workflow.replace(before, after))
                commit = self.commit(f"v1.0.{index}")
                plan = self.plan(commit, f"v1.0.{index}", self.previous)
                self.assertEqual(set(self.actions(plan).values()), {"reuse"})
                self.manifest(plan)

    def test_actual_build_inputs_and_preflight_sources_invalidate(self):
        workflow = self.root / ".github/workflows/docker-images.yml"
        mutations = [
            ("platforms: linux/arm64", "platforms: linux/amd64"),
            ("context: ${{ matrix.context }}", "context: apps/other"),
            ("target: ${{ matrix.target }}", "target: other"),
            ("docker/setup-buildx-action@d7f5e7f509e45cec5c76c4d5afdd7de93d0b3df5", "docker/setup-buildx-action@" + "a" * 40),
            ("actions/checkout@df4cb1c069e1874edd31b4311f1884172cec0e10", "actions/checkout@" + "b" * 40),
            ("provenance: mode=max", "provenance: mode=min"),
            ("push: true", "build-args: REV=changed\n          push: true"),
            ("env:\n", "env:\n  BUILD_FLAG: changed\n"),
            ("runs-on: ubuntu-latest", "runs-on: ubuntu-24.04"),
        ]
        for index, (before, after) in enumerate(mutations, 1):
            with self.subTest(change=before):
                self.assertIn(before, self.workflow)
                workflow.write_text(self.workflow.replace(before, after, 1))
                commit = self.commit(f"v1.1.{index}")
                plan = self.plan(commit, f"v1.1.{index}", self.previous)
                self.assertEqual(set(self.actions(plan).values()), {"build"})
        workflow.write_text(self.workflow)
        for index, name in enumerate(("release-eligibility-check.sh", "prepare-release-plan.sh", "release-artifacts.py"), 1):
            with self.subTest(source=name):
                self.write(f"scripts/{name}", (ENGINE.parent / name).read_text() + "\n# changed policy\n")
                commit = self.commit(f"v1.2.{index}")
                self.assertEqual(set(self.actions(self.plan(commit, f"v1.2.{index}", self.previous)).values()), {"build"})

    def test_unknown_layout_and_local_actions_build_without_reuse(self):
        workflow = self.root / ".github/workflows/docker-images.yml"
        mutations = [
            self.workflow.replace("      - name: Build and push image", "      - name: Renamed image build"),
            self.workflow.replace("  publish-images:", "  renamed-images:"),
            self.workflow.replace("      - name: Set up QEMU", '      - run: cat build-args.txt >> "$GITHUB_ENV"\n      - name: Set up QEMU'),
            self.workflow.replace("      - name: Set up QEMU", "      - uses: org/custom-build@" + "d" * 40 + "\n      - name: Set up QEMU"),
            self.workflow.replace("docker/setup-buildx-action@d7f5e7f509e45cec5c76c4d5afdd7de93d0b3df5\n", "docker/setup-buildx-action@d7f5e7f509e45cec5c76c4d5afdd7de93d0b3df5\n        with:\n          config: ./buildkitd.toml\n"),
            self.workflow.replace("      - name: Set up QEMU", "      - uses: ./.github/actions/custom-build\n      - name: Set up QEMU"),
            self.workflow.replace("      - name: Resolve immutable component image", "    env:\n      BUILD_FLAG: later\n      - name: Resolve immutable component image"),
            self.workflow.replace("docker/build-push-action@10e90e3645eae34f1e60eeb005ba3a3d33f178e8", "custom/build-action@" + "c" * 40),
        ]
        for index, mutation in enumerate(mutations, 1):
            with self.subTest(layout=index):
                workflow.write_text(mutation)
                commit = self.commit(f"v1.3.{index}")
                self.assertIsNone(engine.build_recipe(self.root, commit))
                plan = self.plan(commit, f"v1.3.{index}", self.previous)
                self.assertEqual(set(self.actions(plan).values()), {"build"})
                manifest = self.manifest(plan)
                # Even identical unknown inputs cannot authorize image reuse.
                again = self.plan(commit, f"v1.3.{index}", manifest)
                self.assertEqual(set(self.actions(again).values()), {"build"})
                before = engine.fingerprint(self.root, commit, "api")
                self.write("custom/build-data", str(index))
                other = self.commit(f"v1.4.{index}")
                self.assertNotEqual(before, engine.fingerprint(self.root, other, "api"))

    def test_changed_shell_or_new_helper_disables_reuse(self):
        planner = self.root / "scripts/prepare-release-plan.sh"
        planner.write_text(planner.read_text() + '\n"$script_dir/plan-helper.sh"\n')
        self.write("scripts/plan-helper.sh", "echo first\n")
        commit = self.commit("v1.5.0")
        self.assertIsNone(engine.build_recipe(self.root, commit))
        baseline = self.manifest(self.plan(commit, "v1.5.0"))
        self.write("scripts/plan-helper.sh", "echo changed\n")
        other = self.commit("v1.5.1")
        self.assertEqual(set(self.actions(self.plan(other, "v1.5.1", baseline)).values()), {"build"})
        planner.write_text(planner.read_text() + '\nsource "$UNKNOWN_HELPER"\n')
        unknown = self.commit("v1.5.2")
        self.assertIsNone(engine.build_recipe(self.root, unknown))

    def test_existing_setup_pin_and_build_scalar_reuse_after_rebuild(self):
        workflow = self.root / ".github/workflows/docker-images.yml"
        workflow.write_text(self.workflow.replace("docker/setup-buildx-action@d7f5e7f509e45cec5c76c4d5afdd7de93d0b3df5", "docker/setup-buildx-action@" + "a" * 40)
                            .replace("provenance: mode=max", "provenance: mode=min"))
        second = self.commit("v1.7.0")
        self.assertIsNotNone(engine.build_recipe(self.root, second))
        plan = self.plan(second, "v1.7.0", self.previous)
        self.assertEqual(set(self.actions(plan).values()), {"build"})
        previous = self.manifest(plan)
        self.write("deploy/only", "new deploy\n")
        third = self.commit("v1.7.1")
        self.assertEqual(set(self.actions(self.plan(third, "v1.7.1", previous)).values()), {"reuse"})

    def test_historical_v3_recognition_uses_source_commit_evidence(self):
        before = engine.fingerprint(self.root, self.first, "api")
        owner = self.root / "scripts/release-artifacts.py"
        owner.write_text(owner.read_text().replace(engine.BUILD_INPUT_CLOSURE, "f" * 64))
        unknown = self.commit("v1.8.0")
        self.assertIsNone(engine.build_recipe(self.root, unknown))
        historical_unknown = self.manifest(self.plan(unknown, "v1.8.0"))
        owner.write_text(ENGINE.read_text() + "\n# later audited source\n")
        later = self.commit("v1.8.1")
        self.assertIsNotNone(engine.build_recipe(self.root, later))
        original_evidence = engine.BUILD_INPUT_CLOSURE
        try:
            engine.BUILD_INPUT_CLOSURE = "e" * 64
            self.assertEqual(before, engine.fingerprint(self.root, self.first, "api"))
            engine.validate_manifest(self.previous, REPOSITORY, root=self.root)
            engine.validate_manifest(historical_unknown, REPOSITORY, root=self.root)
        finally:
            engine.BUILD_INPUT_CLOSURE = original_evidence

    def test_historical_v2_manifest_still_validates_but_rebuilds_on_v3(self):
        legacy = copy.deepcopy(self.previous)
        for name, item in legacy["components"].items():
            item["input_fingerprint"] = engine.legacy_fingerprint(self.root, self.first, name)
        engine.validate_manifest(legacy, REPOSITORY, root=self.root)
        self.write("deploy/only", "changed deployment\n")
        second = self.commit("v1.6.0")
        # A historical v2 reused release remains verifiable under v3 tooling.
        historical = copy.deepcopy(legacy)
        historical.update(commit=second, tag="v1.6.0", ref="refs/tags/v1.6.0")
        historical["cosign"]["certificate_identity"] = engine.identity(REPOSITORY, historical["ref"])
        engine.validate_manifest(historical, REPOSITORY, root=self.root)
        self.assertEqual(set(self.actions(self.plan(second, "v1.6.0", legacy)).values()), {"build"})
        mixed = copy.deepcopy(legacy)
        mixed["components"]["api"] = copy.deepcopy(self.previous["components"]["api"])
        engine.validate_manifest(mixed, REPOSITORY, root=self.root)
        workflow = self.root / ".github/workflows/docker-images.yml"
        workflow.write_text(self.workflow.replace("name: Scan linux/arm64 image\n        timeout-minutes: 10", "name: Scan linux/arm64 image\n        timeout-minutes: 11"))
        third = self.commit("v1.6.1")
        historical.update(commit=third, tag="v1.6.1", ref="refs/tags/v1.6.1")
        historical["cosign"]["certificate_identity"] = engine.identity(REPOSITORY, historical["ref"])
        # v3-equivalent target does not make a changed v2 recipe equivalent.
        with self.assertRaisesRegex(ValueError, "component release inputs changed"):
            engine.validate_manifest(historical, REPOSITORY, root=self.root)

    def test_invalid_or_legacy_optional_candidate_builds_all(self):
        second = self.commit("v1.0.1")
        mutations = [
            lambda m: m.pop("schema_version"),
            lambda m: m.pop("components"),
            lambda m: m.update(repository="attacker/IdeaBoard"),
            lambda m: m["components"].pop("auth"),
            lambda m: m["components"].update(unknown={}),
            lambda m: m["components"]["api"].update(context="attacker"),
            lambda m: m["components"]["api"].update(certificate_identity="https://attacker"),
            lambda m: m["components"]["api"].update(certificate_oidc_issuer="https://attacker"),
            lambda m: m["components"]["api"].update(input_fingerprint="sha256:" + "b" * 64),
            lambda m: m["components"]["api"].update(platform="linux/amd64"),
            lambda m: m["components"]["api"].update(target="migrator"),
            lambda m: m["components"]["api"].update(source_run_id="../bad"),
            lambda m: m["components"]["api"].update(source_ref="refs/heads/main"),
            lambda m: m["components"]["api"].update(image="ghcr.io/attacker/api@" + DIGEST),
            lambda m: m["components"]["api"].update(digest="sha256:" + "b" * 64),
            lambda m: m["workflow"].update(name="attacker.yml"),
            lambda m: m.update(ref="refs/heads/main"),
        ]
        for mutate in mutations:
            with self.subTest(mutation=mutate):
                candidate = copy.deepcopy(self.previous)
                mutate(candidate)
                self.assertEqual(set(self.actions(self.plan(second, "v1.0.1", candidate)).values()), {"build"})

    def test_git_file_mode_ignore_and_source_tag_changes_are_inputs(self):
        source = self.root / "apps/auth/source"
        source.chmod(0o755)
        second = self.commit("v1.0.1")
        plan = self.plan(second, "v1.0.1", self.previous)
        self.assertEqual(plan["components"]["auth"]["action"], "build")
        self.assertEqual(plan["components"]["api"]["action"], "reuse")
        self.git("tag", "-f", "v1.0.0", second)
        self.assertEqual(set(self.actions(self.plan(second, "v1.0.1", self.previous)).values()), {"build"})
        self.git("tag", "-f", "v1.0.0", self.first)
        self.write("apps/realtime/.dockerignore", "changed-pattern\n")
        third = self.commit("v1.0.2")
        plan = self.plan(third, "v1.0.2", self.previous)
        self.assertEqual(plan["components"]["realtime"]["action"], "build")

    def test_source_tag_binding_and_nonancestor_refuse_reuse(self):
        self.git("tag", "-f", "v1.0.0", self.first)
        self.write("apps/api/source", "unrelated")
        self.git("checkout", "--orphan", "other")
        second = self.commit("v2.0.0")
        self.assertEqual(set(self.actions(self.plan(second, "v2.0.0", self.previous)).values()), {"build"})
        with self.assertRaises(ValueError):
            engine.validate_manifest(self.previous, REPOSITORY, commit=second, root=self.root)

    def test_aggregate_rejects_mutated_metadata_and_missing_sources(self):
        metadata = {name: engine.component_metadata(self.first_plan, name, DIGEST) for name in engine.COMPONENTS}
        metadata["api"]["source_commit"] = "b" * 40
        with self.assertRaises(ValueError):
            engine.make_manifest(self.first_plan, metadata, self.root)
        with self.assertRaises(ValueError):
            engine.component_metadata(self.first_plan, "api", "latest")
        second = self.commit("v1.0.1")
        reused = self.plan(second, "v1.0.1", self.previous)
        with self.assertRaises(ValueError):
            engine.component_metadata(reused, "api", DIGEST)

    def test_manual_sha_build_preserves_branch_signer_and_cannot_deploy_or_reuse(self):
        plan = engine.make_plan(self.root, REPOSITORY, self.first, "main", "123", self.previous, "refs/heads/main")
        self.assertEqual(set(self.actions(plan).values()), {"build"})
        manifest = self.manifest(plan)
        self.assertEqual(manifest["release_kind"], "build-only")
        self.assertTrue(manifest["components"]["api"]["certificate_identity"].endswith("@refs/heads/main"))
        with self.assertRaises(ValueError):
            engine.validate_manifest(manifest, REPOSITORY)
        second = self.commit("v1.0.1")
        self.assertEqual(set(self.actions(self.plan(second, "v1.0.1", manifest)).values()), {"build"})

    def test_cli_plan_metadata_manifest_validate(self):
        def cli(*args, expect=0):
            result = subprocess.run([sys.executable, str(ENGINE), "--root", str(self.root), *args],
                                    capture_output=True, text=True)
            self.assertEqual(result.returncode, expect, result.stderr)
            return result
        plan_file = self.root / "plan.json"
        cli("plan", "--repository", REPOSITORY, "--commit", self.first, "--tag", "v1.0.0", "--run-id", "123", "--output", str(plan_file))
        metadata_dir = self.root / "metadata"
        metadata_dir.mkdir()
        for name in engine.COMPONENTS:
            cli("metadata", "--plan", str(plan_file), "--component", name, "--digest", DIGEST,
                "--output", str(metadata_dir / f"{name}.json"))
        manifest_file = self.root / "manifest.json"
        cli("manifest", "--plan", str(plan_file), "--metadata-dir", str(metadata_dir), "--output", str(manifest_file))
        cli("validate", "--manifest", str(manifest_file), "--repository", REPOSITORY, "--commit", self.first,
            "--tag", "v1.0.0", "--verify-inputs")
        result = cli("validate", "--manifest", str(manifest_file), "--repository", "attacker/repo", expect=1)
        self.assertNotIn("Traceback", result.stderr)


unittest.main(verbosity=2)
PY
