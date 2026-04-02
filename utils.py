import re
import xml.etree.ElementTree as ET
from functools import lru_cache
from datetime import datetime

import requests

from models import TimestampValue

def estimate_tokens(text: str) -> int:
    return len(text) // 4

def split_into_sentences(text: str) -> list[str]:
    sentences = re.split(r'(?<=[.!?])\s+', text.strip())
    return [sentence for sentence in sentences if sentence.strip()]

def chunk_transcript(text: str, chunk_tokens: int = 3000, overlap_sentences: int = 3) -> list[str]:
    sentences = split_into_sentences(text)
    if not sentences:
        return []

    chunks: list[str] = []
    current_sentences: list[str] = []

    for sentence in sentences:
        sentence_tokens = estimate_tokens(sentence)

        if not current_sentences:
            if sentence_tokens > chunk_tokens:
                chunks.append(sentence)
                continue
            current_sentences.append(sentence)
            continue

        candidate_sentences = current_sentences + [sentence]
        if estimate_tokens(" ".join(candidate_sentences)) <= chunk_tokens:
            current_sentences.append(sentence)
            continue

        chunks.append(" ".join(current_sentences))

        overlap: list[str] = (
            current_sentences[-overlap_sentences:] if overlap_sentences > 0 else []
        )
        while overlap and estimate_tokens(" ".join(overlap + [sentence])) > chunk_tokens:
            overlap = overlap[1:]

        if sentence_tokens > chunk_tokens:
            chunks.append(sentence)
            current_sentences = []
            continue

        current_sentences = overlap + [sentence]

    if current_sentences:
        chunks.append(" ".join(current_sentences))

    return chunks

def sanitize_filename(name: str | None) -> str:
    """
    Sanitizes a string to be safe for filenames.
    """
    if not name:
        return "unknown"
    # Keep alphanumeric, spaces, dots, underscores, dashes
    safe = "".join([c for c in name if c.isalnum() or c in (' ', '.', '_', '-')]).strip()
    return safe if safe else "unknown"


@lru_cache(maxsize=128)
def _fetch_podcast_feed_index(
    podcast_url: str,
) -> tuple[str | None, dict[str, str]]:
    headers = {"User-Agent": "Mozilla/5.0 (PodGist/1.0)"}
    resp = requests.get(podcast_url, headers=headers, timeout=30)
    resp.raise_for_status()

    root = ET.fromstring(resp.content)

    channel = root.find("channel")
    if channel is None and root.tag == "channel":
        channel = root

    podcast_title: str | None = None
    if channel is not None:
        title_text = channel.findtext("title")
        if title_text:
            podcast_title = title_text

    episode_titles: dict[str, str] = {}
    items = channel.findall("item") if channel is not None else []
    for item in items:
        title = item.findtext("title")
        if not title:
            continue

        enclosure = item.find("enclosure")
        if enclosure is not None:
            enclosure_url = enclosure.get("url")
            if enclosure_url:
                episode_titles[enclosure_url] = title

        guid = item.findtext("guid")
        if guid:
            episode_titles[guid] = title

    return podcast_title, episode_titles

def get_podcast_metadata(
    podcast_url: str | None,
    episode_url: str | None,
) -> tuple[str | None, str | None]:
    """
    Fetches the RSS feed at podcast_url and tries to find the title
    and the episode title for the given episode_url.
    Returns (podcast_title, episode_title).
    """
    if not podcast_url or not episode_url:
        return None, None

    try:
        podcast_title, episode_titles = _fetch_podcast_feed_index(podcast_url)
        episode_title = episode_titles.get(episode_url)
        if episode_title is None:
            for candidate_url, candidate_title in episode_titles.items():
                if episode_url in candidate_url or candidate_url in episode_url:
                    episode_title = candidate_title
                    break

        if podcast_title is None:
            podcast_title = "Unknown Podcast"
        if episode_title is None:
            episode_title = "Unknown Episode"
        return podcast_title, episode_title

    except Exception as e:
        print(f"Error fetching metadata for {podcast_url}: {e}")
        return None, None

def parse_timestamp(ts: TimestampValue) -> datetime | None:
    if ts is None:
        return None

    if isinstance(ts, str):
        if ts.isdigit():
            return datetime.fromtimestamp(int(ts))
        try:
            return datetime.fromisoformat(ts.replace("Z", "+00:00"))
        except ValueError:
            return None

    return datetime.fromtimestamp(int(ts))
