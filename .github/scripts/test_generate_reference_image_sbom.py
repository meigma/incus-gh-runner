from __future__ import annotations

import importlib.util
import json
import subprocess
import tempfile
import unittest
from pathlib import Path
from typing import Any


SCRIPT_PATH = Path(__file__).with_name("generate_reference_image_sbom.py")
SPEC = importlib.util.spec_from_file_location("generate_reference_image_sbom", SCRIPT_PATH)
assert SPEC is not None
assert SPEC.loader is not None
generate_reference_image_sbom = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(generate_reference_image_sbom)


class GenerateReferenceImageSBOMTest(unittest.TestCase):
    def test_generates_from_read_only_qcow2_copy(self) -> None:
        with fixture() as (root, archive):
            output = root / "image.cdx.json"
            syft_config = root / "syft.yaml"
            syft_config.write_text("file: {}\n", encoding="utf-8")
            runner = FakeRunner()

            generate_reference_image_sbom.generate_sbom(
                archive=archive,
                output=output,
                name="incus-gh-runner-reference-image",
                version="1.2.3",
                syft_config=syft_config,
                runner=runner,
            )

            self.assertTrue(output.is_file())
            guestfish = next(command for command in runner.commands if command[0] == "guestfish")
            self.assertIn("--format=qcow2", guestfish)
            self.assertIn("--ro", guestfish)
            self.assertIn("copy-out", guestfish)
            syft = next(command for command in runner.commands if command[0] == "syft")
            self.assertEqual(syft[syft.index("--config") + 1], str(syft_config.resolve()))

    def test_rejects_non_qcow2_rootfs(self) -> None:
        with fixture() as (root, archive):
            runner = FakeRunner(image_format="raw")

            with self.assertRaisesRegex(generate_reference_image_sbom.SBOMError, "must be qcow2"):
                generate_reference_image_sbom.generate_sbom(
                    archive=archive,
                    output=root / "image.cdx.json",
                    name="image",
                    version="1.2.3",
                    runner=runner,
                )

    def test_rejects_symlinked_rootfs_member(self) -> None:
        with fixture() as (root, archive):
            runner = FakeRunner(rootfs_symlink=True)

            with self.assertRaisesRegex(
                generate_reference_image_sbom.SBOMError,
                "non-empty rootfs.img",
            ):
                generate_reference_image_sbom.generate_sbom(
                    archive=archive,
                    output=root / "image.cdx.json",
                    name="image",
                    version="1.2.3",
                    runner=runner,
                )

    def test_reports_guestfish_failure_details(self) -> None:
        with fixture() as (root, archive):
            runner = FakeRunner(fail_command="guestfish")

            with self.assertRaisesRegex(
                generate_reference_image_sbom.SBOMError,
                "copy rootfs.*fixture failure",
            ):
                generate_reference_image_sbom.generate_sbom(
                    archive=archive,
                    output=root / "image.cdx.json",
                    name="image",
                    version="1.2.3",
                    runner=runner,
                )

    def test_rejects_empty_package_inventory(self) -> None:
        with fixture() as (root, archive):
            runner = FakeRunner(packages=[])

            with self.assertRaisesRegex(generate_reference_image_sbom.SBOMError, "contains no packages"):
                generate_reference_image_sbom.generate_sbom(
                    archive=archive,
                    output=root / "image.cdx.json",
                    name="image",
                    version="1.2.3",
                    runner=runner,
                )

    def test_refuses_to_overwrite_output(self) -> None:
        with fixture() as (root, archive):
            output = root / "image.cdx.json"
            output.write_text("existing", encoding="utf-8")

            with self.assertRaisesRegex(generate_reference_image_sbom.SBOMError, "refusing to overwrite"):
                generate_reference_image_sbom.generate_sbom(
                    archive=archive,
                    output=output,
                    name="image",
                    version="1.2.3",
                    runner=FakeRunner(),
                )


class FakeRunner:
    def __init__(
        self,
        *,
        image_format: str = "qcow2",
        packages: list[dict[str, Any]] | None = None,
        fail_command: str | None = None,
        rootfs_symlink: bool = False,
    ) -> None:
        self.image_format = image_format
        self.packages = (
            [{"type": "library", "name": "bash"}]
            if packages is None
            else packages
        )
        self.fail_command = fail_command
        self.rootfs_symlink = rootfs_symlink
        self.commands: list[list[str]] = []

    def __call__(self, command: list[str]) -> subprocess.CompletedProcess[str]:
        command = list(command)
        self.commands.append(command)
        if command[0] == self.fail_command:
            raise subprocess.CalledProcessError(
                1,
                command,
                stderr="fixture failure",
            )
        if command[0] == "tar":
            work = Path(command[command.index("--directory") + 1])
            if self.rootfs_symlink:
                target = work / "target.img"
                target.write_bytes(b"qcow2 fixture")
                (work / "rootfs.img").symlink_to(target)
            else:
                (work / "rootfs.img").write_bytes(b"qcow2 fixture")
        elif command[0] == "qemu-img":
            return subprocess.CompletedProcess(command, 0, json.dumps({"format": self.image_format}), "")
        elif command[0] == "guestfish":
            rootfs_directory = Path(command[-1])
            (rootfs_directory / "etc").mkdir()
            (rootfs_directory / "etc/os-release").write_text(
                "ID=ubuntu\nVERSION_ID=24.04\n",
                encoding="utf-8",
            )
        elif command[0] == "syft":
            output_arg = command[command.index("--output") + 1]
            output = Path(output_arg.removeprefix("cyclonedx-json="))
            name = command[command.index("--source-name") + 1]
            version = command[command.index("--source-version") + 1]
            output.write_text(
                json.dumps(
                    {
                        "bomFormat": "CycloneDX",
                        "specVersion": "1.6",
                        "serialNumber": "urn:uuid:fixture",
                        "metadata": {
                            "component": {
                                "type": "file",
                                "name": name,
                                "version": version,
                            }
                        },
                        "components": self.packages,
                    }
                ),
                encoding="utf-8",
            )
        return subprocess.CompletedProcess(command, 0, "", "")


class fixture:
    def __enter__(self) -> tuple[Path, Path]:
        self.temp = tempfile.TemporaryDirectory()
        root = Path(self.temp.name)
        archive = root / "image.tar.xz"
        archive.write_bytes(b"archive fixture")
        return root, archive

    def __exit__(self, *args: object) -> None:
        self.temp.cleanup()


if __name__ == "__main__":
    unittest.main()
