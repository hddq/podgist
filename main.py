import time
import sys
from datetime import datetime
from importlib.metadata import PackageNotFoundError, version as package_version
from gpodder import fetch_episode_actions
from utils import parse_timestamp, get_podcast_metadata, sanitize_filename
from downloader import download_file
from transcriber import transcribe
from summarizer import summarize
from state_manager import (
    load_failed,
    load_last_timestamp,
    mark_failed,
    mark_succeeded,
    save_last_timestamp,
)
from config import PIPELINE_BATCH_SIZE, TRANSCRIPT_DIR, SUMMARY_DIR
import os
import tomllib

POLL_INTERVAL = 600  # 10 minutes


def get_app_version():
    try:
        return package_version("podgist")
    except PackageNotFoundError:
        pyproject_path = os.path.join(os.path.dirname(__file__), "pyproject.toml")
        with open(pyproject_path, "rb") as f:
            data = tomllib.load(f)
        return data["project"]["version"]


def build_work_item(action):
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

    return {
        "action": action,
        "timestamp": dt,
        "timestamp_value": int(dt.timestamp()) if dt else None,
        "episode_url": episode_url,
        "podcast_url": podcast_url,
        "relative_path": relative_path,
        "filepath": None,
        "transcript_path": (
            os.path.join(TRANSCRIPT_DIR, relative_path + ".txt")
            if relative_path else None
        ),
        "summary_path": (
            os.path.join(SUMMARY_DIR, relative_path + ".md")
            if relative_path else None
        ),
        "succeeded": False,
    }

def cleanup_audio_file(filepath):
    if not filepath:
        return

    try:
        os.remove(filepath)
        print(f"🧹 Removed audio file: {filepath}")
    except FileNotFoundError:
        return
    except OSError as e:
        print(f"Warning: failed to remove audio file {filepath}: {e}")


def process_single_episode(action):
    """
    Processes one episode end to end.
    Returns True on full success and False on any failure.
    """
    item = build_work_item(action)

    if not item["episode_url"]:
        print("⚠️ No episode URL found, skipping episode.")
        return True

    if item["summary_path"] and os.path.exists(item["summary_path"]):
        print(f"Summary already exists: {item['summary_path']}")
        return True

    if item["transcript_path"] and os.path.exists(item["transcript_path"]):
        print(f"Transcript already exists: {item['transcript_path']}")
    else:
        item["filepath"] = download_file(
            item["episode_url"],
            relative_path=item["relative_path"],
        )
        if not item["filepath"]:
            return False

        item["transcript_path"] = transcribe(item["filepath"])
        if not item["transcript_path"]:
            return False

    item["summary_path"] = summarize(item["transcript_path"])
    if not item["summary_path"]:
        return False

    cleanup_audio_file(item.get("filepath"))
    return True

def process_actions(since_ts):
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

    for episode_url, failed_entry in failed_queue.items():
        action = failed_entry.get("action") or {}
        print(f"\n🔁 Retrying failed episode: {episode_url}")
        if process_single_episode(action):
            mark_succeeded(episode_url)
            succeeded_count += 1
            print(f"✅ Retry succeeded: {episode_url}")
        else:
            moved_to_dead = mark_failed(episode_url, action)
            if moved_to_dead:
                dead_count += 1
            else:
                failed_count += 1
            print(f"❌ Retry failed: {episode_url}")

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

    print(f"\n🎧 New played episodes: {len(plays)}\n")

    checkpoint_batch_size = PIPELINE_BATCH_SIZE if PIPELINE_BATCH_SIZE > 0 else 1
    print(
        f"ℹ️ Processing episodes sequentially "
        f"(checkpoint save cadence: every {checkpoint_batch_size} episode(s))."
    )

    new_since = since_ts
    processed_count = 0
    for action in plays:
        episode_url = action.get("episode")
        if process_single_episode(action):
            if episode_url:
                mark_succeeded(episode_url)
            succeeded_count += 1
        else:
            moved_to_dead = mark_failed(episode_url, action)
            if moved_to_dead:
                dead_count += 1
            else:
                failed_count += 1

        parsed_ts = parse_timestamp(action.get("timestamp"))
        if parsed_ts is not None:
            timestamp_value = int(parsed_ts.timestamp())
            if timestamp_value > new_since:
                new_since = timestamp_value

        processed_count += 1
        if processed_count % checkpoint_batch_size == 0:
            save_last_timestamp(new_since)

    save_last_timestamp(new_since)
    print(f"\n✅ {succeeded_count} succeeded, ❌ {failed_count} failed, 💀 {dead_count} dead")
    return new_since

def main():
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
