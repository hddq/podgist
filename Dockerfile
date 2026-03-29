# Use an official Python runtime as a parent image
FROM python:3.13-slim

# Set the working directory in the container
WORKDIR /app

# Install system dependencies
RUN apt-get update && apt-get install -y \
    ffmpeg \
    && rm -rf /var/lib/apt/lists/*

# Copy the application code
COPY . .

# Install project dependencies from pyproject.toml
RUN pip install --no-cache-dir .

# Create directories for data persistence
RUN mkdir -p data/downloads data/transcripts data/summaries

# Run the application
CMD ["python", "main.py"]
