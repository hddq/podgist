FROM python:3.14-slim
WORKDIR /app
RUN apt-get update && apt-get install -y \
    ffmpeg \
    && rm -rf /var/lib/apt/lists/*
COPY pyproject.toml ./
RUN pip install --no-cache-dir .
COPY . .
RUN mkdir -p data/downloads data/transcripts data/summaries
CMD ["python", "main.py"]