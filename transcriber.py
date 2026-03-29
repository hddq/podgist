import os
import subprocess
import requests
from config import (
    DOWNLOAD_DIR,
    TRANSCRIPT_DIR,
    WHISPER_API_KEY,
    WHISPER_BASE_URL,
    WHISPER_MODEL,
    WHISPER_TIMEOUT,
)


def convert_to_wav_16k(input_path):
    """
    Converts input audio to 16kHz WAV using ffmpeg.
    Returns a tuple (path to the wav file, boolean indicating if it was newly created).
    """
    output_path = input_path + ".wav"

    if os.path.exists(output_path):
        print(f"WAV file already exists, skipping conversion: {output_path}")
        return output_path, False

    cmd = [
        "ffmpeg",
        "-y",             # overwrite
        "-i", input_path,
        "-ar", "16000",   # 16kHz sample rate
        "-ac", "1",       # mono
        "-c:a", "pcm_s16le",
        output_path
    ]

    # Run ffmpeg silently
    try:
        subprocess.run(cmd, check=True, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        return output_path, True
    except subprocess.CalledProcessError as e:
        print(f"FFmpeg conversion failed: {e}")
        return None, False


def transcribe_with_whisper_server(wav_path):
    """
    Sends audio to an OpenAI-compatible Whisper server and returns transcript text.
    """
    url = f"{WHISPER_BASE_URL}/v1/audio/transcriptions"
    headers = {}
    if WHISPER_API_KEY:
        headers["Authorization"] = f"Bearer {WHISPER_API_KEY}"

    payload = {"model": WHISPER_MODEL}

    try:
        with open(wav_path, "rb") as audio_file:
            files = {"file": (os.path.basename(wav_path), audio_file, "audio/wav")}
            response = requests.post(url, data=payload, files=files, headers=headers, timeout=WHISPER_TIMEOUT)

        response.raise_for_status()
        data = response.json()
        text = data.get("text")
        if not text:
            print(f"Whisper server response missing 'text': {data}")
            return None
        return text
    except requests.exceptions.ConnectionError:
        print(f"Failed to connect to Whisper server at {url}. Is it running?")
        return None
    except requests.exceptions.Timeout:
        print(f"Whisper server request timed out after {WHISPER_TIMEOUT}s.")
        return None
    except Exception as e:
        print(f"Whisper transcription failed: {e}")
        return None


def transcribe(audio_path):
    """
    Transcribes the given audio file using a Whisper server API.
    """
    if not os.path.exists(audio_path):
        print(f"File not found: {audio_path}")
        return

    # Determine relative path to maintain structure
    try:
        # Use abs path for safer relative calculation
        abs_audio = os.path.abspath(audio_path)
        abs_download = os.path.abspath(DOWNLOAD_DIR)
        if abs_audio.startswith(abs_download):
            rel_path = os.path.relpath(abs_audio, start=abs_download)
        else:
            rel_path = os.path.basename(audio_path)
    except Exception:
        rel_path = os.path.basename(audio_path)

    output_base = os.path.join(TRANSCRIPT_DIR, rel_path)

    # Ensure output dir
    os.makedirs(os.path.dirname(output_base), exist_ok=True)

    expected_output = output_base + ".txt"

    if os.path.exists(expected_output):
        print(f"Transcript already exists: {expected_output}")
        return expected_output

    print(f"Converting {audio_path} to 16kHz WAV...")
    wav_path, created_temp = convert_to_wav_16k(audio_path)
    if not wav_path:
        return None

    print(f"Transcribing {rel_path} via Whisper server...")

    try:
        transcript_text = transcribe_with_whisper_server(wav_path)
        if not transcript_text:
            return None

        with open(expected_output, "w", encoding="utf-8") as transcript_file:
            transcript_file.write(transcript_text)

        print(f"Transcription complete: {expected_output}")
        return expected_output
    except Exception as e:
        print(f"Failed to save transcript: {e}")
        return None
    finally:
        # Cleanup temporary wav file ONLY if we created it
        if created_temp and os.path.exists(wav_path):
            os.remove(wav_path)
