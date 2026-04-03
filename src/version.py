import os
import tomllib
from importlib.metadata import PackageNotFoundError, version as package_version

from models import string_key_dict


def get_app_version() -> str:
    try:
        return package_version("podgist")
    except PackageNotFoundError:
        pyproject_path = os.path.join(os.path.dirname(__file__), "..", "pyproject.toml")
        with open(pyproject_path, "rb") as f:
            data = tomllib.load(f)
        project = string_key_dict(data.get("project"))
        if not project:
            return "unknown"

        version = project.get("version")
        return version if isinstance(version, str) else "unknown"
