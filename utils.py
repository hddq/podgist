from datetime import datetime
import requests
import xml.etree.ElementTree as ET
import re

def estimate_tokens(text: str) -> int:
    return len(text) // 4

def split_into_sentences(text: str) -> list[str]:
    sentences = re.split(r'(?<=[.!?])\s+', text.strip())
    return [sentence for sentence in sentences if sentence.strip()]

def chunk_transcript(text: str, chunk_tokens: int = 3000, overlap_sentences: int = 3) -> list[str]:
    sentences = split_into_sentences(text)
    if not sentences:
        return []

    chunks = []
    current_sentences = []

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

        overlap = current_sentences[-overlap_sentences:] if overlap_sentences > 0 else []
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

def sanitize_filename(name):
    """
    Sanitizes a string to be safe for filenames.
    """
    if not name:
        return "unknown"
    # Keep alphanumeric, spaces, dots, underscores, dashes
    safe = "".join([c for c in name if c.isalnum() or c in (' ', '.', '_', '-')]).strip()
    return safe if safe else "unknown"

def get_podcast_metadata(podcast_url, episode_url):
    """
    Fetches the RSS feed at podcast_url and tries to find the title
    and the episode title for the given episode_url.
    Returns (podcast_title, episode_title).
    """
    try:
        # Some feeds might require a User-Agent
        headers = {
            'User-Agent': 'Mozilla/5.0 (PodGist/1.0)'
        }
        resp = requests.get(podcast_url, headers=headers, timeout=30)
        resp.raise_for_status()
        
        # Parse XML
        # We use fromstring which parses bytes or string
        root = ET.fromstring(resp.content)
        
        # Handle cases where root is rss/channel or just channel
        channel = root.find('channel')
        if channel is None:
            # Maybe the root IS the channel (Atom? or just weird RSS)
            # Standard RSS is <rss><channel>...</channel></rss>
            # Check if root tag is channel?
            if root.tag == 'channel':
                channel = root
            else:
                # Fallback, maybe look for title in root (Atom)
                # But let's stick to standard RSS structure primarily
                pass

        podcast_title = "Unknown Podcast"
        if channel is not None:
            t = channel.findtext('title')
            if t:
                podcast_title = t

        episode_title = "Unknown Episode"
        
        # Iterate items to find episode
        # Support both 'item' (RSS) and 'entry' (Atom - though structure differs)
        items = channel.findall('item') if channel is not None else []
        
        found = False
        for item in items:
            # Check enclosure url
            enclosure = item.find('enclosure')
            if enclosure is not None:
                url = enclosure.get('url')
                # Simple check: exact match or contained
                # Often episode_url might have extra params
                if url and (url == episode_url or episode_url in url or url in episode_url):
                    episode_title = item.findtext('title')
                    found = True
                    break
            
            # Check guid
            guid = item.findtext('guid')
            if guid and guid == episode_url:
                 episode_title = item.findtext('title')
                 found = True
                 break

        if not found and channel is not None:
             # Try to see if we can match by just filename if urls are vastly different
             # This is risky but helpful
             pass

        return podcast_title, episode_title

    except Exception as e:
        print(f"Error fetching metadata for {podcast_url}: {e}")
        return None, None

def parse_timestamp(ts):
    if ts is None:
        return None

    # UNIX timestamp (int or numeric string)
    if isinstance(ts, (int, float)) or (isinstance(ts, str) and ts.isdigit()):
        return datetime.fromtimestamp(int(ts))

    # ISO 8601 string
    if isinstance(ts, str):
        try:
            return datetime.fromisoformat(ts.replace("Z", "+00:00"))
        except ValueError:
            return None

    return None
