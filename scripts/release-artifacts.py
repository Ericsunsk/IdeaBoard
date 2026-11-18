#!/usr/bin/env python3
"""Own component input identity and complete immutable release metadata.

The caller authenticates previous release/run provenance and verifies signatures.
This module only permits reuse when Git inputs prove component equivalence.
"""
import argparse
import hashlib
import json
import re
import subprocess
import sys
import textwrap
from pathlib import Path

COMPONENTS = {
    "api": ("API", "apps/api", "runtime"),
    "api-migrate": ("API Migration", "apps/api", "migrator"),
    "auth": ("Auth", "apps/auth", "runtime"),
    "auth-migrate": ("Auth Migration", "apps/auth", "migrator"),
    "realtime": ("Realtime", "apps/realtime", "runtime"),
}
PLATFORM = "linux/arm64"
ISSUER = "https://token.actions.githubusercontent.com"
POLICY = "ideaboard-component-inputs-v3"
LEGACY_POLICY = "ideaboard-component-inputs-v2"
# Audited execution-input closure at Ericsunsk/IdeaBoard@2908e14e2: shell
# bodies/sources, action identities and input bindings. Scalar inputs and action
# pins remain in the actual recipe fingerprint. Unknown dependencies build anew.
# Read this evidence from each source commit so later audits preserve history.
BUILD_INPUT_CLOSURE = "cbe21fdadcd0b678dbc78e8d46bb8dd6457902b69a214d6560713af1201dfa6b"
COMPONENT_FIELDS = {"input_fingerprint", "source_commit", "source_tag", "source_ref", "source_run_id",
                    "certificate_identity", "certificate_oidc_issuer", "platform", "target", "image",
                    "name", "ref", "digest"}
SHA = re.compile(r"[0-9a-f]{40}")
FINGERPRINT = re.compile(r"sha256:[0-9a-f]{64}")
TAG = re.compile(r"v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)")
REPOSITORY = re.compile(r"[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+")


def require(condition, message):
    if not condition:
        raise ValueError(message)


def git(root, *args):
    return subprocess.check_output(["git", "-C", str(root), *args], stderr=subprocess.DEVNULL)


def commit_exists(root, commit):
    require(isinstance(commit, str) and SHA.fullmatch(commit), "invalid source commit")
    require(git(root, "cat-file", "-t", commit).strip() == b"commit", "source is not a commit")


def ancestor(root, source, target):
    commit_exists(root, source)
    commit_exists(root, target)
    require(subprocess.run(["git", "-C", str(root), "merge-base", "--is-ancestor", source, target],
                           stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL).returncode == 0,
            "component source is not a release ancestor")


def identity(repository, ref):
    return f"https://github.com/{repository}/.github/workflows/docker-images.yml@{ref}"


def positive_id(value):
    return ((isinstance(value, int) and not isinstance(value, bool) and value > 0)
            or (isinstance(value, str) and re.fullmatch(r"[1-9][0-9]*", value) is not None))


def canonical_image(repository, name, image):
    prefix = f"ghcr.io/{repository.lower()}/ideaboard-{name}@"
    require(isinstance(image, str) and image.startswith(prefix)
            and FINGERPRINT.fullmatch(image[len(prefix):]), "invalid component image digest")


