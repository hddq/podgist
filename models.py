from __future__ import annotations

from dataclasses import dataclass
from datetime import datetime
from typing import TypeAlias, TypedDict, cast


TimestampValue: TypeAlias = int | float | str | None


class EpisodeAction(TypedDict, total=False):
    action: str
    timestamp: TimestampValue
    episode: str
    podcast: str
    position: int | float | str
    total: int | float | str


class EpisodeActionsResponse(TypedDict, total=False):
    actions: list[EpisodeAction]


class FailedEntry(TypedDict):
    attempts: int
    last_attempt_ts: int
    action: EpisodeAction


class DeadEntry(TypedDict):
    attempts: int
    action: EpisodeAction


class StateData(TypedDict):
    last_timestamp: int
    failed: dict[str, FailedEntry]
    dead: dict[str, DeadEntry]


@dataclass
class WorkItem:
    action: EpisodeAction
    timestamp: datetime | None
    timestamp_value: int | None
    episode_url: str | None
    podcast_url: str | None
    relative_path: str | None
    filepath: str | None = None
    transcript_path: str | None = None
    summary_path: str | None = None
    download_ok: bool = False
    transcript_ready: bool = False
    succeeded: bool = False


def string_key_dict(value: object) -> dict[str, object]:
    if not isinstance(value, dict):
        return {}

    normalized: dict[str, object] = {}
    for key, item in cast(dict[object, object], value).items():
        if isinstance(key, str):
            normalized[key] = item
    return normalized


def normalize_episode_action(value: object) -> EpisodeAction:
    data = string_key_dict(value)
    action: EpisodeAction = {}

    action_name = data.get("action")
    if isinstance(action_name, str):
        action["action"] = action_name

    episode = data.get("episode")
    if isinstance(episode, str):
        action["episode"] = episode

    podcast = data.get("podcast")
    if isinstance(podcast, str):
        action["podcast"] = podcast

    timestamp = data.get("timestamp")
    if isinstance(timestamp, (int, float, str)) or timestamp is None:
        action["timestamp"] = timestamp

    position = data.get("position")
    if isinstance(position, (int, float, str)):
        action["position"] = position

    total = data.get("total")
    if isinstance(total, (int, float, str)):
        action["total"] = total

    return action
