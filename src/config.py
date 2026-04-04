import os
import shutil
from pathlib import Path
from typing import Final, cast

import yaml
from dotenv import load_dotenv
from models import string_key_dict

# Load environment variables (secrets only)
load_dotenv()

YamlMapping = dict[str, object]

CONFIG_DIR: Final[Path] = Path("config")
CONFIG_FILE: Final[Path] = CONFIG_DIR / "config.yaml"
PROMPT_FILE_DEFAULT: Final[Path] = CONFIG_DIR / "prompt.md"
CHUNK_PROMPT_FILE_DEFAULT: Final[Path] = CONFIG_DIR / "prompt_chunk.md"
FINAL_PROMPT_FILE_DEFAULT: Final[Path] = CONFIG_DIR / "prompt_final.md"
CONFIG_EXAMPLE_DIR: Final[Path] = Path("config.example")
CONFIG_EXAMPLE_FILE: Final[Path] = CONFIG_EXAMPLE_DIR / "config.yaml"
PROMPT_EXAMPLE_FILE: Final[Path] = CONFIG_EXAMPLE_DIR / "prompt.md"
CHUNK_PROMPT_EXAMPLE_FILE: Final[Path] = CONFIG_EXAMPLE_DIR / "prompt_chunk.md"
FINAL_PROMPT_EXAMPLE_FILE: Final[Path] = CONFIG_EXAMPLE_DIR / "prompt_final.md"


def ensure_runtime_config_files() -> None:
    """
    Ensures the runtime config directory contains the required files.
    """
    CONFIG_DIR.mkdir(exist_ok=True)

    if not CONFIG_FILE.exists() and CONFIG_EXAMPLE_FILE.exists():
        shutil.copyfile(CONFIG_EXAMPLE_FILE, CONFIG_FILE)
        print(f"Created {CONFIG_FILE} from {CONFIG_EXAMPLE_FILE}.")

    if not PROMPT_FILE_DEFAULT.exists() and PROMPT_EXAMPLE_FILE.exists():
        shutil.copyfile(PROMPT_EXAMPLE_FILE, PROMPT_FILE_DEFAULT)
        print(f"Created {PROMPT_FILE_DEFAULT} from {PROMPT_EXAMPLE_FILE}.")

    if not CHUNK_PROMPT_FILE_DEFAULT.exists() and CHUNK_PROMPT_EXAMPLE_FILE.exists():
        shutil.copyfile(CHUNK_PROMPT_EXAMPLE_FILE, CHUNK_PROMPT_FILE_DEFAULT)
        print(f"Created {CHUNK_PROMPT_FILE_DEFAULT} from {CHUNK_PROMPT_EXAMPLE_FILE}.")

    if not FINAL_PROMPT_FILE_DEFAULT.exists() and FINAL_PROMPT_EXAMPLE_FILE.exists():
        shutil.copyfile(FINAL_PROMPT_EXAMPLE_FILE, FINAL_PROMPT_FILE_DEFAULT)
        print(f"Created {FINAL_PROMPT_FILE_DEFAULT} from {FINAL_PROMPT_EXAMPLE_FILE}.")


def load_yaml_config() -> YamlMapping:
    """
    Loads configuration from config/config.yaml.
    """
    ensure_runtime_config_files()

    if not CONFIG_FILE.exists():
        print("Warning: config/config.yaml not found. Using defaults only.")
        return {}

    try:
        with open(CONFIG_FILE, "r", encoding="utf-8") as f:
            loaded = yaml.safe_load(f)
    except Exception as e:
        print(f"Warning: Failed to load config from {CONFIG_FILE}: {e}")
        return {}

    if loaded is None:
        return {}
    if not isinstance(loaded, dict):
        print(f"Warning: Invalid config format in {CONFIG_FILE}. Using defaults only.")
        return {}
    return string_key_dict(cast(object, loaded))


_yaml_conf = load_yaml_config()


def get_config(key_path: str, default: object | None = None) -> object | None:
    """
    Retrieves a configuration value from the loaded YAML configuration.

    Args:
        key_path (str): Dot-notation path to the key (e.g. "section.subsection.key")
        default: Value to return if key is not found
    """
    keys = key_path.split(".")
    value: object = _yaml_conf
    for key in keys:
        mapping = string_key_dict(value)
        if not mapping:
            return default

        current = mapping.get(key)
        if current is None:
            return default
        value = current

    if value is not None:
        return value

    return default


