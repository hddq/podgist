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

def process_actions(since_ts):
    """
    Fetches and processes actions since the given timestamp.
    Returns the updated timestamp (max of current and processed actions).
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

    max_ts = since_ts
    checkpoint_blocked = False

    for a in plays:
        raw_ts = a.get("timestamp")
        dt = parse_timestamp(raw_ts)
        time_str = dt.isoformat() if dt else "unknown"
        episode_url = a.get('episode')
        podcast_url = a.get('podcast')

        print("─" * 80)
        print(f"🕒 {time_str}")
        print(f"📡 Podcast: {podcast_url}")
        print(f"🎙 Episode: {episode_url}")
        print(f"▶️  Position: {a.get('position')} / {a.get('total')}")

        ts_val = int(dt.timestamp()) if dt else None
        action_succeeded = False

        if episode_url:
             # Fetch metadata to determine structure
             podcast_title, episode_title = get_podcast_metadata(podcast_url, episode_url)
             
             if not podcast_title:
                 podcast_title = "Unknown Podcast"
             if not episode_title:
                 # fallback to extracting from URL
                 episode_title = episode_url.split("/")[-1]
                 if "?" in episode_title:
                     episode_title = episode_title.split("?")[0]
            
             safe_podcast = sanitize_filename(podcast_title)
             safe_episode = sanitize_filename(episode_title)
             
             # Ensure extension
             if not safe_episode.lower().endswith('.mp3') and not safe_episode.lower().endswith('.m4a'):
                  safe_episode += ".mp3" # default guess

             relative_path = os.path.join(safe_podcast, safe_episode)
             
             filepath = download_file(episode_url, relative_path=relative_path)
             if filepath:
                 transcript_path = transcribe(filepath)
                 if transcript_path:
                     summary_path = summarize(transcript_path)
                     if summary_path:
                         action_succeeded = True
        else:
             print("⚠️ No episode URL found, skipping download.")
             # Non-retryable malformed action; allow checkpoint to move past it.
             action_succeeded = True

        if not action_succeeded and not checkpoint_blocked:
             checkpoint_blocked = True
             print("⚠️ Processing failed; keeping checkpoint before this action for retry.")

        # Never move checkpoint past the first failed action in this batch.
        if not checkpoint_blocked and ts_val is not None and ts_val > max_ts:
             max_ts = ts_val
        
    return max_ts

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
                save_last_timestamp(current_since)
            
            print(f"💤 Sleeping for {POLL_INTERVAL} seconds...")
            time.sleep(POLL_INTERVAL)
            
    except KeyboardInterrupt:
        print("\n🛑 Stopping loop. Goodbye!")
        sys.exit(0)

if __name__ == "__main__":
    main()
