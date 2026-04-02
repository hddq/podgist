import os
import sys
import time
import tomllib
from collections.abc import Iterator
from datetime import datetime
from importlib.metadata import PackageNotFoundError, version as package_version

from config import PIPELINE_BATCH_SIZE, TRANSCRIPT_DIR, SUMMARY_DIR
from downloader import download_file
from gpodder import fetch_episode_actions
from models import EpisodeAction, WorkItem, string_key_dict
from state_manager import (
    load_failed,
    load_last_timestamp,
    mark_failed,
    mark_succeeded,
    save_last_timestamp,
)
from summarizer import summarize
from transcriber import transcribe
from utils import get_podcast_metadata, parse_timestamp, sanitize_filename

POLL_INTERVAL = 600  # 10 minutes


def get_app_version() -> str:
    try:
        return package_version("podgist")
    except PackageNotFoundError:
        pyproject_path = os.path.join(os.path.dirname(__file__), "pyproject.toml")
        with open(pyproject_path, "rb") as f:
            data = tomllib.load(f)
        project = string_key_dict(data.get("project"))
        if not project:
            return "unknown"

        version = project.get("version")
        return version if isinstance(version, str) else "unknown"


def build_work_item(action: EpisodeAction) -> WorkItem:
    raw_ts = action.get("timestamp")
    dt = parse_timestamp(raw_ts)
    time_str = dt.isoformat() if dt else "unknown"
    episode_url = action.get("episode")
    podcast_url = action.get("podcast")

    print("─" * 80)
    print(f"🕒 {time_str}")
    print(f"📡 Podcast: {podcast_url}")
    print(f"🎙 Episode: {episode_url}")
    print(f"▶️  Position: {action.get('position')} / {action.get('total')}")

    relative_path = None
    if episode_url:
        podcast_title, episode_title = get_podcast_metadata(podcast_url, episode_url)

        if not podcast_title:
            podcast_title = "Unknown Podcast"
        if not episode_title:
            episode_title = episode_url.split("/")[-1]
            if "?" in episode_title:
                episode_title = episode_title.split("?")[0]

        safe_podcast = sanitize_filename(podcast_title)
        safe_episode = sanitize_filename(episode_title)

        if not safe_episode.lower().endswith(".mp3") and not safe_episode.lower().endswith(".m4a"):
            safe_episode += ".mp3"

        relative_path = os.path.join(safe_podcast, safe_episode)

    return WorkItem(
        action=action,
        timestamp=dt,
        timestamp_value=int(dt.timestamp()) if dt else None,
        episode_url=episode_url,
        podcast_url=podcast_url,
        relative_path=relative_path,
        transcript_path=(
            os.path.join(TRANSCRIPT_DIR, relative_path + ".txt")
            if relative_path
            else None
        ),
        summary_path=(
            os.path.join(SUMMARY_DIR, relative_path + ".md")
            if relative_path
            else None
        ),
    )


def cleanup_audio_file(filepath: str | None) -> None:
    if not filepath:
        return

    try:
        os.remove(filepath)
        print(f"🧹 Removed audio file: {filepath}")
    except FileNotFoundError:
        return
    except OSError as e:
        print(f"Warning: failed to remove audio file {filepath}: {e}")


def chunk_items[T](items: list[T], chunk_size: int) -> Iterator[list[T]]:
    for index in range(0, len(items), chunk_size):
        yield items[index:index + chunk_size]


def prepare_work_item(item: WorkItem) -> bool:
    """
    Applies early success shortcuts before batch phase processing.
    """
    if not item.episode_url:
        print("⚠️ No episode URL found, skipping episode.")
        item.succeeded = True
        return True

    if item.summary_path and os.path.exists(item.summary_path):
        print(f"Summary already exists: {item.summary_path}")
        item.succeeded = True
        return True

    if item.transcript_path and os.path.exists(item.transcript_path):
        print(f"Transcript already exists: {item.transcript_path}")
        item.transcript_ready = True
        return False

    return False


def process_episode_batch(batch_items: list[WorkItem], label: str) -> list[WorkItem]:
    print(f"\n📦 Processing batch {label} ({len(batch_items)} episode(s))")

    for item in batch_items:
        prepare_work_item(item)

    print("⬇️  Phase 1: Download all")
    for item in batch_items:
        if item.succeeded or item.transcript_ready:
            continue

        if item.episode_url is None:
            continue

        item.filepath = download_file(
            item.episode_url,
            relative_path=item.relative_path,
        )
        item.download_ok = bool(item.filepath)

    print("📝 Phase 2: Transcribe all")
    for item in batch_items:
        if item.succeeded or item.transcript_ready:
            continue
        if not item.download_ok or item.filepath is None:
            continue

        item.transcript_path = transcribe(item.filepath)
        item.transcript_ready = bool(item.transcript_path)

    print("🧠 Phase 3: Summarize all")
    for item in batch_items:
        if item.succeeded or not item.transcript_ready or item.transcript_path is None:
            continue

        item.summary_path = summarize(item.transcript_path)
        if item.summary_path:
            item.succeeded = True
            cleanup_audio_file(item.filepath)

    return batch_items


