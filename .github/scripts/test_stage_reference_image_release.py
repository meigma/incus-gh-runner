from __future__ import annotations

import contextlib
import hashlib
import importlib.util
import io
import tempfile
import unittest
from pathlib import Path


SCRIPT_PATH = Path(__file__).with_name("stage_reference_image_release.py")
SPEC = importlib.util.spec_from_file_location("stage_reference_image_release", SCRIPT_PATH)
assert SPEC is not None
assert SPEC.loader is not None
stage_reference_image_release = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(stage_reference_image_release)


class StageReferenceImageReleaseTest(unittest.TestCase):
    def test_stages_versioned_archive_and_checksums(self) -> None:
        with fixture() as (root, source):
            output = root / "dist/reference-image"
            result, stdout, stderr = run_script(source, output)

            self.assertEqual(result, 0, stderr)
            archive_name = (
                "incus-gh-runner-reference-image_1.2.3_ubuntu-24.04_x86_64.tar.xz"
            )
            archive = output / archive_name
            checksum_line = f"{sha256(archive)}  {archive_name}\n"
            self.assertEqual(archive.read_bytes(), source.read_bytes())
            self.assertEqual((output / f"{archive_name}.sha256").read_text(), checksum_line)
            self.assertEqual((output / "checksums.txt").read_text(), checksum_line)
            self.assertIn(archive.as_posix(), stdout)

    def test_accepts_semver_prerelease_tag(self) -> None:
        with fixture() as (root, source):
            result, _, stderr = run_script(source, root / "dist", tag="v1.2.3-rc.1")

            self.assertEqual(result, 0, stderr)

    def test_rejects_invalid_release_tag(self) -> None:
        with fixture() as (root, source):
            result, _, stderr = run_script(source, root / "dist", tag="release-1.2.3")

            self.assertEqual(result, 1)
            self.assertIn("release tag must match", stderr)

    def test_rejects_source_checksum_mismatch(self) -> None:
        with fixture(checksum="0" * 64) as (root, source):
            result, _, stderr = run_script(source, root / "dist")

            self.assertEqual(result, 1)
            self.assertIn("source checksum mismatch", stderr)

    def test_rejects_checksum_for_different_file(self) -> None:
        with fixture(checksum_name="other.tar.xz") as (root, source):
            result, _, stderr = run_script(source, root / "dist")

            self.assertEqual(result, 1)
            self.assertIn("source checksum must name", stderr)

    def test_refuses_to_overwrite_staged_assets(self) -> None:
        with fixture() as (root, source):
            output = root / "dist"
            first, _, first_stderr = run_script(source, output)
            second, _, second_stderr = run_script(source, output)

            self.assertEqual(first, 0, first_stderr)
            self.assertEqual(second, 1)
            self.assertIn("refusing to overwrite", second_stderr)


def run_script(
    source: Path,
    output: Path,
    *,
    tag: str = "v1.2.3",
) -> tuple[int, str, str]:
    stdout = io.StringIO()
    stderr = io.StringIO()
    with contextlib.redirect_stdout(stdout), contextlib.redirect_stderr(stderr):
        result = stage_reference_image_release.main(
            ["--tag", tag, "--source", str(source), "--output", str(output)]
        )
    return result, stdout.getvalue(), stderr.getvalue()


@contextlib.contextmanager
def fixture(*, checksum: str | None = None, checksum_name: str | None = None):
    with tempfile.TemporaryDirectory() as directory:
        root = Path(directory)
        source_dir = root / "build/reference-image"
        source_dir.mkdir(parents=True)
        source = source_dir / "incus-gh-runner-ubuntu-24.04-x86_64.tar.xz"
        source.write_bytes(b"reference image test artifact\n")
        digest = checksum or sha256(source)
        name = checksum_name or source.name
        source.with_name(f"{source.name}.sha256").write_text(
            f"{digest}  {name}\n",
            encoding="utf-8",
        )
        yield root, source


def sha256(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


if __name__ == "__main__":
    unittest.main()
