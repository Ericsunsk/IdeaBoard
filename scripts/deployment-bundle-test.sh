#!/usr/bin/env bash
set -euo pipefail
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PYTHONDONTWRITEBYTECODE=1 python3 - "$root" <<'PY'
import copy
import hashlib
import importlib.util
import io
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tarfile
import tempfile
import unittest

root = Path(sys.argv[1])

def load(name, path):
    spec = importlib.util.spec_from_file_location(name, path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module

bundle = load("deployment_bundle", root / "scripts/deployment-bundle.py")
release = load("release_artifacts", root / "scripts/release-artifacts.py")

class BundleContract(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.temporary = tempfile.TemporaryDirectory(prefix="ideaboard-bundle-tests-")
        cls.base = Path(cls.temporary.name).resolve()
        cls.repo = cls.base / "repository"
        subprocess.run(["git", "clone", "--shared", "--quiet", "--no-checkout", str(root), str(cls.repo)], check=True)
        cls.commit = subprocess.check_output(["git", "-C", str(root), "rev-parse", "HEAD"]).decode().strip()
        subprocess.run(["git", "-C", str(cls.repo), "checkout", "--detach", "--quiet", cls.commit], check=True)
        cls.tag = "v919.818.717"
        subprocess.run(["git", "-C", str(cls.repo), "tag", cls.tag], check=True)
        cls.repository = "Ericsunsk/IdeaBoard"
        plan = release.make_plan(cls.repo, cls.repository, cls.commit, cls.tag, "4567")
        metadata = {name: release.component_metadata(plan, name, "sha256:" + str(i+1) * 64)
                    for i, name in enumerate(release.COMPONENTS)}
        cls.manifest = release.make_manifest(plan, metadata, cls.repo)
        cls.manifest_path = cls.base / "release-manifest.json"
        cls.manifest_path.write_text(json.dumps(cls.manifest))
        cls.archive = cls.base / "deployment-bundle.tar"
        subprocess.run([sys.executable, str(root / "scripts/deployment-bundle.py"), "build", "--root", str(cls.repo),
                        "--manifest", str(cls.manifest_path), "--output", str(cls.archive)], check=True,
                       stdout=subprocess.DEVNULL)
        cls.raw = cls.archive.read_bytes()
        with tarfile.open(fileobj=io.BytesIO(cls.raw), mode="r:") as archive:
            cls.files = {m.name: (m.mode, archive.extractfile(m).read()) for m in archive}
        cls.bin = cls.base / "bin"
        cls.bin.mkdir()
        cosign = cls.bin / "cosign"
        cosign.write_text('''#!/usr/bin/env python3
import hashlib,json,pathlib,sys
args=sys.argv[1:]
if len(args)!=8 or args[0:2]!=['verify-blob','--bundle'] or args[3]!='--certificate-identity' or args[5]!='--certificate-oidc-issuer': sys.exit(81)
s=json.loads(pathlib.Path(args[2]).read_text())
if s!={'digest':hashlib.sha256(pathlib.Path(args[7]).read_bytes()).hexdigest(),'identity':args[4],'issuer':args[6]}:sys.exit(82)
''')
        cosign.chmod(0o755)
        cls.environment = dict(os.environ, PATH=str(cls.bin) + os.pathsep + os.environ["PATH"])

    @classmethod
    def tearDownClass(cls):
        cls.temporary.cleanup()

    def verify(self, raw=None, tag=None, commit=None, repository=None, signature_mutation=None, success=False):
        with tempfile.TemporaryDirectory(dir=self.base) as directory:
            directory = Path(directory)
            archive = directory / "release.tar"
            archive.write_bytes(self.raw if raw is None else raw)
            signature = {"digest": hashlib.sha256(archive.read_bytes()).hexdigest(),
                         "identity": bundle.identity(self.repository, self.tag), "issuer": bundle.ISSUER}
            if signature_mutation:
                signature_mutation(signature)
            signature_path = directory / "bundle.sigstore.json"
            signature_path.write_text(json.dumps(signature))
            output = directory / "deployed"
            result = subprocess.run([sys.executable, str(root / "scripts/deployment-bundle.py"), "verify",
                                     "--archive", str(archive), "--signature", str(signature_path),
                                     "--repository", repository or self.repository, "--tag", tag or self.tag,
                                     "--commit", commit or self.commit, "--output-dir", str(output)],
                                    env=self.environment, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
            if success:
                self.assertEqual(result.returncode, 0, result.stderr.decode())
                self.assertEqual(set(p.relative_to(output).as_posix() for p in output.rglob("*") if p.is_file()),
                                 {*bundle.FILES, bundle.INDEX, bundle.MANIFEST})
                self.assertFalse((output / ".git").exists())
                self.assertFalse((output / "apps").exists())
                actual = json.loads((output / bundle.MANIFEST).read_text())
                self.assertEqual(actual, self.manifest)
                self.assertEqual(json.loads(result.stdout)["archive_sha256"], bundle.digest(archive.read_bytes()))
            else:
                self.assertNotEqual(result.returncode, 0)
                self.assertFalse(output.exists(), "rejected archive wrote deployment files")

    def test_actual_build_verify_and_determinism(self):
        self.verify(success=True)
        second = self.base / "second.tar"
        bundle.build(self.repo, self.manifest_path, second)
        self.assertEqual(second.read_bytes(), self.raw)
        # Worktree changes are never packaged: CI packages the declared Git source.
        compose = self.repo / "docker/compose.yml"
        original = compose.read_bytes()
        compose.write_bytes(b"worktree injection")
        try:
            bundle.build(self.repo, self.manifest_path, second)
            self.assertEqual(second.read_bytes(), self.raw)
        finally:
            compose.write_bytes(original)

    def test_source_input_verification_rejects_forged_manifest(self):
        forged = copy.deepcopy(self.manifest)
        forged["components"]["api"]["input_fingerprint"] = "sha256:" + "0" * 64
        path = self.base / "forged.json"
        path.write_text(json.dumps(forged))
        with self.assertRaises(ValueError):
            bundle.build(self.repo, path, self.base / "forged.tar")

    def test_signer_source_and_signature_rejections(self):
        self.verify(tag="v919.818.718")
        self.verify(commit="f" * 40)
        self.verify(repository="attacker/IdeaBoard")
        self.verify(signature_mutation=lambda s: s.update(digest="0" * 64))
        self.verify(signature_mutation=lambda s: s.update(issuer="https://attacker.example"))
        self.verify(signature_mutation=lambda s: s.update(digest="x" * (bundle.MAX_SIGNATURE_BYTES + 1)))
        self.verify(signature_mutation=lambda s: s.update(identity=s["identity"].replace("refs/tags", "refs/heads")))

    def test_signed_content_integrity_and_identity(self):
        # Even signed malformed archives are refused; signing alone cannot authorize extras.
        for name in ("docker/compose.yml", bundle.MANIFEST):
            files = dict(self.files)
            files[name] = (files[name][0], files[name][1] + b"\nchanged")
            self.verify(bundle.archive_bytes(files))
        for key, value in (("tag", "v1.0.0"), ("repository", "other/repo"), ("commit", "f" * 40),
                           ("schema_version", 2), ("extra", True)):
            files = dict(self.files)
            index = json.loads(files[bundle.INDEX][1])
            index[key] = value
            files[bundle.INDEX] = (0o644, bundle.json_bytes(index))
            self.verify(bundle.archive_bytes(files))
        files = dict(self.files)
        files[bundle.INDEX] = (0o644, files[bundle.INDEX][1].replace(b'"schema_version": 1',
                                                                 b'"schema_version": 1, "schema_version": 1'))
        self.verify(bundle.archive_bytes(files))
        files = dict(self.files)
        del files[".infisical.yaml"]
        self.verify(bundle.archive_bytes(files))

    def test_archive_path_and_type_refusals(self):
        for name in ("../escape", "/etc/passwd", "apps/web/source.ts", ".git/config", "docker//compose.yml"):
            files = dict(self.files)
            files[name] = (0o644, b"unexpected")
            self.verify(bundle.archive_bytes(files))
        for member_type in (tarfile.SYMTYPE, tarfile.LNKTYPE, tarfile.CHRTYPE, tarfile.DIRTYPE):
            raw = io.BytesIO()
            with tarfile.open(fileobj=raw, mode="w", format=tarfile.USTAR_FORMAT) as archive:
                for name, (mode, data) in sorted(self.files.items()):
                    member = tarfile.TarInfo(name)
                    member.mode = mode
                    if name == "docker/compose.yml":
                        member.type = member_type
                        member.linkname = "../../escape"
                    else:
                        member.size = len(data)
                    archive.addfile(member, io.BytesIO(data))
            self.verify(raw.getvalue())
        raw = io.BytesIO()
        with tarfile.open(fileobj=raw, mode="w", format=tarfile.USTAR_FORMAT) as archive:
            for name, (mode, data) in sorted(self.files.items()):
                for _ in range(2 if name == ".infisical.yaml" else 1):
                    member = tarfile.TarInfo(name)
                    member.mode = mode
                    member.size = len(data)
                    archive.addfile(member, io.BytesIO(data))
        self.verify(raw.getvalue())
        self.verify(self.raw + b"extra transport data")
        self.verify(self.raw[:600])

    def test_bounds_output_and_mode_refusals(self):
        self.verify(b"x" * (bundle.MAX_ARCHIVE_BYTES + 1))
        files = dict(self.files)
        files[bundle.INDEX] = (0o644, b" " * (bundle.MAX_INDEX_BYTES + 1))
        self.verify(bundle.archive_bytes(files))
        files = dict(self.files)
        files["scripts/deploy-stack.sh"] = (0o4755, files["scripts/deploy-stack.sh"][1])
        self.verify(bundle.archive_bytes(files))
        with tempfile.TemporaryDirectory(dir=self.base) as directory:
            directory = Path(directory)
            output = directory / "nonempty"
            output.mkdir()
            (output / "keep").write_text("existing")
            with self.assertRaises(ValueError):
                bundle.extract_files(self.files, output)
            link = directory / "link"
            link.symlink_to(output, target_is_directory=True)
            with self.assertRaises(ValueError):
                bundle.extract_files(self.files, link)
            self.assertEqual((output / "keep").read_text(), "existing")

unittest.main(argv=[sys.argv[0]], verbosity=2)
PY
