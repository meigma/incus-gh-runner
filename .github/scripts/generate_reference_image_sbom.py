#!/usr/bin/env python3
"""Generate a CycloneDX SBOM from the final root filesystem in an Incus VM archive."""

from __future__ import annotations

import argparse
import json
import shutil
import stat
import subprocess
import sys
import tempfile
from collections.abc import Callable, Sequence
from pathlib import Path


MAX_ATTESTATION_SIZE = 16 * 1024 * 1024
CommandRunner = Callable[[Sequence[str]], subprocess.CompletedProcess[str]]


class SBOMError(RuntimeError):
    """SBOMError reports an invalid image, tool failure, or invalid SBOM."""


def parse_args(argv: list[str] | None = None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--archive", required=True, type=Path)
    parser.add_argument("--output", required=True, type=Path)
    parser.add_argument("--name", required=True, help="Name recorded in the SBOM")
    parser.add_argument("--version", required=True, help="Version recorded in the SBOM")
    parser.add_argument(
        "--syft-config",
        default=Path("image/sbom.syft.yaml"),
        type=Path,
        help="Syft configuration for the reference-image package inventory",
    )
    return parser.parse_args(argv)


def main(argv: list[str] | None = None) -> int:
    args = parse_args(argv)
    try:
        generate_sbom(
            archive=args.archive,
            output=args.output,
            name=args.name,
            version=args.version,
            syft_config=args.syft_config,
        )
    except SBOMError as exc:
        print(f"error: {exc}", file=sys.stderr)
        return 1
    print(args.output.as_posix())
    return 0


def generate_sbom(
    *,
    archive: Path,
    output: Path,
    name: str,
    version: str,
    syft_config: Path | None = None,
    runner: CommandRunner | None = None,
) -> None:
    if not archive.is_file() or archive.stat().st_size == 0:
        raise SBOMError(f"missing or empty reference-image archive {archive}")
    if output.exists():
        raise SBOMError(f"refusing to overwrite SBOM {output}")
    if not name.strip() or not version.strip():
        raise SBOMError("SBOM name and version must not be empty")
    if syft_config is not None and (
        not syft_config.is_file() or syft_config.stat().st_size == 0
    ):
        raise SBOMError(f"missing or empty Syft configuration {syft_config}")

    run = runner or run_command
    output.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.TemporaryDirectory(prefix="incus-gh-runner-sbom-") as directory:
        work = Path(directory)
        rootfs = work / "rootfs.img"
        rootfs_directory = work / "rootfs"
        generated = work / "sbom.cdx.json"
        rootfs_directory.mkdir()

        invoke(
            run,
            [
                "tar",
                "--extract",
                "--xz",
                "--file",
                str(archive),
                "--directory",
                str(work),
                "rootfs.img",
            ],
            "extract rootfs.img",
        )
        try:
            rootfs_stat = rootfs.lstat()
        except FileNotFoundError as exc:
            raise SBOMError("reference-image archive did not contain rootfs.img") from exc
        if not stat.S_ISREG(rootfs_stat.st_mode) or rootfs_stat.st_size == 0:
            raise SBOMError("reference-image archive did not contain a non-empty rootfs.img")

        image_info = invoke(
            run,
            ["qemu-img", "info", "--output=json", str(rootfs)],
            "inspect rootfs.img",
        )
        try:
            image_format = json.loads(image_info.stdout)["format"]
        except (json.JSONDecodeError, KeyError, TypeError) as exc:
            raise SBOMError("qemu-img returned invalid image metadata") from exc
        if image_format != "qcow2":
            raise SBOMError(f"rootfs.img must be qcow2, got {image_format!r}")

        invoke(
            run,
            [
                "guestfish",
                "--ro",
                "--format=qcow2",
                "--add",
                str(rootfs),
                "--inspector",
                "copy-out",
                "/",
                str(rootfs_directory),
            ],
            "copy rootfs.img read-only",
        )
        if not (rootfs_directory / "etc/os-release").is_file():
            raise SBOMError("copied rootfs.img does not contain /etc/os-release")
        syft_command = ["syft"]
        if syft_config is not None:
            syft_command.extend(["--config", str(syft_config.resolve())])
        syft_command.extend(
            [
                "scan",
                f"dir:{rootfs_directory}",
                "--source-name",
                name,
                "--source-version",
                version,
                "--output",
                f"cyclonedx-json={generated}",
            ]
        )
        invoke(run, syft_command, "generate CycloneDX SBOM")

        validate_sbom(generated, name=name, version=version)
        shutil.move(generated, output)


def validate_sbom(path: Path, *, name: str, version: str) -> None:
    if not path.is_file() or path.stat().st_size == 0:
        raise SBOMError("Syft did not produce a non-empty SBOM")
    if path.stat().st_size > MAX_ATTESTATION_SIZE:
        raise SBOMError(
            f"SBOM is {path.stat().st_size} bytes and exceeds GitHub's "
            f"{MAX_ATTESTATION_SIZE}-byte attestation limit"
        )
    try:
        document = json.loads(path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError) as exc:
        raise SBOMError("Syft produced invalid CycloneDX JSON") from exc
    if not isinstance(document, dict):
        raise SBOMError("Syft produced invalid CycloneDX JSON")
    if (
        document.get("bomFormat") != "CycloneDX"
        or document.get("specVersion") != "1.6"
        or not document.get("serialNumber")
    ):
        raise SBOMError("Syft SBOM must use CycloneDX 1.6")
    components = document.get("components", [])
    if not isinstance(components, list) or not all(
        isinstance(component, dict) for component in components
    ):
        raise SBOMError("Syft SBOM contains an invalid component inventory")
    metadata = document.get("metadata", {})
    if not isinstance(metadata, dict):
        raise SBOMError("Syft SBOM contains invalid metadata")
    source_component = metadata.get("component", {})
    if not isinstance(source_component, dict):
        raise SBOMError("Syft SBOM contains invalid source metadata")
    if source_component.get("name") != name or source_component.get("version") != version:
        raise SBOMError("Syft SBOM does not identify the requested image name and version")
    if not components:
        raise SBOMError("Syft SBOM contains no packages")


def invoke(
    runner: CommandRunner,
    command: Sequence[str],
    operation: str,
) -> subprocess.CompletedProcess[str]:
    try:
        return runner(command)
    except subprocess.CalledProcessError as exc:
        detail = (exc.stderr or exc.stdout or "").strip()
        if len(detail) > 2000:
            detail = detail[-2000:]
        suffix = f": {detail}" if detail else ""
        raise SBOMError(f"failed to {operation}{suffix}") from exc
    except OSError as exc:
        raise SBOMError(f"failed to {operation}: {exc}") from exc


def run_command(command: Sequence[str]) -> subprocess.CompletedProcess[str]:
    return subprocess.run(command, check=True, text=True, capture_output=True)


if __name__ == "__main__":
    raise SystemExit(main())
