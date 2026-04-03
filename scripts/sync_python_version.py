#!/usr/bin/env python3

from __future__ import annotations

import pathlib
import re
import sys


ROOT = pathlib.Path(__file__).resolve().parent.parent


def read_python_version() -> str:
    version = (ROOT / ".python-version").read_text(encoding="utf-8").strip()
    if not re.fullmatch(r"\d+\.\d+", version):
        raise ValueError(
            f".python-version must contain a major.minor version, got {version!r}"
        )
    return version


def replace_one(path: pathlib.Path, pattern: str, replacement: str) -> None:
    content = path.read_text(encoding="utf-8")
    updated, count = re.subn(pattern, replacement, content, count=1, flags=re.MULTILINE)
    if count != 1:
        raise ValueError(f"Expected exactly one match for {pattern!r} in {path}")
    path.write_text(updated, encoding="utf-8")


def main() -> int:
    version = read_python_version()
    version_digits = version.replace(".", "")

    replace_one(
        ROOT / "pyproject.toml",
        r'^requires-python = ">=\d+\.\d+"$',
        f'requires-python = ">={version}"',
    )
    replace_one(
        ROOT / "pyproject.toml",
        r'^pythonVersion = "\d+\.\d+"$',
        f'pythonVersion = "{version}"',
    )
    replace_one(
        ROOT / "Dockerfile",
        r"^ARG PYTHON_VERSION=\d+\.\d+$",
        f"ARG PYTHON_VERSION={version}",
    )
    replace_one(
        ROOT / "README.md",
        r"^    -   Python \d+\.\d+$",
        f"    -   Python {version}",
    )

    print(
        "Synchronized Python version "
        f"{version} (.python-version / pyproject.toml / Dockerfile / README.md / flake.nix)"
    )
    print(
        f"Nix dev shell will resolve package attribute python{version_digits} dynamically."
    )
    return 0


if __name__ == "__main__":
    sys.exit(main())
