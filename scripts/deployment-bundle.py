#!/usr/bin/env python3
"""Build source-verified deployment archives and verify them without a repository."""

import argparse
import hashlib
import io
import json
import os
from pathlib import Path
import re
import stat
import subprocess
import sys
import tarfile
import tempfile
import types

MAX_ARCHIVE_BYTES = 64 * 1024 * 1024
MAX_SIGNATURE_BYTES = 1024 * 1024
MAX_INDEX_BYTES = 65536
ISSUER = "https://token.actions.githubusercontent.com"
TAG = re.compile(r"v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)")
SHA = re.compile(r"[0-9a-f]{40}")
REPOSITORY = re.compile(r"[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+")
FILES = {
    ".infisical.yaml": 0o644,
    "docker/compose.yml": 0o644,
    "docker/compose.npm.yml": 0o644,
    "docker/.env.example": 0o644,
    "docker/images.env.example": 0o644,
    "docker/postgres/init-runtime-roles.sh": 0o755,
    "scripts/deploy-stack.sh": 0o755,
    "scripts/infisical-run.sh": 0o755,
    "scripts/release-artifacts.py": 0o755,
}
MANIFEST = "release-manifest.json"
INDEX = "deployment-index.json"


def require(condition, message):
    if not condition:
        raise ValueError(message)


def unique_object(pairs):
    result = dict(pairs)
    require(len(result) == len(pairs), "duplicate JSON field")
    return result


def json_bytes(value):
    return (json.dumps(value, sort_keys=True, indent=2) + "\n").encode()


def parse_json(data):
    return json.loads(data, object_pairs_hook=unique_object)


def digest(data):
    return "sha256:" + hashlib.sha256(data).hexdigest()


def identity(repository, tag):
    return f"https://github.com/{repository}/.github/workflows/docker-images.yml@refs/tags/{tag}"


def validate_identity(repository, tag, commit):
    require(isinstance(repository, str) and REPOSITORY.fullmatch(repository), "invalid repository")
    require(isinstance(tag, str) and TAG.fullmatch(tag), "invalid release tag")
    require(commit is None or isinstance(commit, str) and SHA.fullmatch(commit), "invalid source commit")


def archive_bytes(files):
    output = io.BytesIO()
    with tarfile.open(fileobj=output, mode="w", format=tarfile.USTAR_FORMAT) as archive:
        for name, (mode, data) in sorted(files.items()):
            member = tarfile.TarInfo(name)
            member.mode = mode
            member.size = len(data)
            archive.addfile(member, io.BytesIO(data))
    return output.getvalue()


def source_file(root, commit, name):
    entry = subprocess.check_output(["git", "-C", str(root), "ls-tree", commit, "--", name],
                                    stderr=subprocess.DEVNULL).decode().strip()
    require(entry and entry.split()[0] in ("100644", "100755") and entry.split("\t", 1)[1] == name,
            "deployment input must be a tracked regular file")
    return subprocess.check_output(["git", "-C", str(root), "show", f"{commit}:{name}"],
                                   stderr=subprocess.DEVNULL)


def build(root, manifest_path, output):
    manifest_data = Path(manifest_path).read_bytes()
    require(0 < len(manifest_data) <= MAX_INDEX_BYTES, "invalid release manifest size")
    manifest = parse_json(manifest_data)
    repository, tag, commit = manifest["repository"], manifest["tag"], manifest["commit"]
    validate_identity(repository, tag, commit)
    current = subprocess.check_output(["git", "-C", str(root), "rev-parse", "HEAD"],
                                      stderr=subprocess.DEVNULL).decode().strip()
    require(current == commit, "checkout does not match manifest source")
    # Load the validator from the declared Git source, not a modified worktree.
    validator = types.ModuleType("deployment_release_validator")
    validator_source = source_file(root, commit, "scripts/release-artifacts.py")
    exec(compile(validator_source, "scripts/release-artifacts.py", "exec"), validator.__dict__)
    validator.validate_manifest(manifest, repository, commit, tag, root=Path(root))
    files = {name: (mode, source_file(root, commit, name)) for name, mode in FILES.items()}
    files[MANIFEST] = (0o644, json_bytes(manifest))
    index = {"schema_version": 1, "repository": repository, "tag": tag, "commit": commit,
             "ref": f"refs/tags/{tag}", "workflow": manifest["workflow"],
             "files": {name: {"mode": mode, "sha256": digest(data)}
                       for name, (mode, data) in files.items()}}
    files[INDEX] = (0o644, json_bytes(index))
    raw = archive_bytes(files)
    require(len(raw) <= MAX_ARCHIVE_BYTES, "deployment archive is too large")
    Path(output).write_bytes(raw)
    return {"archive_sha256": digest(raw), "repository": repository, "tag": tag, "commit": commit}


def bounded_file(path, limit):
    with open(path, "rb") as source:
        info = os.fstat(source.fileno())
        require(stat.S_ISREG(info.st_mode) and 0 < info.st_size <= limit, "invalid input file size/type")
        data = source.read(limit + 1)
    require(0 < len(data) <= limit, "input exceeds size limit")
    return data