def build_recipe(root, commit):
    path = ".github/workflows/docker-images.yml"
    text = git(root, "show", f"{commit}:{path}").decode()
    start = text.find("  publish-images:\n")
    end = text.find("  aggregate-release-manifest:\n", start)
    if start < 0 or end <= start:
        return None
    block = text[start:end]
    require("build-contexts:" not in block, "external build contexts cannot be reused")

    # Take a continuous execution prefix, including workflow env/defaults,
    # preflight, checkout and every setup step. Later scan/sign/metadata steps
    # cannot change bytes already built. Derive the boundary from real steps;
    # do not maintain a second list of build steps or introduce YAML markers.
    builds = list(re.finditer(r"(?m)^      - name: Build and push image\n", block))
    following = re.search(r"(?m)^      - ", block[builds[0].end():]) if len(builds) == 1 else None
    boundary = start + builds[0].end() + following.start() if following else end
    prefix = text[:boundary]
    build = block[builds[0].start():boundary-start] if len(builds) == 1 else ""
    steps = block.find("    steps:\n")
    known = (len(builds) == 1 and following is not None
             and re.search(r"(?m)^        uses: docker/build-push-action@[0-9a-f]{40}$", build)
             and not re.search(r"(?m)^\s*(?:(?:- )?uses: \./|<<:)|[&*][A-Za-z_][A-Za-z_0-9-]*", prefix)
             and steps >= 0
             and not re.search(r"(?m)^    [A-Za-z_-]+:", block[steps+len("    steps:\n"):])
             and not re.search(r"(?m)^(?:env|defaults):", text[boundary:])
             and set(re.findall(r"needs\.([A-Za-z_0-9-]+)\.", prefix)) <= {"preflight"})

    # Recognize a bounded, audited shell input closure instead of trying to
    # infer arbitrary shell data flow. A new run or changed script may read any
    # tracked file, so the conservative path below disables reuse entirely.
    if not known:
        return None
    commands = list(re.finditer(r"(?m)^        run: ([^\n]*)\n((?:          [^\n]*\n|\n)*)", prefix))
    if len(commands) != len(re.findall(r"(?m)^\s*(?:- )?run:", prefix)):
        return None
    actions = re.findall(r"(?m)^        uses: ([^\s]+)$", prefix)
    if (len(actions) != len(re.findall(r"(?m)^\s*(?:- )?uses:", prefix))
            or any(not re.fullmatch(r"[^@]+@[0-9a-f]{40}", action) for action in actions)):
        return None
    sources = sorted(set(re.findall(r"\bscripts/[A-Za-z0-9_./-]+", prefix)))
    try:
        scripts = {source: git(root, "show", f"{commit}:{source}").decode() for source in sources}
        owner = git(root, "show", f"{commit}:scripts/release-artifacts.py").decode()
    except subprocess.CalledProcessError:
        return None
    closure = {"run": [textwrap.dedent(command[2]).strip() if command[1] == "|"
                       else command[1].strip() for command in commands], "scripts": scripts,
               "actions": [action.rsplit("@", 1)[0] for action in actions],
               "keys": re.findall(r"(?m)^( *)([A-Za-z_][A-Za-z_0-9-]*):", prefix),
               "bindings": re.findall(r"\$\{\{\s*(.*?)\s*}}", prefix)}
    encoded = json.dumps(closure, sort_keys=True, separators=(",", ":")).encode()
    evidence = re.search(r'^BUILD_INPUT_CLOSURE = "([0-9a-f]{64})"$', owner, re.MULTILINE)
    if evidence is None or hashlib.sha256(encoded).hexdigest() != evidence[1]:
        return None
    entries = git(root, "ls-tree", "-rz", "--full-tree", commit, "--",
                  *sources, "scripts/release-artifacts.py")
    return prefix.encode() + b"\0" + entries


def legacy_fingerprint(root, commit, name):
    # Historical schema-v2 manifests have no policy field. Keep their exact
    # digest domain and recipe, so signed old and mixed releases still verify.
    text = git(root, "show", f"{commit}:.github/workflows/docker-images.yml")
    start = text.find(b"  publish-images:\n")
    end = text.find(b"  aggregate-release-manifest:\n", start)
    require(start >= 0 and end > start, "unrecognized image build recipe")
    block = text[start:end]
    require(b"      - name: Build and push image\n" in block, "missing canonical image build")
    require(b"build-contexts:" not in block, "external build contexts cannot be reused")
    return component_fingerprint(root, commit, name, LEGACY_POLICY, block)


def fingerprint(root, commit, name):
    recipe = build_recipe(root, commit)
    if recipe is None:
        # Full tracked-source fallback stays verifiable, but make_plan never
        # reuses it: unknown execution/data dependencies need a fresh build.
        recipe = (git(root, "show", f"{commit}:.github/workflows/docker-images.yml") + b"\0"
                  + git(root, "ls-tree", "-rz", "--full-tree", commit))
    return component_fingerprint(root, commit, name, POLICY, recipe)


