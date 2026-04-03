ARG PYTHON_VERSION=3.14
FROM docker.io/library/python:${PYTHON_VERSION}-alpine
WORKDIR /app
RUN apk upgrade --no-cache && \
    apk add --no-cache ffmpeg
COPY pyproject.toml README.md ./
COPY src ./src
RUN pip install --no-cache-dir --upgrade pip && \
    pip install --no-cache-dir .
COPY . .
RUN mkdir -p data/downloads data/transcripts data/summaries
CMD ["python", "src/main.py"]
