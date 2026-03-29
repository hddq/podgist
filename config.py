import os
import yaml
from dotenv import load_dotenv

# Load environment variables (secrets only)
load_dotenv()

def load_yaml_config():
    """
    Loads configuration from config.yaml.
    Looks in current directory and /app/config/ (common for Docker/K8s).
    """
    candidates = [
        "config.local.yaml", # Local override (usually ignored by git)
        "config.yaml",       # Default project config
        "/app/config/config.yaml" # Docker/K8s volume mount
    ]
    
    for path in candidates:
        if os.path.exists(path):
            try:
                with open(path, 'r') as f:
                    return yaml.safe_load(f) or {}
            except Exception as e:
                print(f"Warning: Failed to load config from {path}: {e}")
    
    print("Warning: No config.yaml found. Using defaults only.")
    return {}

_yaml_conf = load_yaml_config()

def get_config(key_path, default=None):
    """
    Retrieves a configuration value from the loaded YAML configuration.
    
    Args:
        key_path (str): Dot-notation path to the key (e.g. "section.subsection.key")
        default: Value to return if key is not found
    """
    keys = key_path.split('.')
    value = _yaml_conf
    try:
        for k in keys:
            if isinstance(value, dict) and k in value:
                value = value[k]
            else:
                value = None
                break
    except Exception:
        value = None
        
    if value is not None:
        return value

    return default

def get_config_bool(key_path, default=False):
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
GPODDER_USERNAME = os.getenv("GPODDER_USERNAME")
GPODDER_PASSWORD = os.getenv("GPODDER_PASSWORD")
GEMINI_API_KEY = os.getenv("GEMINI_API_KEY")
WHISPER_API_KEY = os.getenv("WHISPER_API_KEY")

AUTH = (GPODDER_USERNAME, GPODDER_PASSWORD)

# --- Configuration (From YAML Only) ---

# gPodder
GPODDER_BASE_URL = get_config("gpodder.base_url", "").rstrip("/")
SINCE_TIMESTAMP = int(get_config("gpodder.since_timestamp", 0))

# Paths
DOWNLOAD_DIR = get_config("paths.downloads", "data/downloads")
TRANSCRIPT_DIR = get_config("paths.transcripts", "data/transcripts")
SUMMARY_DIR = get_config("paths.summaries", "data/summaries")
STATE_FILE = get_config("paths.state_file", "data/state.json")
PROMPT_FILE = get_config("paths.prompt_file", "prompt.md")

# LLM Configuration
LLM_PROVIDER = get_config("llm.provider", "gemini").lower()
GEMINI_MODEL = get_config("llm.gemini.model", "gemini-3-flash-preview")
OLLAMA_BASE_URL = get_config("llm.ollama.base_url", "http://localhost:11434")
OLLAMA_MODEL = get_config("llm.ollama.model", "llama3")
OLLAMA_AUTO_PULL = get_config_bool("llm.ollama.auto_pull", True)

# Whisper Server Configuration
WHISPER_BASE_URL = get_config("whisper.base_url", "http://localhost:8000").rstrip("/")
WHISPER_MODEL = get_config("whisper.model", "base")
WHISPER_TIMEOUT = int(get_config("whisper.timeout", 600))
