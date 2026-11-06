# Middleware — Python Harness base image.
# Copyright 2026 The Middleware Authors.

FROM python:3.12-slim

ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1 \
    PYTHONPATH=/app/harness/src:/app/harness/src/gen

WORKDIR /app

COPY harness/pyproject.toml harness/README.md /app/harness/
COPY harness/src /app/harness/src
COPY harness/config.example.yaml /app/harness/config.example.yaml

RUN pip install --no-cache-dir -e /app/harness

ENTRYPOINT ["python", "-m", "middleware_harness"]
CMD ["--config", "/app/harness/config.example.yaml"]