def deduplicate_actions(actions: list[EpisodeAction]) -> list[EpisodeAction]:
    deduped: list[EpisodeAction] = []
    by_episode_url: dict[str, int] = {}

    for action in actions:
        episode_url = action.get("episode")
        if not episode_url:
            deduped.append(action)
            continue

        parsed_ts = parse_timestamp(action.get("timestamp"))
        timestamp_value = int(parsed_ts.timestamp()) if parsed_ts else float("-inf")
        existing_index = by_episode_url.get(episode_url)

        if existing_index is None:
            by_episode_url[episode_url] = len(deduped)
            deduped.append(action)
            continue

        existing_action = deduped[existing_index]
        existing_ts = parse_timestamp(existing_action.get("timestamp"))
        existing_value = int(existing_ts.timestamp()) if existing_ts else float("-inf")
        if timestamp_value >= existing_value:
            deduped[existing_index] = action

    return deduped


def process_batched_work_items(
    work_items: list[WorkItem],
    batch_size: int,
    batch_label_prefix: str,
) -> tuple[int, int, int]:
    succeeded_count = 0
    failed_count = 0
    dead_count = 0

    for batch_index, batch_items in enumerate(chunk_items(work_items, batch_size), start=1):
        process_episode_batch(batch_items, f"{batch_label_prefix}{batch_index}")

        for item in batch_items:
            episode_url = item.episode_url
            if item.succeeded:
                if episode_url:
                    mark_succeeded(episode_url)
                succeeded_count += 1
                continue

            moved_to_dead = mark_failed(episode_url, item.action)
            if moved_to_dead:
                dead_count += 1
            else:
                failed_count += 1

    return succeeded_count, failed_count, dead_count


def process_actions(since_ts: int) -> int:
    """
    Fetches and processes actions since the given timestamp.
    Returns the updated timestamp after processing all fetched actions.
    """
    succeeded_count = 0
    failed_count = 0
    dead_count = 0

    failed_queue = load_failed()
    print(f"📬 Failed queue at poll start: {len(failed_queue)} episode(s)")

    if failed_queue:
        print("\n🔁 Retrying failed episodes...")
    retry_actions = [
        failed_entry["action"]
        for failed_entry in failed_queue.values()
    ]
    retry_actions.sort(key=lambda a: parse_timestamp(a.get("timestamp")) or datetime.min)
    checkpoint_batch_size = PIPELINE_BATCH_SIZE if PIPELINE_BATCH_SIZE > 0 else 1
    print(
        f"ℹ️ Processing episodes in staged batches "
        f"(batch size: {checkpoint_batch_size})."
    )

    if retry_actions:
        retry_work_items = [build_work_item(action) for action in retry_actions]
        retry_succeeded, retry_failed, retry_dead = process_batched_work_items(
            retry_work_items,
            checkpoint_batch_size,
            "retry ",
        )
        succeeded_count += retry_succeeded
        failed_count += retry_failed
        dead_count += retry_dead

    try:
        data = fetch_episode_actions(since=since_ts)
    except Exception as e:
        print(f"Failed to fetch actions: {e}")
        print(f"\n✅ {succeeded_count} succeeded, ❌ {failed_count} failed, 💀 {dead_count} dead")
        return since_ts

    actions = data.get("actions", [])
    plays = [a for a in actions if a.get("action") == "play"]
    plays = [
        a for a in plays
        if (
            (parsed_ts := parse_timestamp(a.get("timestamp"))) is None
            or int(parsed_ts.timestamp()) > since_ts
        )
    ]
    
    if not plays:
        print("No new 'play' actions found.")
        save_last_timestamp(since_ts)
        print(f"\n✅ {succeeded_count} succeeded, ❌ {failed_count} failed, 💀 {dead_count} dead")
        return since_ts

    # sort safely using parsed timestamp
    plays.sort(key=lambda a: parse_timestamp(a.get("timestamp")) or datetime.min)
    deduped_plays = deduplicate_actions(plays)

    print(f"\n🎧 New played episodes: {len(plays)}")
    if len(deduped_plays) != len(plays):
        print(f"🧹 Collapsed to unique episodes this poll: {len(deduped_plays)}")
    else:
        print()

    new_work_items = [build_work_item(action) for action in deduped_plays]
    new_since = since_ts
    for batch_index, batch_items in enumerate(chunk_items(new_work_items, checkpoint_batch_size), start=1):
        process_episode_batch(batch_items, str(batch_index))

        for item in batch_items:
            episode_url = item.episode_url
            if item.succeeded:
                if episode_url:
                    mark_succeeded(episode_url)
                succeeded_count += 1
            else:
                moved_to_dead = mark_failed(episode_url, item.action)
                if moved_to_dead:
                    dead_count += 1
                else:
                    failed_count += 1

            if item.timestamp_value is not None and item.timestamp_value > new_since:
                new_since = item.timestamp_value

        save_last_timestamp(new_since)

    save_last_timestamp(new_since)
    print(f"\n✅ {succeeded_count} succeeded, ❌ {failed_count} failed, 💀 {dead_count} dead")
    return new_since

def main() -> None:
    print(f"🚀 Starting PodGist v{get_app_version()} Loop...")
    current_since = load_last_timestamp()
    print(f"📅 Starting check from timestamp: {current_since}")

    try:
        while True:
            print(f"\nChecking for new actions (since {current_since})...")
            new_since = process_actions(current_since)
            if new_since > current_since:
                current_since = new_since
            
            print(f"💤 Sleeping for {POLL_INTERVAL} seconds...")
            time.sleep(POLL_INTERVAL)
            
    except KeyboardInterrupt:
        print("\n🛑 Stopping loop. Goodbye!")
        sys.exit(0)

if __name__ == "__main__":
    main()
