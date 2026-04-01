import time
import sys
from datetime import datetime
from importlib.metadata import PackageNotFoundError, version as package_version
from gpodder import fetch_episode_actions
from utils import parse_timestamp, get_podcast_metadata, sanitize_filename
from downloader import download_file
from transcriber import transcribe
from summarizer import summarize
from state_manager import load_last_timestamp, save_last_timestamp
from config import PIPELINE_BATCH_SIZE
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
        "transcript_path": None,
        "summary_path": None,
        "succeeded": False,
    }


def chunk_items(items, chunk_size):
    for index in range(0, len(items), chunk_size):
        yield items[index:index + chunk_size]


def cleanup_batch_audio(batch_items):
    for item in batch_items:
        filepath = item.get("filepath")
        if not filepath:
            continue

        try:
            os.remove(filepath)
            print(f"🧹 Removed audio file: {filepath}")
        except FileNotFoundError:
            continue
        except OSError as e:
            print(f"Warning: failed to remove audio file {filepath}: {e}")

def process_actions(since_ts):
    """
    Fetches and processes actions since the given timestamp.
    Returns the updated timestamp after processing successful batches.
    """
    try:
        data = fetch_episode_actions(since=since_ts)
    except Exception as e:
        print(f"Failed to fetch actions: {e}")
        return since_ts

    actions = data.get("actions", [])
    plays = [a for a in actions if a.get("action") == "play"]
    
    if not plays:
        print("No new 'play' actions found.")
        return since_ts

    # sort safely using parsed timestamp
    plays.sort(key=lambda a: parse_timestamp(a.get("timestamp")) or datetime.min)

    print(f"\n🎧 New played episodes: {len(plays)}\n")

    batch_size = PIPELINE_BATCH_SIZE if PIPELINE_BATCH_SIZE > 0 else 1
    current_since = since_ts
    for batch_index, batch_actions in enumerate(chunk_items(plays, batch_size), start=1):
        batch_items = [build_work_item(action) for action in batch_actions]
        batch_failed = False
        print(f"\n📦 Processing batch {batch_index} ({len(batch_items)} episode(s))")

        print("⬇️  Phase 1: Download all")
        for item in batch_items:
            if not item["episode_url"]:
                print("⚠️ No episode URL found, skipping download.")
                item["succeeded"] = True
                continue

            item["filepath"] = download_file(
                item["episode_url"],
                relative_path=item["relative_path"],
            )
            if not item["filepath"]:
                batch_failed = True

        print("📝 Phase 2: Transcribe all")
        for item in batch_items:
            if item["succeeded"]:
                continue
            if not item["filepath"]:
                batch_failed = True
                continue

            item["transcript_path"] = transcribe(item["filepath"])
            if not item["transcript_path"]:
                batch_failed = True

        print("🧠 Phase 3: Summarize all")
        for item in batch_items:
            if item["succeeded"]:
                continue
            if not item["transcript_path"]:
                batch_failed = True
                continue

            item["summary_path"] = summarize(item["transcript_path"])
            if item["summary_path"]:
                item["succeeded"] = True
            else:
                batch_failed = True

        if batch_failed or not all(item["succeeded"] for item in batch_items):
            print("⚠️ Batch failed; checkpoint will not advance and later batches will not run.")
            break

        last_timestamp = batch_items[-1].get("timestamp_value")
        if last_timestamp is not None and last_timestamp > current_since:
            current_since = last_timestamp
            save_last_timestamp(current_since)

        if all(item["succeeded"] for item in batch_items):
            cleanup_batch_audio(batch_items)

    return current_since

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
