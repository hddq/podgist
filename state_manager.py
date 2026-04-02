import json
import os
import tempfile
import time
from config import SINCE_TIMESTAMP, STATE_FILE


MAX_ATTEMPTS = 5


def _default_state():
    return {
        "last_timestamp": int(SINCE_TIMESTAMP),
        "failed": {},
        "dead": {},
    }


def _load_state():
    """
    Loads the full state from disk, returning defaults if the file is missing or invalid.
    """
    if not os.path.exists(STATE_FILE):
        return _default_state()

    try:
        with open(STATE_FILE, "r", encoding="utf-8") as f:
            data = json.load(f)
    except (json.JSONDecodeError, IOError):
        print(f"Warning: Could not read {STATE_FILE}. Using default state.")
        return _default_state()

    if not isinstance(data, dict):
        print(f"Warning: Invalid state format in {STATE_FILE}. Using default state.")
        return _default_state()

    state = _default_state()
    try:
        state["last_timestamp"] = int(data.get("last_timestamp", SINCE_TIMESTAMP))
    except (TypeError, ValueError):
        state["last_timestamp"] = int(SINCE_TIMESTAMP)
    state["failed"] = data.get("failed", {}) if isinstance(data.get("failed", {}), dict) else {}
    state["dead"] = data.get("dead", {}) if isinstance(data.get("dead", {}), dict) else {}
    return state


def _write_state(state):
    """
    Writes the full state atomically so the state file remains valid JSON.
    """
    directory = os.path.dirname(STATE_FILE) or "."
    os.makedirs(directory, exist_ok=True)

    fd, temp_path = tempfile.mkstemp(dir=directory, prefix=".state-", suffix=".json")
    try:
        with os.fdopen(fd, "w", encoding="utf-8") as f:
            json.dump(state, f)
            f.flush()
            os.fsync(f.fileno())
        os.replace(temp_path, STATE_FILE)
    except OSError:
        try:
            os.remove(temp_path)
        except OSError:
            pass
        raise


def load_last_timestamp():
    """
    Loads the last timestamp from state.json.
    If the file doesn't exist or is invalid, returns the default SINCE_TIMESTAMP from config.
    """
    return _load_state()["last_timestamp"]


def save_last_timestamp(timestamp):
    """
    Saves the given timestamp to state.json.
    """
    try:
        state = _load_state()
        state["last_timestamp"] = int(timestamp)
        _write_state(state)
        print(f"State saved: last_timestamp={int(timestamp)}")
    except IOError as e:
        print(f"Error saving state: {e}")


def load_failed():
    """
    Loads the failed episode queue from state.json.
    """
    return _load_state()["failed"]


def mark_failed(episode_url, action):
    """
    Records a failed episode attempt and moves it to the dead letter queue after MAX_ATTEMPTS.
    """
    if not episode_url:
        print("Warning: Cannot mark failure without an episode URL.")
        return False

    state = _load_state()
    failed = state["failed"]
    dead = state["dead"]

    existing = failed.get(episode_url, {})
    attempts = int(existing.get("attempts", 0)) + 1
    entry = {
        "attempts": attempts,
        "last_attempt_ts": int(time.time()),
        "action": action,
    }

    if attempts >= MAX_ATTEMPTS:
        dead[episode_url] = {
            "attempts": attempts,
            "action": action,
        }
        failed.pop(episode_url, None)
        _write_state(state)
        print(f"💀 Moved to dead letter queue after {attempts} attempts: {episode_url}")
        return True

    failed[episode_url] = entry
    _write_state(state)
    return False


def mark_succeeded(episode_url):
    """
    Removes a succeeded episode from the failed queue.
    """
    if not episode_url:
        return

    state = _load_state()
    if episode_url in state["failed"]:
        del state["failed"][episode_url]
        _write_state(state)