def get_config_int(key_path: str, default: int) -> int:
    value = get_config(key_path, default)
    if isinstance(value, bool):
        return int(value)
    if isinstance(value, int):
        return value
    if isinstance(value, float):
        return int(value)
    if isinstance(value, str):
        try:
            return int(value.strip())
        except ValueError:
            return default
    return default


def get_config_str(key_path: str, default: str) -> str:
    value = get_config(key_path, default)
    if isinstance(value, str):
        return value
    return default


def get_config_mapping(
    key_path: str, default: dict[str, object] | None = None
) -> dict[str, object]:
    value = get_config(key_path, default or {})
    mapping = string_key_dict(value)
    return mapping if mapping else (default.copy() if default is not None else {})


def get_config_bool(key_path: str, default: bool = False) -> bool:
    """
    Retrieves a boolean config value with tolerant parsing.
    """
    value = get_config(key_path, default)
    if isinstance(value, bool):
        return value
    if isinstance(value, (int, float)):
        return bool(value)
    if isinstance(value, str):
        normalized = value.strip().lower()
        if normalized in {"true", "1", "yes", "on"}:
            return True
        if normalized in {"false", "0", "no", "off"}:
            return False
    return bool(default)


# --- Secrets (From Environment Only) ---
GPODDER_USERNAME: str | None = os.getenv("GPODDER_USERNAME")
GPODDER_PASSWORD: str | None = os.getenv("GPODDER_PASSWORD")
LLM_API_KEY: str | None = os.getenv("LLM_API_KEY") or os.getenv("OPENAI_API_KEY")
WHISPER_API_KEY: str | None = os.getenv("WHISPER_API_KEY")

AUTH: tuple[str | None, str | None] = (GPODDER_USERNAME, GPODDER_PASSWORD)

# --- Configuration (From YAML Only) ---

# gPodder
GPODDER_BASE_URL: str = get_config_str("gpodder.base_url", "").rstrip("/")
SINCE_TIMESTAMP: int = get_config_int("gpodder.since_timestamp", 0)

# Pipeline
PIPELINE_BATCH_SIZE: int = get_config_int("pipeline.batch_size", 1)
PIPELINE_CHUNK_TOKENS: int = get_config_int("pipeline.chunk_tokens", 3000)
PIPELINE_CHUNKING_THRESHOLD: int = get_config_int("pipeline.chunking_threshold", 4000)

# Paths
DOWNLOAD_DIR: str = get_config_str("paths.downloads", "data/downloads")
TRANSCRIPT_DIR: str = get_config_str("paths.transcripts", "data/transcripts")
SUMMARY_DIR: str = get_config_str("paths.summaries", "data/summaries")
STATE_FILE: str = get_config_str("paths.state_file", "data/state.json")
PROMPT_FILE: str = get_config_str("paths.prompt_file", str(PROMPT_FILE_DEFAULT))
CHUNK_PROMPT_FILE: str = get_config_str(
    "paths.prompt_chunk_file", "config/prompt_chunk.md"
)
FINAL_PROMPT_FILE: str = get_config_str(
    "paths.prompt_final_file", "config/prompt_final.md"
)

# LLM Configuration
LLM_BASE_URL: str = get_config_str("llm.base_url", "").rstrip("/")
LLM_MODEL: str = get_config_str("llm.model", "")
LLM_TIMEOUT: int = get_config_int("llm.timeout", 300)
LLM_EXTRA_BODY: dict[str, object] = get_config_mapping("llm.extra_body")
LLM_PROVIDER: str = get_config_str("llm.provider", "")
LLM_AUTO_PULL: bool = get_config_bool("llm.auto_pull", False)

# Whisper Configuration
WHISPER_BASE_URL: str = get_config_str(
    "whisper.base_url", "http://localhost:8000"
).rstrip("/")
WHISPER_MODEL: str = get_config_str("whisper.model", "base")
WHISPER_TIMEOUT: int = get_config_int("whisper.timeout", 600)
WHISPER_LANGUAGE: str = get_config_str("whisper.language", "")
WHISPER_PROMPT: str = get_config_str("whisper.prompt", "")
