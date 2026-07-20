#!/usr/bin/env python3
"""Stage and validate the versioned Incus reference-image release assets."""

from __future__ import annotations

import argparse
import hashlib
import json
import re
import shutil
import sys
from pathlib import Path


RELEASE_TAG = re.compile(r"^v(?P<version>[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z][0-9A-Za-z.-]*)?)$")
ASSET_TEMPLATE = "incus-gh-runner-reference-image_{version}_ubuntu-24.04_x86_64.tar.xz"
SBOM_TEMPLATE = "incus-gh-runner-reference-image_{version}_ubuntu-24.04_x86_64.cdx.json"
SBOM_NAME = "incus-gh-runner-reference-image"
MAX_ATTESTATION_SIZE = 16 * 1024 * 1024


class StageError(RuntimeError):
    """StageError reports an invalid or incomplete reference-image artifact."""


def parse_args(argv: list[str] | None = None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--tag", required=True, help="Release tag, for example v1.2.3")
    parser.add_argument(
        "--source",
        default=Path("build/reference-image/incus-gh-runner-ubuntu-24.04-x86_64.tar.xz"),
        type=Path,
    )
    parser.add_argument(
        "--sbom",
        default=Path("build/reference-image/incus-gh-runner-ubuntu-24.04-x86_64.cdx.json"),
        type=Path,
    )
    parser.add_argument("--output", default=Path("dist/reference-image"), type=Path)
    return parser.parse_args(argv)


def main(argv: list[str] | None = None) -> int:
    args = parse_args(argv)
    try:
        paths = stage_reference_image(
            tag=args.tag,
            source=args.source,
            source_sbom=args.sbom,
            output_dir=args.output,
        )
    except StageError as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 1

    for path in paths:
        print(path.as_posix())
    return 0


def stage_reference_image(
    *,
    tag: str,
    source: Path,
    source_sbom: Path,
    output_dir: Path,
) -> tuple[Path, Path, Path, Path, Path, Path]:
    version = release_version(tag)
    if not source.is_file() or source.stat().st_size == 0:
        raise StageError(f"missing or empty reference-image archive {source}")

    source_checksum = source.with_name(f"{source.name}.sha256")
    expected_digest, expected_name = parse_checksum(source_checksum)
    if expected_name != source.name:
        raise StageError(
            f"source checksum must name {source.name}, got {expected_name}"
        )

    actual_digest = sha256_file(source)
    if actual_digest != expected_digest:
        raise StageError(
            f"source checksum mismatch for {source.name}: "
            f"expected {expected_digest}, got {actual_digest}"
        )
    validate_sbom(source_sbom, version=version)
    sbom_digest = sha256_file(source_sbom)

    output_dir.mkdir(parents=True, exist_ok=True)
    archive = output_dir / ASSET_TEMPLATE.format(version=version)
    checksum = archive.with_name(f"{archive.name}.sha256")
    sbom = output_dir / SBOM_TEMPLATE.format(version=version)
    sbom_checksum = sbom.with_name(f"{sbom.name}.sha256")
    attestation_checksums = output_dir / "checksums.txt"
    sbom_subject_checksums = output_dir / "sbom-subject.checksums.txt"
    paths = (
        archive,
        checksum,
        sbom,
        sbom_checksum,
        attestation_checksums,
        sbom_subject_checksums,
    )
    collisions = [path for path in paths if path.exists()]
    if collisions:
        names = ", ".join(path.as_posix() for path in collisions)
        raise StageError(f"refusing to overwrite staged release asset(s): {names}")

    try:
        archive.hardlink_to(source)
    except OSError:
        shutil.copy2(source, archive)
    try:
        sbom.hardlink_to(source_sbom)
    except OSError:
        shutil.copy2(source_sbom, sbom)
    archive_checksum_line = f"{actual_digest}  {archive.name}\n"
    sbom_checksum_line = f"{sbom_digest}  {sbom.name}\n"
    checksum.write_text(archive_checksum_line, encoding="utf-8")
    sbom_checksum.write_text(sbom_checksum_line, encoding="utf-8")
    attestation_checksums.write_text(
        archive_checksum_line + sbom_checksum_line,
        encoding="utf-8",
    )
    sbom_subject_checksums.write_text(archive_checksum_line, encoding="utf-8")
    return paths


def release_version(tag: str) -> str:
    match = RELEASE_TAG.fullmatch(tag)
    if match is None:
        raise StageError(f"release tag must match vX.Y.Z or vX.Y.Z-prerelease: {tag}")
    return match.group("version")


def parse_checksum(path: Path) -> tuple[str, str]:
    try:
        lines = [line.strip() for line in path.read_text(encoding="utf-8").splitlines() if line.strip()]
    except FileNotFoundError as exc:
        raise StageError(f"missing source checksum {path}") from exc

    if len(lines) != 1:
        raise StageError(f"source checksum {path} must contain exactly one entry")

    parts = lines[0].split(None, 1)
    if len(parts) != 2:
        raise StageError(f"invalid source checksum entry: {lines[0]}")
    digest, raw_name = parts
    name = raw_name.removeprefix("*").strip()
    if len(digest) != 64 or any(character not in "0123456789abcdefABCDEF" for character in digest):
        raise StageError(f"invalid sha256 digest for {name}")
    if Path(name).name != name:
        raise StageError(f"source checksum must name one file, got {name!r}")
    return digest.lower(), name


def validate_sbom(path: Path, *, version: str) -> None:
    if not path.is_file() or path.stat().st_size == 0:
        raise StageError(f"missing or empty reference-image SBOM {path}")
    if path.stat().st_size > MAX_ATTESTATION_SIZE:
        raise StageError("reference-image SBOM exceeds GitHub's 16 MiB attestation limit")
    try:
        document = json.loads(path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as exc:
        raise StageError(f"invalid CycloneDX JSON in {path}") from exc
    if not isinstance(document, dict):
        raise StageError(f"invalid CycloneDX JSON in {path}")
    if (
        document.get("bomFormat") != "CycloneDX"
        or document.get("specVersion") != "1.6"
        or not document.get("serialNumber")
    ):
        raise StageError("reference-image SBOM must use CycloneDX 1.6")
    components = document.get("components", [])
    if not isinstance(components, list) or not all(
        isinstance(component, dict) for component in components
    ):
        raise StageError("reference-image SBOM contains an invalid component inventory")
    metadata = document.get("metadata", {})
    if not isinstance(metadata, dict):
        raise StageError("reference-image SBOM contains invalid metadata")
    source_component = metadata.get("component", {})
    if not isinstance(source_component, dict):
        raise StageError("reference-image SBOM contains invalid source metadata")
    if source_component.get("name") != SBOM_NAME or source_component.get("version") != version:
        raise StageError("reference-image SBOM does not identify this image release")
    if not components:
        raise StageError("reference-image SBOM contains no packages")


def sha256_file(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as source:
        for chunk in iter(lambda: source.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


if __name__ == "__main__":
    raise SystemExit(main())
