import json
import os
import tempfile
import time
from typing import cast

from config import SINCE_TIMESTAMP, STATE_FILE
from models import (
    DeadEntry,
    EpisodeAction,
    FailedEntry,
    StateData,
    normalize_episode_action,
    string_key_dict,
)


MAX_ATTEMPTS = 5


def _default_state() -> StateData:
    return {
        "last_timestamp": int(SINCE_TIMESTAMP),
        "failed": {},
        "dead": {},
    }


def _normalize_failed_entry(value: object) -> FailedEntry | None:
    data = string_key_dict(value)
    if not data:
        return None

    attempts = data.get("attempts", 0)
    last_attempt_ts = data.get("last_attempt_ts", 0)
    if not isinstance(attempts, int) or not isinstance(last_attempt_ts, int):
        return None

    return {
        "attempts": attempts,
        "last_attempt_ts": last_attempt_ts,
        "action": normalize_episode_action(data.get("action")),
    }


def _normalize_dead_entry(value: object) -> DeadEntry | None:
    data = string_key_dict(value)
    if not data:
        return None

    attempts = data.get("attempts", 0)
    if not isinstance(attempts, int):
        return None

    return {
        "attempts": attempts,
        "action": normalize_episode_action(data.get("action")),
    }


def _load_state() -> StateData:
    """
    Loads the full state from disk, returning defaults if the file is missing or invalid.
    """
    if not os.path.exists(STATE_FILE):
        return _default_state()

    try:
        with open(STATE_FILE, "r", encoding="utf-8") as f:
            data = string_key_dict(cast(object, json.load(f)))
    except (json.JSONDecodeError, IOError):
        print(f"Warning: Could not read {STATE_FILE}. Using default state.")
        return _default_state()

    if not data:
        print(f"Warning: Invalid state format in {STATE_FILE}. Using default state.")
        return _default_state()

    state = _default_state()
    last_timestamp = data.get("last_timestamp", SINCE_TIMESTAMP)
    if isinstance(last_timestamp, bool):
        state["last_timestamp"] = int(last_timestamp)
    elif isinstance(last_timestamp, (int, float, str)):
        try:
            state["last_timestamp"] = int(last_timestamp)
        except ValueError:
            state["last_timestamp"] = int(SINCE_TIMESTAMP)
    else:
        state["last_timestamp"] = int(SINCE_TIMESTAMP)

    raw_failed = data.get("failed", {})
    if isinstance(raw_failed, dict):
        failed_entries: dict[str, FailedEntry] = {}
        for episode_url, raw_entry in cast(dict[object, object], raw_failed).items():
            if not isinstance(episode_url, str):
                continue
            entry = _normalize_failed_entry(raw_entry)
            if entry is not None:
                failed_entries[episode_url] = entry
        state["failed"] = failed_entries

    raw_dead = data.get("dead", {})
    if isinstance(raw_dead, dict):
        dead_entries: dict[str, DeadEntry] = {}
        for episode_url, raw_entry in cast(dict[object, object], raw_dead).items():
            if not isinstance(episode_url, str):
                continue
            entry = _normalize_dead_entry(raw_entry)
            if entry is not None:
                dead_entries[episode_url] = entry
        state["dead"] = dead_entries

    return state


def _write_state(state: StateData) -> None:
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


def load_last_timestamp() -> int:
    """
    Loads the last timestamp from state.json.
    If the file doesn't exist or is invalid, returns the default SINCE_TIMESTAMP from config.
    """
    return _load_state()["last_timestamp"]


def save_last_timestamp(timestamp: int) -> None:
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


def load_failed() -> dict[str, FailedEntry]:
    """
    Loads the failed episode queue from state.json.
    """
    return _load_state()["failed"]


def mark_failed(episode_url: str | None, action: EpisodeAction) -> bool:
    """
    Records a failed episode attempt and moves it to the dead letter queue after MAX_ATTEMPTS.
    """
    if not episode_url:
        print("Warning: Cannot mark failure without an episode URL.")
        return False

    state = _load_state()
    failed = state["failed"]
    dead = state["dead"]

    existing = failed.get(episode_url)
    attempts = (existing["attempts"] if existing is not None else 0) + 1
    entry: FailedEntry = {
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


def mark_succeeded(episode_url: str | None) -> None:
    """
    Removes a succeeded episode from the failed queue.
    """
    if not episode_url:
        return

    state = _load_state()
    if episode_url in state["failed"]:
        del state["failed"][episode_url]
        _write_state(state)
