FROM docker.io/library/python:3.14-alpine
WORKDIR /app
RUN apk upgrade --no-cache && \
    apk add --no-cache ffmpeg
COPY pyproject.toml ./
RUN pip install --no-cache-dir --upgrade pip && \
    pip install --no-cache-dir .
COPY . .
RUN mkdir -p data/downloads data/transcripts data/summaries
CMD ["python", "main.py"]