def component_fingerprint(root, commit, name, policy, recipe):
    commit_exists(root, commit)
    require(name in COMPONENTS, "unknown component")
    _, context, target = COMPONENTS[name]
    entries = git(root, "ls-tree", "-rz", "--full-tree", commit, "--", context).split(b"\0")
    entries = sorted(entry for entry in entries if entry)
    require(entries, "empty component context")
    dockerfile = git(root, "show", f"{commit}:{context}/Dockerfile").decode()
    require(re.search(rf"(?im)^FROM .+ AS {target}\s*$", dockerfile), "missing component build target")
    # Remote ADD and external COPY stages are not represented by the Git tree.
    stages = set(re.findall(r"(?im)^FROM .+ AS ([A-Za-z0-9_.-]+)\s*$", dockerfile))
    for instruction in re.findall(r"(?im)^(?:ADD|COPY)\s+(.+)$", dockerfile):
        require(not re.search(r"https?://|git://|\$", instruction), "external Docker input cannot be reused")
        source = re.search(r"--from=([^\s]+)", instruction)
        require(source is None or source.group(1) in stages or source.group(1).isdigit(),
                "external Docker stage cannot be reused")
    h = hashlib.sha256()
    for value in [policy.encode(), name.encode(), context.encode(), target.encode(), PLATFORM.encode(),
                  recipe, *entries]:
        h.update(len(value).to_bytes(8, "big"))
        h.update(value)
    if context in ("apps/auth", "apps/realtime"):
        node_pin = git(root, "show", f"{commit}:.node-version")
        h.update(len(node_pin).to_bytes(8, "big"))
        h.update(node_pin)
    return "sha256:" + h.hexdigest()


def validate_manifest(manifest, repository, commit=None, tag=None, root=None, allow_build_only=False):
    require(isinstance(manifest, dict) and manifest.get("schema_version") == 2, "manifest requires schema_version2")
    require(REPOSITORY.fullmatch(repository) is not None, "invalid repository")
    require(manifest.get("repository") == repository, "manifest repository mismatch")
    target = manifest.get("commit")
    release_tag = manifest.get("tag")
    require(isinstance(target, str) and SHA.fullmatch(target), "invalid release commit")
    release_ref = manifest.get("ref")
    require(manifest.get("release_kind") in ("production", "build-only"), "invalid release kind")
    build_only = manifest.get("release_kind") == "build-only"
    if build_only:
        require(allow_build_only, "build-only manifests cannot deploy or supply reused images")
        require(isinstance(release_ref, str) and re.fullmatch(r"refs/heads/[A-Za-z0-9._/-]+", release_ref)
                and ".." not in release_ref and "//" not in release_ref and not release_ref.endswith(("/", ".")),
                "invalid build-only signing ref")
        require(release_tag == release_ref.removeprefix("refs/heads/"), "build-only ref name mismatch")
    else:
        require(isinstance(release_tag, str) and TAG.fullmatch(release_tag), "invalid release tag")
        require(release_ref == f"refs/tags/{release_tag}", "release ref mismatch")
    require(commit is None or target == commit, "release commit mismatch")
    require(tag is None or release_tag == tag, "release tag mismatch")
    workflow = manifest.get("workflow", {})
    require(isinstance(workflow, dict) and workflow.get("name") == "docker-images.yml"
            and positive_id(workflow.get("run_id")), "invalid release workflow")
    cosign = manifest.get("cosign", {})
    require(isinstance(cosign, dict) and cosign.get("certificate_identity") == identity(repository, manifest["ref"])
            and cosign.get("certificate_oidc_issuer") == ISSUER, "invalid release signer")
    components = manifest.get("components")
    images = manifest.get("images")
    require(isinstance(components, dict) and set(components) == set(COMPONENTS), "manifest needs exactly five components")
    require(isinstance(images, dict) and set(images) == set(COMPONENTS), "manifest needs exactly five images")
    for name, item in components.items():
        require(isinstance(item, dict) and set(item) == COMPONENT_FIELDS, "invalid component metadata")
        canonical_image(repository, name, item.get("image"))
        require(images[name] == item["image"], "image and provenance digest mismatch")
        require(item.get("name") == name and item.get("ref") == item["image"]
                and item.get("digest") == item["image"].rsplit("@", 1)[1], "invalid component reference metadata")
        source = item.get("source_commit")
        source_tag = item.get("source_tag")
        require(isinstance(source, str) and SHA.fullmatch(source), "invalid component source")
        if build_only:
            require(source == target and source_tag == release_tag and item.get("source_ref") == release_ref,
                    "build-only component source mismatch")
        else:
            require(isinstance(source_tag, str) and TAG.fullmatch(source_tag), "invalid component source tag")
            require(item.get("source_ref") == f"refs/tags/{source_tag}", "component source ref mismatch")
        require(positive_id(item.get("source_run_id")), "invalid component run")
        require(item.get("certificate_identity") == identity(repository, item["source_ref"])
                and item.get("certificate_oidc_issuer") == ISSUER, "invalid component signer")
        require(isinstance(item.get("input_fingerprint"), str)
                and FINGERPRINT.fullmatch(item["input_fingerprint"]), "invalid input fingerprint")
        require(item.get("platform") == PLATFORM, "component platform mismatch")
        require(item.get("target") == COMPONENTS[name][2], "component target mismatch")
        if root is not None:
            ancestor(root, source, target)
            if not build_only:
                source_tag_commit = git(root, "rev-parse", f"refs/tags/{source_tag}^{{commit}}").decode().strip()
                require(source_tag_commit == source, "component source tag/commit mismatch")
            recorded = item["input_fingerprint"]
            if recorded == fingerprint(root, source, name):
                require(source == target or (build_recipe(root, source) is not None
                        and build_recipe(root, target) is not None), "unrecognized build inputs cannot be reused")
                require(recorded == fingerprint(root, target, name), "component release inputs changed")
            else:
                require(recorded == legacy_fingerprint(root, source, name), "component source inputs mismatch")
                require(recorded == legacy_fingerprint(root, target, name), "component release inputs changed")
    return manifest