def verified_files(raw, repository, tag, commit):
    files = {}
    total = 0
    with tarfile.open(fileobj=io.BytesIO(raw), mode="r:") as archive:
        for member in archive:
            require(member.name in {*FILES, MANIFEST, INDEX} and member.name not in files,
                    "unexpected or duplicate archive path")
            require(member.type == tarfile.REGTYPE and not member.pax_headers,
                    "archive contains a nonregular file or extension")
            expected_mode = FILES.get(member.name, 0o644)
            require(member.mode == expected_mode and member.uid == 0 and member.gid == 0
                    and member.mtime == 0 and member.uname == "" and member.gname == "",
                    "noncanonical archive metadata")
            require(0 <= member.size <= MAX_ARCHIVE_BYTES, "archive member exceeds size limit")
            total += member.size
            require(total <= MAX_ARCHIVE_BYTES, "expanded archive exceeds size limit")
            if member.name in (INDEX, MANIFEST):
                require(member.size <= MAX_INDEX_BYTES, "JSON metadata exceeds size limit")
            data = archive.extractfile(member).read()
            require(len(data) == member.size, "truncated archive member")
            files[member.name] = (member.mode, data)
    require(set(files) == {*FILES, MANIFEST, INDEX}, "incomplete deployment archive")
    require(raw == archive_bytes(files), "noncanonical archive encoding or trailing content")
    index = parse_json(files[INDEX][1])
    require(isinstance(index, dict) and set(index) ==
            {"schema_version", "repository", "tag", "ref", "commit", "workflow", "files"},
            "invalid deployment index")
    require(type(index["schema_version"]) is int and index["schema_version"] == 1,
            "unsupported deployment index")
    require(index["repository"] == repository and index["tag"] == tag
            and index["ref"] == f"refs/tags/{tag}", "deployment identity mismatch")
    require(isinstance(index["commit"], str) and SHA.fullmatch(index["commit"]), "invalid indexed source commit")
    validate_identity(repository, tag, index["commit"])
    workflow = index["workflow"]
    require(isinstance(workflow, dict) and set(workflow) == {"name", "run_id"}
            and workflow["name"] == "docker-images.yml"
            and ((type(workflow["run_id"]) is int and workflow["run_id"] > 0)
                 or isinstance(workflow["run_id"], str) and re.fullmatch(r"[1-9][0-9]*", workflow["run_id"])),
            "invalid indexed workflow")
    require(commit is None or index["commit"] == commit, "deployment source mismatch")
    expected_files = {name: {"mode": mode, "sha256": digest(data)}
                      for name, (mode, data) in files.items() if name != INDEX}
    require(index["files"] == expected_files, "deployment content digest/mode mismatch")
    manifest = parse_json(files[MANIFEST][1])
    require(isinstance(manifest, dict) and manifest.get("schema_version") == 2
            and manifest.get("release_kind") == "production", "invalid production image manifest")
    for key in ("repository", "tag", "ref", "commit", "workflow"):
        require(manifest.get(key) == index[key], "image manifest identity mismatch")
    require(manifest.get("cosign") == {"certificate_identity": identity(repository, tag),
                                      "certificate_oidc_issuer": ISSUER}, "image manifest signer mismatch")
    return files, index


def extract_files(files, output):
    output = Path(output).absolute()
    require(not any(path.is_symlink() for path in (output, *output.parents)), "symlink output path")
    if output.exists():
        require(output.is_dir() and not any(output.iterdir()), "output directory must be empty")
    else:
        require(output.parent.is_dir(), "output parent must exist")
        output.mkdir(mode=0o700)
    for name, (mode, data) in sorted(files.items()):
        target = output / name
        target.parent.mkdir(mode=0o700, parents=True, exist_ok=True)
        descriptor = os.open(target, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600)
        with os.fdopen(descriptor, "wb") as destination:
            destination.write(data)
            os.fchmod(destination.fileno(), mode)


def verify(archive_path, signature_path, repository, tag, commit, output):
    validate_identity(repository, tag, commit)
    raw = bounded_file(archive_path, MAX_ARCHIVE_BYTES)
    signature = bounded_file(signature_path, MAX_SIGNATURE_BYTES)
    # Cosign and the archive parser see the same private snapshots, even if an
    # incoming path is replaced during signature verification.
    with tempfile.TemporaryDirectory(prefix="ideaboard-bundle-verify-") as temporary:
        archive = Path(temporary) / "deployment-bundle.tar"
        signature_file = Path(temporary) / "deployment-bundle.sigstore.json"
        archive.write_bytes(raw)
        signature_file.write_bytes(signature)
        subprocess.run(["cosign", "verify-blob", "--bundle", str(signature_file),
                        "--certificate-identity", identity(repository, tag),
                        "--certificate-oidc-issuer", ISSUER, str(archive)],
                       check=True, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    files, index = verified_files(raw, repository, tag, commit)
    extract_files(files, output)
    return {"archive_sha256": digest(raw), "repository": repository, "tag": tag, "commit": index["commit"]}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest="command", required=True)
    builder = commands.add_parser("build")
    builder.add_argument("--root", required=True)
    builder.add_argument("--manifest", required=True)
    builder.add_argument("--output", required=True)
    verifier = commands.add_parser("verify")
    for name in ("archive", "signature", "repository", "tag", "output-dir"):
        verifier.add_argument(f"--{name}", required=True)
    verifier.add_argument("--commit")
    args = parser.parse_args()
    try:
        if args.command == "build":
            result = build(args.root, args.manifest, args.output)
        else:
            result = verify(args.archive, args.signature, args.repository, args.tag, args.commit, args.output_dir)
        print(json.dumps(result, sort_keys=True))
    except (ValueError, OSError, KeyError, TypeError, tarfile.TarError, subprocess.CalledProcessError) as error:
        print(f"deployment-bundle: rejected ({type(error).__name__})", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
