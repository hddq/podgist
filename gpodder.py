from typing import cast

import requests

from config import GPODDER_BASE_URL, AUTH, SINCE_TIMESTAMP
from models import (
    EpisodeAction,
    EpisodeActionsResponse,
    normalize_episode_action,
    string_key_dict,
)


def fetch_episode_actions(since: int | None = None) -> EpisodeActionsResponse:
    username, password = AUTH
    if not GPODDER_BASE_URL or not username:
        raise ValueError("GPODDER_BASE_URL and GPODDER_USERNAME must be set in .env")

    timestamp = since if since is not None else SINCE_TIMESTAMP
    url = f"{GPODDER_BASE_URL}/api/2/episodes/{username}.json"
    params = {"since": timestamp}

    response = requests.get(
        url, auth=(username, password or ""), params=params, timeout=30
    )
    response.raise_for_status()

    payload = string_key_dict(response.json())
    if not payload:
        raise ValueError("Unexpected gPodder response format.")

    raw_actions = payload.get("actions", [])
    actions: list[EpisodeAction] = []
    if isinstance(raw_actions, list):
        actions = [
            normalize_episode_action(action)
            for action in cast(list[object], raw_actions)
        ]

    return {"actions": actions}