def base_component(repository, commit, tag, run_id, name, fp, ref):
    return {"input_fingerprint": fp, "source_commit": commit, "source_tag": tag,
            "source_ref": ref, "source_run_id": str(run_id), "certificate_identity": identity(repository, ref),
            "certificate_oidc_issuer": ISSUER, "platform": PLATFORM, "target": COMPONENTS[name][2]}


def make_plan(root, repository, commit, tag, run_id, previous=None, ref=None):
    require(REPOSITORY.fullmatch(repository) is not None, "invalid repository")
    require(positive_id(run_id), "invalid release run")
    ref = ref or f"refs/tags/{tag}"
    production = bool(TAG.fullmatch(tag) and ref == f"refs/tags/{tag}")
    if not production:
        require(re.fullmatch(r"refs/heads/[A-Za-z0-9._/-]+", ref) and ".." not in ref and "//" not in ref
                and not ref.endswith(("/", ".")) and tag == ref.removeprefix("refs/heads/"),
                "invalid build-only release identity")
    commit_exists(root, commit)
    previous_components = {}
    if previous is not None and production:
        try:
            validate_manifest(previous, repository, root=root)
            ancestor(root, previous["commit"], commit)
            previous_components = previous["components"]
        except (ValueError, subprocess.CalledProcessError, KeyError, TypeError):
            # A reuse candidate is optional; invalid evidence means build anew.
            previous_components = {}
    components = {}
    matrix = []
    reusable = build_recipe(root, commit) is not None
    for name, (display, context, target) in COMPONENTS.items():
        fp = fingerprint(root, commit, name)
        item = {**base_component(repository, commit, tag, run_id, name, fp, ref), "action": "build", "image": ""}
        candidate = previous_components.get(name)
        if reusable and candidate and candidate["input_fingerprint"] == fp:
            ancestor(root, candidate["source_commit"], commit)
            item = {**candidate, "action": "reuse"}
        components[name] = item
        matrix.append({"name": name, "display": display, "context": context, "file": f"{context}/Dockerfile",
                       "target": target, "cache_scope": f"ideaboard-{name}", **item})
    return {"schema_version": 2, "repository": repository, "commit": commit, "tag": tag,
            "ref": ref, "release_kind": "production" if production else "build-only",
            "run_id": str(run_id), "components": components, "matrix": {"include": matrix}}


