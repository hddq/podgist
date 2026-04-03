import os

import requests

from config import DOWNLOAD_DIR


def download_file(
    url: str,
    filename: str | None = None,
    relative_path: str | None = None,
) -> str | None:
    if not os.path.exists(DOWNLOAD_DIR):
        os.makedirs(DOWNLOAD_DIR)

    if relative_path:
        filepath = os.path.join(DOWNLOAD_DIR, relative_path)
    else:
        if not filename:
            # Simple extraction of filename from URL
            filename = url.split("/")[-1]
            # Basic cleanup of query parameters if present
            if "?" in filename:
                filename = filename.split("?")[0]

        # Ensure filename is safe (basic)
        filename = "".join(
            [
                c
                for c in filename
                if c.isalpha() or c.isdigit() or c in (" ", ".", "_", "-")
            ]
        ).strip()
        if not filename:
            filename = "unknown_episode.mp3"

        filepath = os.path.join(DOWNLOAD_DIR, filename)

    # Ensure directory exists
    os.makedirs(os.path.dirname(filepath), exist_ok=True)

    if os.path.exists(filepath):
        print(f"File already exists: {filepath}")
        return filepath

    print(f"Downloading {url} to {filepath}...")
    try:
        # Fake user agent to avoid some 403s from strict servers
        headers = {
            "User-Agent": (
                "Mozilla/5.0 (Windows NT 10.0; Win64; x64) "
                "AppleWebKit/537.36 (KHTML, like Gecko) "
                "Chrome/91.0.4472.124 Safari/537.36"
            )
        }
        with requests.get(url, stream=True, timeout=60, headers=headers) as response:
            response.raise_for_status()
            with open(filepath, "wb") as f:
                for chunk in response.iter_content(chunk_size=8192):
                    f.write(chunk)
        print(f"Download complete: {filepath}")
        return filepath
    except Exception as e:
        print(f"Error downloading {url}: {e}")
        # Clean up partial file
        if os.path.exists(filepath):
            os.remove(filepath)
        return None
