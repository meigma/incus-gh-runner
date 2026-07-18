#!/usr/bin/env python3
"""Stage and validate the versioned Incus reference-image release assets."""

from __future__ import annotations

import argparse
import hashlib
import re
import shutil
import sys
from pathlib import Path


RELEASE_TAG = re.compile(r"^v(?P<version>[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z][0-9A-Za-z.-]*)?)$")
ASSET_TEMPLATE = "incus-gh-runner-reference-image_{version}_ubuntu-24.04_x86_64.tar.xz"


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
    parser.add_argument("--output", default=Path("dist/reference-image"), type=Path)
    return parser.parse_args(argv)


def main(argv: list[str] | None = None) -> int:
    args = parse_args(argv)
    try:
        archive, checksum, attestation_checksums = stage_reference_image(
            tag=args.tag,
            source=args.source,
            output_dir=args.output,
        )
    except StageError as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 1

    for path in (archive, checksum, attestation_checksums):
        print(path.as_posix())
    return 0


def stage_reference_image(*, tag: str, source: Path, output_dir: Path) -> tuple[Path, Path, Path]:
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

    output_dir.mkdir(parents=True, exist_ok=True)
    archive = output_dir / ASSET_TEMPLATE.format(version=version)
    checksum = archive.with_name(f"{archive.name}.sha256")
    attestation_checksums = output_dir / "checksums.txt"
    collisions = [path for path in (archive, checksum, attestation_checksums) if path.exists()]
    if collisions:
        names = ", ".join(path.as_posix() for path in collisions)
        raise StageError(f"refusing to overwrite staged release asset(s): {names}")

    try:
        archive.hardlink_to(source)
    except OSError:
        shutil.copy2(source, archive)
    checksum_line = f"{actual_digest}  {archive.name}\n"
    checksum.write_text(checksum_line, encoding="utf-8")
    attestation_checksums.write_text(checksum_line, encoding="utf-8")
    return archive, checksum, attestation_checksums


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


def sha256_file(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as source:
        for chunk in iter(lambda: source.read(1024 * 1024), b""):
            digest.update(chunk)
    return digest.hexdigest()


if __name__ == "__main__":
    raise SystemExit(main())