def component_metadata(plan, name, digest=None):
    require(isinstance(plan, dict), "invalid release plan")
    require(name in COMPONENTS and name in plan.get("components", {}), "unknown component")
    item = dict(plan["components"][name])
    action = item.pop("action")
    if action == "build":
        require(isinstance(digest, str) and FINGERPRINT.fullmatch(digest), "build requires immutable digest")
        item["image"] = f"ghcr.io/{plan['repository'].lower()}/ideaboard-{name}@{digest}"
    else:
        require(action == "reuse" and digest is None, "invalid reuse metadata")
    canonical_image(plan["repository"], name, item["image"])
    item.update(name=name, ref=item["image"], digest=item["image"].rsplit("@", 1)[1])
    return item


def make_manifest(plan, metadata, root):
    require(isinstance(plan, dict), "invalid release plan")
    require(set(metadata) == set(COMPONENTS), "incomplete image metadata")
    for name, item in metadata.items():
        expected = dict(plan["components"][name])
        expected.pop("action")
        if plan["components"][name]["action"] == "build":
            expected["image"] = item.get("image")
        expected.update(name=name, ref=expected["image"], digest=expected["image"].rsplit("@", 1)[1])
        require(item == expected, "metadata disagrees with release plan")
    manifest = {"schema_version": 2, "repository": plan["repository"], "tag": plan["tag"], "ref": plan["ref"],
                "commit": plan["commit"], "release_kind": plan["release_kind"], "workflow": {"name": "docker-images.yml", "run_id": plan["run_id"]},
                "images": {name: item["image"] for name, item in metadata.items()}, "components": metadata,
                "cosign": {"certificate_identity": identity(plan["repository"], plan["ref"]),
                           "certificate_oidc_issuer": ISSUER}}
    return validate_manifest(manifest, plan["repository"], root=root, allow_build_only=True)


def read_json(path):
    return json.loads(Path(path).read_text())


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--root", default=".")
    commands = parser.add_subparsers(dest="command", required=True)
    plan_parser = commands.add_parser("plan")
    for name in ["repository", "commit", "tag", "run-id"]:
        plan_parser.add_argument(f"--{name}", required=True)
    plan_parser.add_argument("--ref")
    plan_parser.add_argument("--previous")
    plan_parser.add_argument("--output")
    metadata_parser = commands.add_parser("metadata")
    metadata_parser.add_argument("--plan", required=True)
    metadata_parser.add_argument("--component", required=True)
    metadata_parser.add_argument("--digest")
    metadata_parser.add_argument("--output")
    manifest_parser = commands.add_parser("manifest")
    manifest_parser.add_argument("--plan", required=True)
    manifest_parser.add_argument("--metadata-dir", required=True)
    manifest_parser.add_argument("--output")
    validate_parser = commands.add_parser("validate")
    validate_parser.add_argument("--manifest", required=True)
    validate_parser.add_argument("--repository", required=True)
    validate_parser.add_argument("--commit")
    validate_parser.add_argument("--tag")
    validate_parser.add_argument("--verify-inputs", action="store_true")
    args = parser.parse_args()
    root = Path(args.root)
    try:
        if args.command == "plan":
            previous = None
            if args.previous:
                try:
                    previous = read_json(args.previous)
                except (OSError, ValueError):
                    pass
            result = make_plan(root, args.repository, args.commit, args.tag, args.run_id, previous, args.ref)
        elif args.command == "metadata":
            result = component_metadata(read_json(args.plan), args.component, args.digest)
        elif args.command == "manifest":
            metadata = {name: read_json(Path(args.metadata_dir) / f"{name}.json") for name in COMPONENTS}
            result = make_manifest(read_json(args.plan), metadata, root)
        else:
            result = validate_manifest(read_json(args.manifest), args.repository, args.commit, args.tag,
                                       root if args.verify_inputs else None)
        text = json.dumps(result, indent=2, sort_keys=True) + "\n"
        if getattr(args, "output", None):
            Path(args.output).write_text(text)
        else:
            print(text, end="")
    except (ValueError, OSError, KeyError, TypeError, subprocess.CalledProcessError) as error:
        print(f"release-artifacts: {error}", file=sys.stderr)
        raise SystemExit(1) from None


if __name__ == "__main__":
    main()
