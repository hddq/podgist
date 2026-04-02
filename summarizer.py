import os
import requests
import json
from google import genai
from config import (
    GEMINI_API_KEY, 
    SUMMARY_DIR, 
    PROMPT_FILE, 
    CHUNK_PROMPT_FILE,
    FINAL_PROMPT_FILE,
    TRANSCRIPT_DIR,
    LLM_PROVIDER,
    GEMINI_MODEL,
    OLLAMA_BASE_URL,
    OLLAMA_MODEL,
    OLLAMA_AUTO_PULL,
    OLLAMA_NUM_CTX,
    PIPELINE_CHUNK_TOKENS,
    PIPELINE_CHUNKING_THRESHOLD,
)
from utils import chunk_transcript, estimate_tokens

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

def _call_llm(prompt: str) -> str | None:
    if LLM_PROVIDER == "gemini":
        return summarize_with_gemini(prompt)
    if LLM_PROVIDER == "ollama":
        return summarize_with_ollama(prompt)

    print(f"Unknown LLM_PROVIDER: {LLM_PROVIDER}")
    return None

def summarize_with_gemini(prompt):
    """
    Summarizes the prompt using Google Gemini.
    """
    if not GEMINI_API_KEY:
        print("GEMINI_API_KEY is not set.")
        return None

    try:
        client = genai.Client(api_key=GEMINI_API_KEY)
        response = client.models.generate_content(
            model=GEMINI_MODEL, 
            contents=prompt
        )
        return response.text
    except Exception as e:
        print(f"Gemini summarization failed: {e}")
        return None

def summarize_with_ollama(prompt):
    """
    Summarizes the prompt using Ollama.
    """
    base_url = OLLAMA_BASE_URL.rstrip("/")
    url = f"{base_url}/api/generate"
    payload = {
        "model": OLLAMA_MODEL,
        "prompt": prompt,
        "stream": False,
        "options": {
            "num_ctx": OLLAMA_NUM_CTX
        },
        "think": False
    }

    def pull_model_if_missing():
        pull_url = f"{base_url}/api/pull"
        pull_payload = {
            "name": OLLAMA_MODEL,
            "stream": False
        }
        print(f"Ollama model '{OLLAMA_MODEL}' is missing. Pulling it now...")
        pull_response = requests.post(pull_url, json=pull_payload, timeout=1800)
        pull_response.raise_for_status()
        print(f"Successfully pulled Ollama model '{OLLAMA_MODEL}'.")

    try:
        response = requests.post(url, json=payload, timeout=300)
        if response.status_code == 404 and OLLAMA_AUTO_PULL:
            error_text = ""
            try:
                error_text = response.json().get("error", "")
            except Exception:
                pass
            if "not found" in error_text.lower():
                pull_model_if_missing()
                retry_response = requests.post(url, json=payload, timeout=300)
                retry_response.raise_for_status()
                retry_data = retry_response.json()
                return retry_data.get("response")
        response.raise_for_status()
        data = response.json()
        return data.get("response")
    except requests.exceptions.ConnectionError:
        print(f"Failed to connect to Ollama at {url}. Is Ollama running?")
        return None
    except Exception as e:
        print(f"Ollama summarization failed: {e}")
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
    partial_summaries = []

    for chunk_index, chunk_text in enumerate(chunks, start=1):
        prompt = (
            chunk_prompt_template
            .replace("{transcript}", chunk_text)
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

def summarize(transcript_path):
    """
    Summarizes the given transcript file using the configured LLM provider.
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

    print(f"Summarizing {os.path.basename(transcript_path)} using {LLM_PROVIDER}...")

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
