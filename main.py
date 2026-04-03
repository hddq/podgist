import sys
import time
from datetime import datetime

from config import PIPELINE_BATCH_SIZE
from gpodder import fetch_episode_actions
from pipeline import deduplicate_actions, process_action_batches
from state_manager import load_failed, load_last_timestamp, save_last_timestamp
from utils import parse_timestamp
from version import get_app_version

POLL_INTERVAL = 600  # 10 minutes


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
    retry_actions = [failed_entry["action"] for failed_entry in failed_queue.values()]
    retry_actions.sort(
        key=lambda a: parse_timestamp(a.get("timestamp")) or datetime.min
    )
    checkpoint_batch_size = PIPELINE_BATCH_SIZE if PIPELINE_BATCH_SIZE > 0 else 1
    print(
        f"ℹ️ Processing episodes in staged batches "
        f"(batch size: {checkpoint_batch_size})."
    )

    if retry_actions:
        succeeded_count, failed_count, dead_count, _ = process_action_batches(
            retry_actions,
            checkpoint_batch_size,
            "retry ",
            succeeded_count,
            failed_count,
            dead_count,
            False,
            since_ts,
        )

    try:
        data = fetch_episode_actions(since=since_ts)
    except Exception as e:
        print(f"Failed to fetch actions: {e}")
        print(
            f"\n✅ {succeeded_count} succeeded, ❌ {failed_count} failed, 💀 {dead_count} dead"
        )
        return since_ts

    actions = data.get("actions", [])
    plays = [a for a in actions if a.get("action") == "play"]
    plays = [
        a
        for a in plays
        if (
            (parsed_ts := parse_timestamp(a.get("timestamp"))) is None
            or int(parsed_ts.timestamp()) > since_ts
        )
    ]

    if not plays:
        print("No new 'play' actions found.")
        save_last_timestamp(since_ts)
        print(
            f"\n✅ {succeeded_count} succeeded, ❌ {failed_count} failed, 💀 {dead_count} dead"
        )
        return since_ts

    # sort safely using parsed timestamp
    plays.sort(key=lambda a: parse_timestamp(a.get("timestamp")) or datetime.min)
    deduped_plays = deduplicate_actions(plays)

    print(f"\n🎧 New played episodes: {len(plays)}")
    if len(deduped_plays) != len(plays):
        print(f"🧹 Collapsed to unique episodes this poll: {len(deduped_plays)}")
    else:
        print()

    succeeded_count, failed_count, dead_count, new_since = process_action_batches(
        deduped_plays,
        checkpoint_batch_size,
        "",
        succeeded_count,
        failed_count,
        dead_count,
        True,
        since_ts,
    )

    save_last_timestamp(new_since)
    print(
        f"\n✅ {succeeded_count} succeeded, ❌ {failed_count} failed, 💀 {dead_count} dead"
    )
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
