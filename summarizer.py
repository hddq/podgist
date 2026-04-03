import os
from urllib.parse import urlsplit

import requests
from openai import APIConnectionError, APIStatusError, OpenAI

from config import (
    CHUNK_PROMPT_FILE,
    FINAL_PROMPT_FILE,
    LLM_API_KEY,
    LLM_AUTO_PULL,
    LLM_BASE_URL,
    LLM_EXTRA_BODY,
    LLM_MODEL,
    LLM_PROVIDER,
    LLM_TIMEOUT,
    PIPELINE_CHUNK_TOKENS,
    PIPELINE_CHUNKING_THRESHOLD,
    PROMPT_FILE,
    SUMMARY_DIR,
    TRANSCRIPT_DIR,
)
from utils import chunk_transcript, estimate_tokens


def _normalize_openai_base_url(base_url: str) -> str:
    normalized = base_url.rstrip("/")
    parsed = urlsplit(normalized)
    if not parsed.path:
        return f"{normalized}/v1"
    return normalized


def _make_llm_client() -> OpenAI:
    base_url = _normalize_openai_base_url(LLM_BASE_URL)
    return OpenAI(
        base_url=base_url, api_key=LLM_API_KEY or "not-needed", timeout=LLM_TIMEOUT
    )


def _pull_ollama_model() -> bool:
    pull_url = f"{LLM_BASE_URL.rstrip('/')}/api/pull"
    response = requests.post(
        url=pull_url, json={"name": LLM_MODEL, "stream": False}, timeout=1800
    )
    response.raise_for_status()
    return True


def _read_prompt(path: str) -> str | None:
    if not os.path.exists(path):
        print(f"Prompt file not found: {path}")
        return None

    try:
        with open(path, "r", encoding="utf-8") as f:
            return f.read()
    except Exception as e:
        print(f"Error reading prompt file {path}: {e}")
        return None


def _call_ollama_native(prompt: str) -> str | None:
    """Call Ollama via its native /api/chat endpoint, which correctly handles options like num_ctx."""
    chat_url = f"{LLM_BASE_URL.rstrip('/')}/api/chat"
    payload: dict[str, object] = {
        "model": LLM_MODEL,
        "messages": [{"role": "user", "content": prompt}],
        "stream": False,
    }
    if LLM_EXTRA_BODY:
        payload.update(LLM_EXTRA_BODY)
    try:
        response = requests.post(url=chat_url, json=payload, timeout=LLM_TIMEOUT)
        response.raise_for_status()
        data = response.json()
        message = data.get("message", {})
        content = message.get("content")
        return content if isinstance(content, str) else None
    except requests.exceptions.ConnectionError:
        print(f"Failed to connect to Ollama at {LLM_BASE_URL}. Is the server running?")
        return None
    except requests.exceptions.HTTPError as e:
        print(f"Ollama request failed: {e}")
        return None
    except Exception as e:
        print(f"Ollama request failed unexpectedly: {e}")
        return None


def _call_llm(prompt: str) -> str | None:
    if not LLM_BASE_URL or not LLM_MODEL:
        return None

    def _do_call() -> str | None:
        if LLM_PROVIDER.lower() == "ollama":
            return _call_ollama_native(prompt)
        client = _make_llm_client()
        response = client.chat.completions.create(
            model=LLM_MODEL,
            messages=[{"role": "user", "content": prompt}],
            extra_body=LLM_EXTRA_BODY or None,
        )
        choice = response.choices[0] if response.choices else None
        if choice and choice.message and choice.message.content:
            return choice.message.content
        return None

    try:
        return _do_call()
    except APIConnectionError:
        print(f"Failed to connect to LLM at {LLM_BASE_URL}. Is the server running?")
        return None
    except APIStatusError as exc:
        if LLM_AUTO_PULL and exc.status_code == 404:
            print(f"Model '{LLM_MODEL}' not found. Attempting to pull via Ollama...")
            try:
                _pull_ollama_model()
                print(f"Successfully pulled model '{LLM_MODEL}'. Retrying...")
                return _do_call()
            except Exception as retry_exc:
                print(f"LLM retry after pull failed: {retry_exc}")
                return None
        print(f"LLM request failed: {exc}")
        return None
    except Exception as e:
        print(f"LLM request failed: {e}")
        return None


def _summarize_chunked(transcript_text: str) -> str | None:
    chunk_prompt_template = _read_prompt(CHUNK_PROMPT_FILE)
    final_prompt_template = _read_prompt(FINAL_PROMPT_FILE)
    if not chunk_prompt_template or not final_prompt_template:
        return None

    chunks = chunk_transcript(
        transcript_text,
        chunk_tokens=PIPELINE_CHUNK_TOKENS,
    )
    total_chunks = len(chunks)
    partial_summaries: list[str] = []

    for chunk_index, chunk_text in enumerate(chunks, start=1):
        prompt = (
            chunk_prompt_template.replace("{transcript}", chunk_text)
            .replace("{chunk_index}", str(chunk_index))
            .replace("{total_chunks}", str(total_chunks))
        )
        partial_summary = _call_llm(prompt)
        if not partial_summary:
            print(f"Warning: failed to summarize chunk {chunk_index}/{total_chunks}.")
            continue
        partial_summaries.append(partial_summary)

    if not partial_summaries:
        return None

    combined_summaries = "\n\n---\n\n".join(partial_summaries)
    final_prompt = final_prompt_template.replace("{transcript}", combined_summaries)
    return _call_llm(final_prompt)


def summarize(transcript_path: str) -> str | None:
    """
    Summarizes the given transcript file using the configured LLM.
    """
    if not transcript_path or not os.path.exists(transcript_path):
        print(f"Transcript not found: {transcript_path}")
        return None

    # Determine relative path to maintain structure
    try:
        abs_transcript = os.path.abspath(transcript_path)
        abs_transcript_dir = os.path.abspath(TRANSCRIPT_DIR)
        if abs_transcript.startswith(abs_transcript_dir):
            rel_path = os.path.relpath(abs_transcript, start=abs_transcript_dir)
        else:
            rel_path = os.path.basename(transcript_path)
    except Exception:
        rel_path = os.path.basename(transcript_path)

    # Remove .txt suffix if present
    if rel_path.endswith(".txt"):
        rel_path = rel_path[:-4]

    output_filename = rel_path + ".md"
    output_path = os.path.join(SUMMARY_DIR, output_filename)

    if os.path.exists(output_path):
        print(f"Summary already exists: {output_path}")
        return output_path

    # Ensure output dir
    os.makedirs(os.path.dirname(output_path), exist_ok=True)

    # Read transcript
    try:
        with open(transcript_path, "r", encoding="utf-8") as f:
            transcript_text = f.read()
    except Exception as e:
        print(f"Error reading transcript: {e}")
        return None

    token_count = estimate_tokens(transcript_text)
    print(f"Estimated transcript tokens: {token_count}")

    print(f"Summarizing {os.path.basename(transcript_path)}...")

    if token_count <= PIPELINE_CHUNKING_THRESHOLD:
        prompt_template = _read_prompt(PROMPT_FILE)
        if not prompt_template:
            return None
        prompt = prompt_template.replace("{transcript}", transcript_text)
        summary_text = _call_llm(prompt)
    else:
        summary_text = _summarize_chunked(transcript_text)

    if summary_text:
        try:
            with open(output_path, "w", encoding="utf-8") as f:
                f.write(summary_text)
            print(f"Summary saved to {output_path}")
            return output_path
        except Exception as e:
            print(f"Error saving summary: {e}")
            return None
    else:
        print("Failed to generate summary.")
        return None
